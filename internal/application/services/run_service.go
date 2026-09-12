package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	rundomain "control-panel/internal/domain/run"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RunService struct {
	db       *gorm.DB
	registry *CapabilityRegistryService
}

func NewRunService(db *gorm.DB) *RunService {
	return &RunService{db: db, registry: NewCapabilityRegistryService(db)}
}

type RunCapabilityInput struct {
	Namespace   string `json:"namespace"`
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
}

type CreateRunInput struct {
	Name         string
	Description  string
	Metadata     map[string]any
	CreatedBy    string
	Capabilities []RunCapabilityInput
}

func (s *RunService) Create(tenantID string, input CreateRunInput) (*rundomain.Run, error) {
	if tenantID == "" || strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("tenant and run name are required")
	}
	r := &rundomain.Run{ID: uuid.NewString(), TenantID: tenantID, Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), Status: rundomain.StatusDraft, Metadata: input.Metadata, CreatedBy: input.CreatedBy}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		bindings := make([]rundomain.CapabilityBinding, 0, len(input.Capabilities))
		for _, b := range input.Capabilities {
			if b.Namespace == "" || b.PackageName == "" || b.Version == "" {
				return fmt.Errorf("capability binding requires namespace, packageName and version")
			}
			registered, err := s.registry.GetEnabled(tx, tenantID, b.Namespace, b.PackageName, b.Version)
			if err != nil {
				return err
			}
			bindings = append(bindings, rundomain.CapabilityBinding{TenantID: tenantID, RunID: r.ID, Namespace: registered.Namespace, PackageName: registered.Name, Version: registered.Version, ContentHash: registered.ContentHash, ManifestYAML: registered.ManifestYAML, Snapshot: registered.ResourcesSnapshot})
		}
		if err := tx.Create(r).Error; err != nil {
			return err
		}
		for i := range bindings {
			if err := tx.Create(&bindings[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(tenantID, r.ID)
}

func (s *RunService) List(tenantID, status string, limit int) ([]rundomain.Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := s.db.Where("tenant_id = ?", tenantID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []rundomain.Run
	err := q.Preload("Agents").Preload("Bindings").Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *RunService) Get(tenantID, id string) (*rundomain.Run, error) {
	var r rundomain.Run
	if err := s.db.Where("tenant_id = ? AND id = ?", tenantID, id).Preload("Agents").Preload("Bindings").First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, rundomain.ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

func (s *RunService) Transition(tenantID, id, target string) (*rundomain.Run, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var r rundomain.Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, id).First(&r).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return rundomain.ErrNotFound
			}
			return err
		}
		if !rundomain.ValidTransitions[r.Status][target] {
			return rundomain.ErrInvalidTransition
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": target, "updated_at": now}
		switch target {
		case rundomain.StatusRunning:
			if r.StartedAt == nil {
				updates["started_at"] = now
			}
			updates["paused_at"] = nil
		case rundomain.StatusPaused:
			updates["paused_at"] = now
		case rundomain.StatusCompleted:
			updates["completed_at"] = now
		case rundomain.StatusArchived:
			updates["archived_at"] = now
		}
		result := tx.Model(&r).Where("status = ?", r.Status).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return rundomain.ErrConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(tenantID, id)
}

func (s *RunService) AddAgent(tenantID, runID string, agentID uint64, role string) (*rundomain.RunAgent, error) {
	if role = strings.TrimSpace(role); role == "" {
		role = "participant"
	}
	row := &rundomain.RunAgent{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var r rundomain.Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, runID).First(&r).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return rundomain.ErrNotFound
			}
			return err
		}
		// Participants and identity/config/personality snapshots form the
		// immutable Run definition. Execution starts only after this is locked.
		if r.Status != rundomain.StatusDraft {
			return rundomain.ErrFrozen
		}
		var a agent.AgentConfig
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, agentID).First(&a).Error; err != nil {
			return fmt.Errorf("agent not found")
		}
		ph := sha256.Sum256([]byte(a.PersonalityPrompt))
		*row = rundomain.RunAgent{TenantID: tenantID, RunID: runID, AgentID: a.ID, Role: role, AgentNameSnapshot: a.Name, AgentConfigHashSnapshot: a.ContentHash, PersonalityTemplateName: a.PersonalityTemplateName, PersonalityTemplateVersion: a.PersonalityTemplateVersion, PersonalityPromptHash: hex.EncodeToString(ph[:]), Snapshot: map[string]any{"modelId": a.ModelID, "providerId": a.ProviderID}}
		return tx.Create(row).Error
	})
	if err != nil {
		if isDuplicate(err) {
			return nil, rundomain.ErrDuplicate
		}
		return nil, err
	}
	return row, nil
}

type RegisterStateSchemaInput struct {
	Namespace, Name, Version string
	Schema                   map[string]any
	ScopeTypes, SubjectTypes []string
}

func (s *RunService) RegisterStateSchema(tenantID string, input RegisterStateSchemaInput) (*rundomain.StateSchema, error) {
	if tenantID == "" || input.Namespace == "" || input.Name == "" || input.Version == "" || len(input.Schema) == 0 {
		return nil, fmt.Errorf("namespace, name, version and schema are required")
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", input.Schema); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	if _, err := compiler.Compile("schema.json"); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	raw, _ := json.Marshal(input.Schema)
	sum := sha256.Sum256(raw)
	row := &rundomain.StateSchema{TenantID: tenantID, Namespace: input.Namespace, Name: input.Name, Version: input.Version, Schema: input.Schema, ScopeTypes: input.ScopeTypes, SubjectTypes: input.SubjectTypes, ContentHash: hex.EncodeToString(sum[:])}
	if err := s.db.Create(row).Error; err != nil {
		if isDuplicate(err) {
			return nil, rundomain.ErrDuplicate
		}
		return nil, err
	}
	return row, nil
}

type InitializeStateInput struct {
	Namespace, SchemaName, SchemaVersion, SubjectType, SubjectID string
	Data                                                         map[string]any
	IdempotencyKey, Reason, Source                               string
}

func (s *RunService) InitializeState(tenantID, runID string, input InitializeStateInput) (*rundomain.RunState, error) {
	sch, err := s.getSchema(tenantID, input.Namespace, input.SchemaName, input.SchemaVersion)
	if err != nil {
		return nil, err
	}
	if !contains(sch.ScopeTypes, "run") || !contains(sch.SubjectTypes, input.SubjectType) {
		return nil, fmt.Errorf("schema does not allow this scope or subject type")
	}
	if err := validateJSONSchema(sch.Schema, input.Data); err != nil {
		return nil, fmt.Errorf("state validation failed: %w", err)
	}
	if input.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	row := &rundomain.RunState{TenantID: tenantID, RunID: runID, Namespace: input.Namespace, SchemaName: input.SchemaName, SchemaVersion: input.SchemaVersion, SchemaHash: sch.ContentHash, SubjectType: input.SubjectType, SubjectID: input.SubjectID, Revision: 1, Data: input.Data}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing rundomain.RunStateChange
		if e := tx.Where("tenant_id = ? AND idempotency_key = ?", tenantID, input.IdempotencyKey).First(&existing).Error; e == nil {
			if existing.RunID != runID {
				return rundomain.ErrDuplicate
			}
			return tx.Where("tenant_id = ? AND id = ?", tenantID, existing.RunStateID).First(row).Error
		}
		var r rundomain.Run
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, runID).First(&r).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return rundomain.ErrNotFound
			}
			return e
		}
		if runFrozen(r.Status) {
			return rundomain.ErrFrozen
		}
		if e := tx.Create(row).Error; e != nil {
			if isDuplicate(e) {
				return rundomain.ErrDuplicate
			}
			return e
		}
		change := rundomain.RunStateChange{ID: uuid.NewString(), TenantID: tenantID, RunID: runID, RunStateID: row.ID, RevisionBefore: 0, RevisionAfter: 1, Before: map[string]any{}, After: input.Data, Reason: input.Reason, Source: defaultString(input.Source, "api"), IdempotencyKey: input.IdempotencyKey}
		return tx.Create(&change).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

type CommitStateInput struct {
	ExpectedRevision               uint64
	Data                           map[string]any
	IdempotencyKey, Reason, Source string
}

func (s *RunService) CommitState(tenantID, runID string, stateID uint64, input CommitStateInput) (*rundomain.RunState, error) {
	if input.IdempotencyKey == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	var result rundomain.RunState
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var prior rundomain.RunStateChange
		if e := tx.Where("tenant_id = ? AND idempotency_key = ?", tenantID, input.IdempotencyKey).First(&prior).Error; e == nil {
			if prior.RunID != runID || prior.RunStateID != stateID {
				return rundomain.ErrDuplicate
			}
			return tx.Where("tenant_id = ? AND id = ?", tenantID, prior.RunStateID).First(&result).Error
		}
		var r rundomain.Run
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, runID).First(&r).Error; e != nil {
			return rundomain.ErrNotFound
		}
		if runFrozen(r.Status) {
			return rundomain.ErrFrozen
		}
		var state rundomain.RunState
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND run_id = ? AND id = ?", tenantID, runID, stateID).First(&state).Error; e != nil {
			return rundomain.ErrStateNotFound
		}
		if state.Revision != input.ExpectedRevision {
			return rundomain.ErrConflict
		}
		sch, e := s.getSchemaWithDB(tx, tenantID, state.Namespace, state.SchemaName, state.SchemaVersion)
		if e != nil {
			return e
		}
		if sch.ContentHash != state.SchemaHash {
			return fmt.Errorf("locked state schema version changed")
		}
		if e := validateJSONSchema(sch.Schema, input.Data); e != nil {
			return fmt.Errorf("state validation failed: %w", e)
		}
		before := state.Data
		next := state.Revision + 1
		encodedData, e := json.Marshal(input.Data)
		if e != nil {
			return fmt.Errorf("encode state: %w", e)
		}
		res := tx.Model(&rundomain.RunState{}).Where("tenant_id = ? AND id = ? AND revision = ?", tenantID, stateID, input.ExpectedRevision).Updates(map[string]any{"data": string(encodedData), "revision": next, "updated_at": time.Now().UTC()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return rundomain.ErrConflict
		}
		change := rundomain.RunStateChange{ID: uuid.NewString(), TenantID: tenantID, RunID: runID, RunStateID: stateID, RevisionBefore: state.Revision, RevisionAfter: next, Before: before, After: input.Data, Reason: input.Reason, Source: defaultString(input.Source, "api"), IdempotencyKey: input.IdempotencyKey}
		if e := tx.Create(&change).Error; e != nil {
			return e
		}
		return tx.Where("tenant_id = ? AND id = ?", tenantID, stateID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *RunService) States(tenantID, runID string) ([]rundomain.RunState, error) {
	var rows []rundomain.RunState
	err := s.db.Where("tenant_id = ? AND run_id = ?", tenantID, runID).Order("id").Find(&rows).Error
	return rows, err
}
func (s *RunService) StateChanges(tenantID, runID string, stateID uint64, limit int) ([]rundomain.RunStateChange, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.db.Where("tenant_id = ? AND run_id = ?", tenantID, runID)
	if stateID > 0 {
		q = q.Where("run_state_id = ?", stateID)
	}
	var rows []rundomain.RunStateChange
	err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

type AppendRunActivityInput struct {
	Kind, Status, ActorType, ActorID, StepID, Name string
	Input, Output                                  map[string]any
	Error                                          string
	OccurredAt                                     time.Time
}

func (s *RunService) AppendActivity(tenantID, runID string, input AppendRunActivityInput) (*rundomain.RunActivity, error) {
	if input.Kind == "" {
		return nil, fmt.Errorf("activity kind is required")
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}
	row := &rundomain.RunActivity{ID: uuid.NewString(), TenantID: tenantID, RunID: runID, Kind: input.Kind, Status: input.Status, ActorType: input.ActorType, ActorID: input.ActorID, StepID: input.StepID, Name: input.Name, Input: input.Input, Output: input.Output, Error: input.Error, OccurredAt: input.OccurredAt}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var r rundomain.Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND id = ?", tenantID, runID).First(&r).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return rundomain.ErrNotFound
			}
			return err
		}
		if runFrozen(r.Status) {
			return rundomain.ErrFrozen
		}
		return tx.Create(row).Error
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}
func (s *RunService) ListActivities(tenantID, runID string, limit int) ([]rundomain.RunActivity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []rundomain.RunActivity
	err := s.db.Where("tenant_id=? AND run_id=?", tenantID, runID).Order("occurred_at ASC, created_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}
func (s *RunService) getSchema(t, n, name, v string) (*rundomain.StateSchema, error) {
	return s.getSchemaWithDB(s.db, t, n, name, v)
}
func (s *RunService) getSchemaWithDB(db *gorm.DB, t, n, name, v string) (*rundomain.StateSchema, error) {
	var x rundomain.StateSchema
	if err := db.Where("tenant_id=? AND namespace=? AND name=? AND version=?", t, n, name, v).First(&x).Error; err != nil {
		return nil, rundomain.ErrSchemaNotFound
	}
	return &x, nil
}
func runFrozen(status string) bool {
	return status == rundomain.StatusCompleted || status == rundomain.StatusArchived
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func defaultString(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}
func isDuplicate(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate")
}
func validateJSONSchema(doc, data map[string]any) error {
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", doc); err != nil {
		return err
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return err
	}
	return sch.Validate(data)
}
