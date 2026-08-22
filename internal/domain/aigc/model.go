package aigc

import "time"

// Config is the per-tenant AIGC content-labeling configuration
// (GB 45438-2025): purely one row per tenant — no shared default row and
// no fallback; a tenant without its own row is unconfigured.
// SigningKeyEncrypted must never be exposed through any API response.
type Config struct {
	ID                  uint64    `json:"-" gorm:"primaryKey;autoIncrement"`
	TenantID            string    `json:"-" gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_tenant_id;index"`
	USCC                string    `json:"uscc" gorm:"type:varchar(18);not null"`
	CompanyName         string    `json:"companyName" gorm:"type:varchar(255);not null"`
	ContentProducer     string    `json:"contentProducer" gorm:"type:varchar(27);not null"`
	SigningKeyEncrypted string    `json:"-" gorm:"type:text"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func (Config) TableName() string { return "aigc_configs" }
