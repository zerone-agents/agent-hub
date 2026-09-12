package run

import "time"

// PromptFragmentProvenance explains one immutable contribution to a rendered
// prompt. Authorization is intentionally absent: prompt text can guide an
// Agent, but never grants tools, data or communication permissions.
type PromptFragmentProvenance struct {
	Stage         string `json:"stage"`
	Label         string `json:"label"`
	SourceType    string `json:"sourceType"`
	SourceID      string `json:"sourceId"`
	SourceVersion string `json:"sourceVersion"`
	ContentHash   string `json:"contentHash"`
	TokenEstimate int    `json:"tokenEstimate"`
}

// PromptSnapshot is the audit record produced by the H2 Prompt Composer.
type PromptSnapshot struct {
	ID             string                     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string                     `gorm:"type:varchar(64);not null;index" json:"-"`
	RunID          string                     `gorm:"type:char(36);not null;index:idx_prompt_snapshots_run_agent,priority:1" json:"runId"`
	RunAgentID     uint64                     `gorm:"not null;index:idx_prompt_snapshots_run_agent,priority:2" json:"runAgentId"`
	AgentID        uint64                     `gorm:"not null;index" json:"agentId"`
	RenderedText   string                     `gorm:"type:longtext;not null" json:"renderedText"`
	RenderedHash   string                     `gorm:"type:char(64);not null;index" json:"renderedHash"`
	UserInputHash  string                     `gorm:"type:char(64);not null;default:''" json:"userInputHash,omitempty"`
	DeliveryHash   string                     `gorm:"type:char(64);not null;default:'';index" json:"deliveryHash,omitempty"`
	DeliveryStatus string                     `gorm:"type:varchar(24);not null;default:'preview';index" json:"deliveryStatus"`
	Provenance     []PromptFragmentProvenance `gorm:"type:json;serializer:json" json:"provenance"`
	CreatedAt      time.Time                  `json:"createdAt"`
}

func (PromptSnapshot) TableName() string { return "run_prompt_snapshots" }
