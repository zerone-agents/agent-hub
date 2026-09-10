package personality

import (
	"time"

	"control-panel/internal/domain/agent"
)

// Template is a tenant-owned, prompt-first personality definition. Prompt is
// the current editable source of truth; BehaviorProfile is only an optional
// structured projection used by legacy clients and filtering.
type Template struct {
	ID              uint64                 `gorm:"primaryKey;autoIncrement" json:"id"`
	Name            string                 `gorm:"type:varchar(64);uniqueIndex:uk_personalities_tenant_name,priority:2;not null" json:"name"`
	TenantID        string                 `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_personalities_tenant_name,priority:1;index" json:"-"`
	Title           string                 `gorm:"type:varchar(128);not null" json:"title"`
	Description     string                 `gorm:"type:text" json:"description"`
	Prompt          string                 `gorm:"type:text;not null" json:"prompt"`
	BehaviorProfile *agent.BehaviorProfile `gorm:"column:behavior_profile;type:json;serializer:json" json:"behaviorProfile,omitempty"`
	CurrentVersion  int                    `gorm:"column:current_version;not null;default:1" json:"currentVersion"`
	Enabled         bool                   `gorm:"not null;default:true;index" json:"enabled"`
	IsBuiltin       bool                   `gorm:"column:is_builtin;not null;default:false" json:"isBuiltin"`
	CreatedAt       time.Time              `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt       time.Time              `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (Template) TableName() string { return "personality_templates" }

// Version is an immutable prompt snapshot. Editing a template prompt appends
// a row instead of mutating history, so agents can keep an exact provenance.
type Version struct {
	ID              uint64                 `gorm:"primaryKey;autoIncrement" json:"id"`
	TemplateID      uint64                 `gorm:"column:template_id;not null;uniqueIndex:uk_personality_version,priority:1;index" json:"templateId"`
	TenantID        string                 `gorm:"type:varchar(64);not null;default:'';index" json:"-"`
	Version         int                    `gorm:"not null;uniqueIndex:uk_personality_version,priority:2" json:"version"`
	Prompt          string                 `gorm:"type:text;not null" json:"prompt"`
	BehaviorProfile *agent.BehaviorProfile `gorm:"column:behavior_profile;type:json;serializer:json" json:"behaviorProfile,omitempty"`
	ChangeNote      string                 `gorm:"column:change_note;type:varchar(255)" json:"changeNote"`
	CreatedAt       time.Time              `gorm:"column:created_at" json:"createdAt"`
}

func (Version) TableName() string { return "personality_template_versions" }
