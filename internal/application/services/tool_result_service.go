package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ToolResultAccepted = "accepted"
	ToolResultRejected = "rejected"
	ToolResultApplied  = "applied"
	maxToolProposals   = 32
	maxPatchOperations = 64
)

type ToolResultService struct {
	db *gorm.DB
}

// Reject records a policy/user decision without applying any proposed state.
// Repeating the same decision key returns the original record.
func (s *ToolResultService) Reject(tenantID, runID string, input CommitToolResultInput, reason string) (*rundomain.ToolResultRecord, error) {
	if tenantID == "" || runID == "" || strings.TrimSpace(input.ToolName) == "" || strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("tenant, run, tool name, idempotencyKey and rejection reason are required")
	}
	var existing rundomain.ToolResultRecord
	if err := s.db.Where("tenant_id=? AND run_id=? AND idempotency_key=?", tenantID, runID, input.IdempotencyKey).First(&existing).Error; err == nil {
		return &existing, nil
	}
	record := &rundomain.ToolResultRecord{
		ID: uuid.NewString(), TenantID: tenantID, RunID: runID, IdempotencyKey: input.IdempotencyKey,
		ToolName: input.ToolName, ActorType: input.ActorType, ActorID: input.ActorID, CorrelationID: input.CorrelationID, CausationID: input.CausationID, RootEventID: input.RootEventID,
		Status: ToolResultRejected, DecisionReason: reason, Result: input.Result.Result, Cost: input.Result.Cost,
		StateProposals: input.Result.StateProposals, EventProposals: input.Result.EventProposals, LatencyMS: input.Result.LatencyMS, AvailableAt: input.Result.AvailableAt,
	}
	if err := s.db.Create(record).Error; err != nil {
		if isDuplicate(err) && s.db.Where("tenant_id=? AND run_id=? AND idempotency_key=?", tenantID, runID, input.IdempotencyKey).First(&existing).Error == nil {
			return &existing, nil
		}
		return nil, err
	}
	return record, nil
}

func NewToolResultService(db *gorm.DB) *ToolResultService { return &ToolResultService{db: db} }

func (s *ToolResultService) List(tenantID, runID string, limit int) ([]rundomain.ToolResultRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []rundomain.ToolResultRecord
	err := s.db.Where("tenant_id=? AND run_id=?", tenantID, runID).Order("created_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *ToolResultService) Get(tenantID, runID, id string) (*rundomain.ToolResultRecord, error) {
	var row rundomain.ToolResultRecord
	if err := s.db.Where("tenant_id=? AND run_id=? AND id=?", tenantID, runID, id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, rundomain.ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

type CommitToolResultInput struct {
	ToolName, ActorType, ActorID            string
	CorrelationID, CausationID, RootEventID string
	IdempotencyKey                          string
	Result                                  rundomain.ToolResult
}

type ValidatedStateProposal struct {
	StateID          uint64         `json:"stateId"`
	ExpectedRevision uint64         `json:"expectedRevision"`
	NextData         map[string]any `json:"nextData"`
}

type ToolResultValidation struct {
	StateProposals []ValidatedStateProposal `json:"stateProposals"`
}

// Validate performs the proposal and validation phases without writing. Commit
// repeats these checks under row locks, so callers may safely preview a result
// without turning the preview into an authorization bypass.
func (s *ToolResultService) Validate(tenantID, runID string, result rundomain.ToolResult) (*ToolResultValidation, error) {
	return s.validateWithDB(s.db, tenantID, runID, result, false)
}

// Commit is the only path from a remote tool/LLM state proposal to final Run
// state. All proposals and their audit records commit atomically.
func (s *ToolResultService) Commit(tenantID, runID string, input CommitToolResultInput) (*rundomain.ToolResultRecord, error) {
	if tenantID == "" || runID == "" || strings.TrimSpace(input.ToolName) == "" {
		return nil, fmt.Errorf("tenant, run and tool name are required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return nil, fmt.Errorf("idempotencyKey is required")
	}
	if err := validateToolResultEnvelope(input.Result); err != nil {
		return nil, err
	}

	var record rundomain.ToolResultRecord
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id=? AND run_id=? AND idempotency_key=?", tenantID, runID, input.IdempotencyKey).First(&record).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		validated, err := s.validateWithDB(tx, tenantID, runID, input.Result, true)
		if err != nil {
			return err
		}
		changeIDs := make([]string, 0, len(validated.StateProposals))
		events := NewEventService(tx)
		if input.RootEventID != "" {
			if _, err := events.ConsumeUsageTx(tx, tenantID, runID, input.RootEventID, input.Result.Cost.ToolCalls, input.Result.Cost.InputTokens+input.Result.Cost.OutputTokens); err != nil {
				return err
			}
		}
		for i, proposal := range validated.StateProposals {
			var state rundomain.RunState
			if err := tx.Where("tenant_id=? AND run_id=? AND id=?", tenantID, runID, proposal.StateID).First(&state).Error; err != nil {
				return rundomain.ErrStateNotFound
			}
			encoded, err := json.Marshal(proposal.NextData)
			if err != nil {
				return fmt.Errorf("encode proposed state: %w", err)
			}
			next := state.Revision + 1
			updated := tx.Model(&rundomain.RunState{}).
				Where("tenant_id=? AND run_id=? AND id=? AND revision=?", tenantID, runID, state.ID, state.Revision).
				Updates(map[string]any{"data": string(encoded), "revision": next, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return rundomain.ErrConflict
			}
			changeID := uuid.NewString()
			change := rundomain.RunStateChange{
				ID: changeID, TenantID: tenantID, RunID: runID, RunStateID: state.ID,
				RevisionBefore: state.Revision, RevisionAfter: next, Before: state.Data, After: proposal.NextData,
				Reason: input.Result.StateProposals[i].Reason, Source: "tool:" + input.ToolName,
				IdempotencyKey: input.IdempotencyKey + ":state:" + strconv.Itoa(i),
			}
			if err := tx.Create(&change).Error; err != nil {
				return err
			}
			stateEvent, err := events.PublishTx(tx, tenantID, PublishEventInput{
				RunID: runID, Type: "agenthub.state.changed.v1", Source: "tool:" + input.ToolName,
				Scope: eventdomain.Ref{Type: "run", ID: runID}, Subject: eventdomain.Ref{Type: state.SubjectType, ID: state.SubjectID},
				Actor: eventdomain.Ref{Type: input.ActorType, ID: input.ActorID}, CorrelationID: input.CorrelationID,
				CausationID: input.CausationID, RootEventID: input.RootEventID, IdempotencyKey: input.IdempotencyKey + ":state-event:" + strconv.Itoa(i),
				Data: map[string]any{"stateId": state.ID, "changeId": changeID, "revisionBefore": state.Revision, "revisionAfter": next, "namespace": state.Namespace},
			})
			if err != nil {
				return err
			}
			if input.CorrelationID == "" {
				input.CorrelationID = stateEvent.CorrelationID
			}
			changeIDs = append(changeIDs, changeID)
		}
		for i, proposal := range input.Result.EventProposals {
			scheduled := time.Time{}
			if proposal.DelayMS > 0 {
				scheduled = time.Now().UTC().Add(time.Duration(proposal.DelayMS) * time.Millisecond)
			}
			if _, err := events.PublishTx(tx, tenantID, PublishEventInput{
				RunID: runID, Type: proposal.Type, Source: "tool:" + input.ToolName,
				Scope: eventdomain.Ref{Type: "run", ID: runID}, Subject: eventdomain.Ref{Type: proposal.SubjectType, ID: proposal.SubjectID},
				Actor: eventdomain.Ref{Type: input.ActorType, ID: input.ActorID}, CorrelationID: input.CorrelationID,
				CausationID: input.CausationID, RootEventID: input.RootEventID, IdempotencyKey: input.IdempotencyKey + ":event:" + strconv.Itoa(i),
				Data: proposal.Data, ScheduledAt: scheduled,
			}); err != nil {
				return err
			}
		}

		record = rundomain.ToolResultRecord{
			ID: uuid.NewString(), TenantID: tenantID, RunID: runID, IdempotencyKey: input.IdempotencyKey,
			ToolName: input.ToolName, ActorType: input.ActorType, ActorID: input.ActorID,
			CorrelationID: input.CorrelationID, CausationID: input.CausationID, RootEventID: input.RootEventID,
			Status: ToolResultApplied, Result: input.Result.Result, Cost: input.Result.Cost,
			StateProposals: input.Result.StateProposals, EventProposals: input.Result.EventProposals,
			LatencyMS: input.Result.LatencyMS, AvailableAt: input.Result.AvailableAt, CommittedChanges: changeIDs,
		}
		return tx.Create(&record).Error
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *ToolResultService) validateWithDB(db *gorm.DB, tenantID, runID string, result rundomain.ToolResult, lock bool) (*ToolResultValidation, error) {
	if err := validateToolResultEnvelope(result); err != nil {
		return nil, err
	}
	query := db
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run rundomain.Run
	if err := query.Where("tenant_id=? AND id=?", tenantID, runID).First(&run).Error; err != nil {
		return nil, rundomain.ErrNotFound
	}
	if runFrozen(run.Status) {
		return nil, rundomain.ErrFrozen
	}

	validation := &ToolResultValidation{StateProposals: make([]ValidatedStateProposal, 0, len(result.StateProposals))}
	seen := make(map[uint64]struct{}, len(result.StateProposals))
	for _, proposal := range result.StateProposals {
		if proposal.StateID == 0 || proposal.ExpectedRevision == 0 {
			return nil, fmt.Errorf("state proposal requires stateId and expectedRevision")
		}
		if _, duplicate := seen[proposal.StateID]; duplicate {
			return nil, fmt.Errorf("only one proposal per state is allowed in a tool result")
		}
		seen[proposal.StateID] = struct{}{}
		var state rundomain.RunState
		stateQuery := db
		if lock {
			stateQuery = stateQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := stateQuery.Where("tenant_id=? AND run_id=? AND id=?", tenantID, runID, proposal.StateID).First(&state).Error; err != nil {
			return nil, rundomain.ErrStateNotFound
		}
		if state.Revision != proposal.ExpectedRevision {
			return nil, rundomain.ErrConflict
		}
		next, err := applyRestrictedPatch(state.Data, proposal.Patch)
		if err != nil {
			return nil, fmt.Errorf("invalid state proposal for state %d: %w", state.ID, err)
		}
		var schema rundomain.StateSchema
		if err := db.Where("tenant_id=? AND namespace=? AND name=? AND version=?", tenantID, state.Namespace, state.SchemaName, state.SchemaVersion).First(&schema).Error; err != nil {
			return nil, rundomain.ErrSchemaNotFound
		}
		if schema.ContentHash != state.SchemaHash {
			return nil, fmt.Errorf("locked state schema version changed")
		}
		if err := validateJSONSchema(schema.Schema, next); err != nil {
			return nil, fmt.Errorf("state validation failed: %w", err)
		}
		validation.StateProposals = append(validation.StateProposals, ValidatedStateProposal{StateID: state.ID, ExpectedRevision: state.Revision, NextData: next})
	}
	return validation, nil
}

func validateToolResultEnvelope(result rundomain.ToolResult) error {
	if result.LatencyMS < 0 || result.Cost.InputTokens < 0 || result.Cost.OutputTokens < 0 || result.Cost.ToolCalls < 0 || result.Cost.Amount < 0 {
		return fmt.Errorf("tool result cost and latency must be non-negative")
	}
	if len(result.StateProposals) > maxToolProposals || len(result.EventProposals) > maxToolProposals {
		return fmt.Errorf("tool result exceeds proposal limit")
	}
	for _, proposal := range result.StateProposals {
		if len(proposal.Patch) == 0 || len(proposal.Patch) > maxPatchOperations {
			return fmt.Errorf("state proposal patch must contain 1-%d operations", maxPatchOperations)
		}
	}
	for _, proposal := range result.EventProposals {
		if strings.TrimSpace(proposal.Type) == "" || strings.TrimSpace(proposal.SubjectType) == "" || strings.TrimSpace(proposal.SubjectID) == "" || proposal.DelayMS < 0 {
			return fmt.Errorf("event proposal requires type and subject and a non-negative delay")
		}
	}
	return nil
}

func applyRestrictedPatch(current map[string]any, operations []rundomain.PatchOperation) (map[string]any, error) {
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	var next map[string]any
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, err
	}
	for _, operation := range operations {
		segments, err := parseObjectPointer(operation.Path)
		if err != nil {
			return nil, err
		}
		parent := next
		for _, segment := range segments[:len(segments)-1] {
			child, ok := parent[segment].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path %q does not identify an object field", operation.Path)
			}
			parent = child
		}
		key := segments[len(segments)-1]
		switch operation.Op {
		case "add":
			if _, exists := parent[key]; exists {
				return nil, fmt.Errorf("add target %q already exists", operation.Path)
			}
			parent[key] = operation.Value
		case "replace":
			if _, exists := parent[key]; !exists {
				return nil, fmt.Errorf("replace target %q does not exist", operation.Path)
			}
			parent[key] = operation.Value
		case "remove":
			if _, exists := parent[key]; !exists {
				return nil, fmt.Errorf("remove target %q does not exist", operation.Path)
			}
			delete(parent, key)
		case "increment":
			oldValue, ok := number(parent[key])
			if !ok {
				return nil, fmt.Errorf("increment target %q is not numeric", operation.Path)
			}
			delta, ok := number(operation.Value)
			if !ok || math.IsNaN(delta) || math.IsInf(delta, 0) {
				return nil, fmt.Errorf("increment value for %q is not finite numeric", operation.Path)
			}
			parent[key] = oldValue + delta
		default:
			return nil, fmt.Errorf("operation %q is not allowed", operation.Op)
		}
	}
	return next, nil
}

func parseObjectPointer(path string) ([]string, error) {
	if path == "" || path == "/" || !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("patch path must be a non-root JSON pointer")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		if part == "" || part == "-" {
			return nil, fmt.Errorf("array/root patch paths are not supported")
		}
		parts[i] = part
	}
	return parts, nil
}

func number(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}
