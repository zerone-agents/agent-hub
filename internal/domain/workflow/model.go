package workflow

import "time"

const (
	VersionDraft        = "draft"
	VersionPublished    = "published"
	ExecutionRunning    = "running"
	ExecutionCompleted  = "completed"
	ExecutionFailed     = "failed"
	StepPending         = "pending"
	StepRunning         = "running"
	StepWaitingApproval = "waiting_approval"
	StepWaitingDecision = "waiting_decision"
	StepCompleted       = "completed"
	StepSkipped         = "skipped"
	StepFailed          = "failed"
	StepTimedOut        = "timed_out"
	ApprovalPending     = "pending"
	ApprovalApproved    = "approved"
	ApprovalRejected    = "rejected"
	DecisionApprove     = "approve"
	DecisionReject      = "reject"
	DecisionConditional = "conditional_approve"
)

type Definition struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_name,priority:1;index" json:"-"`
	Name        string    `gorm:"type:varchar(160);not null;uniqueIndex:uk_workflow_name,priority:2" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	CreatedBy   string    `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Definition) TableName() string { return "workflow_definitions" }

type Version struct {
	ID           string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID     string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_version,priority:1;index" json:"-"`
	WorkflowID   string         `gorm:"type:char(36);not null;uniqueIndex:uk_workflow_version,priority:2;index" json:"workflowId"`
	Version      int            `gorm:"not null;uniqueIndex:uk_workflow_version,priority:3" json:"version"`
	Status       string         `gorm:"type:varchar(16);not null;index" json:"status"`
	InputSchema  map[string]any `gorm:"type:json;serializer:json" json:"inputSchema,omitempty"`
	OutputSchema map[string]any `gorm:"type:json;serializer:json" json:"outputSchema,omitempty"`
	CreatedBy    string         `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	PublishedAt  *time.Time     `json:"publishedAt,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	Steps        []Step         `gorm:"foreignKey:VersionID" json:"steps,omitempty"`
	Transitions  []Transition   `gorm:"foreignKey:VersionID" json:"transitions,omitempty"`
}

func (Version) TableName() string { return "workflow_versions" }

type Step struct {
	ID                  string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID            string         `gorm:"type:varchar(64);not null;index" json:"-"`
	VersionID           string         `gorm:"type:char(36);not null;uniqueIndex:uk_workflow_step,priority:1;index" json:"versionId"`
	Key                 string         `gorm:"type:varchar(96);not null;uniqueIndex:uk_workflow_step,priority:2" json:"key"`
	Name                string         `gorm:"type:varchar(160);not null" json:"name"`
	Type                string         `gorm:"type:varchar(24);not null" json:"type"`
	ActorType           string         `gorm:"type:varchar(24);not null;default:'agent'" json:"actorType"`
	ActorRef            string         `gorm:"type:varchar(128);not null;default:''" json:"actorRef"`
	DependsOn           []string       `gorm:"type:json;serializer:json" json:"dependsOn,omitempty"`
	Config              map[string]any `gorm:"type:json;serializer:json" json:"config,omitempty"`
	TimeoutSeconds      int            `gorm:"not null;default:0" json:"timeoutSeconds"`
	MaxRetries          int            `gorm:"not null;default:0" json:"maxRetries"`
	EscalationStepKey   string         `gorm:"type:varchar(96);not null;default:''" json:"escalationStepKey,omitempty"`
	CompensationStepKey string         `gorm:"type:varchar(96);not null;default:''" json:"compensationStepKey,omitempty"`
	CreatedAt           time.Time      `json:"createdAt"`
}

func (Step) TableName() string { return "workflow_steps" }

type Transition struct {
	ID          uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID    string         `gorm:"type:varchar(64);not null;index" json:"-"`
	VersionID   string         `gorm:"type:char(36);not null;index" json:"versionId"`
	FromStepKey string         `gorm:"type:varchar(96);not null" json:"fromStepKey"`
	ToStepKey   string         `gorm:"type:varchar(96);not null" json:"toStepKey"`
	Condition   map[string]any `gorm:"type:json;serializer:json" json:"condition,omitempty"`
	Priority    int            `gorm:"not null;default:0" json:"priority"`
}

func (Transition) TableName() string { return "workflow_transitions" }

type Execution struct {
	ID             string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_execution_idem,priority:1;index" json:"-"`
	VersionID      string         `gorm:"type:char(36);not null;index" json:"versionId"`
	RunID          string         `gorm:"type:char(36);not null;default:'';index" json:"runId,omitempty"`
	Status         string         `gorm:"type:varchar(24);not null;index" json:"status"`
	Input          map[string]any `gorm:"type:json;serializer:json" json:"input,omitempty"`
	Output         map[string]any `gorm:"type:json;serializer:json" json:"output,omitempty"`
	IdempotencyKey string         `gorm:"type:varchar(191);not null;uniqueIndex:uk_workflow_execution_idem,priority:2" json:"idempotencyKey"`
	RequestHash    string         `gorm:"type:char(64);not null" json:"-"`
	StartedBy      string         `gorm:"type:varchar(128);not null;default:''" json:"startedBy"`
	StartedAt      time.Time      `json:"startedAt"`
	CompletedAt    *time.Time     `json:"completedAt,omitempty"`
	StepRuns       []StepRun      `gorm:"foreignKey:ExecutionID" json:"stepRuns,omitempty"`
	Approvals      []Approval     `gorm:"foreignKey:ExecutionID" json:"approvals,omitempty"`
}

func (Execution) TableName() string { return "workflow_executions" }

type StepRun struct {
	ID               string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID         string         `gorm:"type:varchar(64);not null;index" json:"-"`
	ExecutionID      string         `gorm:"type:char(36);not null;uniqueIndex:uk_workflow_step_run,priority:1;index" json:"executionId"`
	StepID           string         `gorm:"type:char(36);not null;uniqueIndex:uk_workflow_step_run,priority:2" json:"stepId"`
	StepKey          string         `gorm:"type:varchar(96);not null;index" json:"stepKey"`
	Status           string         `gorm:"type:varchar(24);not null;index" json:"status"`
	Attempt          int            `gorm:"not null;default:1" json:"attempt"`
	AssigneeSnapshot []string       `gorm:"type:json;serializer:json" json:"assigneeSnapshot"`
	Input            map[string]any `gorm:"type:json;serializer:json" json:"input,omitempty"`
	Output           map[string]any `gorm:"type:json;serializer:json" json:"output,omitempty"`
	Error            string         `gorm:"type:text" json:"error,omitempty"`
	DueAt            *time.Time     `json:"dueAt,omitempty"`
	StartedAt        *time.Time     `json:"startedAt,omitempty"`
	CompletedAt      *time.Time     `json:"completedAt,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
}

func (StepRun) TableName() string { return "workflow_step_runs" }

type StepDispatch struct {
	ID          string     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID    string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_dispatch,priority:1;index" json:"-"`
	StepRunID   string     `gorm:"type:char(36);not null;uniqueIndex:uk_workflow_dispatch,priority:2;index" json:"stepRunId"`
	AgentName   string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_dispatch,priority:3" json:"agentName"`
	Attempt     int        `gorm:"not null;uniqueIndex:uk_workflow_dispatch,priority:4" json:"attempt"`
	Status      string     `gorm:"type:varchar(24);not null" json:"status"`
	Error       string     `gorm:"type:text" json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

func (StepDispatch) TableName() string { return "workflow_step_dispatches" }

type StepReceipt struct {
	ID             string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_workflow_receipt_idem,priority:1;index" json:"-"`
	StepRunID      string    `gorm:"type:char(36);not null;index" json:"stepRunId"`
	ActorID        string    `gorm:"type:varchar(128);not null" json:"actorId"`
	Action         string    `gorm:"type:varchar(16);not null" json:"action"`
	RequestHash    string    `gorm:"type:char(64);not null" json:"requestHash"`
	IdempotencyKey string    `gorm:"type:varchar(191);not null;uniqueIndex:uk_workflow_receipt_idem,priority:2" json:"idempotencyKey"`
	ExecutionID    string    `gorm:"type:char(36);not null" json:"executionId"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (StepReceipt) TableName() string { return "workflow_step_receipts" }

type Approval struct {
	ID               string             `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID         string             `gorm:"type:varchar(64);not null;index" json:"-"`
	ExecutionID      string             `gorm:"type:char(36);not null;index" json:"executionId"`
	StepRunID        string             `gorm:"type:char(36);not null;uniqueIndex" json:"stepRunId"`
	Policy           string             `gorm:"type:varchar(24);not null" json:"policy"`
	Quorum           int                `gorm:"not null;default:1" json:"quorum"`
	Status           string             `gorm:"type:varchar(24);not null;index" json:"status"`
	ApproverSnapshot []string           `gorm:"type:json;serializer:json" json:"approverSnapshot"`
	CreatedAt        time.Time          `json:"createdAt"`
	ResolvedAt       *time.Time         `json:"resolvedAt,omitempty"`
	Decisions        []ApprovalDecision `gorm:"foreignKey:ApprovalID" json:"decisions,omitempty"`
}

func (Approval) TableName() string { return "workflow_approvals" }

type ApprovalDecision struct {
	ID             string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_approval_decision_idem,priority:1;index" json:"-"`
	ApprovalID     string         `gorm:"type:char(36);not null;uniqueIndex:uk_approval_actor,priority:1;index" json:"approvalId"`
	ActorID        string         `gorm:"type:varchar(128);not null;uniqueIndex:uk_approval_actor,priority:2" json:"actorId"`
	Decision       string         `gorm:"type:varchar(24);not null" json:"decision"`
	Reason         string         `gorm:"type:text" json:"reason"`
	Conditions     map[string]any `gorm:"type:json;serializer:json" json:"conditions,omitempty"`
	IdempotencyKey string         `gorm:"type:varchar(191);not null;uniqueIndex:uk_approval_decision_idem,priority:2" json:"idempotencyKey"`
	RequestHash    string         `gorm:"type:char(64);not null" json:"-"`
	CreatedAt      time.Time      `json:"createdAt"`
}

func (ApprovalDecision) TableName() string { return "workflow_approval_decisions" }

type Audit struct {
	ID           string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID     string         `gorm:"type:varchar(64);not null;index" json:"-"`
	ExecutionID  string         `gorm:"type:char(36);not null;index:idx_workflow_audit_timeline,priority:1" json:"executionId"`
	ActorID      string         `gorm:"type:varchar(128);not null;default:''" json:"actorId"`
	Action       string         `gorm:"type:varchar(48);not null;index" json:"action"`
	ResourceType string         `gorm:"type:varchar(32);not null" json:"resourceType"`
	ResourceID   string         `gorm:"type:varchar(128);not null" json:"resourceId"`
	Before       map[string]any `gorm:"type:json;serializer:json" json:"before,omitempty"`
	After        map[string]any `gorm:"type:json;serializer:json" json:"after,omitempty"`
	CreatedAt    time.Time      `gorm:"index:idx_workflow_audit_timeline,priority:2" json:"createdAt"`
}

func (Audit) TableName() string { return "workflow_audits" }
