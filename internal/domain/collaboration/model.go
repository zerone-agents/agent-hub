package collaboration

import "time"

const (
	GroupVisibilityPrivate   = "private"
	GroupVisibilityTenant    = "tenant"
	RoleLeader               = "leader"
	RoleMember               = "member"
	RoleObserver             = "observer"
	RoleGuest                = "guest"
	ChannelVisibilityGroup   = "group"
	ChannelVisibilityMembers = "members"
	SubscriptionAll          = "all"
	SubscriptionMentions     = "mentions"
	SubscriptionNone         = "none"
	SessionDraft             = "draft"
	SessionActive            = "active"
	SessionCompleted         = "completed"
)

type Group struct {
	ID          string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID    string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_collaboration_groups_name,priority:1;index" json:"-"`
	Name        string    `gorm:"type:varchar(160);not null;uniqueIndex:uk_collaboration_groups_name,priority:2" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Visibility  string    `gorm:"type:varchar(16);not null" json:"visibility"`
	CreatedBy   string    `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Group) TableName() string { return "collaboration_groups" }

type GroupMember struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID  string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_collaboration_group_members,priority:1;index" json:"-"`
	GroupID   string    `gorm:"type:char(36);not null;uniqueIndex:uk_collaboration_group_members,priority:2;index" json:"groupId"`
	AgentID   uint64    `gorm:"not null;uniqueIndex:uk_collaboration_group_members,priority:3;index" json:"agentId"`
	Role      string    `gorm:"type:varchar(16);not null" json:"role"`
	JoinedAt  time.Time `json:"joinedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (GroupMember) TableName() string { return "collaboration_group_members" }

type Channel struct {
	ID         string    `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID   string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_collaboration_channels_name,priority:1;index" json:"-"`
	GroupID    string    `gorm:"type:char(36);not null;uniqueIndex:uk_collaboration_channels_name,priority:2;index" json:"groupId"`
	Name       string    `gorm:"type:varchar(160);not null;uniqueIndex:uk_collaboration_channels_name,priority:3" json:"name"`
	Topic      string    `gorm:"type:text" json:"topic"`
	Visibility string    `gorm:"type:varchar(16);not null" json:"visibility"`
	CreatedBy  string    `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (Channel) TableName() string { return "collaboration_channels" }

type ChannelSubscription struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID  string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_channel_subscriptions,priority:1;index" json:"-"`
	ChannelID string    `gorm:"type:char(36);not null;uniqueIndex:uk_channel_subscriptions,priority:2;index" json:"channelId"`
	AgentID   uint64    `gorm:"not null;uniqueIndex:uk_channel_subscriptions,priority:3;index" json:"agentId"`
	Mode      string    `gorm:"type:varchar(16);not null" json:"mode"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (ChannelSubscription) TableName() string { return "collaboration_channel_subscriptions" }

type Session struct {
	ID           string               `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID     string               `gorm:"type:varchar(64);not null;index" json:"-"`
	ChannelID    string               `gorm:"type:char(36);not null;index" json:"channelId"`
	Agenda       string               `gorm:"type:text;not null" json:"agenda"`
	HostAgentID  uint64               `gorm:"not null;default:0" json:"hostAgentId,omitempty"`
	Status       string               `gorm:"type:varchar(16);not null;index" json:"status"`
	Summary      string               `gorm:"type:longtext" json:"summary"`
	CreatedBy    string               `gorm:"type:varchar(128);not null;default:''" json:"createdBy"`
	StartedAt    *time.Time           `json:"startedAt,omitempty"`
	CompletedAt  *time.Time           `json:"completedAt,omitempty"`
	CreatedAt    time.Time            `json:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	Participants []SessionParticipant `gorm:"foreignKey:SessionID" json:"participants"`
}

func (Session) TableName() string { return "collaboration_sessions" }

// SessionParticipant is the immutable participant snapshot captured when a
// lightweight session is created. Later group membership or subscription
// changes must not rewrite who took part in the historical conversation.
type SessionParticipant struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	TenantID  string    `gorm:"type:varchar(64);not null;uniqueIndex:uk_session_participants,priority:1;index" json:"-"`
	SessionID string    `gorm:"type:char(36);not null;uniqueIndex:uk_session_participants,priority:2;index" json:"sessionId"`
	AgentID   uint64    `gorm:"not null;uniqueIndex:uk_session_participants,priority:3;index" json:"agentId"`
	Role      string    `gorm:"type:varchar(16);not null;default:'participant'" json:"role"`
	JoinedAt  time.Time `json:"joinedAt"`
}

func (SessionParticipant) TableName() string { return "collaboration_session_participants" }

type MemberAudit struct {
	ID           string         `gorm:"type:char(36);primaryKey" json:"id"`
	TenantID     string         `gorm:"type:varchar(64);not null;index" json:"-"`
	GroupID      string         `gorm:"type:char(36);not null;index:idx_group_member_audits_timeline,priority:1" json:"groupId"`
	ActorID      string         `gorm:"type:varchar(128);not null;default:''" json:"actorId"`
	Action       string         `gorm:"type:varchar(32);not null" json:"action"`
	ResourceType string         `gorm:"type:varchar(32);not null;default:'member';index" json:"resourceType"`
	ResourceID   string         `gorm:"type:varchar(128);not null;default:'';index" json:"resourceId"`
	AgentID      uint64         `gorm:"not null;index" json:"agentId"`
	Before       map[string]any `gorm:"type:json;serializer:json" json:"before,omitempty"`
	After        map[string]any `gorm:"type:json;serializer:json" json:"after,omitempty"`
	CreatedAt    time.Time      `gorm:"index:idx_group_member_audits_timeline,priority:2" json:"createdAt"`
}

func (MemberAudit) TableName() string { return "collaboration_member_audits" }
