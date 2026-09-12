package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/capability"
	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRunTestService(t *testing.T) (*RunService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &capability.Package{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.CapabilityBinding{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}, &rundomain.RunActivity{}, &rundomain.ToolResultRecord{}, &eventdomain.StreamCursor{}, &eventdomain.Envelope{}, &eventdomain.Delivery{}, &eventdomain.DeliveryAttempt{}, &eventdomain.CausalBudget{}))
	require.True(t, db.Migrator().HasIndex(&rundomain.RunStateChange{}, "uk_run_state_change_idempotency"))
	return NewRunService(db), db
}

func TestRunServiceLifecycleSnapshotsAndFreeze(t *testing.T) {
	s, db := newRunTestService(t)
	require.NoError(t, db.Create(&capability.Package{TenantID: "t1", Namespace: "io.test", Name: "test", Version: "1.0.0", ManifestYAML: "manifest-v1", ResourcesSnapshot: map[string]any{"prompt": "v1"}, ContentHash: "sha", Enabled: true}).Error)
	a := agent.AgentConfig{TenantID: "t1", Name: "analyst", ContentHash: "cfg-v1", SystemPrompt: "x", PersonalityPrompt: "careful", PersonalityTemplateName: "reviewer", PersonalityTemplateVersion: 2}
	require.NoError(t, db.Create(&a).Error)
	r, err := s.Create("t1", CreateRunInput{Name: "review", Capabilities: []RunCapabilityInput{{Namespace: "io.test", PackageName: "test", Version: "1.0.0"}}})
	require.NoError(t, err)
	participant, err := s.AddAgent("t1", r.ID, a.ID, "reviewer")
	require.NoError(t, err)
	require.Equal(t, "cfg-v1", participant.AgentConfigHashSnapshot)
	r, err = s.Transition("t1", r.ID, rundomain.StatusRunning)
	require.NoError(t, err)
	require.NotNil(t, r.StartedAt)
	_, err = s.AddAgent("t1", r.ID, a.ID, "late participant")
	require.ErrorIs(t, err, rundomain.ErrFrozen)
	r, err = s.Transition("t1", r.ID, rundomain.StatusCompleted)
	require.NoError(t, err)
	require.NotNil(t, r.CompletedAt)
	_, err = s.AddAgent("t1", r.ID, a.ID+1, "")
	require.ErrorIs(t, err, rundomain.ErrFrozen)
	_, err = s.Transition("t1", r.ID, rundomain.StatusRunning)
	require.ErrorIs(t, err, rundomain.ErrInvalidTransition)
}

func TestRunStateIsolationOptimisticLockIdempotencyAndFreeze(t *testing.T) {
	s, _ := newRunTestService(t)
	_, err := s.RegisterStateSchema("t1", RegisterStateSchemaInput{Namespace: "io.test.progress", Name: "task", Version: "1.0.0", ScopeTypes: []string{"run"}, SubjectTypes: []string{"agent"}, Schema: map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"progress": map[string]any{"type": "integer", "minimum": 0.0, "maximum": 100.0}}, "required": []any{"progress"}}})
	require.NoError(t, err)
	r1, _ := s.Create("t1", CreateRunInput{Name: "one"})
	r2, _ := s.Create("t1", CreateRunInput{Name: "two"})
	init := InitializeStateInput{Namespace: "io.test.progress", SchemaName: "task", SchemaVersion: "1.0.0", SubjectType: "agent", SubjectID: "7", Data: map[string]any{"progress": 10.0}, IdempotencyKey: "init-r1"}
	state, err := s.InitializeState("t1", r1.ID, init)
	require.NoError(t, err)
	replayedInit, err := s.InitializeState("t1", r1.ID, init)
	require.NoError(t, err)
	require.Equal(t, state.ID, replayedInit.ID)
	init.IdempotencyKey = "init-r2"
	init.Data = map[string]any{"progress": 80.0}
	_, err = s.InitializeState("t1", r2.ID, init)
	require.NoError(t, err)
	updated, err := s.CommitState("t1", r1.ID, state.ID, CommitStateInput{ExpectedRevision: 1, Data: map[string]any{"progress": 20.0}, IdempotencyKey: "update-1"})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Revision)
	replayed, err := s.CommitState("t1", r1.ID, state.ID, CommitStateInput{ExpectedRevision: 1, Data: map[string]any{"progress": 99.0}, IdempotencyKey: "update-1"})
	require.NoError(t, err)
	require.Equal(t, float64(20), replayed.Data["progress"])
	_, err = s.CommitState("t1", r1.ID, state.ID, CommitStateInput{ExpectedRevision: 1, Data: map[string]any{"progress": 30.0}, IdempotencyKey: "stale"})
	require.ErrorIs(t, err, rundomain.ErrConflict)
	states2, _ := s.States("t1", r2.ID)
	require.Equal(t, float64(80), states2[0].Data["progress"])
	_, _ = s.Transition("t1", r1.ID, rundomain.StatusRunning)
	_, _ = s.Transition("t1", r1.ID, rundomain.StatusCompleted)
	_, err = s.CommitState("t1", r1.ID, state.ID, CommitStateInput{ExpectedRevision: 2, Data: map[string]any{"progress": 30.0}, IdempotencyKey: "frozen"})
	require.ErrorIs(t, err, rundomain.ErrFrozen)
}

func TestRunServiceTenantIsolationAndActivity(t *testing.T) {
	s, _ := newRunTestService(t)
	r, _ := s.Create("tenant-a", CreateRunInput{Name: "a"})
	_, err := s.Get("tenant-b", r.ID)
	require.ErrorIs(t, err, rundomain.ErrNotFound)
	_, err = s.AppendActivity("tenant-b", r.ID, AppendRunActivityInput{Kind: "run.started"})
	require.ErrorIs(t, err, rundomain.ErrNotFound)
	a, err := s.AppendActivity("tenant-a", r.ID, AppendRunActivityInput{Kind: "agent.step", Status: "completed", StepID: "s1"})
	require.NoError(t, err)
	require.NotZero(t, a.OccurredAt)
	rows, err := s.ListActivities("tenant-a", r.ID, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	_, err = s.Transition("tenant-a", r.ID, rundomain.StatusRunning)
	require.NoError(t, err)
	_, err = s.Transition("tenant-a", r.ID, rundomain.StatusCompleted)
	require.NoError(t, err)
	_, err = s.AppendActivity("tenant-a", r.ID, AppendRunActivityInput{Kind: "late"})
	require.ErrorIs(t, err, rundomain.ErrFrozen)
}
