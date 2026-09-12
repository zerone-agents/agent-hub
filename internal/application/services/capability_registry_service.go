package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"control-panel/internal/domain/capability"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/extensionmanifest"
	"gorm.io/gorm"
)

type CapabilityRegistryService struct{ db *gorm.DB }

func NewCapabilityRegistryService(db *gorm.DB) *CapabilityRegistryService {
	return &CapabilityRegistryService{db: db}
}

type RegisterCapabilityInput struct {
	ManifestYAML      string
	ResourcesSnapshot map[string]any
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
	p := &capability.Package{TenantID: tenantID, Namespace: report.Package.Namespace, Name: report.Package.Name, Version: report.Package.Version, DisplayName: report.Package.DisplayName, ManifestYAML: input.ManifestYAML, ResourcesSnapshot: input.ResourcesSnapshot, ContentHash: hex.EncodeToString(h.Sum(nil)), Enabled: false}
	if err := s.db.Create(p).Error; err != nil {
		if isDuplicate(err) {
			return nil, rundomain.ErrDuplicate
		}
		return nil, err
	}
	return p, nil
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
func (s *CapabilityRegistryService) GetEnabled(tx *gorm.DB, tenantID, namespace, name, version string) (*capability.Package, error) {
	var p capability.Package
	err := tx.Where("tenant_id=? AND namespace=? AND name=? AND version=? AND enabled=?", tenantID, namespace, name, version, true).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("capability %s/%s@%s is not registered and enabled", namespace, name, version)
	}
	return &p, err
}
