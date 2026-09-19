package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/reldynamics"
	rundomain "control-panel/internal/domain/run"

	"gorm.io/gorm"
)

// RelationDynamicsService implements the io.zerone.relationship-dynamics
// capability pack (H6 WS4). Per-run directed attitudes between two agents
// live entirely in RunState (namespace io.zerone.relationship-dynamics,
// schema relation-attitude v1, subjectType relation); the legacy
// agent_relations.relationship_score column stays frozen read-only.
//
// subjectID format: "<fromAgentID>:<toAgentID>" (decimal, direction =
// attitude holder first). The key is derived from agent IDs only, so it is
// stable for the whole run even when no agent_relations row exists and
// survives relation-row rewrites. RunID + tenant scope the state, which gives
// cross-run and cross-tenant isolation for free.
type RelationDynamicsService struct {
	runService *RunService
}

// RelationAttitude is the pack's read model: the holder's outgoing attitude
// toward the counterpart in one run, including the deterministic narration
// sentence used by prompt composition.
type RelationAttitude struct {
	Score     int       `json:"score"`
	Stance    string    `json:"stance"`
	Narration string    `json:"narration"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewRelationDynamicsService(runService *RunService) *RelationDynamicsService {
	return &RelationDynamicsService{runService: runService}
}

// relationAttitudeSchema is the locked v1 JSON Schema for the attitude state.
var relationAttitudeSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"score", "stance", "narration"},
	"properties": map[string]any{
		"score":     map[string]any{"type": "integer", "minimum": reldynamics.MinScore, "maximum": reldynamics.MaxScore},
		"stance":    map[string]any{"type": "string", "enum": []any{"hostile", "wary", "neutral", "friendly", "allied"}},
		"narration": map[string]any{"type": "string"},
	},
}

// EnsureSchemas registers the relation-attitude v1 schema for every tenant
// that owns at least one run. It is idempotent: an existing identical schema
// is not an error. Event handling additionally ensures the schema lazily for
// the event's tenant, because run-state schemas are addressed by tenant.
func (s *RelationDynamicsService) EnsureSchemas() error {
	var tenantIDs []string
	if err := s.runService.db.Model(&rundomain.Run{}).Distinct().Pluck("tenant_id", &tenantIDs).Error; err != nil {
		return err
	}
	for _, tenantID := range tenantIDs {
		if err := s.ensureSchemaForTenant(tenantID); err != nil {
			return err
		}
	}
	return nil
}

func (s *RelationDynamicsService) ensureSchemaForTenant(tenantID string) error {
	_, err := s.runService.RegisterStateSchema(tenantID, RegisterStateSchemaInput{
		Namespace:    reldynamics.Namespace,
		Name:         reldynamics.SchemaName,
		Version:      reldynamics.SchemaVersion,
		Schema:       relationAttitudeSchema,
		ScopeTypes:   []string{"run"},
		SubjectTypes: []string{reldynamics.SubjectType},
	})
	if err != nil {
		if errors.Is(err, rundomain.ErrDuplicate) {
			return nil
		}
		return fmt.Errorf("注册关系态度状态模式失败: %w", err)
	}
	return nil
}

// OnEvent settles one relationship event: apply the locked v1 delta with
// severity multiplier and caps, clamp to -100..100, re-project the stance and
// regenerate the deterministic narration. Duplicate idempotency keys (same
// run, same relation pair) are a no-op; a key reused across runs or pairs is
// rejected as a caller error.
func (s *RelationDynamicsService) OnEvent(tenantID, runID string, fromAgentID, toAgentID uint64, eventType string, severity int, at time.Time, idempotencyKey string) error {
	if tenantID == "" || runID == "" {
		return fmt.Errorf("租户与运行 ID 不能为空")
	}
	if idempotencyKey == "" {
		return fmt.Errorf("幂等键不能为空")
	}
	if _, ok := reldynamics.EventBaseDeltas[eventType]; !ok {
		return fmt.Errorf("未知的关系事件类型: %s", eventType)
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	delta, _ := reldynamics.EventDelta(eventType, severity)

	subjectID := reldynamics.SubjectID(fromAgentID, toAgentID)
	// scope the idempotency key by run and pair so the tenant-wide unique
	// index never conflates distinct events
	key := fmt.Sprintf("%s:%s:%s", runID, subjectID, idempotencyKey)

	// duplicate event → no-op (same run and pair)
	var prior rundomain.RunStateChange
	if err := s.runService.db.Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).First(&prior).Error; err == nil {
		if prior.RunID == runID {
			return nil
		}
		return fmt.Errorf("幂等键已被其他运行使用")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if err := s.ensureSchemaForTenant(tenantID); err != nil {
		return err
	}

	existing, err := s.findState(tenantID, runID, subjectID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		score := reldynamics.ClampScore(delta)
		_, err = s.runService.InitializeState(tenantID, runID, InitializeStateInput{
			Namespace:      reldynamics.Namespace,
			SchemaName:     reldynamics.SchemaName,
			SchemaVersion:  reldynamics.SchemaVersion,
			SubjectType:    reldynamics.SubjectType,
			SubjectID:      subjectID,
			Data:           s.attitudeData(tenantID, toAgentID, score),
			IdempotencyKey: key,
			Reason:         fmt.Sprintf("relation event %s severity %d", eventType, severity),
			Source:         "capability:relationship-dynamics",
		})
		return err
	}
	if err != nil {
		return err
	}

	var current struct {
		Score int `json:"score"`
	}
	raw, _ := json.Marshal(existing.Data)
	if err := json.Unmarshal(raw, &current); err != nil {
		return fmt.Errorf("读取关系态度状态失败: %w", err)
	}
	score := reldynamics.ClampScore(current.Score + delta)
	_, err = s.runService.CommitState(tenantID, runID, existing.ID, CommitStateInput{
		ExpectedRevision: existing.Revision,
		Data:             s.attitudeData(tenantID, toAgentID, score),
		IdempotencyKey:   key,
		Reason:           fmt.Sprintf("relation event %s severity %d", eventType, severity),
		Source:           "capability:relationship-dynamics",
	})
	return err
}

// View returns the holder's outgoing attitude toward the counterpart in the
// run. A relation pair without any settled event yields the neutral default
// (score 0) instead of an error.
func (s *RelationDynamicsService) View(tenantID, runID string, fromAgentID uint64, targetAgentID uint64) (RelationAttitude, error) {
	subjectID := reldynamics.SubjectID(fromAgentID, targetAgentID)
	state, err := s.findState(tenantID, runID, subjectID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.buildAttitude(tenantID, targetAgentID, 0, time.Time{}), nil
	}
	if err != nil {
		return RelationAttitude{}, err
	}
	return s.decodeAttitude(tenantID, targetAgentID, state)
}

// Gate reads the attitude the target holds toward the initiator and applies
// the pure influence hook: under hostility (score < -60) assign-like actions
// initiated by the counterpart require the target's confirmation; inform-style
// actions are unaffected. Core message/task paths consult this function; it
// never rewrites organization mechanics.
func (s *RelationDynamicsService) Gate(tenantID, runID string, targetAgentID uint64, fromAgentID uint64, action string) (allowed bool, needConfirm bool, err error) {
	attitude, err := s.View(tenantID, runID, targetAgentID, fromAgentID)
	if err != nil {
		return false, false, err
	}
	allowed, needConfirm = reldynamics.GateAction(attitude.Score, action)
	return allowed, needConfirm, nil
}

func (s *RelationDynamicsService) findState(tenantID, runID, subjectID string) (*rundomain.RunState, error) {
	var state rundomain.RunState
	err := s.runService.db.Where("tenant_id = ? AND run_id = ? AND namespace = ? AND subject_type = ? AND subject_id = ?",
		tenantID, runID, reldynamics.Namespace, reldynamics.SubjectType, subjectID).First(&state).Error
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *RelationDynamicsService) attitudeData(tenantID string, targetAgentID uint64, score int) map[string]any {
	attitude := s.buildAttitude(tenantID, targetAgentID, score, time.Now().UTC())
	return map[string]any{
		"score":     attitude.Score,
		"stance":    attitude.Stance,
		"narration": attitude.Narration,
	}
}

func (s *RelationDynamicsService) buildAttitude(tenantID string, targetAgentID uint64, score int, updatedAt time.Time) RelationAttitude {
	return RelationAttitude{
		Score:     score,
		Stance:    reldynamics.StanceForScore(score),
		Narration: reldynamics.Narration(s.agentName(tenantID, targetAgentID), score),
		UpdatedAt: updatedAt,
	}
}

func (s *RelationDynamicsService) decodeAttitude(tenantID string, targetAgentID uint64, state *rundomain.RunState) (RelationAttitude, error) {
	var payload struct {
		Score     int    `json:"score"`
		Stance    string `json:"stance"`
		Narration string `json:"narration"`
	}
	raw, err := json.Marshal(state.Data)
	if err != nil {
		return RelationAttitude{}, err
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return RelationAttitude{}, fmt.Errorf("读取关系态度状态失败: %w", err)
	}
	updatedAt := state.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = state.CreatedAt
	}
	attitude := RelationAttitude{Score: payload.Score, Stance: payload.Stance, Narration: payload.Narration, UpdatedAt: updatedAt}
	if attitude.Narration == "" {
		attitude.Narration = reldynamics.Narration(s.agentName(tenantID, targetAgentID), attitude.Score)
	}
	return attitude, nil
}

// agentName resolves the counterpart's display name for the narration
// sentence; it falls back to the numeric ID when the agent row is missing.
func (s *RelationDynamicsService) agentName(tenantID string, agentID uint64) string {
	var row agent.AgentConfig
	err := s.runService.db.Select("name").Where("tenant_id = ? AND id = ?", tenantID, agentID).First(&row).Error
	if err != nil || row.Name == "" {
		return fmt.Sprintf("%d", agentID)
	}
	return row.Name
}
