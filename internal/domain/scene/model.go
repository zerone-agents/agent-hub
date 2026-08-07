package scene

import "time"

type Scene struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(64);uniqueIndex:uk_name;not null" json:"name"`
	AgentID   uint64    `gorm:"column:agent_id;not null;index" json:"agentId"`
	Title     string    `gorm:"type:varchar(128);not null" json:"title"`
	TitleEn   string    `gorm:"column:title_en;type:varchar(128)" json:"titleEn"`
	Prompt    string    `gorm:"type:text;not null" json:"prompt"`
	PromptEn  string    `gorm:"column:prompt_en;type:text" json:"promptEn"`
	Enabled   bool      `gorm:"not null;default:true;index" json:"enabled"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (Scene) TableName() string {
	return "scenes"
}
