// Package event contains the platform-neutral durable event envelope and its
// delivery state. Domain applications add facts to Data; they never add
// columns to these core tables.
package event

import (
	"encoding/json"
	"time"
)

const (
	DeliveryPending    = "pending"
	DeliveryProcessing = "processing"
	DeliveryRetry      = "retry"
	DeliveryDelivered  = "delivered"
	DeliveryCancelled  = "cancelled"
	DeliveryDeadLetter = "dead_letter"
)

type Ref struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Envelope is immutable after acceptance. Sequence is monotonic only within
// tenant + run + scope + subject; it is deliberately not a global order.
type Envelope struct {
	ID             string         `gorm:"type:char(36);primaryKey" json:"id"`
	SpecVersion    string         `gorm:"type:varchar(16);not null;default:'1.0'" json:"specVersion"`
	TenantID       string         `gorm:"type:varchar(64);not null;uniqueIndex:uk_events_idempotency,priority:1;index:idx_events_stream,priority:1" json:"-"`
	RunID          string         `gorm:"type:char(36);not null;index:idx_events_stream,priority:2" json:"runId"`
	Type           string         `gorm:"type:varchar(255);not null;index" json:"type"`
	Source         string         `gorm:"type:varchar(255);not null" json:"source"`
	ScopeType      string         `gorm:"type:varchar(64);not null;index:idx_events_stream,priority:3" json:"-"`
	ScopeID        string         `gorm:"type:varchar(128);not null;index:idx_events_stream,priority:4" json:"-"`
	SubjectType    string         `gorm:"type:varchar(64);not null;index:idx_events_stream,priority:5" json:"-"`
	SubjectID      string         `gorm:"type:varchar(128);not null;index:idx_events_stream,priority:6" json:"-"`
	ActorType      string         `gorm:"type:varchar(64);not null;default:''" json:"-"`
	ActorID        string         `gorm:"type:varchar(128);not null;default:''" json:"-"`
	Visibility     string         `gorm:"type:varchar(32);not null;default:'participants'" json:"visibility"`
	CorrelationID  string         `gorm:"type:varchar(128);not null;index" json:"correlationId"`
	CausationID    string         `gorm:"type:varchar(128);not null;default:'';index" json:"causationId,omitempty"`
	RootEventID    string         `gorm:"type:varchar(128);not null;index" json:"rootEventId"`
	IdempotencyKey string         `gorm:"type:varchar(191);not null;uniqueIndex:uk_events_idempotency,priority:2" json:"idempotencyKey"`
	Sequence       uint64         `gorm:"not null;index:idx_events_stream,priority:7" json:"sequence"`
	DataSchema     string         `gorm:"type:varchar(255);not null;default:''" json:"dataSchema,omitempty"`
	Data           map[string]any `gorm:"type:json;serializer:json" json:"data"`
	OccurredAt     time.Time      `gorm:"not null" json:"occurredAt"`
	ScheduledAt    time.Time      `gorm:"not null;index" json:"scheduledAt"`
	RecordedAt     time.Time      `gorm:"not null;index" json:"recordedAt"`
}

func (Envelope) TableName() string { return "events" }

func (e Envelope) Scope() Ref   { return Ref{Type: e.ScopeType, ID: e.ScopeID} }
func (e Envelope) Subject() Ref { return Ref{Type: e.SubjectType, ID: e.SubjectID} }
func (e Envelope) Actor() Ref   { return Ref{Type: e.ActorType, ID: e.ActorID} }

// MarshalJSON exposes protocol refs as nested objects while retaining scalar
// columns for portable database indexing.
func (e Envelope) MarshalJSON() ([]byte, error) {
	type alias Envelope
	return json.Marshal(struct {
		alias
		Scope   Ref `json:"scope"`
		Subject Ref `json:"subject"`
		Actor   Ref `json:"actor"`
	}{alias: alias(e), Scope: e.Scope(), Subject: e.Subject(), Actor: e.Actor()})
}

// StreamCursor serializes sequence allocation for a single ordered stream.
type StreamCursor struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_event_stream,priority:1"`
	RunID       string    `gorm:"type:char(36);not null;uniqueIndex:uk_event_stream,priority:2"`
	ScopeType   string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_event_stream,priority:3"`
	ScopeID     string    `gorm:"type:varchar(128);not null;uniqueIndex:uk_event_stream,priority:4"`
	SubjectType string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_event_stream,priority:5"`
	SubjectID   string    `gorm:"type:varchar(128);not null;uniqueIndex:uk_event_stream,priority:6"`
	Sequence    uint64    `gorm:"not null;default:0"`
	UpdatedAt   time.Time `gorm:"not null"`
}

func (StreamCursor) TableName() string { return "event_stream_cursors" }

// Delivery is the transactional outbox row. A worker claims it with a lease;
// an expired lease is eligible for retry after process restart.
type Delivery struct {
	ID             string     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID       string     `gorm:"type:varchar(64);not null;index:idx_deliveries_ready,priority:1" json:"-"`
	RunID          string     `gorm:"type:char(36);not null;index" json:"runId"`
	EventID        string     `gorm:"type:char(36);not null;uniqueIndex" json:"eventId"`
	Status         string     `gorm:"type:varchar(24);not null;index:idx_deliveries_ready,priority:2" json:"status"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts    int        `gorm:"not null;default:5" json:"maxAttempts"`
	TimeoutMillis  int64      `gorm:"not null;default:30000" json:"timeoutMillis"`
	NextAttemptAt  time.Time  `gorm:"not null;index:idx_deliveries_ready,priority:3" json:"nextAttemptAt"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
	LastError      string     `gorm:"type:text" json:"lastError,omitempty"`
	DeliveredAt    *time.Time `json:"deliveredAt,omitempty"`
	CancelledAt    *time.Time `json:"cancelledAt,omitempty"`
	DeadLetteredAt *time.Time `json:"deadLetteredAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (Delivery) TableName() string { return "event_deliveries" }

type DeliveryAttempt struct {
	ID         string     `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID   string     `gorm:"type:varchar(64);not null;index" json:"-"`
	DeliveryID string     `gorm:"type:char(36);not null;index" json:"deliveryId"`
	Attempt    int        `gorm:"not null" json:"attempt"`
	Status     string     `gorm:"type:varchar(24);not null" json:"status"`
	Error      string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt  time.Time  `gorm:"not null" json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func (DeliveryAttempt) TableName() string { return "event_delivery_attempts" }

// CausalBudget is a durable guard shared by every event in one root chain.
// Duration is represented by DeadlineAt, so a restart cannot reset the clock.
type CausalBudget struct {
	ID                uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID          string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_causal_budget,priority:1" json:"-"`
	RunID             string    `gorm:"type:char(36);not null;uniqueIndex:uk_causal_budget,priority:2;index" json:"runId"`
	RootEventID       string    `gorm:"type:char(36);not null;uniqueIndex:uk_causal_budget,priority:3;index" json:"rootEventId"`
	MaxEvents         int64     `gorm:"not null;default:1000" json:"maxEvents"`
	MaxToolCalls      int64     `gorm:"not null;default:100" json:"maxToolCalls"`
	MaxTokens         int64     `gorm:"not null;default:1000000" json:"maxTokens"`
	EventsUsed        int64     `gorm:"not null;default:0" json:"eventsUsed"`
	ToolCallsUsed     int64     `gorm:"not null;default:0" json:"toolCallsUsed"`
	TokensUsed        int64     `gorm:"not null;default:0" json:"tokensUsed"`
	DeadlineAt        time.Time `gorm:"not null;index" json:"deadlineAt"`
	Terminated        bool      `gorm:"not null;default:false;index" json:"terminated"`
	TerminationReason string    `gorm:"type:varchar(64);not null;default:''" json:"terminationReason,omitempty"`
	GuardEventID      string    `gorm:"type:char(36);not null;default:''" json:"guardEventId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (CausalBudget) TableName() string { return "event_causal_budgets" }
