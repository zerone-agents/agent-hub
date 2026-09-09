package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
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
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}, &agentrelation.AgentMessage{}))

	a := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	b := agent.AgentConfig{Name: "agent-b", TenantID: "tenant-a"}
	c := agent.AgentConfig{Name: "agent-c", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&c).Error)

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
			addTypedMessageRelation(t, f, f.b, f.a, "speeding-hq", "peer", tt.stance, "async", "summary_only", "inform")

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
			require.Contains(t, calls[0].message, "你对发送方的当前关系："+tt.stance)
			require.Contains(t, calls[0].message, "动作："+tt.action)
		})
	}
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
	require.Zero(t, count, "rejected messages must not be queued")
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

func TestAgentMessageServicePreventsNestedDispatchFromDeliveredTarget(t *testing.T) {
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
	require.True(t, errors.Is(nestedErr, agentrelation.ErrNestedDispatch))
	require.Len(t, f.runner.snapshot(), 1)
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
