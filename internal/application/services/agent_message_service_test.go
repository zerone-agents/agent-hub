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
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{}, &agentrelation.AgentMessage{}, &agentrelation.AgentMessageDedupe{}, &agentrelation.AgentMessageReturnClaim{}, &agentrelation.AgentMessageWait{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.RunRoutePlan{}, &rundomain.RunRouteStep{}, &eventdomain.StreamCursor{}, &eventdomain.Envelope{}, &eventdomain.Delivery{}, &eventdomain.DeliveryAttempt{}, &eventdomain.CausalBudget{}))

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

func TestAgentMessageChainSeparatesAdminAuditFromParticipantVisibility(t *testing.T) {
	f := setupAgentMessageService(t)
	now := time.Now().UTC()
	completed := now.Add(time.Second)
	rows := []*agentrelation.AgentMessage{
		{ID: "m1", TenantID: "tenant-a", RelationID: 1, ConversationID: "conv-1", RootMessageID: "m1", Hop: 1, SourceAgentID: f.a.ID, SourceAgent: f.a.Name, TargetAgentID: f.b.ID, TargetAgent: f.b.Name, Action: "assign", DeliveryPolicy: "async", ContextPolicy: "none", Content: "A asks B", Reply: "B replies", Status: agentrelation.MessageStatusCompleted, CreatedAt: now, CompletedAt: &completed},
		{ID: "m2", TenantID: "tenant-a", RelationID: 2, ConversationID: "conv-1", RootMessageID: "m1", ParentMessageID: "m1", Hop: 2, SourceAgentID: f.b.ID, SourceAgent: f.b.Name, TargetAgentID: f.c.ID, TargetAgent: f.c.Name, Action: "consult", DeliveryPolicy: "async", ContextPolicy: "none", Content: "B asks C", Reply: "C replies", Status: agentrelation.MessageStatusCompleted, CreatedAt: now.Add(time.Second), CompletedAt: &completed},
		{ID: "other-tenant", TenantID: "tenant-b", RelationID: 3, ConversationID: "conv-1", RootMessageID: "m1", Hop: 3, SourceAgentID: f.a.ID, SourceAgent: f.a.Name, TargetAgentID: f.c.ID, TargetAgent: f.c.Name, Action: "inform", DeliveryPolicy: "async", ContextPolicy: "none", Content: "secret", Status: agentrelation.MessageStatusQueued, CreatedAt: now.Add(2 * time.Second)},
	}
	for _, row := range rows {
		require.NoError(t, f.db.Create(row).Error)
	}

	admin, err := f.service.MessageChainAdmin("tenant-a", "conv-1", "", 100)
	require.NoError(t, err)
	require.Equal(t, 2, admin.MessageCount)
	require.True(t, admin.Verified)
	require.Equal(t, []string{"m1", "m2"}, []string{admin.Messages[0].ID, admin.Messages[1].ID})

	aView, err := f.service.MessageChainForAgent("tenant-a", &f.a, "conv-1", "", 100)
	require.NoError(t, err)
	require.Equal(t, 1, aView.MessageCount)
	require.Equal(t, "m1", aView.Messages[0].ID, "A must not read the B-to-C private hop")

	outsider := agent.AgentConfig{ID: 999, Name: "outsider", TenantID: "tenant-a"}
	_, err = f.service.MessageChainForAgent("tenant-a", &outsider, "conv-1", "", 100)
	require.ErrorIs(t, err, agentrelation.ErrMessageNotFound)

	_, err = f.service.MessageChainForAgent("tenant-b", &f.a, "conv-1", "", 100)
	require.ErrorIs(t, err, agentrelation.ErrAgentNotFound)
}

func TestAgentMessageChainRequiresExactlyOneSelector(t *testing.T) {
	f := setupAgentMessageService(t)
	_, err := f.service.MessageChainAdmin("tenant-a", "", "", 100)
	require.EqualError(t, err, "必须且只能提供 conversation_id 或 root_message_id")
	_, err = f.service.MessageChainAdmin("tenant-a", "conv", "root", 100)
	require.EqualError(t, err, "必须且只能提供 conversation_id 或 root_message_id")
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

func TestAgentMessageCallbackWaitIsDurableBeforeTargetExecutionStarts(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.b, f.c, "callback-order", "async", "none", "consult")
	deadline := time.Now().UTC().Add(10 * time.Minute)
	parent := &agentrelation.AgentMessage{
		ID: "parent-b", TenantID: "tenant-a", RunID: "run-1",
		ConversationID: "conversation-1", RootMessageID: "root-1",
		Hop: 1, MaxHops: 8, EventBudget: 64, EventCount: 1,
		TokenBudget: 65536, VisitedAgentIDs: []uint64{f.a.ID, f.b.ID},
		DeadlineAt: &deadline, IdempotencyKey: "parent-b-key",
		SourceAgentID: f.a.ID, SourceAgent: f.a.Name,
		TargetAgentID: f.b.ID, TargetAgent: f.b.Name,
		Scope: "callback-order", Action: "assign", DeliveryPolicy: "async",
		ContextPolicy: "none", Content: "A asks B", Status: agentrelation.MessageStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, f.db.Create(parent).Error)
	waitRepo := repository.NewAgentMessageWaitRepositoryWithDB(f.db)
	f.service.SetMessageWaitService(NewAgentMessageWaitServiceWithRepository(waitRepo))

	started := make(chan error, 1)
	f.runner.onRun = func(_, _, _ string) {
		var count int64
		err := f.db.Model(&agentrelation.AgentMessageWait{}).
			Where("tenant_id = ? AND parent_message_id = ? AND status = ?", "tenant-a", parent.ID, agentrelation.MessageWaitStatusWaiting).
			Count(&count).Error
		if err == nil && count != 1 {
			err = fmt.Errorf("callback wait count at execution start = %d, want 1", count)
		}
		started <- err
	}

	child, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "callback-order", Action: "consult",
		Message: "B asks C", ParentMessageID: parent.ID,
		IdempotencyKey: "child-c-key", CallbackRuntimeSessionID: "runtime-session-b",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusQueued, child.Status)
	select {
	case startErr := <-started:
		require.NoError(t, startErr)
	case <-time.After(2 * time.Second):
		t.Fatal("target execution did not start")
	}
	var wait agentrelation.AgentMessageWait
	require.NoError(t, f.db.Where("tenant_id = ? AND waiting_message_id = ?", "tenant-a", child.ID).First(&wait).Error)
	require.Equal(t, "runtime-session-b", wait.RuntimeSessionID)
	require.Equal(t, parent.ID, wait.ParentMessageID)
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
			require.Contains(t, calls[0].message, "连接类型："+tt.relationType)
			require.NotContains(t, calls[0].message, "人物关系上下文")
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

func TestAgentMessageServiceStrictRunRouteRejectsShortcutButAllowsPlannedRelay(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "task", "sync", "summary_only", "consult")
	addMessageRelation(t, f, f.b, f.c, "task", "sync", "summary_only", "consult")
	addMessageRelation(t, f, f.a, f.c, "task", "sync", "summary_only", "consult")
	plan := rundomain.RunRoutePlan{TenantID: "tenant-a", RunID: "run-1", Mode: rundomain.RouteModeStrict}
	require.NoError(t, f.db.Create(&plan).Error)
	require.NoError(t, f.db.Create([]rundomain.RunRouteStep{
		{TenantID: "tenant-a", PlanID: plan.ID, Sequence: 1, SourceAgentID: f.a.ID, TargetAgentID: f.b.ID, Action: "consult"},
		{TenantID: "tenant-a", PlanID: plan.ID, Sequence: 2, SourceAgentID: f.b.ID, TargetAgentID: f.c.ID, Action: "consult"},
	}).Error)

	// An edge appearing later in the plan is not a valid entry point. Without
	// A's parent message this is hop 1, while B -> C is explicitly step 2.
	early, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "task", Action: "consult", Message: "B starts at step two", RunID: "run-1"})
	require.ErrorIs(t, err, agentrelation.ErrRunRouteDenied)
	require.Equal(t, "run_route_denied", early.GuardReason)
	require.Equal(t, 1, early.Hop)

	first, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.b.Name, Scope: "task", Action: "consult", Message: "A asks B", RunID: "run-1"})
	require.NoError(t, err)
	require.True(t, first.RoutePlanned)
	second, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "task", Action: "consult", Message: "B asks C", ParentMessageID: first.ID})
	require.NoError(t, err)
	require.True(t, second.RoutePlanned)
	require.Equal(t, 2, second.Hop)

	shortcut, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "task", Action: "consult", Message: "A bypasses B", RunID: "run-1"})
	require.ErrorIs(t, err, agentrelation.ErrRunRouteDenied)
	require.Equal(t, agentrelation.MessageStatusGuarded, shortcut.Status)
	require.Equal(t, "run_route_denied", shortcut.GuardReason)
	require.Equal(t, rundomain.RouteModeStrict, shortcut.RouteMode)
	require.False(t, shortcut.RoutePlanned)
}

func TestAgentMessageServiceAdaptiveRunRouteRequiresAndAuditsDeviationReason(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.c, "task", "sync", "summary_only", "consult")
	plan := rundomain.RunRoutePlan{TenantID: "tenant-a", RunID: "run-1", Mode: rundomain.RouteModeAdaptive}
	require.NoError(t, f.db.Create(&plan).Error)
	require.NoError(t, f.db.Create(&rundomain.RunRouteStep{TenantID: "tenant-a", PlanID: plan.ID, Sequence: 1, SourceAgentID: f.a.ID, TargetAgentID: f.b.ID, Action: "consult"}).Error)

	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "task", Action: "consult", Message: "shortcut", RunID: "run-1"})
	require.ErrorIs(t, err, agentrelation.ErrRouteReasonRequired)
	require.Equal(t, "route_deviation_reason_required", guarded.GuardReason)

	allowed, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{TargetAgent: f.c.Name, Scope: "task", Action: "consult", Message: "urgent shortcut", RunID: "run-1", RouteDeviationReason: "B unavailable; deadline at risk"})
	require.NoError(t, err)
	require.Equal(t, rundomain.RouteModeAdaptive, allowed.RouteMode)
	require.False(t, allowed.RoutePlanned)
	require.Equal(t, "B unavailable; deadline at risk", allowed.RouteDeviation)
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

func TestAgentMessageServiceGuardReceiptDoesNotExposePayloadToEitherParticipant(t *testing.T) {
	f := setupAgentMessageService(t)
	secret := "董事会尚未公开的收购底价"

	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "missing", Action: "consult", Message: secret, IdempotencyKey: "guard-secret",
	})
	require.ErrorIs(t, err, agentrelation.ErrRouteNotFound)
	require.Equal(t, agentrelation.MessageStatusGuarded, guarded.Status)
	require.Empty(t, guarded.Content)
	require.Empty(t, guarded.IdempotencyKey)

	senderInbox, err := f.service.Inbox("tenant-a", &f.a, 20)
	require.NoError(t, err)
	require.Len(t, senderInbox, 1)
	require.Equal(t, "route_not_found", senderInbox[0].GuardReason)
	require.Empty(t, senderInbox[0].Content)

	targetInbox, err := f.service.Inbox("tenant-a", &f.b, 20)
	require.NoError(t, err)
	require.Empty(t, targetInbox, "a rejected delivery must never enter the target inbox")
	_, err = f.service.Get("tenant-a", &f.b, guarded.ID)
	require.ErrorIs(t, err, agentrelation.ErrMessageNotFound)

	var audit agentrelation.AgentMessage
	require.NoError(t, f.db.Where("id = ?", guarded.ID).First(&audit).Error)
	require.Equal(t, secret, audit.Content, "administrative audit retains the attempted payload")
}

func TestAgentMessageServiceTerminalChildReconcilesRunningParent(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.b, f.c, "chain", "sync", "summary_only", "consult")
	now := time.Now().UTC()
	parent := agentrelation.AgentMessage{
		ID: "parent-running", TenantID: "tenant-a", RelationID: 1, RunID: "run-1",
		ConversationID: "conv-reconcile", RootMessageID: "parent-running", Hop: 1, MaxHops: 8,
		EventBudget: 64, EventCount: 1, TokenBudget: 65536, VisitedAgentIDs: []uint64{f.a.ID, f.b.ID},
		IdempotencyKey: "parent-running", Scope: "chain", SourceAgentID: f.a.ID, SourceAgent: f.a.Name,
		TargetAgentID: f.b.ID, TargetAgent: f.b.Name, Action: "handoff", DeliveryPolicy: "async",
		ContextPolicy: "summary_only", Content: "A asks B", Status: agentrelation.MessageStatusRunning, CreatedAt: now, StartedAt: &now,
	}
	require.NoError(t, f.db.Create(&parent).Error)

	child, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "chain", Action: "consult", Message: "B asks C",
		ParentMessageID: parent.ID, IdempotencyKey: "child-success",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusCompleted, child.Status)

	var stored agentrelation.AgentMessage
	require.NoError(t, f.db.Where("id = ?", parent.ID).First(&stored).Error)
	require.Equal(t, agentrelation.MessageStatusCompleted, stored.Status)
	require.NotNil(t, stored.CompletedAt)
	require.Equal(t, "B 的真实回复", stored.Reply)

	// A stale target execution result may add detail, but cannot reopen or
	// downgrade the terminal state established by child reconciliation.
	stale := parent
	stale.Status = agentrelation.MessageStatusRunning
	require.NoError(t, f.service.saveStatusWithEvent("tenant-a", &stale))
	require.Equal(t, agentrelation.MessageStatusCompleted, stale.Status)
}

func TestAgentMessageServiceGuardedChildClosesParentWithoutLeakingPayload(t *testing.T) {
	f := setupAgentMessageService(t)
	now := time.Now().UTC()
	parent := agentrelation.AgentMessage{
		ID: "parent-guard", TenantID: "tenant-a", RelationID: 1, RunID: "run-1",
		ConversationID: "conv-guard", RootMessageID: "parent-guard", Hop: 1, MaxHops: 8,
		EventBudget: 64, EventCount: 1, TokenBudget: 65536, VisitedAgentIDs: []uint64{f.a.ID, f.b.ID},
		IdempotencyKey: "parent-guard", Scope: "chain", SourceAgentID: f.a.ID, SourceAgent: f.a.Name,
		TargetAgentID: f.b.ID, TargetAgent: f.b.Name, Action: "handoff", DeliveryPolicy: "async",
		ContextPolicy: "summary_only", Content: "A asks B", Status: agentrelation.MessageStatusRunning, CreatedAt: now, StartedAt: &now,
	}
	require.NoError(t, f.db.Create(&parent).Error)
	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "missing", Action: "consult", Message: "sensitive rejected child",
		ParentMessageID: parent.ID, IdempotencyKey: "child-guard",
	})
	require.ErrorIs(t, err, agentrelation.ErrRouteNotFound)
	require.Empty(t, guarded.Content)

	var stored agentrelation.AgentMessage
	require.NoError(t, f.db.Where("id = ?", parent.ID).First(&stored).Error)
	require.Equal(t, agentrelation.MessageStatusGuarded, stored.Status)
	require.Equal(t, "route_not_found", stored.GuardReason)
}

func TestAgentMessageServiceFailedChildClosesParentAsFailed(t *testing.T) {
	f := setupAgentMessageService(t)
	now := time.Now().UTC()
	parent := agentrelation.AgentMessage{
		ID: "parent-failed", TenantID: "tenant-a", ConversationID: "conv-failed", RootMessageID: "parent-failed",
		Hop: 1, MaxHops: 8, EventBudget: 64, EventCount: 1, TokenBudget: 65536,
		SourceAgentID: f.a.ID, SourceAgent: f.a.Name, TargetAgentID: f.b.ID, TargetAgent: f.b.Name,
		Scope: "chain", Action: "handoff", DeliveryPolicy: "async", ContextPolicy: "none", Content: "work",
		Status: agentrelation.MessageStatusRunning, CreatedAt: now, StartedAt: &now,
	}
	child := agentrelation.AgentMessage{
		ID: "child-failed", TenantID: "tenant-a", ConversationID: parent.ConversationID, RootMessageID: parent.ID,
		ParentMessageID: parent.ID, Hop: 2, MaxHops: 8, EventBudget: 64, EventCount: 2, TokenBudget: 65536,
		SourceAgentID: f.b.ID, SourceAgent: f.b.Name, TargetAgentID: f.c.ID, TargetAgent: f.c.Name,
		Scope: "chain", Action: "consult", DeliveryPolicy: "async", ContextPolicy: "none", Content: "check",
		Status: agentrelation.MessageStatusRunning, CreatedAt: now, StartedAt: &now,
	}
	require.NoError(t, f.db.Create(&parent).Error)
	require.NoError(t, f.db.Create(&child).Error)
	child.Status = agentrelation.MessageStatusFailed
	child.Error = "runtime unavailable"
	child.CompletedAt = &now
	require.NoError(t, f.service.saveStatusWithEvent("tenant-a", &child))

	var stored agentrelation.AgentMessage
	require.NoError(t, f.db.Where("id = ?", parent.ID).First(&stored).Error)
	require.Equal(t, agentrelation.MessageStatusFailed, stored.Status)
	require.Equal(t, "下游 Agent 未能完成协作", stored.Error)
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

func TestAgentMessageServiceFastSyncReturnsCompleted(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "sync-fast", "sync", "none", "consult")
	result, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "sync-fast", Action: "consult", Message: "quick", IdempotencyKey: "sync-fast-1",
	})
	require.NoError(t, err)
	require.Equal(t, agentrelation.MessageStatusCompleted, result.Status)
	require.Equal(t, "B 的真实回复", result.Reply)
}

func TestAgentMessageServiceSlowSyncReturnsRunningAndFinishesAfterRequestCancellation(t *testing.T) {
	f := setupAgentMessageService(t)
	release := make(chan struct{})
	f.runner.blocking = release
	f.service.syncWaitTimeout = 10 * time.Millisecond
	f.service.executionTimeout = time.Second
	addMessageRelation(t, f, f.a, f.b, "sync-slow", "sync", "none", "consult")
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	result, err := f.service.Send(requestCtx, "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "sync-slow", Action: "consult", Message: "slow", IdempotencyKey: "sync-slow-1",
	})
	require.NoError(t, err)
	require.Contains(t, []string{agentrelation.MessageStatusQueued, agentrelation.MessageStatusRunning}, result.Status)
	require.Len(t, f.runner.snapshot(), 1)

	// A retry reuses the durable row and must never start a second execution.
	retry, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "sync-slow", Action: "consult", Message: "slow", IdempotencyKey: "sync-slow-1",
	})
	require.NoError(t, err)
	require.Equal(t, result.ID, retry.ID)
	require.Len(t, f.runner.snapshot(), 1)

	close(release)
	require.Eventually(t, func() bool {
		current, getErr := f.service.Get("tenant-a", &f.a, result.ID)
		return getErr == nil && current.Status == agentrelation.MessageStatusCompleted && current.Reply == "B 的真实回复"
	}, time.Second, 10*time.Millisecond)
	require.Len(t, f.runner.snapshot(), 1)
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
	message := &agentrelation.AgentMessage{ID: "msg-1", ConversationID: "conv-1", RootMessageID: "msg-1", Hop: 1, MaxHops: 8, EventCount: 1, EventBudget: 64, TokenBudget: 65536, DeadlineAt: &deadline, Action: "consult", Content: "请复核结论"}
	envelope := buildAgentMessageEnvelope(source, target, relation, message, false)

	require.Contains(t, envelope, "[Agent Hub 组织消息]")
	require.NotContains(t, envelope, "SPEEDING")
	require.Contains(t, envelope, "接收方 finance-reviewer")
	require.Contains(t, envelope, "发送方是 research-lead")
	require.NotContains(t, envelope, "neutral（0")
	require.NotContains(t, envelope, "人物关系上下文")
	require.NotContains(t, envelope, "不要再向发送方额外发送 report/inform")

	callbackEnvelope := buildAgentMessageEnvelope(source, target, relation, message, true)
	require.Contains(t, callbackEnvelope, "不要再向发送方额外发送 report/inform")
	require.Contains(t, callbackEnvelope, "按等待凭证自动恢复上游会话")
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
	require.Eventually(t, func() bool {
		var current agentrelation.AgentMessage
		return f.db.Where("id = ?", returned.ID).First(&current).Error == nil && current.Status == agentrelation.MessageStatusCompleted
	}, time.Second, 10*time.Millisecond)

	// Changing the current edge to sync turns a separate legal graph cycle into
	// a synchronous call-stack deadlock, so it is persisted as a guard. A
	// separate parent keeps this assertion independent from final-return dedupe.
	require.NoError(t, f.db.Model(&agentrelation.AgentRelation{}).Where("source_agent_id=? AND target_agent_id=?", f.b.ID, f.a.ID).Update("delivery_policy", "sync").Error)
	var syncParent agentrelation.AgentMessage
	require.NoError(t, f.db.Where("tenant_id = ? AND id = ?", "tenant-a", first.ID).First(&syncParent).Error)
	syncParent.ID, syncParent.RootMessageID, syncParent.ConversationID = "return-sync-parent", "return-sync-parent", "return-sync-conversation"
	syncParent.IdempotencyKey, syncParent.Status = "return-sync-parent", agentrelation.MessageStatusRunning
	require.NoError(t, f.db.Create(&syncParent).Error)
	guarded, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{TargetAgent: f.a.Name, Scope: "return", Action: "report", Message: "sync answer", ParentMessageID: syncParent.ID, IdempotencyKey: "return-sync"})
	require.ErrorIs(t, err, agentrelation.ErrSyncDeadlock)
	require.Equal(t, agentrelation.MessageStatusGuarded, guarded.Status)
	require.Equal(t, "sync_wait_cycle", guarded.GuardReason)
}

func TestAgentMessageServiceCallbackWaitBlocksOnlyExactExplicitReturn(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.c, f.b, "callback-return", "async", "summary_only", "report", "inform", "challenge")
	addMessageRelation(t, f, f.c, f.a, "callback-return", "async", "summary_only", "report")

	makeCallbackChild := func(id string) *agentrelation.AgentMessage {
		t.Helper()
		deadline := time.Now().UTC().Add(10 * time.Minute)
		message := &agentrelation.AgentMessage{
			ID: id, TenantID: "tenant-a", RunID: "run-1", ConversationID: "callback-conversation",
			RootMessageID: id, Hop: 2, MaxHops: 8, DeadlineAt: &deadline,
			EventBudget: 64, EventCount: 2, TokenBudget: 65536, VisitedAgentIDs: []uint64{f.b.ID, f.c.ID},
			IdempotencyKey: id + "-message", Scope: "callback-return",
			SourceAgentID: f.b.ID, SourceAgent: f.b.Name, TargetAgentID: f.c.ID, TargetAgent: f.c.Name,
			Action: "consult", DeliveryPolicy: "async", ContextPolicy: "summary_only", Content: "B asks C",
			Status: agentrelation.MessageStatusCompleted, CreatedAt: time.Now().UTC(),
		}
		require.NoError(t, f.db.Create(message).Error)
		require.NoError(t, f.db.Create(&agentrelation.AgentMessageWait{
			ID: id + "-wait", TenantID: "tenant-a", RunID: "run-1", WaitingAgentID: f.b.ID,
			WaitingAgent: f.b.Name, RuntimeSessionID: id + "-session", ParentMessageID: id + "-upstream",
			WaitingMessageID: id, Status: agentrelation.MessageWaitStatusResumed,
			DedupeKey: id + "-dedupe", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}).Error)
		return message
	}

	for _, action := range []string{"report", "inform"} {
		child := makeCallbackChild("callback-child-" + action)
		guarded, sendErr := f.service.Send(context.Background(), "tenant-a", &f.c, SendAgentMessageInput{
			TargetAgent: f.b.Name, Scope: "callback-return", Action: action, Message: "duplicate explicit return",
			ParentMessageID: child.ID, IdempotencyKey: "callback-return-" + action,
		})
		require.ErrorIs(t, sendErr, agentrelation.ErrChainGuarded)
		require.Equal(t, agentrelation.MessageStatusGuarded, guarded.Status)
		require.Equal(t, "callback_return_managed", guarded.GuardReason)
		require.Equal(t, "该请求已由回调自动恢复上游，不得再显式回报", guarded.GuardDescription)
		var unchanged agentrelation.AgentMessage
		require.NoError(t, f.db.Where("tenant_id = ? AND id = ?", "tenant-a", child.ID).First(&unchanged).Error)
		require.Equal(t, agentrelation.MessageStatusCompleted, unchanged.Status)
	}

	// A different action on the exact reverse edge is not a duplicate result.
	challengeParent := makeCallbackChild("callback-child-challenge")
	challenge, err := f.service.Send(context.Background(), "tenant-a", &f.c, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "callback-return", Action: "challenge", Message: "challenge the premise",
		ParentMessageID: challengeParent.ID, IdempotencyKey: "callback-return-challenge",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, challenge.Status)

	// Nor is a report to a different participant: the guard is precise, not a
	// blanket ban on all sends made while a callback exists.
	lateralParent := makeCallbackChild("callback-child-lateral")
	lateral, err := f.service.Send(context.Background(), "tenant-a", &f.c, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "callback-return", Action: "report", Message: "lateral audit copy",
		ParentMessageID: lateralParent.ID, IdempotencyKey: "callback-return-lateral",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, lateral.Status)
}

func TestAgentMessageServiceCallbackReturnGuardIsTenantScoped(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "callback-tenant", "async", "summary_only", "consult")
	addMessageRelation(t, f, f.b, f.a, "callback-tenant", "async", "summary_only", "report")

	parent, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "callback-tenant", Action: "consult", Message: "ask",
		RunID: "run-1", IdempotencyKey: "callback-tenant-parent",
	})
	require.NoError(t, err)
	// A foreign tenant row with the same waiting_message_id must not affect
	// tenant-a authorization. Insert directly to exercise lookup isolation.
	require.NoError(t, f.db.Create(&agentrelation.AgentMessageWait{
		ID: "foreign-wait", TenantID: "tenant-b", RunID: "foreign-run", WaitingAgentID: 999,
		WaitingAgent: "foreign", RuntimeSessionID: "foreign-session", ParentMessageID: "foreign-parent",
		WaitingMessageID: parent.ID, Status: agentrelation.MessageWaitStatusWaiting,
		DedupeKey: "foreign-dedupe", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}).Error)

	returned, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "callback-tenant", Action: "report", Message: "valid return",
		ParentMessageID: parent.ID, IdempotencyKey: "callback-tenant-return",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, returned.Status)
}

func TestAgentMessageServiceAllowsOnlyFirstExactFinalReturn(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.b, f.a, "final-return", "async", "summary_only", "report", "inform", "challenge")
	now := time.Now().UTC()
	parent := &agentrelation.AgentMessage{
		ID: "final-parent", TenantID: "tenant-a", RunID: "run-1", ConversationID: "final-conversation",
		RootMessageID: "final-parent", Hop: 1, MaxHops: 8, DeadlineAt: ptrTime(now.Add(time.Minute)),
		EventBudget: 64, EventCount: 1, TokenBudget: 65536, VisitedAgentIDs: []uint64{f.a.ID, f.b.ID},
		IdempotencyKey: "final-parent-key", Scope: "final-return", SourceAgentID: f.a.ID, SourceAgent: f.a.Name,
		TargetAgentID: f.b.ID, TargetAgent: f.b.Name, Action: "assign", DeliveryPolicy: "async",
		ContextPolicy: "summary_only", Content: "do it", Status: agentrelation.MessageStatusCompleted, CreatedAt: now,
	}
	require.NoError(t, f.db.Create(parent).Error)

	first, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "final-return", Action: "report", Message: "authoritative result",
		ParentMessageID: parent.ID, IdempotencyKey: "final-return-first",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, first.Status)
	require.Eventually(t, func() bool {
		var current agentrelation.AgentMessage
		return f.db.Where("tenant_id = ? AND id = ?", "tenant-a", first.ID).First(&current).Error == nil &&
			isTerminalMessageStatus(current.Status)
	}, time.Second, 10*time.Millisecond)

	duplicate, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "final-return", Action: "inform", Message: "second result",
		ParentMessageID: parent.ID, IdempotencyKey: "final-return-second",
	})
	require.ErrorIs(t, err, agentrelation.ErrChainGuarded)
	require.Equal(t, agentrelation.MessageStatusGuarded, duplicate.Status)
	require.Equal(t, "duplicate_return", duplicate.GuardReason)
	require.Empty(t, duplicate.Content)

	// A non-final action on the same edge is independent of the claim.
	challenge, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "final-return", Action: "challenge", Message: "counterpoint",
		ParentMessageID: parent.ID, IdempotencyKey: "final-return-challenge",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, challenge.Status)

	// A foreign tenant's same parent id cannot reserve tenant-a's slot.
	require.NoError(t, f.db.Create(&agentrelation.AgentMessageReturnClaim{
		TenantID: "tenant-b", ParentMessageID: "other-parent", MessageID: "foreign-return",
		SourceAgentID: f.b.ID, TargetAgentID: f.a.ID, CreatedAt: now,
	}).Error)
	otherParent := *parent
	otherParent.ID, otherParent.RootMessageID, otherParent.IdempotencyKey = "other-parent", "other-parent", "other-parent-key"
	require.NoError(t, f.db.Create(&otherParent).Error)
	other, err := f.service.Send(context.Background(), "tenant-a", &f.b, SendAgentMessageInput{
		TargetAgent: f.a.Name, Scope: "final-return", Action: "report", Message: "tenant-local result",
		ParentMessageID: otherParent.ID, IdempotencyKey: "other-return",
	})
	require.NoError(t, err)
	require.NotEqual(t, agentrelation.MessageStatusGuarded, other.Status)
}

func ptrTime(value time.Time) *time.Time { return &value }

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
