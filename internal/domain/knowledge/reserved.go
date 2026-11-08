package knowledge

// ReservedDataset survives disabling the memory feature. This prevents old
// memory replicas from becoming visible through the ordinary knowledge API.
type ReservedDataset struct {
	DatasetID     string `gorm:"primaryKey;size:128"`
	TenantID      string `gorm:"size:128;not null"`
	ApplicationID string `gorm:"size:128;not null"`
}

func (ReservedDataset) TableName() string { return "memory_reserved_datasets" }
