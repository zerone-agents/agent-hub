package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/extension"
	reldynamics "control-panel/internal/domain/reldynamics"
	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

// personaHookFixture extends the standard message fixture with the H6 pack
// state tables and the persona hooks wired.
func personaHookFixture(t *testing.T) agentMessageFixture {
	f := setupAgentMessageService(t)
	require.NoError(t, f.db.AutoMigrate(&rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}))
	runSvc := NewRunService(f.db)
	f.service.SetPersonaHooks(NewBeliefService(runSvc), NewRelationDynamicsService(runSvc))
	return f
}

// personaGateMessageFixture 在 personaHookFixture 之上接入真实的
// PersonaCapabilityGate（含内置能力包种子），使"停用扩展 → 能力关闭"这一
// 运行时链路可被断言。基座返回 lifecycle，用例可据此停用某个能力包。
func personaGateMessageFixture(t *testing.T) (agentMessageFixture, *ExtensionLifecycleService) {
	t.Helper()
	f := setupAgentMessageService(t)
	require.NoError(t, f.db.AutoMigrate(
		&rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{},
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
	))
	runSvc := NewRunService(f.db)
	f.service.SetPersonaHooks(NewBeliefService(runSvc), NewRelationDynamicsService(runSvc))
	lifecycle := NewExtensionLifecycleService(f.db)
	require.NoError(t, lifecycle.EnsureBuiltinPersonaPacks())
	f.service.SetPersonaCapabilityGate(NewPersonaCapabilityGate(f.db))
	return f, lifecycle
}

// The H6 gate hook: once the target holds a hostile attitude toward the
// source, assign-like sends are flagged for the target's confirmation while
// inform/report are unaffected. The verdict is persisted in the delivery
// audit (GuardReason) and surfaced in the tool result DTO.
func TestAgentMessageRelationGateHostileAssignNeedsConfirmInformUnaffected(t *testing.T) {
	f := personaHookFixture(t)
	runSvc := NewRunService(f.db)
	reldyn := NewRelationDynamicsService(runSvc)
	// B feels betrayed by A twice at max severity: two events at the -40
	// single-event floor reach -80 → hostile (< -60).
	require.NoError(t, reldyn.OnEvent("tenant-a", "run-1", f.b.ID, f.a.ID, "betrayed", 3, time.Now().UTC(), "betrayal-1"))
	require.NoError(t, reldyn.OnEvent("tenant-a", "run-1", f.b.ID, f.a.ID, "betrayed", 3, time.Now().UTC(), "betrayal-2"))
	attitude, err := reldyn.View("tenant-a", "run-1", f.b.ID, f.a.ID)
	require.NoError(t, err)
	require.Equal(t, "hostile", attitude.Stance)

	addMessageRelation(t, f, f.a, f.b, "task", "async", "summary_only", "assign", "inform")

	assign, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "task", Action: "assign", Message: "把报告写完", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "relation_gate_need_confirm", assign.GuardReason)
	require.Equal(t, agentrelation.MessageStatusQueued, assign.Status, "need-confirm must not block delivery")
	stored, err := f.service.Get("tenant-a", &f.a, assign.ID)
	require.NoError(t, err)
	require.Equal(t, "relation_gate_need_confirm", stored.GuardReason, "gate verdict persisted in delivery audit")

	inform, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "task", Action: "inform", Message: " FYI 报告延期", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Empty(t, inform.GuardReason, "inform is unaffected by hostility")
}

// Without the hook (no persona packs injected) behavior is unchanged — the
// disabled-pack Core regression.
func TestAgentMessageGateHookAbsentByDefault(t *testing.T) {
	f := setupAgentMessageService(t)
	addMessageRelation(t, f, f.a, f.b, "task", "async", "summary_only", "assign")
	got, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "task", Action: "assign", Message: "无钩子", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Empty(t, got.GuardReason)
}

// Delivery hook: a durably delivered run message becomes a belief fact for
// the target agent, keyed by the message ID with a deterministic idempotency
// key.
func TestAgentMessageDeliveryRecordsBeliefFact(t *testing.T) {
	f := personaHookFixture(t)
	addMessageRelation(t, f, f.a, f.b, "speeding-hq", "async", "summary_only", "inform")
	got, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "speeding-hq", Action: "inform", Message: "账目有异常", RunID: "run-1",
	})
	require.NoError(t, err)

	runSvc := NewRunService(f.db)
	beliefSvc := NewBeliefService(runSvc)
	beliefs, err := beliefSvc.List("tenant-a", "run-1", f.b.ID, got.ID)
	require.NoError(t, err)
	require.Len(t, beliefs, 1)
	require.Equal(t, got.ID, beliefs[0].FactRef)

	// The target cannot see facts that were never delivered to it.
	other, err := beliefSvc.List("tenant-a", "run-1", f.c.ID, got.ID)
	require.NoError(t, err)
	require.Len(t, other, 0)
}

// 回归 P1：停用 relationship-dynamics 扩展不得放宽投递授权。
//
// 旧实现把"拒绝投递"（!allowed → relation_gate_denied）整段判定放进
// `personaPackEnabled(tenant-a, relationship-dynamics)` 条件里，于是管理员
// 一旦停用该扩展，Gate 根本不再被咨询——关闭一个功能反而扩大了权限，属于
// "功能开关泄漏成安全开关"。判定与开关必须解耦：Gate 只要接线就始终执行，
// 只有 need_confirm 这类"影响力标注"才受开关控制。
//
// 这里用"关系状态损坏"作为可观测信号：Gate 求值失败时会把
// relation_gate_eval_failed 写进投递审计。若 Gate 在停用后被跳过，该标记
// 不会出现——本用例因此能区分"仍被咨询"与"被静默跳过"。
func TestAgentMessageRelationGateStillConsultedAfterPackDisable(t *testing.T) {
	f, lifecycle := personaGateMessageFixture(t)

	// a→c 的关系状态被写成无法解析的 score：Gate 的 View 会报错。
	require.NoError(t, f.db.Create(&rundomain.RunState{
		TenantID: "tenant-a", RunID: "run-1",
		Namespace: reldynamics.Namespace, SchemaName: "relation", SchemaVersion: "1",
		SchemaHash:  strings.Repeat("0", 64),
		SubjectType: reldynamics.SubjectType,
		SubjectID:   reldynamics.SubjectID(f.c.ID, f.a.ID),
		Revision:    1,
		Data:        map[string]any{"score": "not-a-number"},
	}).Error)
	addMessageRelation(t, f, f.a, f.c, "task", "async", "summary_only", "assign")

	// 启用态：Gate 被咨询 → 审计留下求值失败原因，投递不受影响。
	enabledSend, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "task", Action: "assign", Message: "启用态：状态损坏", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "relation_gate_eval_failed", enabledSend.GuardReason)

	// 停用 relationship-dynamics 能力包。
	ext := findSeededExtension(t, f.db, "io.zerone.relationship-dynamics")
	_, err = lifecycle.Disable("default", ext.ID)
	require.NoError(t, err)

	// 停用态：Gate 仍必须被咨询，否则"拒绝投递"这条约束会随功能一起消失。
	disabledSend, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.c.Name, Scope: "task", Action: "assign", Message: "停用态：状态损坏", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "relation_gate_eval_failed", disabledSend.GuardReason,
		"停用能力包不得使 Gate 被跳过：关闭功能不能扩大权限")
}

// 与上一条互补的另一半：need_confirm 是"影响力标注"而非授权约束，仍应受
// 能力开关控制——停用后不再给对方的消息打确认标记，投递行为复原。
func TestAgentMessageRelationGateNeedConfirmFollowsPackSwitch(t *testing.T) {
	f, lifecycle := personaGateMessageFixture(t)
	reldyn := NewRelationDynamicsService(NewRunService(f.db))
	// B 对 A 两次 max severity 的 betrayed：单事件 -40 已到下限，两次累积到
	// -80，落入 hostile（< -60）。
	require.NoError(t, reldyn.OnEvent("tenant-a", "run-1", f.b.ID, f.a.ID, "betrayed", 3, time.Now().UTC(), "betrayal-1"))
	require.NoError(t, reldyn.OnEvent("tenant-a", "run-1", f.b.ID, f.a.ID, "betrayed", 3, time.Now().UTC(), "betrayal-2"))
	attitude, err := reldyn.View("tenant-a", "run-1", f.b.ID, f.a.ID)
	require.NoError(t, err)
	require.Equal(t, "hostile", attitude.Stance)

	addMessageRelation(t, f, f.a, f.b, "task", "async", "summary_only", "assign")

	enabledSend, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "task", Action: "assign", Message: "启用态：敌意指派", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Equal(t, "relation_gate_need_confirm", enabledSend.GuardReason)
	require.Equal(t, agentrelation.MessageStatusQueued, enabledSend.Status, "need-confirm 不阻断投递")

	ext := findSeededExtension(t, f.db, "io.zerone.relationship-dynamics")
	_, err = lifecycle.Disable("default", ext.ID)
	require.NoError(t, err)

	disabledSend, err := f.service.Send(context.Background(), "tenant-a", &f.a, SendAgentMessageInput{
		TargetAgent: f.b.Name, Scope: "task", Action: "assign", Message: "停用态：敌意指派", RunID: "run-1",
	})
	require.NoError(t, err)
	require.Empty(t, disabledSend.GuardReason, "停用后不再打确认标记（标注而非授权约束）")
}
