package skill

import "time"

type Skill struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"type:varchar(64);uniqueIndex:uk_skills_tenant_name,priority:2;not null" json:"name"`
	TenantID      string    `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_skills_tenant_name,priority:1;index" json:"-"`
	Type          string    `gorm:"type:varchar(32);not null;default:'expert'" json:"type"`
	Title         string    `gorm:"type:varchar(128);not null" json:"title"`
	TitleEn       string    `gorm:"column:title_en;type:varchar(128)" json:"titleEn"`
	Description   string    `gorm:"type:text" json:"description"`
	DescriptionEn string    `gorm:"column:description_en;type:text" json:"descriptionEn"`
	URL           string    `gorm:"column:url;type:varchar(512)" json:"url"`
	FileHash      string    `gorm:"column:file_hash;type:varchar(128)" json:"fileHash"`
	FileSize      int64     `gorm:"column:file_size" json:"fileSize"`
	CreatedAt     time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt     time.Time `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (Skill) TableName() string {
	return "skills"
}
