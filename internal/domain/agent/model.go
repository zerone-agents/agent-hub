package agent

import (
	"time"

	"control-panel/internal/domain/skill"
)

type AgentConfig struct {
	ID               uint64            `gorm:"primaryKey;autoIncrement"`
	Name             string            `gorm:"type:varchar(64);uniqueIndex:uk_name;not null"`
	ContentHash      string            `gorm:"column:content_hash;type:varchar(128);not null"`
	SystemPrompt     string            `gorm:"column:system_prompt;type:text;not null"`
	PermissionMode   string            `gorm:"column:permission_mode;type:varchar(32);not null;default:'auto'"`
	MaxTurns         int               `gorm:"column:max_turns;not null;default:50"`
	Title            map[string]string `gorm:"type:json;serializer:json"`
	Description      map[string]string `gorm:"type:json;serializer:json"`
	Icon             string            `gorm:"type:varchar(512)"`
	IconName         string            `gorm:"column:icon_name;type:varchar(64)"`
	IconColor        string            `gorm:"column:icon_color;type:varchar(32)"`
	IconBgColor      string            `gorm:"column:icon_bg_color;type:varchar(64)"`
	ProviderID       *uint64           `gorm:"column:provider_id;index"`
	ModelID          string            `gorm:"column:model_id;type:varchar(64)"`
	FieldOverrides   string            `gorm:"column:field_overrides;type:text"` // JSON，secret fields encrypted
	Source           string            `gorm:"type:varchar(16);not null;default:'remote'"`
	DesktopEnabled   bool              `gorm:"not null;default:false;index"`
	MobileEnabled    bool              `gorm:"not null;default:false"`
	IsDefault        bool              `gorm:"column:is_default;default:false"`
	Group            string            `gorm:"column:group_name;type:varchar(64);default:''"`
	MaxSessionTurns  *int              `gorm:"column:max_session_turns;type:int;default:null"`
	RuntimePort      int               `gorm:"column:runtime_port;default:0"`
	DeploymentStatus string            `gorm:"column:deployment_status;type:varchar(32);default:''"`
	DeployedAt       *time.Time        `gorm:"column:deployed_at"`
	RuntimeToken     string            `gorm:"column:runtime_token;type:text"` // AES-GCM encrypted, write-only from DB POV
	CreatedAt        time.Time         `gorm:"column:created_at"`
	UpdatedAt        time.Time         `gorm:"column:updated_at;index"`
}

func (AgentConfig) TableName() string {
	return "agents"
}

// Client platforms used for agent visibility filtering.
const (
	PlatformDesktop = "desktop"
	PlatformMobile  = "mobile"
)

type AgentSubagent struct {
	AgentID    uint64      `gorm:"primaryKey;index:idx_agent_id"`
	SubagentID uint64      `gorm:"primaryKey;index:idx_subagent_id"`
	Agent      AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	Subagent   AgentConfig `gorm:"foreignKey:SubagentID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt  time.Time   `gorm:"column:created_at"`
}

func (AgentSubagent) TableName() string {
	return "agent_subagents"
}

type Tool struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"type:varchar(64);uniqueIndex:uk_name;not null"`
	Title       string    `gorm:"type:varchar(128)"`
	Description string    `gorm:"type:text"`
	IsDefault   bool      `gorm:"column:is_default;not null;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;index"`
}

func (Tool) TableName() string {
	return "tools"
}

type AgentTool struct {
	AgentID   uint64      `gorm:"primaryKey;index:idx_agent_id"`
	ToolID    uint64      `gorm:"primaryKey;index:idx_tool_id"`
	Agent     AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	Tool      Tool        `gorm:"foreignKey:ToolID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt time.Time   `gorm:"column:created_at"`
}

func (AgentTool) TableName() string {
	return "agent_tools"
}

type AgentSkill struct {
	AgentID   uint64      `gorm:"primaryKey;index:idx_agent_id"`
	SkillID   uint64      `gorm:"primaryKey;index:idx_skill_id"`
	Agent     AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	Skill     skill.Skill `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt time.Time   `gorm:"column:created_at"`
}

func (AgentSkill) TableName() string {
	return "agent_skills"
}
