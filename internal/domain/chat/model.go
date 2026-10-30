package chat

import (
	"time"
)

type Session struct {
	UserID            string    `json:"user_id" gorm:"primaryKey;type:varchar(255);not null"`
	TenantID          string    `json:"tenant_id" gorm:"type:varchar(64);not null;default:'';index"`
	ID                string    `json:"id" gorm:"primaryKey;type:varchar(255);not null"`
	Title             string    `json:"title" gorm:"type:varchar(512)"`
	UserName          string    `json:"user_name" gorm:"type:varchar(255)"`
	DisplayName       string    `json:"display_name" gorm:"type:varchar(255)"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" gorm:"index"`
	Model             string    `json:"model" gorm:"column:model;type:varchar(255)"`
	ModelSelectionID  string    `json:"model_selection_id" gorm:"column:model_selection_id;type:varchar(255)"`
	SystemPrompt      string    `json:"system_prompt" gorm:"type:text"`
	Status            string    `json:"status" gorm:"type:varchar(50)"`
	Mode              string    `json:"mode" gorm:"type:varchar(50)"`
	ProviderID        string    `json:"provider_id" gorm:"type:varchar(255)"`
	AgentID           string    `json:"agent_id" gorm:"type:varchar(255)"`
	PermissionProfile string    `json:"permission_profile" gorm:"type:varchar(255)"`
	Hidden            bool      `json:"hidden" gorm:"default:false"`
	ExtraDirectories  string    `json:"extra_directories" gorm:"type:text"`
	IsUserBound       bool      `json:"is_user_bound" gorm:"default:false"`
	RuntimeSessionID  string    `json:"runtime_session_id" gorm:"type:varchar(255)"`
	Source            string    `json:"source" gorm:"type:varchar(50)"`

	Messages []Message `json:"messages,omitempty" gorm:"foreignKey:UserID,SessionID;references:UserID,ID;constraint:OnDelete:CASCADE"`
}

func (Session) TableName() string { return "cloud_sessions" }

type Message struct {
	UserID     string    `json:"user_id" gorm:"primaryKey;type:varchar(255);not null"`
	TenantID   string    `json:"tenant_id" gorm:"type:varchar(64);not null;default:'';index"`
	ID         string    `json:"id" gorm:"primaryKey;type:varchar(255);not null"`
	SessionID  string    `json:"session_id" gorm:"primaryKey;type:varchar(255);not null"`
	Role       string    `json:"role" gorm:"type:varchar(50)"`
	Content    string    `json:"content" gorm:"type:longtext"`
	CreatedAt  time.Time `json:"created_at"`
	Hidden     bool      `json:"hidden" gorm:"default:false"`
	TokenUsage string    `json:"token_usage" gorm:"type:text"`
	Feedback   string    `json:"feedback" gorm:"type:text"`
	Aigc       string    `json:"aigc" gorm:"type:text"`
}

func (Message) TableName() string { return "cloud_messages" }

// SessionSyncState：v2 同步键的权威状态行（tenant/user/session 唯一）。
// version=0 是 claim 占位（claimed-empty / claimed-legacy，无快照身份）；
// deleted=true 为永久 tombstone，identity 字段保留删除 envelope 的最小身份。
// 该行是同步协议的 ownership/version 权威，不随 cloud_sessions 级联删除
// （issue #232 契约 §8）。
type SessionSyncState struct {
	// 代理主键：业务唯一键是 (tenant_id, user_id, session_id) 唯一索引；
	// 代理 ID 仅为 GORM 行更新（Save）提供定位。
	ID          uint64 `gorm:"primaryKey;autoIncrement"`
	TenantID    string `gorm:"type:varchar(64);not null;uniqueIndex:uk_session_sync_state,priority:1"`
	UserID      string `gorm:"type:varchar(255);not null;uniqueIndex:uk_session_sync_state,priority:2"`
	SessionID   string `gorm:"type:varchar(255);not null;uniqueIndex:uk_session_sync_state,priority:3"`
	SourceID    string `gorm:"type:varchar(36);not null;default:''"`
	Version     int64  `gorm:"not null;default:0"`
	SnapshotID  string `gorm:"type:varchar(36);not null;default:''"`
	Digest      string `gorm:"type:varchar(64);not null;default:''"`
	Kind        string `gorm:"type:varchar(16);not null;default:''"`
	LegacyClaim bool   `gorm:"not null;default:false"`
	Deleted     bool   `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (SessionSyncState) TableName() string { return "session_sync_states" }
