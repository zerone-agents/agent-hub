package services

import (
	"context"
	"testing"
	"time"

	"control-panel/internal/domain/agentrelation"
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
