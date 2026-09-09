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

// RelationEventBaseDeltas is the authoritative, bounded mapping from a
// perceived event to a relationship-score change. Runtime agents choose an
// event type and severity; they never get to write an arbitrary score.
var RelationEventBaseDeltas = map[string]int{
	"task_completed":     10,
	"task_failed":        -8,
	"promise_kept":       12,
	"promise_broken":     -20,
	"helped":             8,
	"obstructed":         -12,
	"protected":          18,
	"betrayed":           -35,
	"credit_shared":      8,
	"credit_stolen":      -25,
	"public_praise":      6,
	"public_humiliation": -20,
	"truth_verified":     8,
	"lied":               -22,
	"reconciled":         20,
}

var RelationEventVisibilities = map[string]struct{}{
	"private":      {},
	"participants": {},
	"public":       {},
}

// AgentRelation is one directed edge in an organization graph. A bidirectional
// relationship is intentionally represented by two independently editable rows.
type AgentRelation struct {
	ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID      string `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_agent_relations_edge,priority:1;index" json:"-"`
	Scope         string `gorm:"type:varchar(64);not null;default:'global';uniqueIndex:uk_agent_relations_edge,priority:2" json:"scope"`
	SourceAgentID uint64 `gorm:"column:source_agent_id;not null;uniqueIndex:uk_agent_relations_edge,priority:3;index" json:"sourceAgentId"`
	TargetAgentID uint64 `gorm:"column:target_agent_id;not null;uniqueIndex:uk_agent_relations_edge,priority:4;index" json:"targetAgentId"`
	RelationType  string `gorm:"column:relation_type;type:varchar(32);not null;index" json:"relationType"`
	Stance        string `gorm:"type:varchar(32);not null;default:'neutral'" json:"stance"`
	// RelationshipScore is the source of truth for the current, directional
	// social attitude. Stance is its cached human-readable projection.
	RelationshipScore int               `gorm:"column:relationship_score;not null;default:0" json:"relationshipScore"`
	LastChangedAt     *time.Time        `gorm:"column:last_changed_at;index" json:"lastChangedAt,omitempty"`
	AllowedActions    []string          `gorm:"column:allowed_actions;type:json;serializer:json" json:"allowedActions"`
	ContextPolicy     string            `gorm:"column:context_policy;type:varchar(32);not null;default:'summary_only'" json:"contextPolicy"`
	DeliveryPolicy    string            `gorm:"column:delivery_policy;type:varchar(16);not null;default:'async'" json:"deliveryPolicy"`
	Constraint        string            `gorm:"type:text" json:"constraint"`
	Enabled           bool              `gorm:"not null;index" json:"enabled"`
	SourceAgent       agent.AgentConfig `gorm:"foreignKey:SourceAgentID;constraint:OnDelete:CASCADE" json:"-"`
	TargetAgent       agent.AgentConfig `gorm:"foreignKey:TargetAgentID;constraint:OnDelete:CASCADE" json:"-"`
	CreatedAt         time.Time         `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt         time.Time         `gorm:"column:updated_at;index" json:"updatedAt"`
}

func (AgentRelation) TableName() string { return "agent_relations" }

// AgentRelationEvent is the append-only explanation for one relationship
// score transition. Keeping before/after values makes the game replayable even
// if scoring rules change in a later release.
type AgentRelationEvent struct {
	ID             string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	TenantID       string    `gorm:"type:varchar(64);not null;default:'';uniqueIndex:uk_agent_relation_events_key,priority:1;index:idx_agent_relation_events_relation_time,priority:1" json:"-"`
	RelationID     uint64    `gorm:"column:relation_id;not null;uniqueIndex:uk_agent_relation_events_key,priority:2;index:idx_agent_relation_events_relation_time,priority:2;index" json:"relationId"`
	Scope          string    `gorm:"type:varchar(64);not null;index" json:"scope"`
	SourceAgentID  uint64    `gorm:"column:source_agent_id;not null;index" json:"sourceAgentId"`
	TargetAgentID  uint64    `gorm:"column:target_agent_id;not null;index" json:"targetAgentId"`
	EventType      string    `gorm:"column:event_type;type:varchar(32);not null;index" json:"eventType"`
	Severity       int       `gorm:"not null;default:1" json:"severity"`
	Delta          int       `gorm:"not null" json:"delta"`
	ScoreBefore    int       `gorm:"column:score_before;not null" json:"scoreBefore"`
	ScoreAfter     int       `gorm:"column:score_after;not null" json:"scoreAfter"`
	StanceBefore   string    `gorm:"column:stance_before;type:varchar(32);not null" json:"stanceBefore"`
	StanceAfter    string    `gorm:"column:stance_after;type:varchar(32);not null" json:"stanceAfter"`
	Reason         string    `gorm:"type:text" json:"reason"`
	Visibility     string    `gorm:"type:varchar(16);not null;default:'private'" json:"visibility"`
	ActorType      string    `gorm:"column:actor_type;type:varchar(16);not null" json:"actorType"`
	ActorID        string    `gorm:"column:actor_id;type:varchar(128);not null;default:''" json:"actorId,omitempty"`
	SourceKind     string    `gorm:"column:source_kind;type:varchar(32);not null;default:'manual'" json:"sourceKind"`
	SourceID       string    `gorm:"column:source_id;type:varchar(128);not null;default:''" json:"sourceId,omitempty"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(191);not null;uniqueIndex:uk_agent_relation_events_key,priority:3" json:"idempotencyKey"`
	RuleVersion    string    `gorm:"column:rule_version;type:varchar(16);not null;default:'v1'" json:"ruleVersion"`
	OccurredAt     time.Time `gorm:"column:occurred_at;index:idx_agent_relation_events_relation_time,priority:3" json:"occurredAt"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"createdAt"`
}

func (AgentRelationEvent) TableName() string { return "agent_relation_events" }
