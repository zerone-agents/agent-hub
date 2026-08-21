package agent

import (
	"time"

	"control-panel/internal/domain/skill"
)

type AgentConfig struct {
	ID               uint64            `gorm:"primaryKey;autoIncrement"`
	Name             string            `gorm:"type:varchar(64);uniqueIndex:uk_agents_tenant_name,priority:2;not null"`
	TenantID         string            `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_agents_tenant_name,priority:1;index"`
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
	ModelSelectionID string            `gorm:"column:model_selection_id;type:varchar(128);default:''"` // 区分同 modelId 的多条 catalog 记录（如不同 contextWindow 的同款模型）
	FieldOverrides   string            `gorm:"column:field_overrides;type:text"`                       // JSON，secret fields encrypted
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
	AgentID    uint64      `gorm:"primaryKey"` // agent_id 是复合 PK 首列，单列索引冗余且曾跨表撞名（sqlite 索引名全局唯一）
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
	Name        string    `gorm:"type:varchar(64);uniqueIndex:uk_tools_tenant_name,priority:2;not null"`
	TenantID    string    `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_tools_tenant_name,priority:1;index"`
	Title       string    `gorm:"type:varchar(128)"`
	Description string    `gorm:"type:text"`
	IsDefault   bool      `gorm:"column:is_default;not null;default:false"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;index"`
}

func (Tool) TableName() string {
	return "tools"
}

// PresetToolNames 是以共享模板行（tenant_id=”）写入 tools 表的全部预设
// 工具名单，来源 = ToolService.SeedBuiltins（前三个：Skill/Task/MultiTask）
// + ToolService.SeedIfEmpty（其余六个）。pkg/database 的租户迁移按此名单
// 把旧存量预设行归零为共享——seeding 与迁移两边必须同源引用本常量，
// 新增预设工具时只改这里。
var PresetToolNames = []string{
	"Skill", "Task", "MultiTask",
	"Bash", "Read", "Write", "Edit", "Glob", "Grep",
}

type AgentTool struct {
	AgentID   uint64      `gorm:"primaryKey"`
	ToolID    uint64      `gorm:"primaryKey;index:idx_tool_id"`
	Agent     AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	Tool      Tool        `gorm:"foreignKey:ToolID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt time.Time   `gorm:"column:created_at"`
}

func (AgentTool) TableName() string {
	return "agent_tools"
}

type AgentSkill struct {
	AgentID   uint64      `gorm:"primaryKey"`
	SkillID   uint64      `gorm:"primaryKey;index:idx_skill_id"`
	Agent     AgentConfig `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"-"`
	Skill     skill.Skill `gorm:"foreignKey:SkillID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt time.Time   `gorm:"column:created_at"`
}

func (AgentSkill) TableName() string {
	return "agent_skills"
}
