package capability

import "time"

// Package is the H1 tenant-local static registry record. Distribution,
// signatures, dependency resolution and executable installation belong to H7.
type Package struct {
	ID                   uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID             string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_capability_packages,priority:1;index" json:"-"`
	Namespace            string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_capability_packages,priority:2" json:"namespace"`
	Name                 string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_capability_packages,priority:3" json:"name"`
	Version              string         `gorm:"type:varchar(32);not null;uniqueIndex:uk_capability_packages,priority:4" json:"version"`
	DisplayName          string         `gorm:"type:varchar(160);not null;default:''" json:"displayName"`
	ManifestYAML         string         `gorm:"type:longtext;not null" json:"manifestYAML"`
	ResourcesSnapshot    map[string]any `gorm:"type:json;serializer:json" json:"resourcesSnapshot"`
	ContentHash          string         `gorm:"type:char(64);not null" json:"contentHash"`
	SourceType           string         `gorm:"type:varchar(32);not null;default:'upload'" json:"sourceType"`
	SourceRef            string         `gorm:"type:varchar(512);not null;default:''" json:"sourceRef"`
	RequestedPermissions map[string]any `gorm:"type:json;serializer:json" json:"requestedPermissions"`
	ApprovedPermissions  map[string]any `gorm:"type:json;serializer:json" json:"approvedPermissions"`
	ApprovalStatus       string         `gorm:"type:varchar(24);not null;default:'approved';index" json:"approvalStatus"`
	ApprovedBy           string         `gorm:"type:varchar(160);not null;default:''" json:"approvedBy"`
	ApprovedAt           *time.Time     `json:"approvedAt,omitempty"`
	Enabled              bool           `gorm:"not null;default:false;index" json:"enabled"`
	CreatedAt            time.Time      `json:"createdAt"`
	UpdatedAt            time.Time      `json:"updatedAt"`
}

func (Package) TableName() string { return "capability_packages" }

// ResourceProvenance records which immutable package version introduced a
// resource. Domain modules can attach their concrete row IDs later without
// learning anything about the package's business semantics.
type ResourceProvenance struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_capability_resource,priority:1;index" json:"-"`
	PackageID   uint64    `gorm:"not null;uniqueIndex:uk_capability_resource,priority:2;index" json:"packageId"`
	Kind        string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_capability_resource,priority:3" json:"kind"`
	ResourceKey string    `gorm:"type:varchar(180);not null;uniqueIndex:uk_capability_resource,priority:4" json:"resourceKey"`
	Version     string    `gorm:"type:varchar(32);not null;default:''" json:"version"`
	SourceRef   string    `gorm:"type:varchar(512);not null;default:''" json:"sourceRef"`
	ContentHash string    `gorm:"type:char(64);not null" json:"contentHash"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (ResourceProvenance) TableName() string { return "capability_resource_provenance" }
