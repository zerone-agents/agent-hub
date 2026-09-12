package capability

import "time"

// Package is the H1 tenant-local static registry record. Distribution,
// signatures, dependency resolution and executable installation belong to H7.
type Package struct {
	ID                uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID          string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_capability_packages,priority:1;index" json:"-"`
	Namespace         string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_capability_packages,priority:2" json:"namespace"`
	Name              string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_capability_packages,priority:3" json:"name"`
	Version           string         `gorm:"type:varchar(32);not null;uniqueIndex:uk_capability_packages,priority:4" json:"version"`
	DisplayName       string         `gorm:"type:varchar(160);not null;default:''" json:"displayName"`
	ManifestYAML      string         `gorm:"type:longtext;not null" json:"manifestYAML"`
	ResourcesSnapshot map[string]any `gorm:"type:json;serializer:json" json:"resourcesSnapshot"`
	ContentHash       string         `gorm:"type:char(64);not null" json:"contentHash"`
	Enabled           bool           `gorm:"not null;default:false;index" json:"enabled"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

func (Package) TableName() string { return "capability_packages" }
