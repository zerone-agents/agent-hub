package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type recordingAgentMessageRunner struct {
	mu       sync.Mutex
	calls    []agentMessageRunnerCall
	reply    string
	err      error
	blocking <-chan struct{}
	onRun    func(tenantID, agentName, message string)
}

type agentMessageRunnerCall struct {
	tenantID string
	agent    string
	message  string
}

func (r *recordingAgentMessageRunner) RunOneShot(_ context.Context, tenantID, agentName, message string) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, agentMessageRunnerCall{tenantID: tenantID, agent: agentName, message: message})
	onRun := r.onRun
	r.mu.Unlock()
	if onRun != nil {
		onRun(tenantID, agentName, message)
	}
	if r.blocking != nil {
		<-r.blocking
	}
	return r.reply, r.err
}

func (r *recordingAgentMessageRunner) snapshot() []agentMessageRunnerCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agentMessageRunnerCall(nil), r.calls...)
}

type agentMessageFixture struct {
	service *AgentMessageService
	db      *gorm.DB
	runner  *recordingAgentMessageRunner
	a       agent.AgentConfig
	b       agent.AgentConfig
	c       agent.AgentConfig
}

func setupAgentMessageService(t *testing.T) agentMessageFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentMessage{}, &agentrelation.AgentMessageDedupe{}, &rundomain.Run{}, &rundomain.RunAgent{}, &eventdomain.StreamCursor{}, &eventdomain.Envelope{}, &eventdomain.Delivery{}, &eventdomain.DeliveryAttempt{}, &eventdomain.CausalBudget{}))

	a := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	b := agent.AgentConfig{Name: "agent-b", TenantID: "tenant-a"}
	c := agent.AgentConfig{Name: "agent-c", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&rundomain.Run{ID: "run-1", TenantID: "tenant-a", Name: "chain", Status: rundomain.StatusRunning}).Error)
	for _, item := range []agent.AgentConfig{a, b, c} {
		require.NoError(t, db.Create(&rundomain.RunAgent{TenantID: "tenant-a", RunID: "run-1", AgentID: item.ID, AgentNameSnapshot: item.Name}).Error)
	}

	runner := &recordingAgentMessageRunner{reply: "B 的真实回复"}
	service := newAgentMessageService(
		repository.NewAgentRelationRepositoryWithDB(db),
		repository.NewAgentMessageRepositoryWithDB(db),
		repository.NewAgentRepositoryWithDB(db),
		runner,
	)
	return agentMessageFixture{service: service, db: db, runner: runner, a: a, b: b, c: c}
}

func addMessageRelation(t *testing.T, f agentMessageFixture, source, target agent.AgentConfig, scope, delivery, contextPolicy string, actions ...string) agentrelation.AgentRelation {
	return addTypedMessageRelation(t, f, source, target, scope, "peer", "friendly", delivery, contextPolicy, actions...)
}

func addTypedMessageRelation(
	t *testing.T,
	f agentMessageFixture,
	source, target agent.AgentConfig,
	scope, relationType, stance, delivery, contextPolicy string,
	actions ...string,
) agentrelation.AgentRelation {
	t.Helper()
	relation := agentrelation.AgentRelation{
		TenantID:          source.TenantID,
		Scope:             scope,
		SourceAgentID:     source.ID,
		TargetAgentID:     target.ID,
		RelationType:      relationType,
		Stance:            stance,
		RelationshipScore: agentrelation.InitialScoreForStance(stance),
		AllowedActions:    actions,
		ContextPolicy:     contextPolicy,
		DeliveryPolicy:    delivery,
		Constraint:        "先核对事实，再给出独立判断",
		Enabled:           true,
	}
	require.NoError(t, f.db.Create(&relation).Error)
	return relation
}

// TestAgentMessageServiceRoutesEveryOrganizationRelationship is the compact
// regression matrix for the contracts exposed by the relationship editor.
// Each case proves that the structural label, stance, action whitelist,
// context policy, and delivery mode survive all the way into a real dispatch.
func TestAgentMessageServiceRoutesEveryOrganizationRelationship(t *testing.T) {
	tests := []struct {
		name          string
		relationType  string
		stance        string
		action        string
		contextPolicy string
		delivery      string
	}{
		{name: "subordinate reports to leader", relationType: "reports_to", stance: "allied", action: "report", contextPolicy: "summary_only", delivery: "async"},
		{name: "leader reviews subordinate", relationType: "oversight", stance: "friendly", action: "review", contextPolicy: "summary_only", delivery: "sync"},
		{name: "peer hands off work", relationType: "peer", stance: "friendly", action: "handoff", contextPolicy: "shared_thread", delivery: "async"},
		{name: "leader consults advisor", relationType: "advisor", stance: "friendly", action: "consult", contextPolicy: "shared_thread", delivery: "sync"},
		{name: "reviewer challenges leader", relationType: "reviewer", stance: "wary", action: "challenge", contextPolicy: "none", delivery: "async"},
		{name: "representative reports to regulator", relationType: "representative", stance: "neutral", action: "report", contextPolicy: "shared_thread", delivery: "sync"},
		{name: "opponent challenges proposal", relationType: "opponent", stance: "hostile", action: "challenge", contextPolicy: "summary_only", delivery: "async"},
		{name: "external stakeholder consults", relationType: "external", stance: "competitive", action: "consult", contextPolicy: "summary_only", delivery: "async"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setupAgentMessageService(t)
			addTypedMessageRelation(t, f, f.a, f.b, "speeding-hq", tt.relationType, tt.stance, tt.delivery, tt.contextPolicy, tt.action)

			got, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
				TargetAgent:    f.b.Name,
				Scope:          "speeding-hq",
				Action:         tt.action,
				Message:        "执行关系案例",
				ContextSummary: "关系案例摘要",
				SharedContext:  "关系案例完整会话",
			})
			require.NoError(t, err)

			if tt.delivery == "sync" {
				require.Equal(t, agentrelation.MessageStatusCompleted, got.Status)
			} else {
				require.Equal(t, agentrelation.MessageStatusQueued, got.Status)
				require.Eventually(t, func() bool {
					status, statusErr := f.service.Get("tenant-a", &f.a, got.ID)
					return statusErr == nil && status.Status == agentrelation.MessageStatusCompleted
				}, time.Second, 10*time.Millisecond)
			}

			calls := f.runner.snapshot()
			require.Len(t, calls, 1)
			require.Contains(t, calls[0].message, "结构关系："+tt.relationType)
			require.Contains(t, calls[0].message, "你对发送方的当前关系：neutral（0，尚无反向关系状态）")
			require.Contains(t, calls[0].message, "动作："+tt.action)
		})
	}
}

// A directed organization is allowed to relay work A -> B -> C across two
// independent turns. The loop guard only rejects a nested dispatch while B is
// actively handling A's delivery; it must not permanently prevent B from
// starting a later, explicit turn of its own.
func TestAgentMessageServiceSupportsSequentialThreeAgentChain(t *testing.T) {
	f := setupAgentMessageService(t)
	addTypedMessageRelation(t, f, f.a, f.b, "case-chain", "peer", "friendly", "sync", "summary_only", "handoff")
	addTypedMessageRelation(t, f, f.b, f.c, "case-chain", "reports_to", "neutral", "sync", "summary_only", "report")

	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "case-chain", Action: "handoff", Message: "A 交给 B",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusCompleted, first.Status)

	second, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "case-chain", Action: "report", Message: "B 完成后报告 C",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusCompleted, second.Status)

	calls := f.runner.snapshot()
	require.Len(t, calls, 2)
	require.Equal(t, []string{f.b.Name, f.c.Name}, []string{calls[0].agent, calls[1].agent})
}

func TestAgentMessageServiceSyncDeliveryUsesDirectedAuthorizedRoute(t *testing.T) {
	f := setupAgentMessageService(t)
	relation := addMessageRelation(t, f, f.a, f.b, "speeding-hq", "sync", "summary_only", "consult", "review")

	got, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent:    "agent-b",
		Scope:          "speeding-hq",
		Action:         "review",
		Message:        "请复核这项收购",
		ContextSummary: "标的估值为 20 亿美元",
		SharedContext:  "这段完整私密对话不得越权传递",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusCompleted, got.Status)
	require.Equal(t, "B 的真实回复", got.Reply)
	require.Equal(t, relation.ID, got.RelationID)

	calls := f.runner.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, "tenant-a", calls[0].tenantID)
	require.Equal(t, "agent-b", calls[0].agent)
	require.Contains(t, calls[0].message, "发送方是 agent-a")
	require.Contains(t, calls[0].message, "动作：review")
	require.Contains(t, calls[0].message, "先核对事实，再给出独立判断")
	require.Contains(t, calls[0].message, "标的估值为 20 亿美元")
	require.NotContains(t, calls[0].message, "这段完整私密对话不得越权传递")

	var stored agentrelation.AgentMessage
	require.NoError(t, f.db.First(&stored, "id = ?", got.ID).Error)
	require.Equal(t, "标的估值为 20 亿美元", stored.SharedContext)
	require.Equal(t, agentrelation.MessageStatusCompleted, stored.Status)
}

func TestAgentMessageServiceRejectsReverseAndUnauthorizedAction(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "speeding-hq", "sync", "none", "review")

	_, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: "agent-a", Scope: "speeding-hq", Action: "review", Message: "反向发送",
	})
	require.ErrorIs(t, err, agentrelation.ErrRouteNotFound)

	_, err = f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: "agent-b", Scope: "speeding-hq", Action: "assign", Message: "越权指派",
	})
	require.ErrorIs(t, err, agentrelation.ErrActionNotAllowed)
	require.Empty(t, f.runner.snapshot())

	var count int64
	require.NoError(t, f.db.Model(&agentrelation.AgentMessage{}).Count(&count).Error)
	require.Equal(t, int64(2), count, "rejected hops remain visible as guards")
	var guards []agentrelation.AgentMessage
	require.NoError(t, f.db.Order("created_at ASC").Find(&guards).Error)
	require.Equal(t, []string{"route_not_found", "action_not_allowed"}, []string{guards[0].GuardReason, guards[1].GuardReason})
	for _, guard := range guards {
		require.Equal(t, agentrelation.MessageStatusGuarded, guard.Status)
	}
}

func TestAgentMessageServiceNoneContextDropsAllCallerContext(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "private", "sync", "none", "inform")

	_, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: "agent-b", Scope: "private", Action: "inform", Message: "只传这句话",
		ContextSummary: "摘要秘密", SharedContext: "完整秘密",
	})
	require.NoError(t, err)
	calls := f.runner.snapshot()
	require.Len(t, calls, 1)
	require.NotContains(t, calls[0].message, "摘要秘密")
	require.NotContains(t, calls[0].message, "完整秘密")

	var stored agentrelation.AgentMessage
	require.NoError(t, f.db.First(&stored).Error)
	require.Empty(t, stored.SharedContext)
}

func TestAgentMessageServiceAsyncCanBePolledByEitherParty(t *testing.T) {
	f := setupAgentMessageService(t)
	release := make(chan struct{})
	f.runner.blocking = release
	addMessageRelation(t, f, f.a, f.b, "speeding-hq", "async", "shared_thread", "handoff")

	got, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: "agent-b", Scope: "speeding-hq", Action: "handoff", Message: "接手谈判",
		SharedContext: "谈判完整上下文",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusQueued, got.Status)
	require.NotEmpty(t, got.ID)

	require.Eventually(t, func() bool {
		status, statusErr := f.service.Get("tenant-a", &f.b, got.ID)
		return statusErr == nil && status.Status == agentrelation.MessageStatusRunning
	}, time.Second, 10*time.Millisecond)
	close(release)
	require.Eventually(t, func() bool {
		status, statusErr := f.service.Get("tenant-a", &f.a, got.ID)
		return statusErr == nil && status.Status == agentrelation.MessageStatusCompleted && status.Reply == "B 的真实回复"
	}, time.Second, 10*time.Millisecond)

	foreign := agent.AgentConfig{ID: f.a.ID, Name: f.a.Name, TenantID: "tenant-b"}
	_, err = f.service.Get("tenant-b", &foreign, got.ID)
	require.ErrorIs(t, err, agentrelation.ErrMessageNotFound)
}

func TestAgentMessageServiceQueuesConcurrentDeliveriesToSameTarget(t *testing.T) {
	f := setupAgentMessageService(t)
	release := make(chan struct{})
	f.runner.blocking = release
	addMessageRelation(t, f, f.a, f.b, "speeding-hq", "async", "summary_only", "consult")
	addMessageRelation(t, f, f.c, f.b, "speeding-hq", "async", "summary_only", "challenge")

	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "speeding-hq", Action: "consult", Message: "第一条消息",
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return len(f.runner.snapshot()) == 1
	}, time.Second, 10*time.Millisecond)

	second, err := f.service.Send(context.Background(), "tenant-a", &f.c, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "speeding-hq", Action: "challenge", Message: "第二条消息",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusQueued, second.Status)
	time.Sleep(30 * time.Millisecond)
	require.Len(t, f.runner.snapshot(), 1, "the second delivery must wait instead of failing or entering the runtime concurrently")

	queued, err := f.service.Get("tenant-a", &f.c, second.ID)
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusQueued, queued.Status)
	require.Empty(t, queued.Error)

	close(release)
	require.Eventually(t, func() bool {
		firstStatus, firstErr := f.service.Get("tenant-a", &f.a, first.ID)
		secondStatus, secondErr := f.service.Get("tenant-a", &f.c, second.ID)
		return firstErr == nil && secondErr == nil &&
			firstStatus.Status == agentrelation.MessageStatusCompleted &&
			secondStatus.Status == agentrelation.MessageStatusCompleted
	}, time.Second, 10*time.Millisecond)
	require.Len(t, f.runner.snapshot(), 2)
}

func TestAgentMessageServiceAllowsAuthorizedNestedDispatchToThirdAgent(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "speeding-hq", "sync", "summary_only", "consult")
	addMessageRelation(t, f, f.b, f.c, "speeding-hq", "sync", "summary_only", "inform")

	var nestedErr error
	f.runner.onRun = func(_, agentName, _ string) {
		if agentName != f.b.Name {
			return
		}
		_, nestedErr = f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
			TargetAgent: f.c.Name, Scope: "speeding-hq", Action: "inform", Message: "继续转发",
		})
	}

	_, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "speeding-hq", Action: "consult", Message: "先问 B",
	})
	require.NoError(t, err)
	require.NoError(t, nestedErr)
	require.Len(t, f.runner.snapshot(), 2)
}

func TestAgentMessageServiceAsyncRecipientContinuesSameChain(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "project", "async", "summary_only", "handoff")
	addMessageRelation(t, f, f.b, f.c, "project", "async", "summary_only", "report")

	nested := make(chan error, 1)
	f.runner.onRun = func(_, agentName, _ string) {
		if agentName != f.b.Name {
			return
		}
		var parent agentrelation.AgentMessage
		if err := f.db.Where("target_agent_id=?", f.b.ID).Order("created_at DESC").First(&parent).Error; err != nil {
			nested <- err
			return
		}
		_, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
			TargetAgent: f.c.Name, Scope: "project", Action: "report", Message: "B completed, C please review",
			ParentMessageID: parent.ID, IdempotencyKey: "async-hop-2",
		})
		nested <- err
	}

	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "project", Action: "handoff", Message: "A asks B",
		IdempotencyKey: "async-hop-1",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusQueued, first.Status)
	select {
	case err := <-nested:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("B did not continue the asynchronous chain")
	}
	require.Eventually(t, func() bool {
		var row agentrelation.AgentMessage
		err := f.db.Where("idempotency_key=?", "async-hop-2").First(&row).Error
		return err == nil && row.TargetAgent == f.c.Name && row.ParentMessageID == first.ID && row.Hop == 2 && row.ConversationID == first.ConversationID
	}, time.Second, 10*time.Millisecond)
}

func TestExtractOneShotReply(t *testing.T) {
	sse := "event: assistant\ndata: {\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"第一段\"}]}}\n\n" +
		"event: assistant\ndata: {\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"第二段\"}]}}\n\n" +
		"event: result\ndata: {\"subtype\":\"success\"}\n\n"
	reply, err := extractOneShotReply(sse)
	require.NoError(t, err)
	require.Equal(t, "第一段\n第二段", reply)

	_, err = extractOneShotReply("event: result\ndata: {\"subtype\":\"error\",\"errors\":[\"模型失败\"]}\n\n")
	require.EqualError(t, err, "模型失败")
}

func TestBuildAgentMessageEnvelopeUsesPlatformNeutralProtocolLabel(t *testing.T) {
	source := &agent.AgentConfig{Name: "research-lead"}
	target := &agent.AgentConfig{Name: "finance-reviewer"}
	relation := &agentrelation.AgentRelation{
		Scope:          "research-team",
		RelationType:   "peer",
		ContextPolicy:  "summary_only",
		AllowedActions: []string{"consult"},
	}

	deadline := time.Now().UTC().Add(time.Minute)
	envelope := buildAgentMessageEnvelope(source, target, relation, nil, &agentrelation.AgentMessage{ID: "msg-1", ConversationID: "conv-1", RootMessageID: "msg-1", Hop: 1, MaxHops: 8, EventCount: 1, EventBudget: 64, TokenBudget: 65536, DeadlineAt: &deadline, Action: "consult", Content: "请复核结论"})

	require.Contains(t, envelope, "[Agent Hub 组织消息]")
	require.NotContains(t, envelope, "SPEEDING")
	require.Contains(t, envelope, "接收方 finance-reviewer")
	require.Contains(t, envelope, "发送方是 research-lead")
}

func TestAgentMessageServiceDerivesAuthorizedMultiHopChain(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "chain", "sync", "summary_only", "handoff")
	addMessageRelation(t, f, f.b, f.c, "chain", "sync", "summary_only", "report")
	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "chain", Action: "handoff", Message: "A to B", RunID: "run-1", IdempotencyKey: "hop-1"})
	require.NoError(t, err)
	second, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "chain", Action: "report", Message: "B to C", ParentMessageID: first.ID, IdempotencyKey: "hop-2", Hop: 99, RootMessageID: "forged"})
	require.NoError(t, err)
	require.Equal(t, 2, second.Hop)
	require.Equal(t, first.RootMessageID, second.RootMessageID)
	require.Equal(t, first.ConversationID, second.ConversationID)
	require.Equal(t, first.ID, second.ParentMessageID)
	require.Equal(t, "run-1", second.RunID)
	require.Equal(t, []uint64{f.a.ID, f.b.ID, f.c.ID}, second.VisitedAgentIDs)
	require.Greater(t, second.TokensUsed, first.TokensUsed)

	rows, err := f.service.ListRun("tenant-a", "run-1", 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	var eventTypes []string
	require.NoError(t, f.db.Model(&eventdomain.Envelope{}).Where("tenant_id=? AND run_id=?", "tenant-a", "run-1").Order("recorded_at ASC, sequence ASC").Pluck("type", &eventTypes).Error)
	require.Equal(t, []string{
		"agenthub.message.accepted.v1", "agenthub.message.queued.v1", "agenthub.message.running.v1", "agenthub.message.completed.v1",
		"agenthub.message.accepted.v1", "agenthub.message.queued.v1", "agenthub.message.running.v1", "agenthub.message.completed.v1",
	}, eventTypes)
	var outboxCount int64
	require.NoError(t, f.db.Model(&eventdomain.Delivery{}).Count(&outboxCount).Error)
	require.Equal(t, int64(len(eventTypes)), outboxCount)
}

func TestAgentMessageServicePausedRunRejectsNewDelivery(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "paused", "sync", "summary_only", "inform")
	require.NoError(t, f.db.Model(&rundomain.Run{}).Where("id=?", "run-1").Update("status", rundomain.StatusPaused).Error)
	_, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "paused", Action: "inform", Message: "must wait", RunID: "run-1", IdempotencyKey: "paused-1"})
	require.ErrorIs(t, err, agentrelation.ErrRouteNotFound)
	var messages int64
	require.NoError(t, f.db.Model(&agentrelation.AgentMessage{}).Count(&messages).Error)
	require.Zero(t, messages)
}

func TestAgentMessageServiceAuthorizationGuardIsInUnifiedRunEvents(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "audit", "sync", "summary_only", "review")
	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "audit", Action: "assign", Message: "unauthorized", RunID: "run-1", IdempotencyKey: "guard-audit"})
	require.ErrorIs(t, err, agentrelation.ErrActionNotAllowed)
	require.Equal(t, agentrelation.MessageStatusGuarded, guarded.Status)
	var events []eventdomain.Envelope
	require.NoError(t, f.db.Where("run_id=?", "run-1").Order("recorded_at ASC, sequence ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, "agenthub.message.accepted.v1", events[0].Type)
	require.Equal(t, "agenthub.message.guarded.v1", events[1].Type)
	require.Equal(t, events[0].ID, events[1].CausationID)
	require.Equal(t, guarded.RootMessageID, events[0].RootEventID)
}

func TestAgentMessageServiceExplainsRunParticipantGuard(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.c, "audit", "async", "summary_only", "consult")
	require.NoError(t, f.db.Where("run_id=? AND agent_id=?", "run-1", f.c.ID).Delete(&rundomain.RunAgent{}).Error)

	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "audit", Action: "consult", Message: "review", RunID: "run-1", IdempotencyKey: "participant-guard",
	})
	require.ErrorIs(t, err, agentrelation.ErrRunParticipantDenied)
	require.Equal(t, "run_participant_denied", guarded.GuardReason)
	require.Equal(t, "目标 Agent 未加入本次运行", guarded.GuardDescription)
}

func TestAgentMessageServiceAllowsAsyncReturnButRejectsSyncWaitCycle(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "return", "sync", "summary_only", "consult")
	addMessageRelation(t, f, f.b, f.a, "return", "async", "summary_only", "report")
	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "return", Action: "consult", Message: "ask", IdempotencyKey: "return-1"})
	require.NoError(t, err)
	returned, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.a.Name, Scope: "return", Action: "report", Message: "answer", ParentMessageID: first.ID, IdempotencyKey: "return-2"})
	require.NoError(t, err)
	require.Equal(t, 2, returned.Hop)
	require.Equal(t, f.a.ID, returned.VisitedAgentIDs[len(returned.VisitedAgentIDs)-1])

	// Changing the current edge to sync turns the same legal graph cycle into
	// a synchronous call-stack deadlock, so it is persisted as a guard.
	require.NoError(t, f.db.Model(&agentrelation.AgentRelation{}).Where("source_agent_id=? AND target_agent_id=?", f.b.ID, f.a.ID).Update("delivery_policy", "sync").Error)
	require.NoError(t, f.db.Model(&agentrelation.AgentMessage{}).Where("id=?", first.ID).Update("status", agentrelation.MessageStatusRunning).Error)
	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.a.Name, Scope: "return", Action: "report", Message: "sync answer", ParentMessageID: first.ID, IdempotencyKey: "return-sync"})
	require.ErrorIs(t, err, agentrelation.ErrSyncDeadlock)
	require.Equal(t, agentrelation.MessageStatusGuarded, guarded.Status)
	require.Equal(t, "sync_wait_cycle", guarded.GuardReason)
}

func TestAgentMessageServiceGuardsBudgetAndDeduplicatesRepeatedHop(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "loop", "sync", "summary_only", "inform")
	addMessageRelation(t, f, f.b, f.a, "loop", "async", "summary_only", "report")
	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "loop", Action: "inform", Message: "one", IdempotencyKey: "loop-1"})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&agentrelation.AgentMessage{}).Where("id=?", first.ID).Updates(map[string]any{"max_hops": 1, "event_budget": 1}).Error)
	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.a.Name, Scope: "loop", Action: "report", Message: "two", ParentMessageID: first.ID, IdempotencyKey: "loop-2"})
	require.ErrorIs(t, err, agentrelation.ErrChainGuarded)
	require.Equal(t, "max_hops_exceeded", guarded.GuardReason)
	again, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.a.Name, Scope: "loop", Action: "report", Message: "must not duplicate", ParentMessageID: first.ID, IdempotencyKey: "loop-2"})
	require.NoError(t, err)
	require.Equal(t, guarded.ID, again.ID)
}
