package services

import (
	"fmt"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/decision"
	"control-panel/internal/domain/workflow"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func decisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &collaboration.Group{}, &collaboration.GroupMember{}, &workflow.Definition{}, &workflow.Version{}, &workflow.Step{}, &workflow.Transition{}, &workflow.Execution{}, &workflow.StepRun{}, &workflow.StepDispatch{}, &workflow.Approval{}, &workflow.ApprovalDecision{}, &workflow.Audit{}, &decision.Decision{}, &decision.Elector{}, &decision.Vote{}, &decision.Result{}, &decision.Audit{}, &decision.Escalation{}))
	db.Create(&collaboration.Group{ID: "g1", TenantID: "t1", Name: "board", Visibility: collaboration.GroupVisibilityPrivate})
	for i, r := range []string{collaboration.RoleLeader, collaboration.RoleMember, collaboration.RoleObserver} {
		db.Create(&agent.AgentConfig{ID: uint64(i + 1), TenantID: "t1", Name: fmt.Sprintf("a%d", i+1), ContentHash: "h", SystemPrompt: "p"})
		db.Create(&collaboration.GroupMember{TenantID: "t1", GroupID: "g1", AgentID: uint64(i + 1), Role: r, JoinedAt: time.Now()})
	}
	return db
}

func TestDecisionValidatesWorkflowAndEscalationTenant(t *testing.T) {
	db := decisionTestDB(t)
	s := NewDecisionService(db)
	in := CreateDecisionInput{GroupID: "g1", WorkflowRunID: "foreign", Title: "x", QuorumPercent: 50, ApprovalPercent: 50, TimeoutAction: decision.TimeoutNone, Electors: []ElectorInput{{AgentID: 1, Weight: 1}}}
	_, err := s.Create("t1", in, "admin")
	require.ErrorContains(t, err, "workflow run not found")
	db.Create(&agent.AgentConfig{ID: 9, TenantID: "t2", Name: "foreign", ContentHash: "h", SystemPrompt: "p"})
	in.WorkflowRunID = ""
	in.TimeoutAction = decision.TimeoutEscalate
	in.EscalateAgentID = 9
	_, err = s.Create("t1", in, "admin")
	require.ErrorContains(t, err, "not found in tenant")
	d := createDecision(t, s, nil, decision.TimeoutNone)
	_, err = s.Get("t2", d.ID)
	require.ErrorIs(t, err, ErrDecisionNotFound)
	_, err = s.CastVote("t2", d.ID, 1, decision.VoteApprove, "", "x")
	require.ErrorIs(t, err, ErrDecisionNotFound)
	_, err = s.EscalateFailure("t1", d.ID, decision.TimeoutTransfer, 9, "failed", "admin")
	require.ErrorContains(t, err, "not found in tenant")
}

func TestWorkflowDecisionPassAndRejectCloseLoop(t *testing.T) {
	db := decisionTestDB(t)
	ws := NewWorkflowService(db)
	ds := NewDecisionService(db)
	def, err := ws.CreateDefinition("t1", "board-flow", "", "admin")
	require.NoError(t, err)
	config := map[string]any{"groupId": "g1", "quorumPercent": 50, "approvalPercent": 50, "electors": []any{map[string]any{"agentId": float64(1), "weight": float64(2)}, map[string]any{"agentId": float64(2), "weight": float64(1), "canVeto": true}}}
	v, err := ws.CreateVersion("t1", def.ID, CreateWorkflowVersionInput{Steps: []WorkflowStepInput{{Key: "vote", Name: "Board vote", Type: "decision", Config: config}, {Key: "execute", Name: "Execute", Type: "task", ActorRef: "a1", DependsOn: []string{"vote"}}}})
	require.NoError(t, err)
	v, err = ws.PublishVersion("t1", v.ID)
	require.NoError(t, err)
	ex, err := ws.StartExecution("t1", v.ID, "", nil, "idem-pass", "admin")
	require.NoError(t, err)
	require.Equal(t, workflow.StepWaitingDecision, ex.StepRuns[0].Status)
	var d decision.Decision
	require.NoError(t, db.Where("tenant_id=? AND workflow_run_id=?", "t1", ex.ID).First(&d).Error)
	_, err = ds.CastVote("t1", d.ID, 1, decision.VoteApprove, "", "a1")
	require.NoError(t, err)
	closed, err := ds.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	require.Equal(t, decision.StatusPassed, closed.Status)
	ex, err = ws.Execution("t1", ex.ID)
	require.NoError(t, err)
	var voteRun, executeRun workflow.StepRun
	for _, r := range ex.StepRuns {
		if r.StepKey == "vote" {
			voteRun = r
		}
		if r.StepKey == "execute" {
			executeRun = r
		}
	}
	require.Equal(t, workflow.StepCompleted, voteRun.Status)
	require.Equal(t, workflow.StepRunning, executeRun.Status)

	ex2, err := ws.StartExecution("t1", v.ID, "", nil, "idem-reject", "admin")
	require.NoError(t, err)
	d = decision.Decision{}
	require.NoError(t, db.Where("tenant_id=? AND workflow_run_id=?", "t1", ex2.ID).First(&d).Error)
	_, err = ds.CastVote("t1", d.ID, 2, decision.VoteReject, "veto", "a2")
	require.NoError(t, err)
	_, err = ds.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	ex2, err = ws.Execution("t1", ex2.ID)
	require.NoError(t, err)
	require.Equal(t, workflow.ExecutionFailed, ex2.Status)
}

func TestWorkflowDecisionNotifiesAgentsVotesAndDispatchesSuccessor(t *testing.T) {
	db := decisionTestDB(t)
	ws := NewWorkflowService(db)
	dispatcher := &recordingWorkflowDispatcher{wake: make(chan struct{}, 10)}
	ws.SetDispatcher(dispatcher)
	ds := NewDecisionService(db)
	ds.SetWorkflowService(ws)
	def, err := ws.CreateDefinition("t1", "autonomous-board", "", "admin")
	require.NoError(t, err)
	config := map[string]any{"groupId": "g1", "quorumPercent": 100, "approvalPercent": 50, "electors": []any{map[string]any{"agentId": float64(1), "weight": float64(2)}, map[string]any{"agentId": float64(2), "weight": float64(1)}}}
	v, err := ws.CreateVersion("t1", def.ID, CreateWorkflowVersionInput{Steps: []WorkflowStepInput{{Key: "vote", Name: "Board vote", Type: "decision", Config: config}, {Key: "execute", Name: "Execute", Type: "task", ActorRef: "a3", DependsOn: []string{"vote"}}}})
	require.NoError(t, err)
	v, err = ws.PublishVersion("t1", v.ID)
	require.NoError(t, err)
	ex, err := ws.StartExecution("t1", v.ID, "", map[string]any{"proposal": "p"}, "decision-e2e", "admin")
	require.NoError(t, err)
	calls := dispatcher.wait(t, 2)
	require.ElementsMatch(t, []string{"a1", "a2"}, []string{calls[0].agent, calls[1].agent})
	for _, call := range calls {
		require.Contains(t, call.prompt, "decision_vote")
		require.Contains(t, call.prompt, "决策 ID")
	}
	var d decision.Decision
	require.NoError(t, db.Where("tenant_id=? AND workflow_run_id=?", "t1", ex.ID).First(&d).Error)
	for _, name := range []string{"a1", "a2"} {
		var a agent.AgentConfig
		require.NoError(t, db.Where("tenant_id=? AND name=?", "t1", name).First(&a).Error)
		_, err = ds.CastAgentVote("t1", &a, d.ID, decision.VoteApprove, "fake runner autonomous vote")
		require.NoError(t, err)
	}
	_, err = ds.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	calls = dispatcher.wait(t, 3)
	require.Equal(t, "a3", calls[2].agent)
	require.Contains(t, calls[2].prompt, "workflow_step_complete")
}

func startEscalatingDecisionWorkflow(t *testing.T, timeoutAction string) (*gorm.DB, *WorkflowService, *DecisionService, *recordingWorkflowDispatcher, *decision.Decision) {
	t.Helper()
	db := decisionTestDB(t)
	ws := NewWorkflowService(db)
	dispatcher := &recordingWorkflowDispatcher{wake: make(chan struct{}, 10)}
	ws.SetDispatcher(dispatcher)
	ds := NewDecisionService(db)
	ds.SetWorkflowService(ws)
	def, err := ws.CreateDefinition("t1", "escalation-flow", "", "admin")
	require.NoError(t, err)
	config := map[string]any{"groupId": "g1", "quorumPercent": 50, "approvalPercent": 50, "timeoutAction": timeoutAction, "electors": []any{map[string]any{"agentId": float64(1), "weight": float64(1)}, map[string]any{"agentId": float64(2), "weight": float64(1)}}}
	if timeoutAction != "" && timeoutAction != decision.TimeoutNone {
		config["escalateAgentId"] = float64(3)
	}
	v, err := ws.CreateVersion("t1", def.ID, CreateWorkflowVersionInput{Steps: []WorkflowStepInput{{Key: "vote", Name: "Vote", Type: "decision", Config: config, TimeoutSeconds: 60, EscalationStepKey: "escalate"}, {Key: "escalate", Name: "Escalate", Type: "task", ActorRef: "a3", DependsOn: []string{"vote"}}}})
	require.NoError(t, err)
	v, err = ws.PublishVersion("t1", v.ID)
	require.NoError(t, err)
	ex, err := ws.StartExecution("t1", v.ID, "", nil, "start-"+timeoutAction, "admin")
	require.NoError(t, err)
	dispatcher.wait(t, 2)
	var d decision.Decision
	require.NoError(t, db.Where("tenant_id=? AND workflow_run_id=?", "t1", ex.ID).First(&d).Error)
	return db, ws, ds, dispatcher, &d
}

func TestWorkflowDecisionTimeoutActuallyDispatchesEscalation(t *testing.T) {
	db, _, ds, dispatcher, d := startEscalatingDecisionWorkflow(t, decision.TimeoutEscalate)
	past := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&decision.Decision{}).Where("id=?", d.ID).Update("deadline_at", past).Error)
	_, err := ds.ApplyTimeout("t1", d.ID, "scheduler")
	require.NoError(t, err)
	calls := dispatcher.wait(t, 3)
	require.Equal(t, "a3", calls[2].agent)
	require.Contains(t, calls[2].prompt, "workflow_step_complete")
}

func TestWorkflowDecisionFailureActuallyDispatchesEscalation(t *testing.T) {
	_, _, ds, dispatcher, d := startEscalatingDecisionWorkflow(t, decision.TimeoutNone)
	_, err := ds.EscalateFailure("t1", d.ID, decision.TimeoutTransfer, 3, "voting backend failed", "scheduler")
	require.NoError(t, err)
	calls := dispatcher.wait(t, 3)
	require.Equal(t, "a3", calls[2].agent)
	require.Contains(t, calls[2].prompt, "workflow_step_complete")
}

func createDecision(t *testing.T, s *DecisionService, deadline *time.Time, action string) *decision.Decision {
	t.Helper()
	d, err := s.Create("t1", CreateDecisionInput{GroupID: "g1", Title: "proposal", QuorumPercent: 60, ApprovalPercent: 60, DeadlineAt: deadline, TimeoutAction: action, EscalateAgentID: 3, Electors: []ElectorInput{{AgentID: 1, Weight: 3}, {AgentID: 2, Weight: 2, CanVeto: true}, {AgentID: 3, Weight: 1}}}, "admin")
	require.NoError(t, err)
	return d
}

func TestDecisionFreezesElectorateAndComputesWeightedPass(t *testing.T) {
	db := decisionTestDB(t)
	s := NewDecisionService(db)
	d := createDecision(t, s, nil, decision.TimeoutNone)
	require.Equal(t, "leader", d.Electorate[0].Role)
	require.NoError(t, db.Model(&collaboration.GroupMember{}).Where("tenant_id=? AND group_id=? AND agent_id=?", "t1", "g1", 1).Update("role", "guest").Error)
	_, err := s.CastVote("t1", d.ID, 1, decision.VoteApprove, "sound", "agent:1")
	require.NoError(t, err)
	_, err = s.CastVote("t1", d.ID, 3, decision.VoteApprove, "ok", "agent:3")
	require.NoError(t, err)
	got, err := s.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	require.Equal(t, decision.StatusPassed, got.Status)
	require.True(t, got.Result.QuorumMet)
	require.Equal(t, 4, got.Result.ApproveWeight)
	require.Equal(t, "leader", got.Electorate[0].Role)
}

func TestDecisionVetoOverridesWeightedMajority(t *testing.T) {
	s := NewDecisionService(decisionTestDB(t))
	d := createDecision(t, s, nil, decision.TimeoutNone)
	_, _ = s.CastVote("t1", d.ID, 1, decision.VoteApprove, "", "a1")
	_, _ = s.CastVote("t1", d.ID, 2, decision.VoteReject, "material risk", "a2")
	_, _ = s.CastVote("t1", d.ID, 3, decision.VoteApprove, "", "a3")
	got, err := s.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	require.Equal(t, decision.StatusRejected, got.Status)
	require.True(t, got.Result.Vetoed)
	audits, err := s.Audits("t1", d.ID)
	require.NoError(t, err)
	require.Len(t, audits, 5)
	var vetoVoteAudited bool
	for _, audit := range audits {
		if audit.Action == "vote_cast" && audit.Payload["vetoApplied"] == true {
			vetoVoteAudited = true
		}
	}
	require.True(t, vetoVoteAudited)
}

func TestDecisionNoQuorum(t *testing.T) {
	s := NewDecisionService(decisionTestDB(t))
	d := createDecision(t, s, nil, decision.TimeoutNone)
	_, err := s.CastVote("t1", d.ID, 3, decision.VoteApprove, "", "a3")
	require.NoError(t, err)
	got, err := s.Close("t1", d.ID, "admin")
	require.NoError(t, err)
	require.Equal(t, decision.StatusNoQuorum, got.Status)
	require.False(t, got.Result.QuorumMet)
}

func TestDecisionTimeoutEscalationIsIdempotent(t *testing.T) {
	s := NewDecisionService(decisionTestDB(t))
	past := time.Now().Add(-time.Minute)
	d := createDecision(t, s, &past, decision.TimeoutTransfer)
	got, err := s.ApplyTimeout("t1", d.ID, "scheduler")
	require.NoError(t, err)
	require.Equal(t, decision.StatusEscalated, got.Status)
	got, err = s.ApplyTimeout("t1", d.ID, "scheduler")
	require.NoError(t, err)
	require.Equal(t, decision.StatusEscalated, got.Status)
	var count int64
	s.db.Model(&decision.Escalation{}).Where("decision_id=?", d.ID).Count(&count)
	require.EqualValues(t, 1, count)
}

func TestDecisionRejectsNonElectorAndDuplicateVote(t *testing.T) {
	s := NewDecisionService(decisionTestDB(t))
	d := createDecision(t, s, nil, decision.TimeoutNone)
	_, err := s.CastVote("t1", d.ID, 99, decision.VoteApprove, "", "x")
	require.ErrorContains(t, err, "frozen electorate")
	_, err = s.CastVote("t1", d.ID, 1, decision.VoteApprove, "", "x")
	require.NoError(t, err)
	_, err = s.CastVote("t1", d.ID, 1, decision.VoteReject, "", "x")
	require.ErrorIs(t, err, ErrCollaborationConflict)
}

func TestDecisionFailureTransfersWithAudit(t *testing.T) {
	s := NewDecisionService(decisionTestDB(t))
	d := createDecision(t, s, nil, decision.TimeoutNone)
	got, err := s.EscalateFailure("t1", d.ID, decision.TimeoutTransfer, 2, "review step failed", "workflow:w1")
	require.NoError(t, err)
	require.Equal(t, decision.StatusEscalated, got.Status)
	var e decision.Escalation
	require.NoError(t, s.db.Where("decision_id=?", d.ID).First(&e).Error)
	require.EqualValues(t, 2, e.TargetAgentID)
	require.Equal(t, "review step failed", e.Reason)
	audits, err := s.Audits("t1", d.ID)
	require.NoError(t, err)
	require.Equal(t, "failure_escalated", audits[len(audits)-1].Action)
}
