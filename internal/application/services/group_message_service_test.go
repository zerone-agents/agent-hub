package services

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	eventdomain "control-panel/internal/domain/event"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGroupMessageService(t *testing.T) (*AgentMessageService, *gorm.DB, *recordingAgentMessageRunner, []agent.AgentConfig, collaboration.Group, collaboration.Channel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{}, &agentrelation.AgentMessage{}, &agentrelation.AgentMessageDedupe{}, &agentrelation.AgentMessageDispatch{}, &agentrelation.AgentMessageDispatchCursor{}, &collaboration.Group{}, &collaboration.GroupMember{}, &collaboration.Channel{}, &collaboration.ChannelSubscription{}, &collaboration.Session{}, &collaboration.SessionParticipant{}, &collaboration.MemberAudit{}, &eventdomain.StreamCursor{}, &eventdomain.Envelope{}, &eventdomain.Delivery{}, &eventdomain.DeliveryAttempt{}, &eventdomain.CausalBudget{}))
	agents := []agent.AgentConfig{{Name: "leader", TenantID: "tenant-a"}, {Name: "member", TenantID: "tenant-a"}, {Name: "observer", TenantID: "tenant-a"}}
	for i := range agents {
		require.NoError(t, db.Create(&agents[i]).Error)
	}
	group := collaboration.Group{ID: "group-1", TenantID: "tenant-a", Name: "research", Visibility: collaboration.GroupVisibilityPrivate}
	channel := collaboration.Channel{ID: "channel-1", TenantID: "tenant-a", GroupID: group.ID, Name: "general", Visibility: collaboration.ChannelVisibilityGroup}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&channel).Error)
	for i, role := range []string{collaboration.RoleLeader, collaboration.RoleMember, collaboration.RoleObserver} {
		require.NoError(t, db.Create(&collaboration.GroupMember{TenantID: "tenant-a", GroupID: group.ID, AgentID: agents[i].ID, Role: role, JoinedAt: time.Now().UTC()}).Error)
	}
	runner := &recordingAgentMessageRunner{reply: "done"}
	service := newAgentMessageService(repository.NewAgentRelationRepositoryWithDB(db), repository.NewAgentMessageRepositoryWithDB(db), repository.NewAgentRepositoryWithDB(db), runner)
	return service, db, runner, agents, group, channel
}

func TestGroupSendFansOutAsIndependentAsyncDeliveries(t *testing.T) {
	service, db, runner, agents, group, _ := setupGroupMessageService(t)
	dispatch, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{GroupID: group.ID, Message: "review", Action: "consult", Aggregation: agentrelation.AggregationAllReplies, IdempotencyKey: "fanout-1"})
	require.NoError(t, err)
	require.Equal(t, 2, dispatch.RecipientCount)
	require.Contains(t, []string{agentrelation.DispatchStatusQueued, agentrelation.DispatchStatusRunning, agentrelation.DispatchStatusCompleted}, dispatch.Status)
	require.Eventually(t, func() bool { return len(runner.snapshot()) == 2 }, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		d, e := service.GroupMessageStatus("tenant-a", &agents[0], dispatch.ID)
		return e == nil && d.Status == agentrelation.DispatchStatusCompleted && d.CompletedCount == 2
	}, time.Second, 10*time.Millisecond)
	var rows []agentrelation.AgentMessage
	require.NoError(t, db.Where("dispatch_id=?", dispatch.ID).Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "async", row.DeliveryPolicy)
		require.Equal(t, dispatch.ID, row.ConversationID)
	}
	// Idempotent retry returns the same dispatch and never fans out again.
	again, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{GroupID: group.ID, Message: "review", Action: "consult", IdempotencyKey: "fanout-1"})
	require.NoError(t, err)
	require.Equal(t, dispatch.ID, again.ID)
	require.Len(t, runner.snapshot(), 2)
}

func TestChannelPublishHonorsSubscriptionAndAudience(t *testing.T) {
	service, _, runner, agents, group, channel := setupGroupMessageService(t)
	_, err := service.collaboration.PutSubscription("tenant-a", channel.ID, agents[1].ID, collaboration.SubscriptionNone)
	require.NoError(t, err)
	_, err = service.GroupSend("tenant-a", &agents[0], GroupMessageInput{GroupID: group.ID, ChannelID: channel.ID, Message: "leaders only", Audience: "leaders"})
	require.ErrorContains(t, err, "没有符合") // sender is excluded; there is no other leader.
	dispatch, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{ChannelID: channel.ID, Message: "observer update", Audience: "role", AudienceRole: collaboration.RoleObserver})
	require.NoError(t, err)
	require.Equal(t, 1, dispatch.RecipientCount)
	require.Eventually(t, func() bool { calls := runner.snapshot(); return len(calls) == 1 && calls[0].agent == "observer" }, time.Second, 10*time.Millisecond)
}

func TestSessionLifecycleRequiresHostOrLeader(t *testing.T) {
	service, _, _, agents, _, channel := setupGroupMessageService(t)
	room, err := service.collaboration.CreateSession("tenant-a", channel.ID, "review", agents[1].ID, "test")
	require.NoError(t, err)
	_, err = service.StartSession("tenant-a", &agents[2], room.ID)
	require.ErrorContains(t, err, "主持人")
	started, err := service.StartSession("tenant-a", &agents[1], room.ID)
	require.NoError(t, err)
	require.Equal(t, collaboration.SessionActive, started.Status)
	ended, err := service.EndSession("tenant-a", &agents[0], room.ID, "accepted")
	require.NoError(t, err)
	require.Equal(t, "accepted", ended.Summary)
}

func TestGroupSendRoundRobinPersistsAndRotates(t *testing.T) {
	service, _, _, agents, group, _ := setupGroupMessageService(t)
	for i, expected := range []string{"member", "observer", "member", "observer"} {
		dispatch, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{GroupID: group.ID, Message: "next", Audience: "round_robin", IdempotencyKey: fmt.Sprintf("rr-%d", i)})
		require.NoError(t, err)
		require.Equal(t, 1, dispatch.RecipientCount)
		require.Len(t, dispatch.Deliveries, 1)
		require.Equal(t, expected, dispatch.Deliveries[0].TargetAgent)
	}
	restarted := newAgentMessageService(repository.NewAgentRelationRepositoryWithDB(service.messageRepo.DB()), repository.NewAgentMessageRepositoryWithDB(service.messageRepo.DB()), repository.NewAgentRepositoryWithDB(service.messageRepo.DB()), service.runner)
	dispatch, err := restarted.GroupSend("tenant-a", &agents[0], GroupMessageInput{GroupID: group.ID, Message: "after restart", Audience: "round_robin", IdempotencyKey: "rr-restart"})
	require.NoError(t, err)
	require.Equal(t, "member", dispatch.Deliveries[0].TargetAgent)
}

func TestChannelRoundRobinFiltersSubscriptionsBeforeSelection(t *testing.T) {
	service, _, _, agents, _, channel := setupGroupMessageService(t)
	_, err := service.collaboration.PutSubscription("tenant-a", channel.ID, agents[1].ID, collaboration.SubscriptionNone)
	require.NoError(t, err)
	first, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{ChannelID: channel.ID, Message: "one", Audience: "round_robin", IdempotencyKey: "channel-rr-1"})
	require.NoError(t, err)
	require.Equal(t, "observer", first.Deliveries[0].TargetAgent)
	_, err = service.collaboration.PutSubscription("tenant-a", channel.ID, agents[1].ID, collaboration.SubscriptionAll)
	require.NoError(t, err)
	second, err := service.GroupSend("tenant-a", &agents[0], GroupMessageInput{ChannelID: channel.ID, Message: "two", Audience: "round_robin", IdempotencyKey: "channel-rr-2"})
	require.NoError(t, err)
	require.Equal(t, "member", second.Deliveries[0].TargetAgent)
}

func TestRoundRobinSelectionIsConcurrentSafe(t *testing.T) {
	service, _, _, agents, group, _ := setupGroupMessageService(t)
	candidates := []agent.AgentConfig{agents[1], agents[2]}
	results := make(chan string, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selected, err := service.selectRoundRobinRecipient("tenant-a", GroupMessageInput{GroupID: group.ID}, candidates)
			require.NoError(t, err)
			results <- selected.Name
		}()
	}
	wg.Wait()
	close(results)
	counts := map[string]int{}
	for name := range results {
		counts[name]++
	}
	require.Equal(t, 10, counts["member"])
	require.Equal(t, 10, counts["observer"])
}
