package run

import "time"

// ToolResult is the platform-neutral result envelope returned by every tool.
// Tools may describe desired state changes, but only ToolResultService can
// validate and commit them as authoritative Run state.
type ToolResult struct {
	Result         map[string]any  `json:"result,omitempty"`
	Cost           ToolCost        `json:"cost"`
	StateProposals []StateProposal `json:"stateProposals,omitempty"`
	EventProposals []EventProposal `json:"eventProposals,omitempty"`
	LatencyMS      int64           `json:"latencyMs"`
	AvailableAt    *time.Time      `json:"availableAt,omitempty"`
}

type ToolCost struct {
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
	ToolCalls    int64   `json:"toolCalls,omitempty"`
	Amount       float64 `json:"amount,omitempty"`
	Currency     string  `json:"currency,omitempty"`
}

type StateProposal struct {
	StateID          uint64           `json:"stateId"`
	ExpectedRevision uint64           `json:"expectedRevision"`
	Patch            []PatchOperation `json:"patch"`
	Reason           string           `json:"reason,omitempty"`
}

// PatchOperation is deliberately smaller than arbitrary JSON Patch. H2 only
// permits deterministic field changes; scripts, expressions and whole-state
// replacement are not accepted from a model or remote tool.
type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type EventProposal struct {
	Type        string         `json:"type"`
	SubjectType string         `json:"subjectType,omitempty"`
	SubjectID   string         `json:"subjectId,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
	DelayMS     int64          `json:"delayMs,omitempty"`
}

type ToolResultRecord struct {
	ID               string          `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID         string          `gorm:"type:varchar(64);not null;uniqueIndex:uk_tool_result_idempotency,priority:1;index" json:"-"`
	RunID            string          `gorm:"type:char(36);not null;uniqueIndex:uk_tool_result_idempotency,priority:2;index" json:"runId"`
	IdempotencyKey   string          `gorm:"type:varchar(191);not null;uniqueIndex:uk_tool_result_idempotency,priority:3" json:"idempotencyKey"`
	ToolName         string          `gorm:"type:varchar(160);not null" json:"toolName"`
	ActorType        string          `gorm:"type:varchar(32);not null;default:''" json:"actorType"`
	ActorID          string          `gorm:"type:varchar(128);not null;default:''" json:"actorId"`
	CorrelationID    string          `gorm:"type:varchar(64);not null;default:'';index" json:"correlationId,omitempty"`
	CausationID      string          `gorm:"type:varchar(64);not null;default:'';index" json:"causationId,omitempty"`
	RootEventID      string          `gorm:"type:varchar(64);not null;default:'';index" json:"rootEventId,omitempty"`
	Status           string          `gorm:"type:varchar(24);not null" json:"status"`
	DecisionReason   string          `gorm:"type:text" json:"decisionReason,omitempty"`
	Result           map[string]any  `gorm:"type:json;serializer:json" json:"result,omitempty"`
	Cost             ToolCost        `gorm:"type:json;serializer:json" json:"cost"`
	StateProposals   []StateProposal `gorm:"type:json;serializer:json" json:"stateProposals,omitempty"`
	EventProposals   []EventProposal `gorm:"type:json;serializer:json" json:"eventProposals,omitempty"`
	LatencyMS        int64           `gorm:"not null;default:0" json:"latencyMs"`
	AvailableAt      *time.Time      `json:"availableAt,omitempty"`
	CommittedChanges []string        `gorm:"type:json;serializer:json" json:"committedChangeIds,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

func (ToolResultRecord) TableName() string { return "tool_result_records" }
