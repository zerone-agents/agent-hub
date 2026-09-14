package services

import (
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/reldynamics"
	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

func newRelationDynamicsTestService(t *testing.T) (*RelationDynamicsService, *RunService) {
	t.Helper()
	s, _ := newRunTestService(t)
	return NewRelationDynamicsService(s), s
}

func createRelationTestRun(t *testing.T, s *RunService, tenantID, name string) *rundomain.Run {
	t.Helper()
	r, err := s.Create(tenantID, CreateRunInput{Name: name})
	require.NoError(t, err)
	return r
}

func TestRelationDynamicsEnsureSchemasIdempotent(t *testing.T) {
	dyn, runs := newRelationDynamicsTestService(t)
	createRelationTestRun(t, runs, "org-schema", "schema-run")
	require.NoError(t, dyn.EnsureSchemas())
	require.NoError(t, dyn.EnsureSchemas(), "schema registration must be idempotent")
}

func TestRelationDynamicsOnEventViewAndIdempotency(t *testing.T) {
	dyn, runs := newRelationDynamicsTestService(t)
	tenantID := "org-rel"
	victim := agent.AgentConfig{TenantID: tenantID, Name: "阿伟"}
	traitor := agent.AgentConfig{TenantID: tenantID, Name: "老六"}
	require.NoError(t, runs.db.Create(&victim).Error)
	require.NoError(t, runs.db.Create(&traitor).Error)
	run := createRelationTestRun(t, runs, tenantID, "betrayal-run")
	at := time.Now().UTC()

	// victim perceives betrayal by traitor: -35
	require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "betrayed", 1, at, "evt-1"))
	attitude, err := dyn.View(tenantID, run.ID, victim.ID, traitor.ID)
	require.NoError(t, err)
	require.Equal(t, -35, attitude.Score)
	require.Equal(t, "wary", attitude.Stance)
	require.Equal(t, "你对 老六 的态度：警惕（-35）", attitude.Narration)

	// duplicate idempotency key = no-op
	require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "betrayed", 1, at, "evt-1"))
	attitude, err = dyn.View(tenantID, run.ID, victim.ID, traitor.ID)
	require.NoError(t, err)
	require.Equal(t, -35, attitude.Score)

	// two severe betrayals drop to hostile with clamp at -100
	require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "betrayed", 3, at.Add(time.Minute), "evt-2"))
	require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "betrayed", 3, at.Add(2*time.Minute), "evt-3"))
	attitude, err = dyn.View(tenantID, run.ID, victim.ID, traitor.ID)
	require.NoError(t, err)
	require.Equal(t, -100, attitude.Score)
	require.Equal(t, "hostile", attitude.Stance)
	require.Equal(t, "你对 老六 的态度：敌对（-100）", attitude.Narration)

	// positive events recover within caps
	require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "aided", 1, at.Add(3*time.Minute), "evt-4"))
	attitude, err = dyn.View(tenantID, run.ID, victim.ID, traitor.ID)
	require.NoError(t, err)
	require.Equal(t, -85, attitude.Score)

	// unknown event type is rejected in Chinese
	require.ErrorContains(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "hugged", 1, at, "evt-5"), "未知的关系事件类型")
}

func TestRelationDynamicsGateHook(t *testing.T) {
	dyn, runs := newRelationDynamicsTestService(t)
	tenantID := "org-gate"
	victim := agent.AgentConfig{TenantID: tenantID, Name: "victim"}
	traitor := agent.AgentConfig{TenantID: tenantID, Name: "traitor"}
	require.NoError(t, runs.db.Create(&victim).Error)
	require.NoError(t, runs.db.Create(&traitor).Error)
	run := createRelationTestRun(t, runs, tenantID, "gate-run")
	at := time.Now().UTC()

	// neutral default: assign passes without confirmation
	allowed, confirm, err := dyn.Gate(tenantID, run.ID, victim.ID, traitor.ID, "assign")
	require.NoError(t, err)
	require.True(t, allowed)
	require.False(t, confirm)

	// drive the victim's attitude toward traitor to hostile
	for i, key := range []string{"g-1", "g-2", "g-3"} {
		require.NoError(t, dyn.OnEvent(tenantID, run.ID, victim.ID, traitor.ID, "betrayed", 3, at.Add(time.Duration(i)*time.Minute), key))
	}

	// hostile: traitor-initiated assign requires victim confirmation, inform unaffected
	allowed, confirm, err = dyn.Gate(tenantID, run.ID, victim.ID, traitor.ID, "assign")
	require.NoError(t, err)
	require.True(t, allowed)
	require.True(t, confirm)
	allowed, confirm, err = dyn.Gate(tenantID, run.ID, victim.ID, traitor.ID, "inform")
	require.NoError(t, err)
	require.True(t, allowed)
	require.False(t, confirm)

	// the reverse direction (traitor's attitude toward victim) stays neutral:
	// victim-initiated assign needs no confirmation from traitor
	allowed, confirm, err = dyn.Gate(tenantID, run.ID, traitor.ID, victim.ID, "assign")
	require.NoError(t, err)
	require.True(t, allowed)
	require.False(t, confirm)
}

func TestRelationDynamicsCrossRunIsolation(t *testing.T) {
	dyn, runs := newRelationDynamicsTestService(t)
	tenantID := "org-iso"
	a := agent.AgentConfig{TenantID: tenantID, Name: "agent-a"}
	b := agent.AgentConfig{TenantID: tenantID, Name: "agent-b"}
	require.NoError(t, runs.db.Create(&a).Error)
	require.NoError(t, runs.db.Create(&b).Error)
	runA := createRelationTestRun(t, runs, tenantID, "run-a")
	runB := createRelationTestRun(t, runs, tenantID, "run-b")
	at := time.Now().UTC()

	require.NoError(t, dyn.OnEvent(tenantID, runA.ID, a.ID, b.ID, "betrayed", 3, at, "iso-1"))
	attitudeA, err := dyn.View(tenantID, runA.ID, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, -40, attitudeA.Score)
	require.Equal(t, "wary", attitudeA.Stance)

	// run B must not see run A's attitude: neutral default, gate unaffected
	attitudeB, err := dyn.View(tenantID, runB.ID, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, 0, attitudeB.Score)
	require.Equal(t, "neutral", attitudeB.Stance)
	_, confirm, err := dyn.Gate(tenantID, runB.ID, a.ID, b.ID, "assign")
	require.NoError(t, err)
	require.False(t, confirm)

	// settling an event in run B does not disturb run A
	require.NoError(t, dyn.OnEvent(tenantID, runB.ID, a.ID, b.ID, "cooperated", 1, at, "iso-2"))
	attitudeB, err = dyn.View(tenantID, runB.ID, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, 8, attitudeB.Score)
	attitudeA, err = dyn.View(tenantID, runA.ID, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, -40, attitudeA.Score)
}

func TestRelationDynamicsCrossTenantIsolation(t *testing.T) {
	dyn, runs := newRelationDynamicsTestService(t)
	a := agent.AgentConfig{TenantID: "tenant-x", Name: "agent-a"}
	b := agent.AgentConfig{TenantID: "tenant-x", Name: "agent-b"}
	require.NoError(t, runs.db.Create(&a).Error)
	require.NoError(t, runs.db.Create(&b).Error)
	runX := createRelationTestRun(t, runs, "tenant-x", "run-x")
	at := time.Now().UTC()
	require.NoError(t, dyn.OnEvent("tenant-x", runX.ID, a.ID, b.ID, "betrayed", 3, at, "x-1"))

	// tenant-y cannot see tenant-x's attitude state
	attitude, err := dyn.View("tenant-y", runX.ID, a.ID, b.ID)
	require.NoError(t, err)
	require.Equal(t, 0, attitude.Score)
}

func TestRelationDynamicsSubjectIDFormat(t *testing.T) {
	require.Equal(t, "3:9", reldynamics.SubjectID(3, 9))
}
