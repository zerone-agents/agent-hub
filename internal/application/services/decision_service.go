package services

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/decision"
	"control-panel/internal/domain/workflow"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDecisionNotFound = errors.New("decision not found")
	ErrDecisionClosed   = errors.New("decision is not open")
)

type DecisionService struct {
	db       *gorm.DB
	workflow *WorkflowService
}

func NewDecisionService(db *gorm.DB) *DecisionService            { return &DecisionService{db: db} }
func (s *DecisionService) SetWorkflowService(w *WorkflowService) { s.workflow = w }

type ElectorInput struct {
	AgentID uint64 `json:"agentId"`
	Weight  int    `json:"weight"`
	CanVeto bool   `json:"canVeto"`
}
type CreateDecisionInput struct {
	GroupID, WorkflowRunID, Title, Description string
	QuorumPercent, ApprovalPercent             int
	TimeoutAction                              string
	EscalateAgentID                            uint64
	DeadlineAt                                 *time.Time
	Electors                                   []ElectorInput
	WorkflowStepRunID                          string
}

func decisionAudit(tx *gorm.DB, tenantID, id, actor, action string, payload map[string]any) error {
	return tx.Create(&decision.Audit{ID: uuid.NewString(), TenantID: tenantID, DecisionID: id, ActorID: actor, Action: action, Payload: payload}).Error
}

func (s *DecisionService) Create(tenantID string, in CreateDecisionInput, actor string) (*decision.Decision, error) {
	var row *decision.Decision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = createDecisionTx(tx, tenantID, in, actor)
		return err
	})
	return row, err
}

func validateDecisionAgent(tx *gorm.DB, tenantID string, agentID uint64) error {
	if agentID == 0 {
		return fmt.Errorf("agent is required")
	}
	var count int64
	if err := tx.Model(&agent.AgentConfig{}).Where("tenant_id=? AND id=?", tenantID, agentID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("agent %d not found in tenant", agentID)
	}
	return nil
}

func createDecisionTx(tx *gorm.DB, tenantID string, in CreateDecisionInput, actor string) (*decision.Decision, error) {
	in.Title, in.TimeoutAction = strings.TrimSpace(in.Title), strings.ToLower(strings.TrimSpace(in.TimeoutAction))
	if in.TimeoutAction == "" {
		in.TimeoutAction = decision.TimeoutNone
	}
	if tenantID == "" || in.GroupID == "" || in.Title == "" || len(in.Electors) == 0 {
		return nil, fmt.Errorf("groupId, title and electors are required")
	}
	if in.QuorumPercent < 1 || in.QuorumPercent > 100 || in.ApprovalPercent < 1 || in.ApprovalPercent > 100 {
		return nil, fmt.Errorf("percentages must be between 1 and 100")
	}
	if in.TimeoutAction != decision.TimeoutNone && in.TimeoutAction != decision.TimeoutEscalate && in.TimeoutAction != decision.TimeoutTransfer {
		return nil, fmt.Errorf("invalid timeoutAction")
	}
	if in.TimeoutAction != decision.TimeoutNone && in.EscalateAgentID == 0 {
		return nil, fmt.Errorf("escalateAgentId is required for timeout action")
	}
	if in.WorkflowRunID != "" {
		var count int64
		if err := tx.Model(&workflow.Execution{}).Where("tenant_id=? AND id=?", tenantID, in.WorkflowRunID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("workflow run not found in tenant")
		}
	}
	if in.WorkflowStepRunID != "" {
		var count int64
		if err := tx.Model(&workflow.StepRun{}).Where("tenant_id=? AND execution_id=? AND id=?", tenantID, in.WorkflowRunID, in.WorkflowStepRunID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("workflow step run not found in tenant")
		}
		if err := tx.Model(&decision.Decision{}).Where("tenant_id=? AND workflow_step_run_id=?", tenantID, in.WorkflowStepRunID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, ErrCollaborationConflict
		}
	}
	if in.TimeoutAction != decision.TimeoutNone {
		if err := validateDecisionAgent(tx, tenantID, in.EscalateAgentID); err != nil {
			return nil, err
		}
	}
	row := &decision.Decision{ID: uuid.NewString(), TenantID: tenantID, GroupID: in.GroupID, WorkflowRunID: in.WorkflowRunID, WorkflowStepRunID: in.WorkflowStepRunID, Title: in.Title, Description: strings.TrimSpace(in.Description), Status: decision.StatusOpen, QuorumPercent: in.QuorumPercent, ApprovalPercent: in.ApprovalPercent, TimeoutAction: in.TimeoutAction, EscalateAgentID: in.EscalateAgentID, DeadlineAt: in.DeadlineAt, CreatedBy: actor}
	err := func() error {
		var count int64
		if err := tx.Model(&collaboration.Group{}).Where("tenant_id=? AND id=?", tenantID, in.GroupID).Count(&count).Error; err != nil || count == 0 {
			return ErrCollaborationNotFound
		}
		seen := map[uint64]bool{}
		for _, e := range in.Electors {
			if e.AgentID == 0 || e.Weight <= 0 || seen[e.AgentID] {
				return fmt.Errorf("electors must be unique members with positive weight")
			}
			seen[e.AgentID] = true
			var member collaboration.GroupMember
			if err := tx.Where("tenant_id=? AND group_id=? AND agent_id=?", tenantID, in.GroupID, e.AgentID).First(&member).Error; err != nil {
				return fmt.Errorf("agent %d is not a group member", e.AgentID)
			}
			row.Electorate = append(row.Electorate, decision.Elector{TenantID: tenantID, DecisionID: row.ID, AgentID: e.AgentID, Role: member.Role, Weight: e.Weight, CanVeto: e.CanVeto})
		}
		if err := tx.Omit("Electorate", "Votes", "Result").Create(row).Error; err != nil {
			return err
		}
		if err := tx.Create(&row.Electorate).Error; err != nil {
			return err
		}
		return decisionAudit(tx, tenantID, row.ID, actor, "decision_opened", map[string]any{"electorate": row.Electorate, "quorumPercent": row.QuorumPercent, "approvalPercent": row.ApprovalPercent})
	}()
	return row, err
}

func (s *DecisionService) Get(tenantID, id string) (*decision.Decision, error) {
	var row decision.Decision
	err := s.db.Preload("Electorate").Preload("Votes").Preload("Result").Where("tenant_id=? AND id=?", tenantID, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDecisionNotFound
	}
	return &row, err
}
func (s *DecisionService) List(tenantID, groupID string) ([]decision.Decision, error) {
	q := s.db.Where("tenant_id=?", tenantID)
	if groupID != "" {
		q = q.Where("group_id=?", groupID)
	}
	var rows []decision.Decision
	return rows, q.Order("created_at DESC").Find(&rows).Error
}

func (s *DecisionService) CastVote(tenantID, id string, agentID uint64, choice, reason, actor string) (*decision.Vote, error) {
	choice = strings.ToLower(strings.TrimSpace(choice))
	if choice != decision.VoteApprove && choice != decision.VoteReject && choice != decision.VoteAbstain {
		return nil, fmt.Errorf("invalid vote choice")
	}
	var out decision.Vote
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var d decision.Decision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&d).Error; err != nil {
			return ErrDecisionNotFound
		}
		if d.Status != decision.StatusOpen {
			return ErrDecisionClosed
		}
		if d.DeadlineAt != nil && !time.Now().UTC().Before(*d.DeadlineAt) {
			return fmt.Errorf("decision deadline has passed")
		}
		var e decision.Elector
		if err := tx.Where("tenant_id=? AND decision_id=? AND agent_id=?", tenantID, id, agentID).First(&e).Error; err != nil {
			return fmt.Errorf("agent is not in the frozen electorate")
		}
		out = decision.Vote{ID: uuid.NewString(), TenantID: tenantID, DecisionID: id, AgentID: agentID, Choice: choice, Weight: e.Weight, VetoApplied: e.CanVeto && choice == decision.VoteReject, Reason: strings.TrimSpace(reason)}
		if err := tx.Create(&out).Error; err != nil {
			if isDuplicate(err) {
				return ErrCollaborationConflict
			}
			return err
		}
		return decisionAudit(tx, tenantID, id, actor, "vote_cast", map[string]any{"voteId": out.ID, "agentId": agentID, "choice": choice, "weight": e.Weight, "vetoApplied": out.VetoApplied, "reason": out.Reason})
	})
	return &out, err
}

// CastAgentVote derives voter identity exclusively from the authenticated
// runtime. MCP callers cannot nominate another elector in their arguments.
func (s *DecisionService) CastAgentVote(tenantID string, source *agent.AgentConfig, id, choice, reason string) (*decision.Vote, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, fmt.Errorf("runtime agent identity is invalid")
	}
	return s.CastVote(tenantID, id, source.ID, choice, reason, source.Name)
}

func computeDecision(electors []decision.Elector, votes []decision.Vote, d *decision.Decision) *decision.Result {
	r := &decision.Result{ID: uuid.NewString(), TenantID: d.TenantID, DecisionID: d.ID, ComputedAt: time.Now().UTC()}
	for _, e := range electors {
		r.TotalWeight += e.Weight
	}
	for _, v := range votes {
		r.ParticipatingWeight += v.Weight
		switch v.Choice {
		case decision.VoteApprove:
			r.ApproveWeight += v.Weight
		case decision.VoteReject:
			r.RejectWeight += v.Weight
		case decision.VoteAbstain:
			r.AbstainWeight += v.Weight
		}
		r.Vetoed = r.Vetoed || v.VetoApplied
	}
	r.QuorumMet = r.ParticipatingWeight*100 >= r.TotalWeight*d.QuorumPercent
	switch {
	case !r.QuorumMet:
		r.Outcome = decision.StatusNoQuorum
	case r.Vetoed:
		r.Outcome = decision.StatusRejected
	case r.ApproveWeight*100 >= r.ParticipatingWeight*d.ApprovalPercent:
		r.Outcome = decision.StatusPassed
	default:
		r.Outcome = decision.StatusRejected
	}
	r.Explanation = fmt.Sprintf("participation %d/%d weight; approve=%d reject=%d abstain=%d; quorum=%t veto=%t", r.ParticipatingWeight, r.TotalWeight, r.ApproveWeight, r.RejectWeight, r.AbstainWeight, r.QuorumMet, r.Vetoed)
	return r
}

func (s *DecisionService) Close(tenantID, id, actor string) (*decision.Decision, error) {
	var workflowRunID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var d decision.Decision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&d).Error; err != nil {
			return ErrDecisionNotFound
		}
		if d.Status != decision.StatusOpen {
			return ErrDecisionClosed
		}
		workflowRunID = d.WorkflowRunID
		var es []decision.Elector
		var vs []decision.Vote
		if err := tx.Where("tenant_id=? AND decision_id=?", tenantID, id).Find(&es).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id=? AND decision_id=?", tenantID, id).Find(&vs).Error; err != nil {
			return err
		}
		r := computeDecision(es, vs, &d)
		now := time.Now().UTC()
		d.Status = r.Outcome
		d.ClosedAt = &now
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		if err := tx.Save(&d).Error; err != nil {
			return err
		}
		if err := finalizeWorkflowDecisionTx(tx, &d, r, actor); err != nil {
			return err
		}
		return decisionAudit(tx, tenantID, id, actor, "decision_closed", map[string]any{"outcome": r.Outcome, "quorumMet": r.QuorumMet, "vetoed": r.Vetoed, "resultId": r.ID})
	})
	if err != nil {
		return nil, err
	}
	if workflowRunID != "" && s.workflow != nil {
		if err := s.workflow.ContinueAfterDecision(tenantID, workflowRunID); err != nil {
			return nil, err
		}
	}
	return s.Get(tenantID, id)
}

func finalizeWorkflowDecisionTx(tx *gorm.DB, d *decision.Decision, result *decision.Result, actor string) error {
	if d.WorkflowRunID == "" || d.WorkflowStepRunID == "" {
		return nil
	}
	var sr workflow.StepRun
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND execution_id=? AND id=?", d.TenantID, d.WorkflowRunID, d.WorkflowStepRunID).First(&sr).Error; err != nil {
		return fmt.Errorf("linked workflow step not found: %w", err)
	}
	if sr.Status != workflow.StepWaitingDecision {
		return ErrWorkflowConflict
	}
	now := time.Now().UTC()
	sr.CompletedAt = &now
	sr.Output = map[string]any{"decisionId": d.ID, "decisionOutcome": result.Outcome, "quorumMet": result.QuorumMet, "vetoed": result.Vetoed}
	if result.Outcome == decision.StatusPassed {
		sr.Status = workflow.StepCompleted
	} else {
		sr.Status = workflow.StepFailed
		sr.Error = "collective decision " + result.Outcome
	}
	if err := tx.Save(&sr).Error; err != nil {
		return err
	}
	if err := auditWorkflow(tx, d.TenantID, d.WorkflowRunID, actor, "collective_decision_"+result.Outcome, "decision", d.ID, nil, sr.Output); err != nil {
		return err
	}
	return advanceWorkflow(tx, d.TenantID, d.WorkflowRunID)
}

// ApplyTimeout is an idempotent scheduler primitive. A scheduler/workflow may call it after DeadlineAt.
func (s *DecisionService) ApplyTimeout(tenantID, id, actor string) (*decision.Decision, error) {
	var workflowRunID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var d decision.Decision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&d).Error; err != nil {
			return ErrDecisionNotFound
		}
		workflowRunID = d.WorkflowRunID
		if d.Status != decision.StatusOpen {
			return nil
		}
		if d.DeadlineAt == nil || time.Now().UTC().Before(*d.DeadlineAt) {
			return fmt.Errorf("decision is not due")
		}
		now := time.Now().UTC()
		d.ClosedAt = &now
		d.Status = decision.StatusTimedOut
		action := d.TimeoutAction
		if action == decision.TimeoutEscalate || action == decision.TimeoutTransfer {
			if err := validateDecisionAgent(tx, tenantID, d.EscalateAgentID); err != nil {
				return err
			}
			d.Status = decision.StatusEscalated
			if err := tx.Create(&decision.Escalation{ID: uuid.NewString(), TenantID: tenantID, DecisionID: id, Kind: action, TargetAgentID: d.EscalateAgentID, Reason: "decision deadline elapsed"}).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&d).Error; err != nil {
			return err
		}
		if err := finalizeWorkflowDecisionTimeoutTx(tx, &d, action, "collective_decision_timed_out", actor); err != nil {
			return err
		}
		return decisionAudit(tx, tenantID, id, actor, "decision_timed_out", map[string]any{"action": action, "targetAgentId": d.EscalateAgentID, "status": d.Status})
	})
	if err != nil {
		return nil, err
	}
	if workflowRunID != "" && s.workflow != nil {
		if err := s.workflow.ContinueAfterDecision(tenantID, workflowRunID); err != nil {
			return nil, err
		}
	}
	return s.Get(tenantID, id)
}

func finalizeWorkflowDecisionTimeoutTx(tx *gorm.DB, d *decision.Decision, action, auditAction, actor string) error {
	if d.WorkflowRunID == "" || d.WorkflowStepRunID == "" {
		return nil
	}
	var sr workflow.StepRun
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND execution_id=? AND id=?", d.TenantID, d.WorkflowRunID, d.WorkflowStepRunID).First(&sr).Error; err != nil {
		return err
	}
	if sr.Status != workflow.StepWaitingDecision {
		return ErrWorkflowConflict
	}
	var step workflow.Step
	if err := tx.Where("tenant_id=? AND id=?", d.TenantID, sr.StepID).First(&step).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	sr.CompletedAt = &now
	sr.Output = map[string]any{"decisionId": d.ID, "decisionOutcome": d.Status, "timeoutAction": action}
	sr.Status = workflow.StepTimedOut
	if step.EscalationStepKey == "" {
		sr.Status = workflow.StepFailed
		sr.Error = "collective decision timed out"
	}
	if err := tx.Save(&sr).Error; err != nil {
		return err
	}
	if step.EscalationStepKey != "" {
		if err := forceActivateStep(tx, d.TenantID, d.WorkflowRunID, step.EscalationStepKey); err != nil {
			return err
		}
	}
	if err := auditWorkflow(tx, d.TenantID, d.WorkflowRunID, actor, auditAction, "decision", d.ID, nil, sr.Output); err != nil {
		return err
	}
	return advanceWorkflow(tx, d.TenantID, d.WorkflowRunID)
}

// EscalateFailure is the generic workflow integration primitive for a failed
// step. Core records and routes the failure; extensions decide whether a
// personality or domain circumstance should invoke it.
func (s *DecisionService) EscalateFailure(tenantID, id, kind string, targetAgentID uint64, reason, actor string) (*decision.Decision, error) {
	kind, reason = strings.ToLower(strings.TrimSpace(kind)), strings.TrimSpace(reason)
	if kind != decision.TimeoutEscalate && kind != decision.TimeoutTransfer {
		return nil, fmt.Errorf("kind must be escalate or transfer")
	}
	if targetAgentID == 0 || reason == "" {
		return nil, fmt.Errorf("targetAgentId and reason are required")
	}
	var workflowRunID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := validateDecisionAgent(tx, tenantID, targetAgentID); err != nil {
			return err
		}
		var d decision.Decision
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&d).Error; err != nil {
			return ErrDecisionNotFound
		}
		workflowRunID = d.WorkflowRunID
		if d.Status != decision.StatusOpen {
			return ErrDecisionClosed
		}
		now := time.Now().UTC()
		d.Status, d.ClosedAt = decision.StatusEscalated, &now
		if err := tx.Create(&decision.Escalation{ID: uuid.NewString(), TenantID: tenantID, DecisionID: id, Kind: kind, TargetAgentID: targetAgentID, Reason: reason}).Error; err != nil {
			return err
		}
		if err := tx.Save(&d).Error; err != nil {
			return err
		}
		if err := finalizeWorkflowDecisionTimeoutTx(tx, &d, kind, "collective_decision_failure_escalated", actor); err != nil {
			return err
		}
		return decisionAudit(tx, tenantID, id, actor, "failure_escalated", map[string]any{"kind": kind, "targetAgentId": targetAgentID, "reason": reason})
	})
	if err != nil {
		return nil, err
	}
	if workflowRunID != "" && s.workflow != nil {
		if err := s.workflow.ContinueAfterDecision(tenantID, workflowRunID); err != nil {
			return nil, err
		}
	}
	return s.Get(tenantID, id)
}

func (s *DecisionService) Audits(tenantID, id string) ([]decision.Audit, error) {
	var rows []decision.Audit
	return rows, s.db.Where("tenant_id=? AND decision_id=?", tenantID, id).Order("created_at,id").Find(&rows).Error
}
