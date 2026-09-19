package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func collaborationTestService(t *testing.T) (*CollaborationService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &collaboration.Group{}, &collaboration.GroupMember{}, &collaboration.Channel{}, &collaboration.ChannelSubscription{}, &collaboration.Session{}, &collaboration.SessionParticipant{}, &collaboration.MemberAudit{}))
	return NewCollaborationService(db), db
}

func TestCollaborationGroupChannelAndSessionLifecycle(t *testing.T) {
	svc, db := collaborationTestService(t)
	agentRow := agent.AgentConfig{TenantID: "tenant-a", Name: "analyst"}
	require.NoError(t, db.Create(&agentRow).Error)
	g, err := svc.CreateGroup("tenant-a", "Risk team", "", "private", "manager")
	require.NoError(t, err)
	_, err = svc.AddMember("tenant-a", g.ID, agentRow.ID, "leader", "manager")
	require.NoError(t, err)
	_, err = svc.AddMember("tenant-b", g.ID, agentRow.ID, "member", "other")
	require.ErrorIs(t, err, ErrCollaborationNotFound)

	ch, err := svc.CreateChannel("tenant-a", g.ID, "incident", "Coordinate response", "group", "manager")
	require.NoError(t, err)
	sub, err := svc.PutSubscription("tenant-a", ch.ID, agentRow.ID, "mentions")
	require.NoError(t, err)
	require.Equal(t, "mentions", sub.Mode)
	recipients, err := svc.ListChannelRecipients("tenant-a", ch.ID)
	require.NoError(t, err)
	require.Len(t, recipients, 1)

	room, err := svc.CreateSession("tenant-a", ch.ID, "Review response", agentRow.ID, "manager")
	require.NoError(t, err)
	require.Equal(t, collaboration.SessionDraft, room.Status)
	require.Len(t, room.Participants, 1)
	require.Equal(t, agentRow.ID, room.Participants[0].AgentID)
	require.Equal(t, "host", room.Participants[0].Role)
	room, err = svc.StartSession("tenant-a", room.ID)
	require.NoError(t, err)
	require.Equal(t, collaboration.SessionActive, room.Status)
	room, err = svc.CompleteSession("tenant-a", room.ID, "Proceed with mitigation")
	require.NoError(t, err)
	require.Equal(t, collaboration.SessionCompleted, room.Status)
	require.NotNil(t, room.CompletedAt)
	audits, err := svc.MemberAudits("tenant-a", g.ID)
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, audit := range audits {
		actions[audit.Action] = true
		require.NotEmpty(t, audit.ResourceType)
		require.NotEmpty(t, audit.ResourceID)
	}
	for _, action := range []string{"group_created", "member_added", "channel_created", "subscription_upserted", "session_created", "session_started", "session_completed"} {
		require.True(t, actions[action], action)
	}
}

func TestCollaborationSessionExplicitParticipantsAreSnapshotted(t *testing.T) {
	svc, db := collaborationTestService(t)
	a := agent.AgentConfig{TenantID: "tenant-a", Name: "a"}
	b := agent.AgentConfig{TenantID: "tenant-a", Name: "b"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	g, err := svc.CreateGroup("tenant-a", "Council", "", "private", "manager")
	require.NoError(t, err)
	_, err = svc.AddMember("tenant-a", g.ID, a.ID, "leader", "manager")
	require.NoError(t, err)
	_, err = svc.AddMember("tenant-a", g.ID, b.ID, "observer", "manager")
	require.NoError(t, err)
	ch, err := svc.CreateChannel("tenant-a", g.ID, "planning", "", "members", "manager")
	require.NoError(t, err)
	room, err := svc.CreateSession("tenant-a", ch.ID, "Plan", a.ID, "manager", b.ID)
	require.NoError(t, err)
	require.Len(t, room.Participants, 2, "host is included even when omitted from explicit participants")
	require.NoError(t, svc.RemoveMember("tenant-a", g.ID, b.ID, "manager"))
	reloaded, err := svc.Session("tenant-a", room.ID)
	require.NoError(t, err)
	require.Len(t, reloaded.Participants, 2, "membership changes must not rewrite the session snapshot")
}

func TestCollaborationMemberAuditAndValidation(t *testing.T) {
	svc, db := collaborationTestService(t)
	a := agent.AgentConfig{TenantID: "tenant-a", Name: "member-a"}
	require.NoError(t, db.Create(&a).Error)
	g, err := svc.CreateGroup("tenant-a", "Operations", "", "tenant", "manager")
	require.NoError(t, err)
	_, err = svc.AddMember("tenant-a", g.ID, a.ID, "owner", "manager")
	require.Error(t, err)
	_, err = svc.AddMember("tenant-a", g.ID, a.ID, "member", "manager")
	require.NoError(t, err)
	_, err = svc.UpdateMember("tenant-a", g.ID, a.ID, "observer", "manager")
	require.NoError(t, err)
	require.NoError(t, svc.RemoveMember("tenant-a", g.ID, a.ID, "manager"))
	audits, err := svc.MemberAudits("tenant-a", g.ID)
	require.NoError(t, err)
	require.Len(t, audits, 4)
	require.Equal(t, "member_removed", audits[0].Action)
}
