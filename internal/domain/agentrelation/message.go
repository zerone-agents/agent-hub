package agentrelation

import "time"

const (
	MessageStatusQueued    = "queued"
	MessageStatusRunning   = "running"
	MessageStatusCompleted = "completed"
	MessageStatusFailed    = "failed"
)

// AgentMessage is the durable audit record for one relation-authorized
// delivery. The original content and the target's reply are stored separately
// so a caller can poll an asynchronous delivery without reconstructing a chat
// transcript. RelationID anchors the exact policy that authorized the send.
type AgentMessage struct {
	ID             string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	TenantID       string     `gorm:"type:varchar(64);not null;default:'';index:idx_agent_messages_tenant_created,priority:1;index" json:"-"`
	RelationID     uint64     `gorm:"column:relation_id;not null;index" json:"relationId"`
	Scope          string     `gorm:"type:varchar(64);not null;index" json:"scope"`
	SourceAgentID  uint64     `gorm:"column:source_agent_id;not null;index" json:"sourceAgentId"`
	SourceAgent    string     `gorm:"column:source_agent;type:varchar(64);not null" json:"sourceAgent"`
	TargetAgentID  uint64     `gorm:"column:target_agent_id;not null;index" json:"targetAgentId"`
	TargetAgent    string     `gorm:"column:target_agent;type:varchar(64);not null" json:"targetAgent"`
	Action         string     `gorm:"type:varchar(32);not null;index" json:"action"`
	DeliveryPolicy string     `gorm:"column:delivery_policy;type:varchar(16);not null" json:"deliveryPolicy"`
	ContextPolicy  string     `gorm:"column:context_policy;type:varchar(32);not null" json:"contextPolicy"`
	Content        string     `gorm:"type:text;not null" json:"content"`
	SharedContext  string     `gorm:"column:shared_context;type:text" json:"sharedContext,omitempty"`
	Reply          string     `gorm:"type:longtext" json:"reply,omitempty"`
	Status         string     `gorm:"type:varchar(16);not null;index" json:"status"`
	Error          string     `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at;index:idx_agent_messages_tenant_created,priority:2" json:"createdAt"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"`
	CompletedAt    *time.Time `gorm:"column:completed_at" json:"completedAt,omitempty"`
}

func (AgentMessage) TableName() string { return "agent_messages" }
