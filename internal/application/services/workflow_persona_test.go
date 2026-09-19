package services

import (
	"testing"
	"time"

	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/workflow"

	"github.com/stretchr/testify/require"
)

// The H6 step-completion hook: completing a workflow step emits a
// deterministic mood event to the completer and a task_completed relation
// event from completer toward the task source, both idempotent.
func TestWorkflowStepCompletionEmitsPersonaEvents(t *testing.T) {
	s, db := workflowTestService(t)
	require.NoError(t, db.AutoMigrate(&rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}))
	started := time.Now().UTC()
	require.NoError(t, db.Create(&rundomain.Run{ID: "run-h6", TenantID: "t1", Name: "case", Status: rundomain.StatusRunning, StartedAt: &started}).Error)

	runSvc := NewRunService(db)
	emotionSvc := NewEmotionService(runSvc)
	reldynSvc := NewRelationDynamicsService(runSvc)
	s.SetPersonaHooks(emotionSvc, reldynSvc)

	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "draft", Name: "Draft", Type: "task", ActorRef: "agent-a"}})
	// StartedBy is a different agent than the completer, so the relation
	// event between source and completer must fire.
	ex, err := s.StartExecution("t1", v.ID, "run-h6", map[string]any{"title": "x"}, "start-1", "agent-b")
	require.NoError(t, err)
	var stepRunID string
	for _, sr := range ex.StepRuns {
		stepRunID = sr.ID
	}

	ex, err = s.CompleteStep("t1", stepRunID, map[string]any{"ok": true}, "done-1", "agent-a")
	require.NoError(t, err)
	require.Equal(t, workflow.ExecutionCompleted, ex.Status)

	var completerID, starterID uint64
	require.NoError(t, db.Table("agents").Select("id").Where("tenant_id=? AND name=?", "t1", "agent-a").Scan(&completerID).Error)
	require.NoError(t, db.Table("agents").Select("id").Where("tenant_id=? AND name=?", "t1", "agent-b").Scan(&starterID).Error)

	mood, err := emotionSvc.Status("t1", "run-h6", completerID)
	require.NoError(t, err)
	require.Greater(t, mood.Intensity, 0, "task_completed must move the completer's mood")

	attitude, err := reldynSvc.View("t1", "run-h6", completerID, starterID)
	require.NoError(t, err)
	require.Equal(t, 10, attitude.Score, "one task_completed event settled completer→source")

	// Idempotent replay: same idempotency key returns the same execution and
	// does not double-apply the persona events.
	ex, err = s.CompleteStep("t1", stepRunID, map[string]any{"ok": true}, "done-1", "agent-a")
	require.NoError(t, err)
	require.Equal(t, workflow.ExecutionCompleted, ex.Status)
	moodReplay, err := emotionSvc.Status("t1", "run-h6", completerID)
	require.NoError(t, err)
	require.Equal(t, mood.Intensity, moodReplay.Intensity, "replayed completion must not double-apply the mood event")
	attitudeReplay, err := reldynSvc.View("t1", "run-h6", completerID, starterID)
	require.NoError(t, err)
	require.Equal(t, 10, attitudeReplay.Score)
}

// Without hooks the workflow service behaves exactly as before (disabled-pack
// regression).
func TestWorkflowStepCompletionWithoutPersonaHooks(t *testing.T) {
	s, _ := workflowTestService(t)
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "draft", Name: "Draft", Type: "task", ActorRef: "agent-a"}})
	ex, err := s.StartExecution("t1", v.ID, "", map[string]any{"title": "x"}, "start-1", "agent-b")
	require.NoError(t, err)
	var stepRunID string
	for _, sr := range ex.StepRuns {
		stepRunID = sr.ID
	}
	ex, err = s.CompleteStep("t1", stepRunID, map[string]any{"ok": true}, "done-1", "agent-a")
	require.NoError(t, err)
	require.Equal(t, workflow.ExecutionCompleted, ex.Status)
}
