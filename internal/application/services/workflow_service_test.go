package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/decision"
	"control-panel/internal/domain/run"
	"control-panel/internal/domain/workflow"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type workflowDispatchCall struct{ agent, prompt string }
type recordingWorkflowDispatcher struct {
	mu    sync.Mutex
	calls []workflowDispatchCall
	wake  chan struct{}
}

func (d *recordingWorkflowDispatcher) Dispatch(_ context.Context, _ string, agent, prompt string) error {
	d.mu.Lock()
	d.calls = append(d.calls, workflowDispatchCall{agent, prompt})
	d.mu.Unlock()
	select {
	case d.wake <- struct{}{}:
	default:
	}
	return nil
}
func (d *recordingWorkflowDispatcher) wait(t *testing.T, n int) []workflowDispatchCall {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		d.mu.Lock()
		out := append([]workflowDispatchCall(nil), d.calls...)
		d.mu.Unlock()
		if len(out) >= n {
			return out
		}
		select {
		case <-d.wake:
		case <-deadline:
			t.Fatalf("wanted %d dispatches, got %d", n, len(out))
		}
	}
}

func workflowTestService(t *testing.T) (*WorkflowService, *gorm.DB) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&run.Run{}, &agent.AgentConfig{}, &collaboration.Group{}, &collaboration.GroupMember{}, &decision.Decision{}, &decision.Elector{}, &decision.Vote{}, &decision.Result{}, &decision.Audit{}, &decision.Escalation{}, &workflow.Definition{}, &workflow.Version{}, &workflow.Step{}, &workflow.Transition{}, &workflow.Execution{}, &workflow.StepRun{}, &workflow.StepDispatch{}, &workflow.StepReceipt{}, &workflow.Approval{}, &workflow.ApprovalDecision{}, &workflow.Audit{}); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"agent-a", "agent-b", "agent-c"} {
		if e := db.Create(&agent.AgentConfig{TenantID: "t1", Name: name, SystemPrompt: "", ContentHash: name}).Error; e != nil {
			t.Fatal(e)
		}
	}
	return NewWorkflowService(db), db
}
func createPublishedWorkflow(t *testing.T, s *WorkflowService, tenant string, steps []WorkflowStepInput) *workflow.Version {
	t.Helper()
	for i := range steps {
		if steps[i].ActorType == "" {
			steps[i].ActorType = "agent"
		}
		if steps[i].ActorRef == "" {
			steps[i].ActorRef = "agent-a"
		}
	}
	d, e := s.CreateDefinition(tenant, "review", "", "owner")
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.CreateVersion(tenant, d.ID, CreateWorkflowVersionInput{Steps: steps, CreatedBy: "owner"})
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.PublishVersion(tenant, v.ID)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestWorkflowSerialAndParallelAdvance(t *testing.T) {
	s, _ := workflowTestService(t)
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "draft", Name: "Draft", Type: "task"}, {Key: "finance", Name: "Finance", Type: "task", DependsOn: []string{"draft"}}, {Key: "legal", Name: "Legal", Type: "task", DependsOn: []string{"draft"}}, {Key: "publish", Name: "Publish", Type: "handoff", DependsOn: []string{"finance", "legal"}}})
	ex, e := s.StartExecution("t1", v.ID, "", map[string]any{"title": "x"}, "start-1", "u")
	if e != nil {
		t.Fatal(e)
	}
	by := map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	if by["draft"].Status != workflow.StepRunning || by["finance"].Status != workflow.StepPending {
		t.Fatalf("unexpected initial status: %+v", by)
	}
	ex, e = s.CompleteStep("t1", by["draft"].ID, map[string]any{"ok": true}, "done-1", "agent-a")
	if e != nil {
		t.Fatal(e)
	}
	by = map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	if by["finance"].Status != workflow.StepRunning || by["legal"].Status != workflow.StepRunning || by["publish"].Status != workflow.StepPending {
		t.Fatalf("parallel fanout failed: %+v", by)
	}
	ex, _ = s.CompleteStep("t1", by["finance"].ID, nil, "done-2", "agent-a")
	by = map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	if by["publish"].Status != workflow.StepPending {
		t.Fatal("join opened before all dependencies")
	}
	ex, _ = s.CompleteStep("t1", by["legal"].ID, nil, "done-3", "agent-a")
	by = map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	if by["publish"].Status != workflow.StepRunning {
		t.Fatal("join did not open")
	}
	ex, _ = s.CompleteStep("t1", by["publish"].ID, nil, "done-4", "agent-a")
	if ex.Status != workflow.ExecutionCompleted {
		t.Fatalf("got %s", ex.Status)
	}
}

func TestApprovalFreezesApproversAndSupportsConditionalApproval(t *testing.T) {
	s, _ := workflowTestService(t)
	var agents []agent.AgentConfig
	s.db.Where("tenant_id=?", "t1").Find(&agents)
	s.db.Create(&collaboration.Group{ID: "g1", TenantID: "t1", Name: "reviewers", Visibility: "private"})
	for _, row := range agents {
		s.db.Create(&collaboration.GroupMember{TenantID: "t1", GroupID: "g1", AgentID: row.ID, Role: "member", JoinedAt: time.Now()})
	}
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "approve", Name: "Approve", Type: "approval", ActorType: "role", ActorRef: "member", Config: map[string]any{"groupId": "g1", "policy": "quorum", "quorum": 2}}})
	ex, e := s.StartExecution("t1", v.ID, "", nil, "start-1", "u")
	if e != nil {
		t.Fatal(e)
	}
	if len(ex.Approvals) != 1 {
		t.Fatal("approval missing")
	}
	a := ex.Approvals[0]
	if len(a.ApproverSnapshot) != 3 {
		t.Fatalf("snapshot=%v", a.ApproverSnapshot)
	}
	ex, e = s.DecideApproval("t1", a.ID, "agent-a", workflow.DecisionConditional, "only after review", map[string]any{"review": true}, "vote-a")
	if e != nil {
		t.Fatal(e)
	}
	if ex.Status != workflow.ExecutionRunning {
		t.Fatal("quorum resolved too early")
	}
	ex, e = s.DecideApproval("t1", a.ID, "agent-b", workflow.DecisionApprove, "", nil, "vote-b")
	if e != nil {
		t.Fatal(e)
	}
	if ex.Status != workflow.ExecutionCompleted {
		t.Fatalf("got %s", ex.Status)
	}
	if _, e = s.DecideApproval("other", a.ID, "agent-c", workflow.DecisionApprove, "", nil, "vote-c"); e != ErrWorkflowNotFound {
		t.Fatalf("tenant isolation: %v", e)
	}
}

func TestTimeoutEscalationIsAuditedAndCanResolve(t *testing.T) {
	s, db := workflowTestService(t)
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "work", Name: "Work", Type: "task", TimeoutSeconds: 1, EscalationStepKey: "escalate"}, {Key: "escalate", Name: "Escalate", Type: "task", DependsOn: []string{"work"}}})
	ex, e := s.StartExecution("t1", v.ID, "", nil, "start-1", "u")
	if e != nil {
		t.Fatal(e)
	}
	var work workflow.StepRun
	db.Where("execution_id=? AND step_key=?", ex.ID, "work").First(&work)
	past := time.Now().UTC().Add(-time.Minute)
	db.Model(&work).Update("due_at", past)
	ex, e = s.ProcessTimeouts("t1", ex.ID, "scheduler", time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	by := map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	if by["work"].Status != workflow.StepTimedOut || by["escalate"].Status != workflow.StepRunning {
		t.Fatalf("bad escalation: %+v", by)
	}
	ex, e = s.CompleteStep("t1", by["escalate"].ID, nil, "escalated", "agent-a")
	if e != nil {
		t.Fatal(e)
	}
	if ex.Status != workflow.ExecutionCompleted {
		t.Fatalf("got %s", ex.Status)
	}
	audits, e := s.Audits("t1", ex.ID)
	if e != nil || len(audits) < 3 {
		t.Fatalf("audits=%d err=%v", len(audits), e)
	}
}

func TestWorkflowConditionalBranchSkipsUnmatchedPath(t *testing.T) {
	s, _ := workflowTestService(t)
	d, err := s.CreateDefinition("t1", "branch", "", "owner")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateVersion("t1", d.ID, CreateWorkflowVersionInput{Steps: []WorkflowStepInput{{Key: "check", Name: "Check", Type: "task", ActorRef: "agent-a"}, {Key: "yes", Name: "Yes", Type: "task", ActorRef: "agent-a", DependsOn: []string{"check"}}, {Key: "no", Name: "No", Type: "task", ActorRef: "agent-a", DependsOn: []string{"check"}}}, Transitions: []WorkflowTransitionInput{{FromStepKey: "check", ToStepKey: "yes", Condition: map[string]any{"equals": map[string]any{"approved": true}}}, {FromStepKey: "check", ToStepKey: "no", Condition: map[string]any{"equals": map[string]any{"approved": false}}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.PublishVersion("t1", v.ID)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := s.StartExecution("t1", v.ID, "", nil, "branch-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	var check workflow.StepRun
	for _, r := range ex.StepRuns {
		if r.StepKey == "check" {
			check = r
		}
	}
	ex, err = s.CompleteStep("t1", check.ID, map[string]any{"approved": true}, "branch-done", "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]string{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r.Status
	}
	if by["yes"] != workflow.StepRunning || by["no"] != workflow.StepSkipped {
		t.Fatalf("branch statuses: %+v", by)
	}
}

func TestWorkflowDispatchesAToParallelBAndCThenApproval(t *testing.T) {
	s, _ := workflowTestService(t)
	dispatcher := &recordingWorkflowDispatcher{wake: make(chan struct{}, 10)}
	s.SetDispatcher(dispatcher)
	var agents []agent.AgentConfig
	s.db.Where("tenant_id=?", "t1").Find(&agents)
	s.db.Create(&collaboration.Group{ID: "leaders", TenantID: "t1", Name: "leaders", Visibility: "private"})
	for _, a := range agents {
		s.db.Create(&collaboration.GroupMember{TenantID: "t1", GroupID: "leaders", AgentID: a.ID, Role: "leader", JoinedAt: time.Now()})
	}
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "a", Name: "A", Type: "task", ActorRef: "agent-a"}, {Key: "b", Name: "B", Type: "task", ActorRef: "agent-b", DependsOn: []string{"a"}}, {Key: "c", Name: "C", Type: "task", ActorRef: "agent-c", DependsOn: []string{"a"}}, {Key: "approve", Name: "Approve", Type: "approval", ActorType: "role", ActorRef: "leader", Config: map[string]any{"groupId": "leaders", "policy": "quorum", "quorum": 2}, DependsOn: []string{"b", "c"}}})
	ex, err := s.StartExecution("t1", v.ID, "", map[string]any{"case": "e2e"}, "e2e-start", "owner")
	if err != nil {
		t.Fatal(err)
	}
	calls := dispatcher.wait(t, 1)
	if calls[0].agent != "agent-a" || !strings.Contains(calls[0].prompt, "workflow_step_complete") {
		t.Fatalf("bad first dispatch: %+v", calls[0])
	}
	by := map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	ex, err = s.CompleteStep("t1", by["a"].ID, map[string]any{"done": true}, "e2e-a", "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	calls = dispatcher.wait(t, 3)
	seen := map[string]bool{}
	for _, c := range calls {
		seen[c.agent] = true
	}
	if !seen["agent-b"] || !seen["agent-c"] {
		t.Fatalf("parallel dispatch missing: %+v", calls)
	}
	by = map[string]workflow.StepRun{}
	for _, r := range ex.StepRuns {
		by[r.StepKey] = r
	}
	ex, err = s.CompleteStep("t1", by["b"].ID, nil, "e2e-b", "agent-b")
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.CompleteStep("t1", by["c"].ID, nil, "e2e-c", "agent-c")
	if err != nil {
		t.Fatal(err)
	}
	calls = dispatcher.wait(t, 6)
	approvalPrompt := false
	for _, c := range calls {
		if strings.Contains(c.prompt, "approval_id=") {
			approvalPrompt = true
		}
	}
	if !approvalPrompt {
		t.Fatal("approval dispatch omitted approval_id")
	}
	if len(ex.Approvals) != 1 {
		t.Fatal("approval missing")
	}
	approval := ex.Approvals[0]
	ex, err = s.DecideApproval("t1", approval.ID, "agent-a", workflow.DecisionApprove, "", nil, "e2e-vote-a")
	if err != nil {
		t.Fatal(err)
	}
	ex, err = s.DecideApproval("t1", approval.ID, "agent-b", workflow.DecisionApprove, "", nil, "e2e-vote-b")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != workflow.ExecutionCompleted {
		t.Fatalf("execution=%s", ex.Status)
	}
}

func TestWorkflowFailureConvergesAndIdempotencyBindsPayload(t *testing.T) {
	s, _ := workflowTestService(t)
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "a", Name: "A", Type: "task", ActorRef: "agent-a"}, {Key: "b", Name: "B", Type: "task", ActorRef: "agent-b", DependsOn: []string{"a"}}})
	ex, err := s.StartExecution("t1", v.ID, "", map[string]any{"x": 1}, "same", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartExecution("t1", v.ID, "", map[string]any{"x": 2}, "same", "owner"); err != ErrWorkflowConflict {
		t.Fatalf("expected key collision, got %v", err)
	}
	var a workflow.StepRun
	for _, r := range ex.StepRuns {
		if r.StepKey == "a" {
			a = r
		}
	}
	ex, err = s.FailStep("t1", a.ID, "boom", "fail-a", "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Status != workflow.ExecutionFailed {
		t.Fatalf("did not converge: %s", ex.Status)
	}
	for _, r := range ex.StepRuns {
		if r.StepKey == "b" && r.Status != workflow.StepSkipped {
			t.Fatalf("downstream=%s", r.Status)
		}
	}
	same, err := s.FailStep("t1", a.ID, "boom", "fail-a", "agent-a")
	if err != nil || same.ID != ex.ID {
		t.Fatalf("idempotent retry: %v", err)
	}
	if _, err = s.FailStep("t1", a.ID, "different", "fail-a", "agent-a"); err != ErrWorkflowConflict {
		t.Fatalf("payload collision=%v", err)
	}
}

func TestDecisionElectorsAreDispatchedAndCloseDispatchesNextStep(t *testing.T) {
	s, _ := workflowTestService(t)
	dispatcher := &recordingWorkflowDispatcher{wake: make(chan struct{}, 10)}
	s.SetDispatcher(dispatcher)
	ds := NewDecisionService(s.db)
	ds.SetWorkflowService(s)
	var agents []agent.AgentConfig
	s.db.Where("tenant_id=?", "t1").Order("id").Find(&agents)
	s.db.Create(&collaboration.Group{ID: "board", TenantID: "t1", Name: "board", Visibility: "private"})
	for _, a := range agents[:2] {
		s.db.Create(&collaboration.GroupMember{TenantID: "t1", GroupID: "board", AgentID: a.ID, Role: "member", JoinedAt: time.Now()})
	}
	electors := []any{map[string]any{"agentId": float64(agents[0].ID), "weight": float64(1)}, map[string]any{"agentId": float64(agents[1].ID), "weight": float64(1)}}
	v := createPublishedWorkflow(t, s, "t1", []WorkflowStepInput{{Key: "prepare", Name: "Prepare", Type: "task", ActorRef: "agent-a"}, {Key: "vote", Name: "Vote", Type: "decision", DependsOn: []string{"prepare"}, Config: map[string]any{"groupId": "board", "electors": electors, "quorumPercent": 50, "approvalPercent": 50}}, {Key: "execute", Name: "Execute", Type: "task", ActorRef: "agent-c", DependsOn: []string{"vote"}}})
	ex, err := s.StartExecution("t1", v.ID, "", nil, "decision-e2e", "owner")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.wait(t, 1)
	var prepare workflow.StepRun
	for _, r := range ex.StepRuns {
		if r.StepKey == "prepare" {
			prepare = r
		}
	}
	ex, err = s.CompleteStep("t1", prepare.ID, nil, "prepare-done", "agent-a")
	if err != nil {
		t.Fatal(err)
	}
	calls := dispatcher.wait(t, 3)
	decisionPrompts := 0
	for _, c := range calls {
		if strings.Contains(c.prompt, "decision_vote") && strings.Contains(c.prompt, "决策 ID") {
			decisionPrompts++
		}
	}
	if decisionPrompts != 2 {
		t.Fatalf("decision prompts=%d calls=%+v", decisionPrompts, calls)
	}
	var d decision.Decision
	if err := s.db.Where("tenant_id=? AND workflow_run_id=?", "t1", ex.ID).First(&d).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = ds.CastVote("t1", d.ID, agents[0].ID, decision.VoteApprove, "ok", "agent-a"); err != nil {
		t.Fatal(err)
	}
	if _, err = ds.CastVote("t1", d.ID, agents[1].ID, decision.VoteApprove, "ok", "agent-b"); err != nil {
		t.Fatal(err)
	}
	if _, err = ds.Close("t1", d.ID, "system"); err != nil {
		t.Fatal(err)
	}
	calls = dispatcher.wait(t, 4)
	foundNext := false
	for _, c := range calls {
		if c.agent == "agent-c" && strings.Contains(c.prompt, "workflow_step_complete") {
			foundNext = true
		}
	}
	if !foundNext {
		t.Fatalf("next step was not dispatched: %+v", calls)
	}
}
