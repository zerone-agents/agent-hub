package chat

import (
	"time"
)

type Session struct {
	UserID            string    `json:"user_id" gorm:"primaryKey;type:varchar(255);not null"`
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
