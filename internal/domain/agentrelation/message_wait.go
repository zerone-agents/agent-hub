package agentrelation

import "time"

const (
	MessageWaitStatusWaiting   = "waiting"
	MessageWaitStatusResuming  = "resuming"
	MessageWaitStatusResumed   = "resumed"
	MessageWaitStatusExpired   = "expired"
	MessageWaitStatusCancelled = "cancelled"
)

// AgentMessageWait is the durable continuation registered when an agent sends
// an asynchronous child message and suspends its current execution. Completion
// of WaitingMessageID claims this row and resumes ParentMessageID exactly once.
// DedupeKey is unique inside a tenant and makes registration replay-safe.
type AgentMessageWait struct {
	ID               string     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID         string     `gorm:"type:varchar(64);not null;uniqueIndex:uk_agent_message_wait_dedupe,priority:1;index:idx_agent_message_wait_ready,priority:1" json:"-"`
	RunID            string     `gorm:"type:char(36);not null;index;index:idx_agent_message_wait_ready,priority:2" json:"runId"`
	WaitingAgentID   uint64     `gorm:"not null;index" json:"waitingAgentId"`
	WaitingAgent     string     `gorm:"type:varchar(64);not null" json:"waitingAgent"`
	RuntimeSessionID string     `gorm:"type:varchar(191);not null" json:"runtimeSessionId"`
	ParentMessageID  string     `gorm:"type:char(36);not null;index" json:"parentMessageId"`
	WaitingMessageID string     `gorm:"type:char(36);not null;index;index:idx_agent_message_wait_ready,priority:3" json:"waitingMessageId"`
	Status           string     `gorm:"type:varchar(16);not null;index:idx_agent_message_wait_ready,priority:4" json:"status"`
	DeadlineAt       *time.Time `gorm:"index:idx_agent_message_wait_deadline" json:"deadlineAt,omitempty"`
	ResumeAttempts   int        `gorm:"not null;default:0" json:"resumeAttempts"`
	ResumeEventID    string     `gorm:"type:varchar(64);not null;default:'';index" json:"resumeEventId,omitempty"`
	DedupeKey        string     `gorm:"type:varchar(191);not null;uniqueIndex:uk_agent_message_wait_dedupe,priority:2" json:"dedupeKey"`
	LastError        string     `gorm:"type:text" json:"lastError,omitempty"`
	CancelReason     string     `gorm:"type:text" json:"cancelReason,omitempty"`
	CreatedAt        time.Time  `gorm:"not null" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"not null" json:"updatedAt"`
	ClaimedAt        *time.Time `json:"claimedAt,omitempty"`
	ResumedAt        *time.Time `json:"resumedAt,omitempty"`
	ExpiredAt        *time.Time `json:"expiredAt,omitempty"`
	CancelledAt      *time.Time `json:"cancelledAt,omitempty"`
}

func (AgentMessageWait) TableName() string { return "agent_message_waits" }
