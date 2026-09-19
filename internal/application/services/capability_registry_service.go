package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/capability"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/extensionmanifest"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type CapabilityRegistryService struct{ db *gorm.DB }

func NewCapabilityRegistryService(db *gorm.DB) *CapabilityRegistryService {
	return &CapabilityRegistryService{db: db}
}

type RegisterCapabilityInput struct {
	ManifestYAML      string
	ResourcesSnapshot map[string]any
	SourceType        string
	SourceRef         string
}

func (s *CapabilityRegistryService) Register(tenantID string, input RegisterCapabilityInput) (*capability.Package, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant is required")
	}
	report := extensionmanifest.ValidateManifest([]byte(input.ManifestYAML))
	if !report.Valid || report.Package == nil {
		return nil, fmt.Errorf("invalid capability manifest: %s", strings.Join(report.Errors, "; "))
	}
	resourcesJSON, err := json.Marshal(input.ResourcesSnapshot)
	if err != nil {
		return nil, fmt.Errorf("encode resources snapshot: %w", err)
	}
	h := sha256.New()
	_, _ = h.Write([]byte(input.ManifestYAML))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(resourcesJSON)
	manifest := capabilityManifestMetadata{}
	if err := yaml.Unmarshal([]byte(input.ManifestYAML), &manifest); err != nil {
		return nil, fmt.Errorf("decode capability manifest: %w", err)
	}
	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = "upload"
	}
	p := &capability.Package{TenantID: tenantID, Namespace: report.Package.Namespace, Name: report.Package.Name, Version: report.Package.Version, DisplayName: report.Package.DisplayName, ManifestYAML: input.ManifestYAML, ResourcesSnapshot: input.ResourcesSnapshot, ContentHash: hex.EncodeToString(h.Sum(nil)), SourceType: sourceType, SourceRef: strings.TrimSpace(input.SourceRef), RequestedPermissions: manifest.Permissions, ApprovedPermissions: map[string]any{}, ApprovalStatus: "pending", Enabled: false}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		for _, resource := range manifest.provenance(tenantID, p.ID, p.ContentHash) {
			if err := tx.Create(&resource).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if isDuplicate(err) {
			return nil, rundomain.ErrDuplicate
		}
		return nil, err
	}
	return p, nil
}

type capabilityManifestMetadata struct {
	Permissions map[string]any `yaml:"permissions"`
	Contributes map[string][]struct {
		ID      string `yaml:"id"`
		Version string `yaml:"version"`
		File    string `yaml:"file"`
	} `yaml:"contributes"`
}

func (m capabilityManifestMetadata) provenance(tenantID string, packageID uint64, packageHash string) []capability.ResourceProvenance {
	rows := make([]capability.ResourceProvenance, 0)
	for kind, resources := range m.Contributes {
		for _, item := range resources {
			key := item.ID
			if key == "" {
				key = item.File
			}
			rows = append(rows, capability.ResourceProvenance{TenantID: tenantID, PackageID: packageID, Kind: kind, ResourceKey: key, Version: item.Version, SourceRef: item.File, ContentHash: packageHash})
		}
	}
	return rows
}

func (s *CapabilityRegistryService) List(tenantID string, enabled *bool) ([]capability.Package, error) {
	q := s.db.Where("tenant_id=?", tenantID)
	if enabled != nil {
		q = q.Where("enabled=?", *enabled)
	}
	var rows []capability.Package
	err := q.Order("namespace,name,version").Find(&rows).Error
	return rows, err
}
func (s *CapabilityRegistryService) SetEnabled(tenantID string, id uint64, enabled bool) (*capability.Package, error) {
	if enabled {
		var p capability.Package
		if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&p).Error; err != nil {
			return nil, err
		}
		if p.ApprovalStatus != "approved" {
			return nil, ErrCapabilityApprovalRequired
		}
	}
	res := s.db.Model(&capability.Package{}).Where("tenant_id=? AND id=?", tenantID, id).Update("enabled", enabled)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	var p capability.Package
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

var ErrCapabilityApprovalRequired = errors.New("capability permissions must be approved before enabling")

func (s *CapabilityRegistryService) Approve(tenantID string, id uint64, approvedBy string, permissions map[string]any) (*capability.Package, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return nil, fmt.Errorf("approver is required")
	}
	var p capability.Package
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&p).Error; err != nil {
		return nil, err
	}
	if permissions == nil {
		permissions = p.RequestedPermissions
	}
	requestedJSON, _ := json.Marshal(p.RequestedPermissions)
	approvedJSON, _ := json.Marshal(permissions)
	if string(requestedJSON) != string(approvedJSON) {
		return nil, fmt.Errorf("approved permissions must exactly match the reviewed manifest")
	}
	now := time.Now().UTC()
	p.ApprovalStatus = "approved"
	p.ApprovedPermissions = permissions
	p.ApprovedBy = approvedBy
	p.ApprovedAt = &now
	if err := s.db.Save(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *CapabilityRegistryService) Resources(tenantID string, packageID uint64) ([]capability.ResourceProvenance, error) {
	var rows []capability.ResourceProvenance
	err := s.db.Where("tenant_id=? AND package_id=?", tenantID, packageID).Order("kind,resource_key").Find(&rows).Error
	return rows, err
}
func (s *CapabilityRegistryService) GetEnabled(tx *gorm.DB, tenantID, namespace, name, version string) (*capability.Package, error) {
	var p capability.Package
	err := tx.Where("tenant_id=? AND namespace=? AND name=? AND version=? AND enabled=?", tenantID, namespace, name, version, true).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("capability %s/%s@%s is not registered and enabled", namespace, name, version)
	}
	return &p, err
}
