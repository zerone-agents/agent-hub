package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	rundomain "control-panel/internal/domain/run"
	subjectivememory "control-panel/internal/domain/subjectivememory"

	"github.com/google/uuid"
)

// MemoryService wires the io.zerone.subjective-memory capability pack to
// run-scoped state. Each memory is its own RunState row (schema memory-entry
// v1, subject=agent, subjectID=memory ID) so recallCount updates stay cheap
// and idempotent; RunState itself gives strict per-run and per-tenant
// isolation by construction.
type MemoryService struct {
	run *RunService
}

func NewMemoryService(runService *RunService) *MemoryService {
	return &MemoryService{run: runService}
}

// memorySchema is the v1 JSON Schema for one memory entry. agentId is stored
// inside the state because the RunState subjectID carries the memory ID.
func memorySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"agentId":        map[string]any{"type": "integer", "minimum": 1},
			"factRef":        map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			"interpretation": map[string]any{"type": "string", "minLength": 1, "maxLength": subjectivememory.MaxInterpretationRunes},
			"importance":     map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"emotionTag":     map[string]any{"type": "string", "maxLength": 32},
			"recordedAt":     map[string]any{"type": "string", "minLength": 1},
			"recallCount":    map[string]any{"type": "integer", "minimum": 0},
		},
		"required": []any{"agentId", "factRef", "interpretation", "importance", "recordedAt", "recallCount"},
	}
}

// EnsureSchemas idempotently registers the memory-entry schema for every
// tenant that owns runs. Code reality: state schemas are registered
// per-tenant (state_schemas unique key includes tenant_id), so a
// tenant-free EnsureSchemas seeds all currently known tenants.
func (s *MemoryService) EnsureSchemas() error {
	var tenants []string
	if err := s.run.db.Model(&rundomain.Run{}).Distinct().Pluck("tenant_id", &tenants).Error; err != nil {
		return err
	}
	for _, tenantID := range tenants {
		if err := s.ensureSchema(tenantID); err != nil {
			return err
		}
	}
	return nil
}

func (s *MemoryService) ensureSchema(tenantID string) error {
	if _, err := s.run.getSchema(tenantID, subjectivememory.Namespace, subjectivememory.SchemaName, subjectivememory.SchemaVersion); err == nil {
		return nil
	} else if !errors.Is(err, rundomain.ErrSchemaNotFound) {
		return err
	}
	_, err := s.run.RegisterStateSchema(tenantID, RegisterStateSchemaInput{
		Namespace:    subjectivememory.Namespace,
		Name:         subjectivememory.SchemaName,
		Version:      subjectivememory.SchemaVersion,
		Schema:       memorySchema(),
		ScopeTypes:   []string{"run"},
		SubjectTypes: []string{subjectivememory.SubjectType},
	})
	if err != nil && !errors.Is(err, rundomain.ErrDuplicate) {
		return err
	}
	return nil
}

// Record lets an agent write its own subjective interpretation of a fact.
// importance is the agent's self-assessment and is clamped to 0..100. A
// duplicate idempotencyKey replays the original write and returns the
// existing entry; each memory gets its own RunState row.
func (s *MemoryService) Record(tenantID, runID string, agentID uint64, factRef string, interpretation string, importance int, at time.Time, idempotencyKey string) (*subjectivememory.MemoryEntry, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return nil, fmt.Errorf("幂等键不能为空")
	}
	if err := subjectivememory.Validate(factRef, interpretation); err != nil {
		return nil, err
	}
	if at.IsZero() {
		return nil, fmt.Errorf("记录时间不能为空")
	}
	if err := s.ensureSchema(tenantID); err != nil {
		return nil, err
	}
	entry := subjectivememory.MemoryEntry{
		ID:             uuid.NewString(),
		AgentID:        agentID,
		FactRef:        strings.TrimSpace(factRef),
		Interpretation: strings.TrimSpace(interpretation),
		Importance:     subjectivememory.ClampImportance(importance),
		RecordedAt:     at.UTC(),
		RecallCount:    0,
	}
	state, err := s.run.InitializeState(tenantID, runID, InitializeStateInput{
		Namespace:      subjectivememory.Namespace,
		SchemaName:     subjectivememory.SchemaName,
		SchemaVersion:  subjectivememory.SchemaVersion,
		SubjectType:    subjectivememory.SubjectType,
		SubjectID:      entry.ID,
		Data:           entryToData(entry),
		IdempotencyKey: idempotencyKey,
		Reason:         "memory.record",
		Source:         "capability:io.zerone.subjective-memory",
	})
	if err != nil {
		return nil, err
	}
	decoded, err := decodeMemory(state.Data)
	if err != nil {
		return nil, err
	}
	decoded.ID = state.SubjectID
	return decoded, nil
}

// Recall returns the top-N memories for the agent in this run, ranked by the
// pure retrieval score. Soft-forgotten entries (low importance, old, rarely
// recalled) sink below the cutoff but are never deleted. Returned memories
// get a best-effort recallCount increment, idempotent per (memory, run) via
// a deterministic key.
func (s *MemoryService) Recall(tenantID, runID string, agentID uint64, query string, limit int, at time.Time) ([]subjectivememory.MemoryEntry, error) {
	if limit <= 0 {
		limit = subjectivememory.DefaultRecallLimit
	}
	rows, err := s.run.States(tenantID, runID)
	if err != nil {
		return nil, err
	}
	entries := make([]subjectivememory.MemoryEntry, 0, len(rows))
	statesByID := map[string]rundomain.RunState{}
	for _, row := range rows {
		if row.Namespace != subjectivememory.Namespace || row.SubjectType != subjectivememory.SubjectType {
			continue
		}
		entry, err := decodeMemory(row.Data)
		if err != nil || entry.AgentID != agentID {
			continue
		}
		entry.ID = row.SubjectID
		entries = append(entries, *entry)
		statesByID[entry.ID] = row
	}
	ranked := subjectivememory.Rank(entries, query, at, limit)
	for i := range ranked {
		row, ok := statesByID[ranked[i].ID]
		if !ok {
			continue
		}
		next := entryToData(ranked[i])
		next["recallCount"] = ranked[i].RecallCount + 1
		updated, err := s.run.CommitState(tenantID, runID, row.ID, CommitStateInput{
			ExpectedRevision: row.Revision,
			Data:             next,
			IdempotencyKey:   fmt.Sprintf("%s:recall:%s", ranked[i].ID, runID),
			Reason:           "memory.recall",
			Source:           "capability:io.zerone.subjective-memory",
		})
		if err != nil {
			// Best-effort: a frozen run or a racing commit must not fail recall.
			continue
		}
		if decoded, err := decodeMemory(updated.Data); err == nil {
			ranked[i].RecallCount = decoded.RecallCount
		}
	}
	return ranked, nil
}

func entryToData(e subjectivememory.MemoryEntry) map[string]any {
	data := map[string]any{
		"agentId":        e.AgentID,
		"factRef":        e.FactRef,
		"interpretation": e.Interpretation,
		"importance":     e.Importance,
		"recordedAt":     e.RecordedAt.UTC().Format(time.RFC3339),
		"recallCount":    e.RecallCount,
	}
	if e.EmotionTag != "" {
		data["emotionTag"] = e.EmotionTag
	}
	return data
}

func decodeMemory(data map[string]any) (*subjectivememory.MemoryEntry, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var e subjectivememory.MemoryEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, fmt.Errorf("解析记忆状态失败: %w", err)
	}
	return &e, nil
}
