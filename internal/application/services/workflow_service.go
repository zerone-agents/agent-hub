package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/decision"
	"control-panel/internal/domain/run"
	"control-panel/internal/domain/usage"
	"control-panel/internal/domain/workflow"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrWorkflowNotFound = errors.New("workflow resource not found")
	ErrWorkflowConflict = errors.New("workflow resource conflict")
)

type WorkflowStepDispatcher interface {
	Dispatch(context.Context, string, string, string) error
}
type WorkflowAgentDispatcher struct{ runner AgentMessageRunner }

func NewWorkflowAgentDispatcher(runner AgentMessageRunner) *WorkflowAgentDispatcher {
	return &WorkflowAgentDispatcher{runner: runner}
}
func (d *WorkflowAgentDispatcher) Dispatch(ctx context.Context, tenantID, agentName, prompt string) error {
	_, err := d.runner.RunOneShot(ctx, tenantID, agentName, prompt)
	return err
}

type WorkflowService struct {
	db         *gorm.DB
	dispatcher WorkflowStepDispatcher
	// H6 persona hooks (WS6): nil packs leave workflow behavior unchanged.
	personaEmotion *EmotionService
	personaRelDyn  *RelationDynamicsService
	// H7 P1 persona gate: nil leaves hooks always on (测试基座可不接)；
	// 非 nil 时钩子生效前按扩展生命周期运行时查询，停用立即生效。
	personaGate *PersonaCapabilityGate
	// H7.5 usage hook: nil leaves behavior unchanged; Record never blocks.
	usage *UsageService
}

func NewWorkflowService(db *gorm.DB) *WorkflowService             { return &WorkflowService{db: db} }
func (s *WorkflowService) SetDispatcher(d WorkflowStepDispatcher) { s.dispatcher = d }

// SetUsageService 注入 H7.5 用量采集。advisory：Record 永不阻塞、永不
// panic，步骤完成路径不因埋点失败而受影响。
func (s *WorkflowService) SetUsageService(u *UsageService) { s.usage = u }

// SetPersonaHooks injects the H6 emotion and relation-dynamics packs for the
// step-completion influence hook. Emitted events are deterministic and
// idempotent; failures are logged and never break step completion.
func (s *WorkflowService) SetPersonaHooks(emotion *EmotionService, reldyn *RelationDynamicsService) {
	s.personaEmotion, s.personaRelDyn = emotion, reldyn
}

// SetPersonaCapabilityGate 接入 H7 能力门控（main.go 接线时调用）：
// 对应内置扩展被管理员停用后，情绪/动态关系钩子不再生效。
func (s *WorkflowService) SetPersonaCapabilityGate(g *PersonaCapabilityGate) { s.personaGate = g }

// personaPackEnabled 报告某人物能力包当前是否生效；未接 gate 时保持
// 原有"恒生效"行为（测试基座与旧接线兼容）。
func (s *WorkflowService) personaPackEnabled(tenantID, pack string) bool {
	if s.personaGate == nil {
		return true
	}
	return s.personaGate.Enabled(tenantID, pack)
}

type WorkflowStepInput struct {
	Key                 string         `json:"key"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	ActorType           string         `json:"actorType"`
	ActorRef            string         `json:"actorRef"`
	DependsOn           []string       `json:"dependsOn"`
	Config              map[string]any `json:"config"`
	TimeoutSeconds      int            `json:"timeoutSeconds"`
	MaxRetries          int            `json:"maxRetries"`
	EscalationStepKey   string         `json:"escalationStepKey"`
	CompensationStepKey string         `json:"compensationStepKey"`
}
type WorkflowTransitionInput struct {
	FromStepKey string         `json:"fromStepKey"`
	ToStepKey   string         `json:"toStepKey"`
	Condition   map[string]any `json:"condition"`
	Priority    int            `json:"priority"`
}
type CreateWorkflowVersionInput struct {
	InputSchema  map[string]any            `json:"inputSchema"`
	OutputSchema map[string]any            `json:"outputSchema"`
	Steps        []WorkflowStepInput       `json:"steps"`
	Transitions  []WorkflowTransitionInput `json:"transitions"`
	CreatedBy    string                    `json:"-"`
}

func (s *WorkflowService) CreateDefinition(tenantID, name, description, actor string) (*workflow.Definition, error) {
	name = strings.TrimSpace(name)
	if tenantID == "" || name == "" {
		return nil, fmt.Errorf("name is required")
	}
	row := &workflow.Definition{ID: uuid.NewString(), TenantID: tenantID, Name: name, Description: strings.TrimSpace(description), CreatedBy: actor}
	if err := s.db.Create(row).Error; err != nil {
		if isDuplicate(err) {
			return nil, ErrWorkflowConflict
		}
		return nil, err
	}
	return row, nil
}
func (s *WorkflowService) Definitions(tenantID string) ([]workflow.Definition, error) {
	var x []workflow.Definition
	return x, s.db.Where("tenant_id=?", tenantID).Order("created_at desc").Find(&x).Error
}
func (s *WorkflowService) Definition(tenantID, id string) (*workflow.Definition, []workflow.Version, error) {
	var d workflow.Definition
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).First(&d).Error; err != nil {
		return nil, nil, ErrWorkflowNotFound
	}
	var v []workflow.Version
	err := s.db.Where("tenant_id=? AND workflow_id=?", tenantID, id).Order("version desc").Preload("Steps").Preload("Transitions").Find(&v).Error
	return &d, v, err
}

func validateWorkflowSteps(steps []WorkflowStepInput, transitions []WorkflowTransitionInput) error {
	if len(steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	keys := map[string]bool{}
	for _, x := range steps {
		x.Key = strings.TrimSpace(x.Key)
		if x.Key == "" || x.Name == "" || keys[x.Key] {
			return fmt.Errorf("step keys and names must be unique and non-empty")
		}
		keys[x.Key] = true
		if x.Type != "task" && x.Type != "handoff" && x.Type != "approval" && x.Type != "decision" {
			return fmt.Errorf("invalid step type %q", x.Type)
		}
		if x.ActorType != "" && x.ActorType != "agent" && x.ActorType != "role" && x.ActorType != "group" {
			return fmt.Errorf("invalid actor type %q", x.ActorType)
		}
	}
	for _, x := range steps {
		for _, d := range x.DependsOn {
			if !keys[d] || d == x.Key {
				return fmt.Errorf("invalid dependency %q", d)
			}
		}
		if x.EscalationStepKey != "" && !keys[x.EscalationStepKey] {
			return fmt.Errorf("invalid escalation step")
		}
		if x.CompensationStepKey != "" && !keys[x.CompensationStepKey] {
			return fmt.Errorf("invalid compensation step")
		}
	}
	for _, t := range transitions {
		if !keys[t.FromStepKey] || !keys[t.ToStepKey] || t.FromStepKey == t.ToStepKey {
			return fmt.Errorf("invalid transition")
		}
	}
	// dependency graph cycle check
	state := map[string]int{}
	deps := map[string][]string{}
	for _, x := range steps {
		deps[x.Key] = x.DependsOn
	}
	var visit func(string) error
	visit = func(k string) error {
		if state[k] == 1 {
			return fmt.Errorf("workflow dependency cycle")
		}
		if state[k] == 2 {
			return nil
		}
		state[k] = 1
		for _, d := range deps[k] {
			if err := visit(d); err != nil {
				return err
			}
		}
		state[k] = 2
		return nil
	}
	for k := range keys {
		if err := visit(k); err != nil {
			return err
		}
	}
	return nil
}
func (s *WorkflowService) CreateVersion(tenantID, workflowID string, in CreateWorkflowVersionInput) (*workflow.Version, error) {
	if err := validateWorkflowSteps(in.Steps, in.Transitions); err != nil {
		return nil, err
	}
	var out workflow.Version
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var d workflow.Definition
		if err := tx.Where("tenant_id=? AND id=?", tenantID, workflowID).First(&d).Error; err != nil {
			return ErrWorkflowNotFound
		}
		var max int
		tx.Model(&workflow.Version{}).Where("tenant_id=? AND workflow_id=?", tenantID, workflowID).Select("COALESCE(MAX(version),0)").Scan(&max)
		out = workflow.Version{ID: uuid.NewString(), TenantID: tenantID, WorkflowID: workflowID, Version: max + 1, Status: workflow.VersionDraft, InputSchema: in.InputSchema, OutputSchema: in.OutputSchema, CreatedBy: in.CreatedBy}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		for _, x := range in.Steps {
			row := workflow.Step{ID: uuid.NewString(), TenantID: tenantID, VersionID: out.ID, Key: strings.TrimSpace(x.Key), Name: strings.TrimSpace(x.Name), Type: x.Type, ActorType: x.ActorType, ActorRef: x.ActorRef, DependsOn: x.DependsOn, Config: x.Config, TimeoutSeconds: x.TimeoutSeconds, MaxRetries: x.MaxRetries, EscalationStepKey: x.EscalationStepKey, CompensationStepKey: x.CompensationStepKey}
			if row.ActorType == "" {
				row.ActorType = "agent"
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			out.Steps = append(out.Steps, row)
		}
		for _, x := range in.Transitions {
			row := workflow.Transition{TenantID: tenantID, VersionID: out.ID, FromStepKey: x.FromStepKey, ToStepKey: x.ToStepKey, Condition: x.Condition, Priority: x.Priority}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			out.Transitions = append(out.Transitions, row)
		}
		return nil
	})
	return &out, err
}
func (s *WorkflowService) PublishVersion(tenantID, id string) (*workflow.Version, error) {
	var v workflow.Version
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).Preload("Steps").First(&v).Error; err != nil {
		return nil, ErrWorkflowNotFound
	}
	if err := compileWorkflowSchema(v.InputSchema); err != nil {
		return nil, fmt.Errorf("invalid input schema: %w", err)
	}
	if err := compileWorkflowSchema(v.OutputSchema); err != nil {
		return nil, fmt.Errorf("invalid output schema: %w", err)
	}
	for _, step := range v.Steps {
		if step.Type == "decision" {
			continue // electorate is validated and frozen when the execution starts
		}
		names, err := s.resolveAssignees(s.db, tenantID, step)
		if err != nil {
			return nil, err
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("step %q resolves to no agents", step.Key)
		}
		if schema, ok := step.Config["outputSchema"].(map[string]any); ok {
			if err := compileWorkflowSchema(schema); err != nil {
				return nil, fmt.Errorf("step %q output schema: %w", step.Key, err)
			}
		}
	}
	now := time.Now().UTC()
	res := s.db.Model(&v).Where("tenant_id=? AND id=? AND status=?", tenantID, id, workflow.VersionDraft).Updates(map[string]any{"status": workflow.VersionPublished, "published_at": now})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrWorkflowConflict
	}
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, id).Preload("Steps").Preload("Transitions").First(&v).Error; err != nil {
		return nil, err
	}
	return &v, nil
}
func compileWorkflowSchema(doc map[string]any) error {
	if len(doc) == 0 {
		return nil
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("workflow-schema.json", doc); err != nil {
		return err
	}
	_, err := c.Compile("workflow-schema.json")
	return err
}

func (s *WorkflowService) resolveAssignees(db *gorm.DB, tenantID string, step workflow.Step) ([]string, error) {
	kind, ref := step.ActorType, strings.TrimSpace(step.ActorRef)
	if kind == "" {
		kind = "agent"
	}
	var names []string
	switch kind {
	case "agent":
		if ref == "" {
			return nil, fmt.Errorf("step %q requires actorRef", step.Key)
		}
		var a agent.AgentConfig
		if err := db.Where("tenant_id=? AND name=?", tenantID, ref).First(&a).Error; err != nil {
			return nil, fmt.Errorf("step %q agent not found", step.Key)
		}
		names = []string{a.Name}
	case "group", "role":
		groupID := ref
		role := ""
		if kind == "role" {
			role = ref
			if v, ok := step.Config["groupId"].(string); ok {
				groupID = v
			}
			if groupID == "" {
				return nil, fmt.Errorf("step %q role requires config.groupId", step.Key)
			}
		}
		q := db.Table("collaboration_group_members gm").Select("a.name").Joins("JOIN agents a ON a.id=gm.agent_id AND a.tenant_id=gm.tenant_id").Where("gm.tenant_id=? AND gm.group_id=?", tenantID, groupID)
		if role != "" {
			q = q.Where("gm.role=?", role)
		}
		if err := q.Order("gm.agent_id").Scan(&names).Error; err != nil {
			return nil, err
		}
	}
	return names, nil
}

func (s *WorkflowService) StartExecution(tenantID, versionID, runID string, input map[string]any, idempotencyKey, actor string) (*workflow.Execution, error) {
	if idempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	fingerprint := requestFingerprint(versionID, runID, input)
	var existing workflow.Execution
	if err := s.db.Where("tenant_id=? AND idempotency_key=?", tenantID, idempotencyKey).First(&existing).Error; err == nil {
		if existing.RequestHash != fingerprint {
			return nil, ErrWorkflowConflict
		}
		return s.Execution(tenantID, existing.ID)
	}
	var out workflow.Execution
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var v workflow.Version
		if err := tx.Where("tenant_id=? AND id=? AND status=?", tenantID, versionID, workflow.VersionPublished).Preload("Steps").First(&v).Error; err != nil {
			return ErrWorkflowNotFound
		}
		if len(v.InputSchema) > 0 {
			if err := validateJSONSchema(v.InputSchema, input); err != nil {
				return fmt.Errorf("workflow input: %w", err)
			}
		}
		if runID != "" {
			var n int64
			if err := tx.Model(&run.Run{}).Where("tenant_id=? AND id=?", tenantID, runID).Count(&n).Error; err != nil || n == 0 {
				return fmt.Errorf("run not found")
			}
		}
		out = workflow.Execution{ID: uuid.NewString(), TenantID: tenantID, VersionID: versionID, RunID: runID, Status: workflow.ExecutionRunning, Input: input, IdempotencyKey: idempotencyKey, RequestHash: fingerprint, StartedBy: actor, StartedAt: time.Now().UTC()}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		for _, step := range v.Steps {
			var assignees []string
			if step.Type != "decision" {
				var err error
				assignees, err = s.resolveAssignees(tx, tenantID, step)
				if err != nil || len(assignees) == 0 {
					return fmt.Errorf("step %q has no assignee", step.Key)
				}
			}
			sr := workflow.StepRun{ID: uuid.NewString(), TenantID: tenantID, ExecutionID: out.ID, StepID: step.ID, StepKey: step.Key, Status: workflow.StepPending, Attempt: 1, Input: input, AssigneeSnapshot: assignees}
			if len(step.DependsOn) == 0 {
				activateStep(&sr, step)
			}
			if err := tx.Create(&sr).Error; err != nil {
				return err
			}
			if sr.Status != workflow.StepPending {
				if err := auditWorkflow(tx, tenantID, out.ID, "system", "step_activated", "step_run", sr.ID, nil, map[string]any{"status": sr.Status, "assignees": sr.AssigneeSnapshot}); err != nil {
					return err
				}
			}
			if sr.Status == workflow.StepWaitingApproval {
				if err := createApproval(tx, tenantID, out.ID, &sr, step); err != nil {
					return err
				}
			} else if sr.Status == workflow.StepWaitingDecision {
				if _, err := createWorkflowDecision(tx, tenantID, out.ID, &sr, step, actor); err != nil {
					return err
				}
			}
		}
		return auditWorkflow(tx, tenantID, out.ID, actor, "execution_started", "execution", out.ID, nil, map[string]any{"status": out.Status})
	})
	if err != nil {
		if isDuplicate(err) {
			return nil, ErrWorkflowConflict
		}
		return nil, err
	}
	ex, err := s.Execution(tenantID, out.ID)
	if err == nil {
		s.dispatchActive(ex)
	}
	return ex, err
}

func requestFingerprint(values ...any) string {
	b, _ := json.Marshal(values)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (s *WorkflowService) dispatchActive(ex *workflow.Execution) {
	if s.dispatcher == nil || ex == nil {
		return
	}
	for _, sr := range ex.StepRuns {
		if sr.Status != workflow.StepRunning && sr.Status != workflow.StepWaitingApproval && sr.Status != workflow.StepWaitingDecision {
			continue
		}
		recipients := append([]string(nil), sr.AssigneeSnapshot...)
		if sr.Status == workflow.StepWaitingDecision {
			if err := s.db.Table("collective_decision_electors e").Select("a.name").Joins("JOIN collective_decisions d ON d.id=e.decision_id AND d.tenant_id=e.tenant_id").Joins("JOIN agents a ON a.id=e.agent_id AND a.tenant_id=e.tenant_id").Where("e.tenant_id=? AND d.workflow_step_run_id=?", ex.TenantID, sr.ID).Order("e.agent_id").Scan(&recipients).Error; err != nil {
				continue
			}
		}
		for _, agentName := range recipients {
			d := workflow.StepDispatch{ID: uuid.NewString(), TenantID: ex.TenantID, StepRunID: sr.ID, AgentName: agentName, Attempt: sr.Attempt, Status: "queued"}
			if err := s.db.Create(&d).Error; err != nil {
				continue
			}
			prompt := fmt.Sprintf("你收到一个 Agent Hub 工作流步骤。执行实例：%s；步骤：%s；步骤运行 ID：%s。完成后调用 organization MCP 的 workflow_step_complete；无法完成则调用 workflow_step_fail。必须传 step_run_id=%s 和稳定的 idempotency_key。输入：%s", ex.ID, sr.StepKey, sr.ID, sr.ID, mustJSON(sr.Input))
			if sr.Status == workflow.StepWaitingApproval {
				var approval workflow.Approval
				if err := s.db.Where("tenant_id=? AND step_run_id=?", ex.TenantID, sr.ID).First(&approval).Error; err != nil {
					continue
				}
				prompt = fmt.Sprintf("你是工作流审批人。执行实例：%s；审批 ID：%s；步骤运行 ID：%s。请调用 approval_vote，并传 approval_id=%s。输入：%s", ex.ID, approval.ID, sr.ID, approval.ID, mustJSON(sr.Input))
			} else if sr.Status == workflow.StepWaitingDecision {
				var collective decision.Decision
				if err := s.db.Where("tenant_id=? AND workflow_step_run_id=?", ex.TenantID, sr.ID).First(&collective).Error; err != nil {
					continue
				}
				prompt = fmt.Sprintf("你是群体决策的冻结投票人。执行实例：%s；决策 ID：%s。请调用 decision_vote，并传 decision_id=%s、choice（approve/reject/abstain）及理由。", ex.ID, collective.ID, collective.ID)
			}
			go func(row workflow.StepDispatch, name, text string) {
				err := s.dispatcher.Dispatch(context.Background(), ex.TenantID, name, text)
				now := time.Now().UTC()
				updates := map[string]any{"status": "delivered", "completed_at": now}
				if err != nil {
					updates["status"] = "failed"
					updates["error"] = err.Error()
				}
				_ = s.db.Model(&workflow.StepDispatch{}).Where("tenant_id=? AND id=?", ex.TenantID, row.ID).Updates(updates).Error
			}(d, agentName, prompt)
		}
	}
}

// ContinueAfterDecision is called only after the decision transaction commits.
// The state machine already activated downstream steps in that transaction;
// this method performs the external dispatch side effect exactly once via the
// persistent StepDispatch uniqueness guard.
func (s *WorkflowService) ContinueAfterDecision(tenantID, executionID string) error {
	ex, err := s.Execution(tenantID, executionID)
	if err != nil {
		return err
	}
	s.dispatchActive(ex)
	return nil
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func activateStep(sr *workflow.StepRun, step workflow.Step) {
	now := time.Now().UTC()
	sr.StartedAt = &now
	if step.TimeoutSeconds > 0 {
		d := now.Add(time.Duration(step.TimeoutSeconds) * time.Second)
		sr.DueAt = &d
	}
	if step.Type == "approval" {
		sr.Status = workflow.StepWaitingApproval
	} else if step.Type == "decision" {
		sr.Status = workflow.StepWaitingDecision
	} else {
		sr.Status = workflow.StepRunning
	}
}

func electorInputs(v any) []ElectorInput {
	var out []ElectorInput
	rows, _ := v.([]any)
	for _, raw := range rows {
		m, _ := raw.(map[string]any)
		id := uint64(intValue(m["agentId"], 0))
		weight := intValue(m["weight"], 1)
		veto, _ := m["canVeto"].(bool)
		if id > 0 {
			out = append(out, ElectorInput{AgentID: id, Weight: weight, CanVeto: veto})
		}
	}
	return out
}

func createWorkflowDecision(tx *gorm.DB, tenantID, executionID string, sr *workflow.StepRun, step workflow.Step, actor string) (*decision.Decision, error) {
	groupID, _ := step.Config["groupId"].(string)
	title, _ := step.Config["title"].(string)
	if title == "" {
		title = step.Name
	}
	description, _ := step.Config["description"].(string)
	timeoutAction, _ := step.Config["timeoutAction"].(string)
	in := CreateDecisionInput{GroupID: groupID, WorkflowRunID: executionID, WorkflowStepRunID: sr.ID, Title: title, Description: description, QuorumPercent: intValue(step.Config["quorumPercent"], 50), ApprovalPercent: intValue(step.Config["approvalPercent"], 50), TimeoutAction: timeoutAction, EscalateAgentID: uint64(intValue(step.Config["escalateAgentId"], 0)), DeadlineAt: sr.DueAt, Electors: electorInputs(step.Config["electors"])}
	return createDecisionTx(tx, tenantID, in, actor)
}
func strSlice(v any) []string {
	var out []string
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
func intValue(v any, def int) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return def
}
func createApproval(tx *gorm.DB, tenantID, executionID string, sr *workflow.StepRun, step workflow.Step) error {
	policy := "all"
	if v, ok := step.Config["policy"].(string); ok && v != "" {
		policy = v
	}
	approvers := append([]string(nil), sr.AssigneeSnapshot...)
	if len(approvers) == 0 {
		return fmt.Errorf("approval step %q has no approvers", step.Key)
	}
	q := intValue(step.Config["quorum"], len(approvers))
	if q < 1 {
		q = 1
	}
	return tx.Create(&workflow.Approval{ID: uuid.NewString(), TenantID: tenantID, ExecutionID: executionID, StepRunID: sr.ID, Policy: policy, Quorum: q, Status: workflow.ApprovalPending, ApproverSnapshot: approvers}).Error
}
func auditWorkflow(tx *gorm.DB, t, e, a, action, rt, rid string, before, after map[string]any) error {
	return tx.Create(&workflow.Audit{ID: uuid.NewString(), TenantID: t, ExecutionID: e, ActorID: a, Action: action, ResourceType: rt, ResourceID: rid, Before: before, After: after}).Error
}

func (s *WorkflowService) Execution(tenantID, id string) (*workflow.Execution, error) {
	var x workflow.Execution
	err := s.db.Where("tenant_id=? AND id=?", tenantID, id).Preload("StepRuns").Preload("Approvals.Decisions").First(&x).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkflowNotFound
	}
	return &x, err
}
func (s *WorkflowService) Executions(tenantID, workflowID, status string, limit int) ([]workflow.Execution, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := s.db.Where("tenant_id=?", tenantID)
	if status != "" {
		q = q.Where("status=?", status)
	}
	if workflowID != "" {
		q = q.Where("version_id IN (?)", s.db.Model(&workflow.Version{}).Select("id").Where("tenant_id=? AND workflow_id=?", tenantID, workflowID))
	}
	var x []workflow.Execution
	return x, q.Order("started_at desc").Limit(limit).Preload("StepRuns").Find(&x).Error
}
func (s *WorkflowService) Audits(tenantID, executionID string) ([]workflow.Audit, error) {
	var x []workflow.Audit
	return x, s.db.Where("tenant_id=? AND execution_id=?", tenantID, executionID).Order("created_at,id").Find(&x).Error
}

func (s *WorkflowService) CompleteStep(tenantID, id string, output map[string]any, idem, actor string) (*workflow.Execution, error) {
	return s.finishStep(tenantID, id, output, "", idem, actor, true)
}
func (s *WorkflowService) FailStep(tenantID, id, errorText, idem, actor string) (*workflow.Execution, error) {
	return s.finishStep(tenantID, id, nil, errorText, idem, actor, false)
}
func (s *WorkflowService) finishStep(tenantID, id string, output map[string]any, errorText, idem, actor string, success bool) (*workflow.Execution, error) {
	if idem == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	action := "fail"
	if success {
		action = "complete"
	}
	fingerprint := requestFingerprint(id, action, output, errorText)
	var prior workflow.StepReceipt
	if err := s.db.Where("tenant_id=? AND idempotency_key=?", tenantID, idem).First(&prior).Error; err == nil {
		if prior.RequestHash != fingerprint {
			return nil, ErrWorkflowConflict
		}
		return s.Execution(tenantID, prior.ExecutionID)
	}
	var executionID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var sr workflow.StepRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&sr).Error; err != nil {
			return ErrWorkflowNotFound
		}
		executionID = sr.ExecutionID
		if sr.Status != workflow.StepRunning {
			return ErrWorkflowConflict
		}
		allowed := false
		for _, name := range sr.AssigneeSnapshot {
			if name == actor {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("actor is not assigned to this step")
		}
		var step workflow.Step
		if err := tx.Where("tenant_id=? AND id=?", tenantID, sr.StepID).First(&step).Error; err != nil {
			return err
		}
		if success {
			if schema, ok := step.Config["outputSchema"].(map[string]any); ok && len(schema) > 0 {
				if err := validateJSONSchema(schema, output); err != nil {
					return fmt.Errorf("step output: %w", err)
				}
			}
		}
		now := time.Now().UTC()
		before := map[string]any{"status": sr.Status}
		if success {
			sr.Status = workflow.StepCompleted
			sr.Output = output
			sr.CompletedAt = &now
		} else if sr.Attempt <= step.MaxRetries {
			sr.Attempt++
			sr.Error = errorText
			activateStep(&sr, step)
		} else {
			sr.Status = workflow.StepFailed
			sr.Error = errorText
			sr.CompletedAt = &now
		}
		if err := tx.Save(&sr).Error; err != nil {
			return err
		}
		if err := tx.Create(&workflow.StepReceipt{ID: uuid.NewString(), TenantID: tenantID, StepRunID: sr.ID, ActorID: actor, Action: action, RequestHash: fingerprint, IdempotencyKey: idem, ExecutionID: executionID}).Error; err != nil {
			return err
		}
		if sr.Status == workflow.StepFailed && step.CompensationStepKey != "" {
			if err := forceActivateStep(tx, tenantID, executionID, step.CompensationStepKey); err != nil {
				return err
			}
		}
		if err := auditWorkflow(tx, tenantID, executionID, actor, "step_"+sr.Status, "step_run", sr.ID, before, map[string]any{"status": sr.Status, "attempt": sr.Attempt, "idempotencyKey": idem}); err != nil {
			return err
		}
		return advanceWorkflow(tx, tenantID, executionID)
	})
	if err != nil {
		return nil, err
	}
	// H7.5 埋点：workflow_step（advisory，Record 内部 recover + 异步落库）。
	if s.usage != nil {
		stepErr := ""
		if !success {
			stepErr = errorText
		}
		s.usage.Record(UsageRecordInput{
			Kind: usage.KindWorkflowStep, TenantID: tenantID, RunID: executionID, Error: stepErr,
		})
	}
	s.emitStepPersonaEvents(tenantID, executionID, id, actor, success, idem)
	ex, err := s.Execution(tenantID, executionID)
	if err == nil {
		s.dispatchActive(ex)
	}
	return ex, err
}

// emitStepPersonaEvents is the H6 WS6 task-completion hook: the completing
// agent receives a deterministic mood event (task_completed/task_failed), and
// when both the task source and the completer are agents, a task_completed
// relation event is settled from completer toward source. Keys are
// deterministic (`h6:<flow>:<id>`) so replays are no-ops. All failures are
// log-and-continue. H7 P1: each pack only fires while its builtin extension
// stays enabled (runtime gate check, disable takes effect immediately).
func (s *WorkflowService) emitStepPersonaEvents(tenantID string, executionID, stepRunID, actor string, success bool, idem string) {
	emotionOn := s.personaEmotion != nil && s.personaPackEnabled(tenantID, PersonaPackEmotion)
	reldynOn := s.personaRelDyn != nil && s.personaPackEnabled(tenantID, PersonaPackRelationshipDynamics)
	if !emotionOn && !reldynOn {
		return
	}
	var ex workflow.Execution
	if err := s.db.Where("tenant_id=? AND id=?", tenantID, executionID).First(&ex).Error; err != nil {
		log.Printf("[h6] step persona hook: execution %s lookup failed: %v", executionID, err)
		return
	}
	if ex.RunID == "" {
		return
	}
	var completer agent.AgentConfig
	if err := s.db.Where("tenant_id=? AND name=?", tenantID, actor).First(&completer).Error; err != nil {
		log.Printf("[h6] step persona hook: completer %q not an agent: %v", actor, err)
		return
	}
	eventType := "task_failed"
	if success {
		eventType = "task_completed"
	}
	at := time.Now().UTC()
	if emotionOn {
		if err := s.personaEmotion.OnEvent(tenantID, ex.RunID, completer.ID, eventType, 1, at, fmt.Sprintf("h6:step-emotion:%s:%s", stepRunID, idem)); err != nil {
			log.Printf("[h6] emotion step event failed: step=%s agent=%d: %v", stepRunID, completer.ID, err)
		}
	}
	if reldynOn && ex.StartedBy != "" && ex.StartedBy != actor {
		var starter agent.AgentConfig
		if err := s.db.Where("tenant_id=? AND name=?", tenantID, ex.StartedBy).First(&starter).Error; err != nil {
			log.Printf("[h6] reldyn step event skipped: starter %q not an agent: %v", ex.StartedBy, err)
			return
		}
		if err := s.personaRelDyn.OnEvent(tenantID, ex.RunID, completer.ID, starter.ID, "task_completed", 1, at, fmt.Sprintf("h6:step-reldyn:%s:%s", stepRunID, idem)); err != nil {
			log.Printf("[h6] reldyn step event failed: step=%s pair=%d->%d: %v", stepRunID, completer.ID, starter.ID, err)
		}
	}
}

func forceActivateStep(tx *gorm.DB, tenantID, executionID, key string) error {
	var target workflow.StepRun
	if err := tx.Where("tenant_id=? AND execution_id=? AND step_key=?", tenantID, executionID, key).First(&target).Error; err != nil {
		return err
	}
	if target.Status != workflow.StepPending {
		return nil
	}
	var step workflow.Step
	if err := tx.Where("tenant_id=? AND id=?", tenantID, target.StepID).First(&step).Error; err != nil {
		return err
	}
	activateStep(&target, step)
	if err := tx.Save(&target).Error; err != nil {
		return err
	}
	if target.Status == workflow.StepWaitingApproval {
		return createApproval(tx, tenantID, executionID, &target, step)
	} else if target.Status == workflow.StepWaitingDecision {
		_, err := createWorkflowDecision(tx, tenantID, executionID, &target, step, "workflow")
		return err
	}
	return nil
}

func advanceWorkflow(tx *gorm.DB, tenantID, executionID string) error {
	var runs []workflow.StepRun
	if err := tx.Where("tenant_id=? AND execution_id=?", tenantID, executionID).Find(&runs).Error; err != nil {
		return err
	}
	var steps []workflow.Step
	var ex workflow.Execution
	if err := tx.Where("tenant_id=? AND id=?", tenantID, executionID).First(&ex).Error; err != nil {
		return err
	}
	if err := tx.Where("tenant_id=? AND version_id=?", tenantID, ex.VersionID).Find(&steps).Error; err != nil {
		return err
	}
	var transitions []workflow.Transition
	if err := tx.Where("tenant_id=? AND version_id=?", tenantID, ex.VersionID).Order("priority desc").Find(&transitions).Error; err != nil {
		return err
	}
	status := map[string]string{}
	byKey := map[string]workflow.StepRun{}
	for _, r := range runs {
		status[r.StepKey] = r.Status
		byKey[r.StepKey] = r
	}
	chosenTransition := map[string]string{}
	for _, tr := range transitions {
		if _, set := chosenTransition[tr.FromStepKey]; set {
			continue
		}
		src := byKey[tr.FromStepKey]
		if src.Status == workflow.StepCompleted && conditionMatches(tr.Condition, src.Output) {
			chosenTransition[tr.FromStepKey] = tr.ToStepKey
		}
	}
	changed := true
	for changed {
		changed = false
		for _, step := range steps {
			r := byKey[step.Key]
			if r.Status != workflow.StepPending {
				continue
			}
			ready := true
			blockedByFailure := false
			for _, d := range step.DependsOn {
				if status[d] == workflow.StepFailed || status[d] == workflow.StepTimedOut {
					blockedByFailure = true
				}
				if status[d] != workflow.StepCompleted && status[d] != workflow.StepSkipped {
					ready = false
				}
			}
			if blockedByFailure {
				r.Status = workflow.StepSkipped
				now := time.Now().UTC()
				r.CompletedAt = &now
				if err := tx.Save(&r).Error; err != nil {
					return err
				}
				if err := auditWorkflow(tx, tenantID, executionID, "system", "step_skipped", "step_run", r.ID, nil, map[string]any{"reason": "upstream_failed"}); err != nil {
					return err
				}
				status[step.Key] = r.Status
				byKey[step.Key] = r
				changed = true
				continue
			}
			var inbound []workflow.Transition
			for _, tr := range transitions {
				if tr.ToStepKey == step.Key {
					inbound = append(inbound, tr)
				}
			}
			if ready && len(inbound) > 0 {
				matched, settled := false, true
				for _, tr := range inbound {
					src := byKey[tr.FromStepKey]
					if src.Status != workflow.StepCompleted && src.Status != workflow.StepSkipped && src.Status != workflow.StepFailed && src.Status != workflow.StepTimedOut {
						settled = false
					}
					if src.Status == workflow.StepCompleted && chosenTransition[tr.FromStepKey] == step.Key {
						matched = true
					}
				}
				if settled && !matched {
					r.Status = workflow.StepSkipped
					now := time.Now().UTC()
					r.CompletedAt = &now
					if err := tx.Save(&r).Error; err != nil {
						return err
					}
					if err := auditWorkflow(tx, tenantID, executionID, "system", "step_skipped", "step_run", r.ID, nil, map[string]any{"reason": "condition_unmatched"}); err != nil {
						return err
					}
					status[step.Key] = r.Status
					byKey[step.Key] = r
					changed = true
					continue
				}
				ready = matched
			}
			if ready {
				activateStep(&r, step)
				if err := tx.Save(&r).Error; err != nil {
					return err
				}
				if err := auditWorkflow(tx, tenantID, executionID, "system", "step_activated", "step_run", r.ID, nil, map[string]any{"status": r.Status, "assignees": r.AssigneeSnapshot}); err != nil {
					return err
				}
				if r.Status == workflow.StepWaitingApproval {
					if err := createApproval(tx, tenantID, executionID, &r, step); err != nil {
						return err
					}
				} else if r.Status == workflow.StepWaitingDecision {
					if _, err := createWorkflowDecision(tx, tenantID, executionID, &r, step, "workflow"); err != nil {
						return err
					}
				}
				changed = true
				status[step.Key] = r.Status
				byKey[step.Key] = r
			}
		}
	}
	var active, failed int64
	var outputs = map[string]any{}
	if err := tx.Model(&workflow.StepRun{}).Where("tenant_id=? AND execution_id=? AND status IN ?", tenantID, executionID, []string{workflow.StepPending, workflow.StepRunning, workflow.StepWaitingApproval, workflow.StepWaitingDecision}).Count(&active).Error; err != nil {
		return err
	}
	if err := tx.Model(&workflow.StepRun{}).Where("tenant_id=? AND execution_id=? AND status = ?", tenantID, executionID, workflow.StepFailed).Count(&failed).Error; err != nil {
		return err
	}
	if active == 0 {
		now := time.Now().UTC()
		final := workflow.ExecutionCompleted
		if failed > 0 {
			final = workflow.ExecutionFailed
		}
		var done []workflow.StepRun
		tx.Where("tenant_id=? AND execution_id=?", tenantID, executionID).Find(&done)
		for _, r := range done {
			if r.Output != nil {
				outputs[r.StepKey] = r.Output
			}
		}
		ex.Status = final
		ex.Output = outputs
		ex.CompletedAt = &now
		var version workflow.Version
		if err := tx.Where("tenant_id=? AND id=?", tenantID, ex.VersionID).First(&version).Error; err != nil {
			return err
		}
		if final == workflow.ExecutionCompleted && len(version.OutputSchema) > 0 {
			if err := validateJSONSchema(version.OutputSchema, outputs); err != nil {
				return fmt.Errorf("workflow output: %w", err)
			}
		}
		if err := tx.Save(&ex).Error; err != nil {
			return err
		}
		return auditWorkflow(tx, tenantID, executionID, "system", "execution_"+final, "execution", executionID, nil, map[string]any{"status": final, "output": outputs})
	}
	return nil
}

// conditionMatches intentionally implements a small, deterministic condition
// language: every key/value in `equals` must equal the completed source output.
// Empty conditions always match. Rich expressions belong in an extension,
// never in eval or tenant-provided executable code.
func conditionMatches(condition, output map[string]any) bool {
	if len(condition) == 0 {
		return true
	}
	raw, ok := condition["equals"]
	if !ok {
		return false
	}
	expected, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range expected {
		if fmt.Sprint(output[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

func (s *WorkflowService) DecideApproval(tenantID, id, actor, decision, reason string, conditions map[string]any, idem string) (*workflow.Execution, error) {
	if decision != workflow.DecisionApprove && decision != workflow.DecisionReject && decision != workflow.DecisionConditional {
		return nil, fmt.Errorf("invalid decision")
	}
	if actor == "" || idem == "" {
		return nil, fmt.Errorf("actor and idempotencyKey are required")
	}
	fingerprint := requestFingerprint(id, actor, decision, reason, conditions)
	var prior workflow.ApprovalDecision
	if err := s.db.Where("tenant_id=? AND idempotency_key=?", tenantID, idem).First(&prior).Error; err == nil {
		if prior.RequestHash != fingerprint {
			return nil, ErrWorkflowConflict
		}
		var approval workflow.Approval
		if err := s.db.Where("tenant_id=? AND id=?", tenantID, prior.ApprovalID).First(&approval).Error; err != nil {
			return nil, err
		}
		return s.Execution(tenantID, approval.ExecutionID)
	}
	var executionID string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var a workflow.Approval
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND id=?", tenantID, id).First(&a).Error; err != nil {
			return ErrWorkflowNotFound
		}
		executionID = a.ExecutionID
		if a.Status != workflow.ApprovalPending {
			return ErrWorkflowConflict
		}
		if len(a.ApproverSnapshot) == 0 {
			return fmt.Errorf("approval has no frozen approvers")
		}
		allowed := false
		for _, x := range a.ApproverSnapshot {
			if x == actor {
				allowed = true
			}
		}
		if !allowed {
			return fmt.Errorf("actor is not an approver")
		}
		d := workflow.ApprovalDecision{ID: uuid.NewString(), TenantID: tenantID, ApprovalID: id, ActorID: actor, Decision: decision, Reason: reason, Conditions: conditions, IdempotencyKey: idem, RequestHash: fingerprint}
		if err := tx.Create(&d).Error; err != nil {
			if isDuplicate(err) {
				return ErrWorkflowConflict
			}
			return err
		}
		var decisions []workflow.ApprovalDecision
		if err := tx.Where("tenant_id=? AND approval_id=?", tenantID, id).Find(&decisions).Error; err != nil {
			return err
		}
		reject := false
		approved := 0
		for _, d := range decisions {
			if d.Decision == workflow.DecisionReject {
				reject = true
			}
			if d.Decision == workflow.DecisionApprove || d.Decision == workflow.DecisionConditional {
				approved++
			}
		}
		resolved := reject || approved >= a.Quorum
		if !resolved {
			return auditWorkflow(tx, tenantID, executionID, actor, "approval_decision", "approval", id, nil, map[string]any{"decision": decision})
		}
		now := time.Now().UTC()
		a.ResolvedAt = &now
		if reject {
			a.Status = workflow.ApprovalRejected
		} else {
			a.Status = workflow.ApprovalApproved
		}
		if err := tx.Save(&a).Error; err != nil {
			return err
		}
		stepStatus := workflow.StepCompleted
		if reject {
			stepStatus = workflow.StepFailed
		}
		var sr workflow.StepRun
		if err := tx.Where("tenant_id=? AND id=?", tenantID, a.StepRunID).First(&sr).Error; err != nil {
			return err
		}
		sr.Status = stepStatus
		sr.CompletedAt = &now
		sr.Output = map[string]any{"approvalStatus": a.Status}
		if err := tx.Save(&sr).Error; err != nil {
			return err
		}
		if err := auditWorkflow(tx, tenantID, executionID, actor, "approval_"+a.Status, "approval", id, nil, map[string]any{"decision": decision, "status": a.Status}); err != nil {
			return err
		}
		return advanceWorkflow(tx, tenantID, executionID)
	})
	if err != nil {
		return nil, err
	}
	ex, err := s.Execution(tenantID, executionID)
	if err == nil {
		s.dispatchActive(ex)
	}
	return ex, err
}

func (s *WorkflowService) ProcessTimeouts(tenantID, executionID, actor string, now time.Time) (*workflow.Execution, error) {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var runs []workflow.StepRun
		if err := tx.Where("tenant_id=? AND execution_id=? AND due_at IS NOT NULL AND due_at <= ? AND status IN ?", tenantID, executionID, now, []string{workflow.StepRunning, workflow.StepWaitingApproval}).Find(&runs).Error; err != nil {
			return err
		}
		for _, sr := range runs {
			var step workflow.Step
			if err := tx.Where("tenant_id=? AND id=?", tenantID, sr.StepID).First(&step).Error; err != nil {
				return err
			}
			sr.Status = workflow.StepTimedOut
			sr.CompletedAt = &now
			if step.EscalationStepKey == "" {
				sr.Status = workflow.StepFailed
			}
			if err := tx.Save(&sr).Error; err != nil {
				return err
			}
			if step.EscalationStepKey != "" {
				if err := forceActivateStep(tx, tenantID, executionID, step.EscalationStepKey); err != nil {
					return err
				}
			}
			if err := auditWorkflow(tx, tenantID, executionID, actor, "step_timed_out", "step_run", sr.ID, nil, map[string]any{"escalationStepKey": step.EscalationStepKey}); err != nil {
				return err
			}
		}
		return advanceWorkflow(tx, tenantID, executionID)
	})
	if err != nil {
		return nil, err
	}
	return s.Execution(tenantID, executionID)
}

func SortStepRuns(rows []workflow.StepRun) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.Before(rows[j].CreatedAt) })
}
