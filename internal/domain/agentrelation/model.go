package agentrelation

import (
	"time"

	"control-panel/internal/domain/agent"
)

const (
	DefaultScope          = "global"
	DefaultContextPolicy  = "summary_only"
	DefaultDeliveryPolicy = "async"
)

var RelationTypes = map[string]struct{}{
	"reports_to":     {},
	"peer":           {},
	"advisor":        {},
	"reviewer":       {},
	"oversight":      {},
	"representative": {},
	"opponent":       {},
	"external":       {},
}

var BidirectionalRelationTypes = map[string]struct{}{
	"peer":     {},
	"opponent": {},
	"external": {},
}

var Stances = map[string]struct{}{
	"allied":      {},
	"friendly":    {},
	"neutral":     {},
	"wary":        {},
	"competitive": {},
	"hostile":     {},
}

var Actions = map[string]struct{}{
	"inform":    {},
	"consult":   {},
	"assign":    {},
	"report":    {},
	"submit":    {},
	"review":    {},
	"challenge": {},
	"handoff":   {},
	"escalate":  {},
	"invite":    {},
}

var ContextPolicies = map[string]struct{}{
	"none":          {},
	"summary_only":  {},
	"shared_thread": {},
}

var DeliveryPolicies = map[string]struct{}{
	"sync":  {},
	"async": {},
}

// AgentRelation is one directed edge in an organization graph. A bidirectional
// relationship is intentionally represented by two independently editable rows.
type AgentRelation struct {
	ID             uint64            `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID       string            `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_agent_relations_edge,priority:1;index" json:"-"`
	Scope          string            `gorm:"type:varchar(64);not null;default:'global';uniqueIndex:uk_agent_relations_edge,priority:2" json:"scope"`
	SourceAgentID  uint64            `gorm:"column:source_agent_id;not null;uniqueIndex:uk_agent_relations_edge,priority:3;index" json:"sourceAgentId"`
	TargetAgentID  uint64            `gorm:"column:target_agent_id;not null;uniqueIndex:uk_agent_relations_edge,priority:4;index" json:"targetAgentId"`
	RelationType   string            `gorm:"column:relation_type;type:varchar(32);not null;index" json:"relationType"`
	Stance         string            `gorm:"type:varchar(32);not null;default:'neutral'" json:"stance"`
	AllowedActions []string          `gorm:"column:allowed_actions;type:json;serializer:json" json:"allowedActions"`
	ContextPolicy  string            `gorm:"column:context_policy;type:varchar(32);not null;default:'summary_only'" json:"contextPolicy"`
	DeliveryPolicy string            `gorm:"column:delivery_policy;type:varchar(16);not null;default:'async'" json:"deliveryPolicy"`
	Constraint     string            `gorm:"type:text" json:"constraint"`
	Enabled        bool              `gorm:"not null;index" json:"enabled"`
	SourceAgent    agent.AgentConfig `gorm:"foreignKey:SourceAgentID;constraint:OnDelete:CASCADE" json:"-"`
	TargetAgent    agent.AgentConfig `gorm:"foreignKey:TargetAgentID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt      time.Time         `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time         `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (AgentRelation) TableName() string { return "agent_relations" }
