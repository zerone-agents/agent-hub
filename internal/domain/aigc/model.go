package aigc

import "time"

// Config is the single-row global AIGC content-labeling configuration
// (GB 45438-2025). The row id is always 1. SigningKeyEncrypted must never
// be exposed through any API response.
type Config struct {
	ID                  uint64    `json:"-" gorm:"primaryKey"`
	USCC                string    `json:"uscc" gorm:"type:varchar(18);not null"`
	CompanyName         string    `json:"companyName" gorm:"type:varchar(255);not null"`
	ContentProducer     string    `json:"contentProducer" gorm:"type:varchar(27);not null"`
	SigningKeyEncrypted string    `json:"-" gorm:"type:text"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func (Config) TableName() string { return "aigc_configs" }
