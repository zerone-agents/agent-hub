package run

import "time"

const (
	StatusDraft     = "draft"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusArchived  = "archived"
)

var ValidTransitions = map[string]map[string]bool{
	StatusDraft:     {StatusRunning: true, StatusArchived: true},
	StatusRunning:   {StatusPaused: true, StatusCompleted: true},
	StatusPaused:    {StatusRunning: true, StatusCompleted: true},
	StatusCompleted: {StatusArchived: true},
}

type Run struct {
	ID          string              `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID    string              `gorm:"type:varchar(64);not null;index:idx_runs_tenant_status,priority:1" json:"-"`
	Name        string              `gorm:"type:varchar(160);not null" json:"name"`
	Description string              `gorm:"type:text" json:"description"`
	Status      string              `gorm:"type:varchar(16);not null;index:idx_runs_tenant_status,priority:2" json:"status"`
	Metadata    map[string]any      `gorm:"type:json;serializer:json" json:"metadata"`
	CreatedBy   string              `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	StartedAt   *time.Time          `json:"startedAt,omitempty"`
	PausedAt    *time.Time          `json:"pausedAt,omitempty"`
	CompletedAt *time.Time          `json:"completedAt,omitempty"`
	ArchivedAt  *time.Time          `json:"archivedAt,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Agents      []RunAgent          `gorm:"foreignKey:RunID" json:"agents,omitempty"`
	Bindings    []CapabilityBinding `gorm:"foreignKey:RunID" json:"capabilityBindings,omitempty"`
}

func (Run) TableName() string { return "runs" }

type RunAgent struct {
	ID                         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID                   string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_run_agents,priority:1;index" json:"-"`
	RunID                      string         `gorm:"type:char(36);not null;uniqueIndex:uk_run_agents,priority:2;index" json:"runId"`
	AgentID                    uint64         `gorm:"not null;uniqueIndex:uk_run_agents,priority:3;index" json:"agentId"`
	Role                       string         `gorm:"type:varchar(64);not null;default:'participant'" json:"role"`
	AgentNameSnapshot          string         `gorm:"type:varchar(64);not null" json:"agentNameSnapshot"`
	AgentConfigHashSnapshot    string         `gorm:"type:varchar(128);not null" json:"agentConfigHashSnapshot"`
	PersonalityTemplateName    string         `gorm:"type:varchar(64);not null;default:''" json:"personalityTemplateName"`
	PersonalityTemplateVersion int            `gorm:"not null;default:0" json:"personalityTemplateVersion"`
	PersonalityPromptHash      string         `gorm:"type:char(64);not null;default:''" json:"personalityPromptHash"`
	Snapshot                   map[string]any `gorm:"type:json;serializer:json" json:"snapshot"`
	CreatedAt                  time.Time      `json:"createdAt"`
}

func (RunAgent) TableName() string { return "run_agents" }

type CapabilityBinding struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_run_capability_bindings,priority:1;index" json:"-"`
	RunID        string         `gorm:"type:char(36);not null;uniqueIndex:uk_run_capability_bindings,priority:2;index" json:"runId"`
	Namespace    string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_run_capability_bindings,priority:3" json:"namespace"`
	PackageName  string         `gorm:"type:varchar(160);not null" json:"packageName"`
	Version      string         `gorm:"type:varchar(32);not null" json:"version"`
	ContentHash  string         `gorm:"type:varchar(128);not null" json:"contentHash"`
	ManifestYAML string         `gorm:"type:longtext;not null" json:"manifestYAML"`
	Snapshot     map[string]any `gorm:"type:json;serializer:json" json:"snapshot"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func (CapabilityBinding) TableName() string { return "run_capability_bindings" }

type StateSchema struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID     string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_state_schemas,priority:1;index" json:"-"`
	Namespace    string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_state_schemas,priority:2" json:"namespace"`
	Name         string         `gorm:"type:varchar(96);not null;uniqueIndex:uk_state_schemas,priority:3" json:"name"`
	Version      string         `gorm:"type:varchar(32);not null;uniqueIndex:uk_state_schemas,priority:4" json:"version"`
	Schema       map[string]any `gorm:"column:schema_json;type:json;serializer:json" json:"schema"`
	ScopeTypes   []string       `gorm:"type:json;serializer:json" json:"scopeTypes"`
	SubjectTypes []string       `gorm:"type:json;serializer:json" json:"subjectTypes"`
	ContentHash  string         `gorm:"type:char(64);not null" json:"contentHash"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func (StateSchema) TableName() string { return "state_schemas" }

type RunState struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_run_states,priority:1;index" json:"-"`
	RunID         string         `gorm:"type:char(36);not null;uniqueIndex:uk_run_states,priority:2;index" json:"runId"`
	Namespace     string         `gorm:"type:varchar(160);not null;uniqueIndex:uk_run_states,priority:3" json:"namespace"`
	SchemaName    string         `gorm:"type:varchar(96);not null" json:"schemaName"`
	SchemaVersion string         `gorm:"type:varchar(32);not null" json:"schemaVersion"`
	SchemaHash    string         `gorm:"type:char(64);not null" json:"schemaHash"`
	SubjectType   string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_run_states,priority:4" json:"subjectType"`
	SubjectID     string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_run_states,priority:5" json:"subjectId"`
	Revision      uint64         `gorm:"not null;default:1" json:"revision"`
	Data          map[string]any `gorm:"type:json;serializer:json" json:"data"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

func (RunState) TableName() string { return "run_states" }

type RunStateChange struct {
	ID             string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string         `gorm:"type:varchar(64);not null;index;uniqueIndex:uk_run_state_change_idempotency,priority:1" json:"-"`
	RunID          string         `gorm:"type:char(36);not null;index" json:"runId"`
	RunStateID     uint64         `gorm:"not null;index" json:"runStateId"`
	RevisionBefore uint64         `gorm:"not null" json:"revisionBefore"`
	RevisionAfter  uint64         `gorm:"not null" json:"revisionAfter"`
	Before         map[string]any `gorm:"type:json;serializer:json" json:"before"`
	After          map[string]any `gorm:"type:json;serializer:json" json:"after"`
	Reason         string         `gorm:"type:text" json:"reason"`
	Source         string         `gorm:"type:varchar(128);not null;default:'api'" json:"source"`
	IdempotencyKey string         `gorm:"type:varchar(191);not null;uniqueIndex:uk_run_state_change_idempotency,priority:2" json:"idempotencyKey"`
	CreatedAt      time.Time      `json:"createdAt"`
}

func (RunStateChange) TableName() string { return "run_state_changes" }

// RunActivity is the H1 append-only execution timeline. It records observable
// lifecycle/step/tool facts; the causal, retryable event bus remains an H2 concern.
type RunActivity struct {
	ID         string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID   string         `gorm:"type:varchar(64);not null;index" json:"-"`
	RunID      string         `gorm:"type:char(36);not null;index:idx_run_activities_timeline,priority:1" json:"runId"`
	Kind       string         `gorm:"type:varchar(48);not null;index" json:"kind"`
	Status     string         `gorm:"type:varchar(32);not null;default:''" json:"status"`
	ActorType  string         `gorm:"type:varchar(32);not null;default:''" json:"actorType"`
	ActorID    string         `gorm:"type:varchar(128);not null;default:''" json:"actorId"`
	StepID     string         `gorm:"type:varchar(128);not null;default:'';index" json:"stepId"`
	Name       string         `gorm:"type:varchar(160);not null;default:''" json:"name"`
	Input      map[string]any `gorm:"type:json;serializer:json" json:"input,omitempty"`
	Output     map[string]any `gorm:"type:json;serializer:json" json:"output,omitempty"`
	Error      string         `gorm:"type:text" json:"error,omitempty"`
	OccurredAt time.Time      `gorm:"not null;index:idx_run_activities_timeline,priority:2" json:"occurredAt"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func (RunActivity) TableName() string { return "run_activities" }
