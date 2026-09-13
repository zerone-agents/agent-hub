package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCollaborationNotFound = errors.New("collaboration resource not found")
	ErrCollaborationConflict = errors.New("collaboration resource conflict")
)

type CollaborationService struct{ db *gorm.DB }

func NewCollaborationService(db *gorm.DB) *CollaborationService { return &CollaborationService{db: db} }

func recordCollaborationAudit(tx *gorm.DB, tenantID, groupID, actor, resourceType, resourceID, action string, agentID uint64, before, after map[string]any) error {
	return tx.Create(&collaboration.MemberAudit{ID: uuid.NewString(), TenantID: tenantID, GroupID: groupID, ActorID: actor, ResourceType: resourceType, ResourceID: resourceID, Action: action, AgentID: agentID, Before: before, After: after}).Error
}

func validGroupVisibility(v string) bool {
	return v == collaboration.GroupVisibilityPrivate || v == collaboration.GroupVisibilityTenant
}
func validChannelVisibility(v string) bool {
	return v == collaboration.ChannelVisibilityGroup || v == collaboration.ChannelVisibilityMembers
}
func validGroupRole(v string) bool {
	switch v {
	case collaboration.RoleLeader, collaboration.RoleMember, collaboration.RoleObserver, collaboration.RoleGuest:
		return true
	}
	return false
}
func validSubscriptionMode(v string) bool {
	return v == collaboration.SubscriptionAll || v == collaboration.SubscriptionMentions || v == collaboration.SubscriptionNone
}

func (s *CollaborationService) CreateGroup(tenantID, name, description, visibility, actor string) (*collaboration.Group, error) {
	name, visibility = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(visibility))
	if visibility == "" {
		visibility = collaboration.GroupVisibilityPrivate
	}
	if tenantID == "" || name == "" || !validGroupVisibility(visibility) {
		return nil, fmt.Errorf("name and valid visibility are required")
	}
	row := &collaboration.Group{ID: uuid.NewString(), TenantID: tenantID, Name: name, Description: strings.TrimSpace(description), Visibility: visibility, CreatedBy: actor}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, row.ID, actor, "group", row.ID, "group_created", 0, nil, map[string]any{"name": row.Name, "visibility": row.Visibility})
	}); err != nil {
		if isDuplicate(err) {
			return nil, ErrCollaborationConflict
		}
		return nil, err
	}
	return row, nil
}

func (s *CollaborationService) Groups(tenantID string) ([]collaboration.Group, error) {
	var rows []collaboration.Group
	return rows, s.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&rows).Error
}
func (s *CollaborationService) Group(tenantID, id string) (*collaboration.Group, error) {
	var row collaboration.Group
	if err := s.db.Where("tenant_id = ? AND id = ?", tenantID, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCollaborationNotFound
		}
		return nil, err
	}
	return &row, nil
}
func (s *CollaborationService) UpdateGroup(tenantID, id, name, description, visibility string, actors ...string) (*collaboration.Group, error) {
	row, err := s.Group(tenantID, id)
	if err != nil {
		return nil, err
	}
	before := map[string]any{"name": row.Name, "description": row.Description, "visibility": row.Visibility}
	if strings.TrimSpace(name) != "" {
		row.Name = strings.TrimSpace(name)
	}
	if visibility != "" {
		visibility = strings.ToLower(strings.TrimSpace(visibility))
		if !validGroupVisibility(visibility) {
			return nil, fmt.Errorf("invalid visibility")
		}
		row.Visibility = visibility
	}
	row.Description = strings.TrimSpace(description)
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	if err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, id, actor, "group", id, "group_updated", 0, before, map[string]any{"name": row.Name, "description": row.Description, "visibility": row.Visibility})
	}); err != nil {
		return nil, err
	}
	return row, nil
}
func (s *CollaborationService) DeleteGroup(tenantID, id string, actors ...string) error {
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var group collaboration.Group
		if err := tx.Where("tenant_id=? AND id=?", tenantID, id).First(&group).Error; err != nil {
			return err
		}
		if err := recordCollaborationAudit(tx, tenantID, id, actor, "group", id, "group_deleted", 0, map[string]any{"name": group.Name, "visibility": group.Visibility}, nil); err != nil {
			return err
		}
		var channelIDs []string
		if err := tx.Model(&collaboration.Channel{}).Where("tenant_id=? AND group_id=?", tenantID, id).Pluck("id", &channelIDs).Error; err != nil {
			return err
		}
		if len(channelIDs) > 0 {
			var sessionIDs []string
			if err := tx.Model(&collaboration.Session{}).Where("tenant_id=? AND channel_id IN ?", tenantID, channelIDs).Pluck("id", &sessionIDs).Error; err != nil {
				return err
			}
			if len(sessionIDs) > 0 {
				if err := tx.Where("tenant_id=? AND session_id IN ?", tenantID, sessionIDs).Delete(&collaboration.SessionParticipant{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("tenant_id=? AND channel_id IN ?", tenantID, channelIDs).Delete(&collaboration.ChannelSubscription{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tenant_id=? AND channel_id IN ?", tenantID, channelIDs).Delete(&collaboration.Session{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id=? AND group_id=?", tenantID, id).Delete(&collaboration.Channel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id=? AND group_id=?", tenantID, id).Delete(&collaboration.GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Where("tenant_id=? AND id=?", tenantID, id).Delete(&collaboration.Group{}).Error
	})
}

func (s *CollaborationService) Members(tenantID, groupID string) ([]collaboration.GroupMember, error) {
	if _, err := s.Group(tenantID, groupID); err != nil {
		return nil, err
	}
	var rows []collaboration.GroupMember
	return rows, s.db.Where("tenant_id=? AND group_id=?", tenantID, groupID).Order("joined_at").Find(&rows).Error
}
func (s *CollaborationService) GetGroupMember(tenantID, groupID string, agentID uint64) (*collaboration.GroupMember, error) {
	var row collaboration.GroupMember
	if err := s.db.Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, groupID, agentID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCollaborationNotFound
		}
		return nil, err
	}
	return &row, nil
}

type ChannelRecipient struct {
	Member           collaboration.GroupMember `json:"member"`
	SubscriptionMode string                    `json:"subscriptionMode"`
}

func (s *CollaborationService) ListChannelRecipients(tenantID, channelID string) ([]ChannelRecipient, error) {
	ch, err := s.Channel(tenantID, channelID)
	if err != nil {
		return nil, err
	}
	members, err := s.Members(tenantID, ch.GroupID)
	if err != nil {
		return nil, err
	}
	subs, err := s.Subscribers(tenantID, channelID)
	if err != nil {
		return nil, err
	}
	modeByAgent := make(map[uint64]string, len(subs))
	for _, sub := range subs {
		modeByAgent[sub.AgentID] = sub.Mode
	}
	rows := make([]ChannelRecipient, 0, len(members))
	for _, member := range members {
		mode := modeByAgent[member.AgentID]
		if mode == "" {
			mode = collaboration.SubscriptionAll
		}
		if mode != collaboration.SubscriptionNone {
			rows = append(rows, ChannelRecipient{Member: member, SubscriptionMode: mode})
		}
	}
	return rows, nil
}
func (s *CollaborationService) AddMember(tenantID, groupID string, agentID uint64, role, actor string) (*collaboration.GroupMember, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if !validGroupRole(role) {
		return nil, fmt.Errorf("invalid member role")
	}
	row := &collaboration.GroupMember{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var g collaboration.Group
		if err := tx.Where("tenant_id=? AND id=?", tenantID, groupID).First(&g).Error; err != nil {
			return ErrCollaborationNotFound
		}
		var a agent.AgentConfig
		if err := tx.Where("tenant_id=? AND id=?", tenantID, agentID).First(&a).Error; err != nil {
			return fmt.Errorf("agent not found")
		}
		*row = collaboration.GroupMember{TenantID: tenantID, GroupID: groupID, AgentID: agentID, Role: role, JoinedAt: time.Now().UTC()}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, groupID, actor, "member", fmt.Sprint(agentID), "member_added", agentID, nil, map[string]any{"role": role})
	})
	if err != nil {
		if isDuplicate(err) {
			return nil, ErrCollaborationConflict
		}
		return nil, err
	}
	return row, nil
}
func (s *CollaborationService) UpdateMember(tenantID, groupID string, agentID uint64, role, actor string) (*collaboration.GroupMember, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if !validGroupRole(role) {
		return nil, fmt.Errorf("invalid member role")
	}
	var row collaboration.GroupMember
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, groupID, agentID).First(&row).Error; err != nil {
			return ErrCollaborationNotFound
		}
		before := row.Role
		row.Role = role
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, groupID, actor, "member", fmt.Sprint(agentID), "role_changed", agentID, map[string]any{"role": before}, map[string]any{"role": role})
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func (s *CollaborationService) RemoveMember(tenantID, groupID string, agentID uint64, actor string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row collaboration.GroupMember
		if err := tx.Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, groupID, agentID).First(&row).Error; err != nil {
			return ErrCollaborationNotFound
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id=? AND agent_id=? AND channel_id IN (?)", tenantID, agentID, tx.Model(&collaboration.Channel{}).Select("id").Where("tenant_id=? AND group_id=?", tenantID, groupID)).Delete(&collaboration.ChannelSubscription{}).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, groupID, actor, "member", fmt.Sprint(agentID), "member_removed", agentID, map[string]any{"role": row.Role}, nil)
	})
}
func (s *CollaborationService) MemberAudits(tenantID, groupID string) ([]collaboration.MemberAudit, error) {
	var rows []collaboration.MemberAudit
	return rows, s.db.Where("tenant_id=? AND group_id=?", tenantID, groupID).Order("created_at DESC").Limit(200).Find(&rows).Error
}

func (s *CollaborationService) CreateChannel(tenantID, groupID, name, topic, visibility, actor string) (*collaboration.Channel, error) {
	name = strings.TrimSpace(name)
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	if visibility == "" {
		visibility = collaboration.ChannelVisibilityGroup
	}
	if name == "" || !validChannelVisibility(visibility) {
		return nil, fmt.Errorf("name and valid visibility are required")
	}
	if _, err := s.Group(tenantID, groupID); err != nil {
		return nil, err
	}
	row := &collaboration.Channel{ID: uuid.NewString(), TenantID: tenantID, GroupID: groupID, Name: name, Topic: strings.TrimSpace(topic), Visibility: visibility, CreatedBy: actor}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, groupID, actor, "channel", row.ID, "channel_created", 0, nil, map[string]any{"name": row.Name, "topic": row.Topic, "visibility": row.Visibility})
	}); err != nil {
		if isDuplicate(err) {
			return nil, ErrCollaborationConflict
		}
		return nil, err
	}
	return row, nil
}
func (s *CollaborationService) Channels(tenantID, groupID string) ([]collaboration.Channel, error) {
	var rows []collaboration.Channel
	return rows, s.db.Where("tenant_id=? AND group_id=?", tenantID, groupID).Order("created_at").Find(&rows).Error
}
func (s *CollaborationService) Channel(tenantID, id string) (*collaboration.Channel, error) {
	var row collaboration.Channel
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&row).Error; err != nil {
		return nil, ErrCollaborationNotFound
	}
	return &row, nil
}
func (s *CollaborationService) UpdateChannel(tenantID, id, name, topic, visibility string, actors ...string) (*collaboration.Channel, error) {
	row, err := s.Channel(tenantID, id)
	if err != nil {
		return nil, err
	}
	before := map[string]any{"name": row.Name, "topic": row.Topic, "visibility": row.Visibility}
	if strings.TrimSpace(name) != "" {
		row.Name = strings.TrimSpace(name)
	}
	if visibility != "" {
		visibility = strings.ToLower(strings.TrimSpace(visibility))
		if !validChannelVisibility(visibility) {
			return nil, fmt.Errorf("invalid visibility")
		}
		row.Visibility = visibility
	}
	row.Topic = strings.TrimSpace(topic)
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	if err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		return recordCollaborationAudit(tx, tenantID, row.GroupID, actor, "channel", row.ID, "channel_updated", 0, before, map[string]any{"name": row.Name, "topic": row.Topic, "visibility": row.Visibility})
	}); err != nil {
		return nil, err
	}
	return row, nil
}
func (s *CollaborationService) DeleteChannel(tenantID, id string, actors ...string) error {
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row collaboration.Channel
		if err := tx.Where("tenant_id=? AND id=?", tenantID, id).First(&row).Error; err != nil {
			return ErrCollaborationNotFound
		}
		if err := tx.Where("tenant_id=? AND channel_id=?", tenantID, id).Delete(&collaboration.ChannelSubscription{}).Error; err != nil {
			return err
		}
		var sessionIDs []string
		if err := tx.Model(&collaboration.Session{}).Where("tenant_id=? AND channel_id=?", tenantID, id).Pluck("id", &sessionIDs).Error; err != nil {
			return err
		}
		if len(sessionIDs) > 0 {
			if err := tx.Where("tenant_id=? AND session_id IN ?", tenantID, sessionIDs).Delete(&collaboration.SessionParticipant{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("tenant_id=? AND channel_id=?", tenantID, id).Delete(&collaboration.Session{}).Error; err != nil {
			return err
		}
		if err := recordCollaborationAudit(tx, tenantID, row.GroupID, actor, "channel", row.ID, "channel_deleted", 0, map[string]any{"name": row.Name, "topic": row.Topic, "visibility": row.Visibility}, nil); err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}
func (s *CollaborationService) Subscribers(tenantID, channelID string) ([]collaboration.ChannelSubscription, error) {
	var rows []collaboration.ChannelSubscription
	return rows, s.db.Where("tenant_id=? AND channel_id=?", tenantID, channelID).Find(&rows).Error
}
func (s *CollaborationService) PutSubscription(tenantID, channelID string, agentID uint64, mode string, actors ...string) (*collaboration.ChannelSubscription, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if !validSubscriptionMode(mode) {
		return nil, fmt.Errorf("invalid subscription mode")
	}
	ch, err := s.Channel(tenantID, channelID)
	if err != nil {
		return nil, err
	}
	var count int64
	if err = s.db.Model(&collaboration.GroupMember{}).Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, ch.GroupID, agentID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, fmt.Errorf("agent is not a group member")
	}
	row := &collaboration.ChannelSubscription{TenantID: tenantID, ChannelID: channelID, AgentID: agentID, Mode: mode}
	var old collaboration.ChannelSubscription
	before := map[string]any(nil)
	if e := s.db.Where("tenant_id=? AND channel_id=? AND agent_id=?", tenantID, channelID, agentID).First(&old).Error; e == nil {
		before = map[string]any{"mode": old.Mode}
	}
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("tenant_id=? AND channel_id=? AND agent_id=?", tenantID, channelID, agentID).Assign(collaboration.ChannelSubscription{Mode: mode}).FirstOrCreate(row).Error; e != nil {
			return e
		}
		return recordCollaborationAudit(tx, tenantID, ch.GroupID, actor, "subscription", fmt.Sprintf("%s:%d", channelID, agentID), "subscription_upserted", agentID, before, map[string]any{"mode": mode})
	})
	return row, err
}

func (s *CollaborationService) CreateSession(tenantID, channelID, agenda string, host uint64, actor string, participantAgentIDs ...uint64) (*collaboration.Session, error) {
	agenda = strings.TrimSpace(agenda)
	if agenda == "" {
		return nil, fmt.Errorf("agenda is required")
	}
	ch, err := s.Channel(tenantID, channelID)
	if err != nil {
		return nil, err
	}
	if host > 0 {
		var n int64
		if err = s.db.Model(&collaboration.GroupMember{}).Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, ch.GroupID, host).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, fmt.Errorf("host must be a group member")
		}
	}
	if len(participantAgentIDs) == 0 {
		recipients, e := s.ListChannelRecipients(tenantID, channelID)
		if e != nil {
			return nil, e
		}
		for _, recipient := range recipients {
			participantAgentIDs = append(participantAgentIDs, recipient.Member.AgentID)
		}
	}
	if host > 0 {
		participantAgentIDs = append(participantAgentIDs, host)
	}
	unique := make(map[uint64]struct{}, len(participantAgentIDs))
	for _, id := range participantAgentIDs {
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("session requires at least one participant")
	}
	var memberCount int64
	ids := make([]uint64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	if err = s.db.Model(&collaboration.GroupMember{}).Where("tenant_id=? AND group_id=? AND agent_id IN ?", tenantID, ch.GroupID, ids).Distinct("agent_id").Count(&memberCount).Error; err != nil {
		return nil, err
	}
	if int(memberCount) != len(ids) {
		return nil, fmt.Errorf("participants must be group members")
	}
	row := &collaboration.Session{ID: uuid.NewString(), TenantID: tenantID, ChannelID: channelID, Agenda: agenda, HostAgentID: host, Status: collaboration.SessionDraft, CreatedBy: actor}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(row).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		for _, id := range ids {
			role := "participant"
			if id == host {
				role = "host"
			}
			if e := tx.Create(&collaboration.SessionParticipant{TenantID: tenantID, SessionID: row.ID, AgentID: id, Role: role, JoinedAt: now}).Error; e != nil {
				return e
			}
		}
		return recordCollaborationAudit(tx, tenantID, ch.GroupID, actor, "session", row.ID, "session_created", 0, nil, map[string]any{"channelId": channelID, "agenda": agenda, "hostAgentId": host, "participantAgentIds": ids})
	})
	if err != nil {
		return nil, err
	}
	return s.Session(tenantID, row.ID)
}
func (s *CollaborationService) Sessions(tenantID, channelID string) ([]collaboration.Session, error) {
	var rows []collaboration.Session
	return rows, s.db.Where("tenant_id=? AND channel_id=?", tenantID, channelID).Preload("Participants").Order("created_at DESC").Find(&rows).Error
}
func (s *CollaborationService) Session(tenantID, id string) (*collaboration.Session, error) {
	var row collaboration.Session
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).Preload("Participants").First(&row).Error; err != nil {
		return nil, ErrCollaborationNotFound
	}
	return &row, nil
}
func (s *CollaborationService) StartSession(tenantID, id string, actors ...string) (*collaboration.Session, error) {
	return s.transitionSession(tenantID, id, collaboration.SessionDraft, collaboration.SessionActive, "", actors...)
}
func (s *CollaborationService) CompleteSession(tenantID, id, summary string, actors ...string) (*collaboration.Session, error) {
	if strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("summary is required")
	}
	return s.transitionSession(tenantID, id, collaboration.SessionActive, collaboration.SessionCompleted, summary, actors...)
}
func (s *CollaborationService) transitionSession(tenantID, id, from, to, summary string, actors ...string) (*collaboration.Session, error) {
	now := time.Now().UTC()
	updates := map[string]any{"status": to, "updated_at": now}
	if to == collaboration.SessionActive {
		updates["started_at"] = now
	}
	if to == collaboration.SessionCompleted {
		updates["completed_at"] = now
		updates["summary"] = strings.TrimSpace(summary)
	}
	actor := ""
	if len(actors) > 0 {
		actor = actors[0]
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var session collaboration.Session
		if e := tx.Where("tenant_id=? AND id=?", tenantID, id).First(&session).Error; e != nil {
			return ErrCollaborationNotFound
		}
		ch, e := s.Channel(tenantID, session.ChannelID)
		if e != nil {
			return e
		}
		r := tx.Model(&collaboration.Session{}).Where("tenant_id=? AND id=? AND status=?", tenantID, id, from).Updates(updates)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrCollaborationConflict
		}
		action := "session_started"
		if to == collaboration.SessionCompleted {
			action = "session_completed"
		}
		return recordCollaborationAudit(tx, tenantID, ch.GroupID, actor, "session", id, action, 0, map[string]any{"status": from}, map[string]any{"status": to, "summary": strings.TrimSpace(summary)})
	})
	if err != nil {
		return nil, err
	}
	return s.Session(tenantID, id)
}
