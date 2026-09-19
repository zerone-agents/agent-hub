package decision

import "time"

const (
	StatusOpen      = "open"
	StatusPassed    = "passed"
	StatusRejected  = "rejected"
	StatusNoQuorum  = "no_quorum"
	StatusTimedOut  = "timed_out"
	StatusEscalated = "escalated"
	VoteApprove     = "approve"
	VoteReject      = "reject"
	VoteAbstain     = "abstain"
	TimeoutNone     = "none"
	TimeoutEscalate = "escalate"
	TimeoutTransfer = "transfer"
)

// Decision is an immutable electorate snapshot plus its computed outcome.
// Domain-specific motives (personality, politics, game rules) deliberately live outside Core.
type Decision struct {
	ID                string     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID          string     `gorm:"type:varchar(64);not null;index" json:"-"`
	GroupID           string     `gorm:"type:char(36);not null;index" json:"groupId"`
	WorkflowRunID     string     `gorm:"type:char(36);not null;default:'';index" json:"workflowRunId,omitempty"`
	WorkflowStepRunID string     `gorm:"type:char(36);not null;default:'';index" json:"workflowStepRunId,omitempty"`
	Title             string     `gorm:"type:varchar(200);not null" json:"title"`
	Description       string     `gorm:"type:text" json:"description"`
	Status            string     `gorm:"type:varchar(24);not null;index" json:"status"`
	QuorumPercent     int        `gorm:"not null" json:"quorumPercent"`
	ApprovalPercent   int        `gorm:"not null" json:"approvalPercent"`
	TimeoutAction     string     `gorm:"type:varchar(16);not null" json:"timeoutAction"`
	EscalateAgentID   uint64     `gorm:"not null;default:0" json:"escalateAgentId,omitempty"`
	DeadlineAt        *time.Time `json:"deadlineAt,omitempty"`
	ClosedAt          *time.Time `json:"closedAt,omitempty"`
	CreatedBy         string     `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	Electorate        []Elector  `gorm:"foreignKey:DecisionID" json:"electorate,omitempty"`
	Votes             []Vote     `gorm:"foreignKey:DecisionID" json:"votes,omitempty"`
	Result            *Result    `gorm:"foreignKey:DecisionID" json:"result,omitempty"`
}

func (Decision) TableName() string { return "collective_decisions" }

// Elector freezes membership, role, voting weight and veto authority at opening time.
type Elector struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID   string `gorm:"type:varchar(64);not null;uniqueIndex:uk_decision_elector,priority:1" json:"-"`
	DecisionID string `gorm:"type:char(36);not null;uniqueIndex:uk_decision_elector,priority:2;index" json:"decisionId"`
	AgentID    uint64 `gorm:"not null;uniqueIndex:uk_decision_elector,priority:3" json:"agentId"`
	Role       string `gorm:"type:varchar(32);not null" json:"role"`
	Weight     int    `gorm:"not null" json:"weight"`
	CanVeto    bool   `gorm:"not null;default:false" json:"canVeto"`
}

func (Elector) TableName() string { return "collective_decision_electors" }

type Vote struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_decision_vote,priority:1" json:"-"`
	DecisionID  string    `gorm:"type:char(36);not null;uniqueIndex:uk_decision_vote,priority:2;index" json:"decisionId"`
	AgentID     uint64    `gorm:"not null;uniqueIndex:uk_decision_vote,priority:3" json:"agentId"`
	Choice      string    `gorm:"type:varchar(16);not null" json:"choice"`
	Weight      int       `gorm:"not null" json:"weight"`
	VetoApplied bool      `gorm:"not null;default:false" json:"vetoApplied"`
	Reason      string    `gorm:"type:text" json:"reason"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (Vote) TableName() string { return "collective_decision_votes" }

type Result struct {
	ID                  string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID            string    `gorm:"type:varchar(64);not null;index" json:"-"`
	DecisionID          string    `gorm:"type:char(36);not null;uniqueIndex" json:"decisionId"`
	Outcome             string    `gorm:"type:varchar(24);not null" json:"outcome"`
	TotalWeight         int       `json:"totalWeight"`
	ParticipatingWeight int       `json:"participatingWeight"`
	ApproveWeight       int       `json:"approveWeight"`
	RejectWeight        int       `json:"rejectWeight"`
	AbstainWeight       int       `json:"abstainWeight"`
	QuorumMet           bool      `json:"quorumMet"`
	Vetoed              bool      `json:"vetoed"`
	Explanation         string    `gorm:"type:text" json:"explanation"`
	ComputedAt          time.Time `json:"computedAt"`
}

func (Result) TableName() string { return "collective_decision_results" }

type Audit struct {
	ID         string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID   string         `gorm:"type:varchar(64);not null;index" json:"-"`
	DecisionID string         `gorm:"type:char(36);not null;index:idx_decision_audit,priority:1" json:"decisionId"`
	ActorID    string         `gorm:"type:varchar(128);not null;default:''" json:"actorId"`
	Action     string         `gorm:"type:varchar(32);not null" json:"action"`
	Payload    map[string]any `gorm:"type:json;serializer:json" json:"payload,omitempty"`
	CreatedAt  time.Time      `gorm:"index:idx_decision_audit,priority:2" json:"createdAt"`
}

func (Audit) TableName() string { return "collective_decision_audits" }

type Escalation struct {
	ID            string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID      string    `gorm:"type:varchar(64);not null;index" json:"-"`
	DecisionID    string    `gorm:"type:char(36);not null;index" json:"decisionId"`
	Kind          string    `gorm:"type:varchar(16);not null" json:"kind"`
	TargetAgentID uint64    `gorm:"not null;default:0" json:"targetAgentId,omitempty"`
	Reason        string    `gorm:"type:text" json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (Escalation) TableName() string { return "collective_decision_escalations" }
