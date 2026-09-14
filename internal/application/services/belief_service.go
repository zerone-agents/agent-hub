package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/domain/belief"
	rundomain "control-panel/internal/domain/run"
)

// BeliefService wires the io.zerone.belief capability pack to run-scoped
// state. Each (agent, factRef) pair is one RunState row in the belief-state
// v1 schema; the RunState unique index has no agent column, so the agent id
// is encoded into SubjectID as "<agentID>/<factRef>" while SubjectType stays
// "agent" per the H6 contract.
const (
	BeliefNamespace     = "io.zerone.belief"
	BeliefSchemaName    = "belief-state"
	BeliefSchemaVersion = "v1"

	beliefSubjectType = "agent"
	beliefSourceTag   = "belief"
)

type BeliefService struct {
	run *RunService
}

func NewBeliefService(runService *RunService) *BeliefService {
	return &BeliefService{run: runService}
}

// Belief is the service-level view of one belief row.
type Belief struct {
	StateID     uint64
	FactRef     string
	Status      string
	Confidence  int
	Source      string
	Statement   string
	LastEventAt time.Time
	Revision    uint64
}

// DisputeEntry is one holder's stance within a disputed factRef.
type DisputeEntry struct {
	AgentID    uint64 `json:"agentId"`
	Status     string `json:"status"`
	Confidence int    `json:"confidence"`
}

// Dispute is a factRef held with materially different stances by ≥2 agents.
type Dispute struct {
	FactRef string         `json:"factRef"`
	Entries []DisputeEntry `json:"entries"`
}

// beliefStateSchema is the locked v1 JSON Schema for the belief-state rows.
func beliefStateSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"factRef":     map[string]any{"type": "string", "minLength": 1},
			"status":      map[string]any{"type": "string", "enum": []any{belief.StatusKnown, belief.StatusBelieved, belief.StatusDoubted, belief.StatusDisputed, belief.StatusForgotten}},
			"confidence":  map[string]any{"type": "integer", "minimum": 0, "maximum": 100},
			"source":      map[string]any{"type": "string", "enum": []any{belief.SourceDelivery, belief.SourceObservation, belief.SourceClaim}},
			"lastEventAt": map[string]any{"type": "string", "format": "date-time"},
			"statement":   map[string]any{"type": "string", "maxLength": 500},
		},
		"required": []any{"factRef", "status", "confidence", "source", "lastEventAt"},
	}
}

// EnsureSchemas validates the v1 state definition. Schema rows are
// tenant-scoped in run_states, so per-tenant registration happens lazily on
// the first write for that tenant (see ensureBeliefSchema); this entry point
// is idempotent and safe to call on every startup.
func (s *BeliefService) EnsureSchemas() error {
	sample := map[string]any{
		"factRef":     "fact:sample",
		"status":      belief.StatusKnown,
		"confidence":  belief.InitialDeliveryConfidence,
		"source":      belief.SourceDelivery,
		"lastEventAt": time.Now().UTC().Format(time.RFC3339),
	}
	if err := validateJSONSchema(beliefStateSchema(), sample); err != nil {
		return fmt.Errorf("belief 状态定义无效: %w", err)
	}
	return nil
}

// ensureBeliefSchema registers the belief-state v1 schema for one tenant,
// treating an existing registration as success.
func (s *BeliefService) ensureBeliefSchema(tenantID string) error {
	_, err := s.run.RegisterStateSchema(tenantID, RegisterStateSchemaInput{
		Namespace:    BeliefNamespace,
		Name:         BeliefSchemaName,
		Version:      BeliefSchemaVersion,
		Schema:       beliefStateSchema(),
		ScopeTypes:   []string{"run"},
		SubjectTypes: []string{beliefSubjectType},
	})
	if err == nil || errors.Is(err, rundomain.ErrDuplicate) {
		return nil
	}
	return err
}

func beliefSubjectID(agentID uint64, factRef string) string {
	return fmt.Sprintf("%d/%s", agentID, factRef)
}

func parseBeliefSubjectID(subjectID string) (uint64, string, bool) {
	idx := strings.Index(subjectID, "/")
	if idx <= 0 {
		return 0, "", false
	}
	agentID, err := strconv.ParseUint(subjectID[:idx], 10, 64)
	if err != nil {
		return 0, "", false
	}
	return agentID, subjectID[idx+1:], true
}

func beliefToData(b belief.Belief) map[string]any {
	return map[string]any{
		"factRef":     b.FactRef,
		"status":      b.Status,
		"confidence":  b.Confidence,
		"source":      b.Source,
		"lastEventAt": b.LastEventAt.UTC().Format(time.RFC3339),
		"statement":   b.Statement,
	}
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

func beliefFromData(state *rundomain.RunState) (belief.Belief, error) {
	d := state.Data
	factRef, _ := d["factRef"].(string)
	status, _ := d["status"].(string)
	source, _ := d["source"].(string)
	statement, _ := d["statement"].(string)
	lastEventAt, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", d["lastEventAt"]))
	if err != nil {
		return belief.Belief{}, fmt.Errorf("belief 状态损坏（lastEventAt 无法解析）: %w", err)
	}
	b := belief.Belief{FactRef: factRef, Status: status, Confidence: intFromAny(d["confidence"]), Source: source, Statement: statement, LastEventAt: lastEventAt}
	if b.FactRef == "" || !belief.ValidStatus(b.Status) || !belief.ValidSource(b.Source) {
		return belief.Belief{}, fmt.Errorf("belief 状态损坏（字段非法）")
	}
	return b, nil
}

// findBeliefState locates the run-scoped state row for one (agent, factRef).
func (s *BeliefService) findBeliefState(tenantID, runID string, subjectID string) (*rundomain.RunState, error) {
	states, err := s.run.States(tenantID, runID)
	if err != nil {
		return nil, err
	}
	for i := range states {
		st := &states[i]
		if st.Namespace == BeliefNamespace && st.SubjectType == beliefSubjectType && st.SubjectID == subjectID {
			return st, nil
		}
	}
	return nil, nil
}

func validateBeliefEvent(factRef string, at time.Time, idempotencyKey string) error {
	if strings.TrimSpace(factRef) == "" {
		return fmt.Errorf("事实引用不能为空")
	}
	if at.IsZero() {
		return fmt.Errorf("事件时间不能为空")
	}
	if idempotencyKey == "" {
		return fmt.Errorf("幂等键不能为空")
	}
	return nil
}

// RecordDelivery records that a fact was delivered to the agent. The first
// delivery creates the belief row (source=delivery); a replayed idempotency
// key is a no-op, and a genuinely repeated delivery of the same fact is
// confirming evidence that raises confidence. Beliefs only ever exist for
// delivered facts, so unknown-fact injection is impossible by construction.
func (s *BeliefService) RecordDelivery(tenantID, runID string, agentID uint64, factRef string, at time.Time, idempotencyKey string) error {
	factRef = strings.TrimSpace(factRef)
	if err := validateBeliefEvent(factRef, at, idempotencyKey); err != nil {
		return err
	}
	if err := s.ensureBeliefSchema(tenantID); err != nil {
		return err
	}
	subjectID := beliefSubjectID(agentID, factRef)
	state, err := s.findBeliefState(tenantID, runID, subjectID)
	if err != nil {
		return err
	}
	if state == nil {
		b := belief.NewFromDelivery(factRef, at)
		_, err = s.run.InitializeState(tenantID, runID, InitializeStateInput{
			Namespace:      BeliefNamespace,
			SchemaName:     BeliefSchemaName,
			SchemaVersion:  BeliefSchemaVersion,
			SubjectType:    beliefSubjectType,
			SubjectID:      subjectID,
			Data:           beliefToData(b),
			IdempotencyKey: idempotencyKey,
			Reason:         "事实首次送达，建立看法",
			Source:         beliefSourceTag,
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, rundomain.ErrDuplicate) {
			return err
		}
		// Concurrent creation won the race; load the row and settle the
		// repeated delivery as confirming evidence below.
		state, err = s.findBeliefState(tenantID, runID, subjectID)
		if err != nil {
			return err
		}
		if state == nil {
			return nil
		}
	}
	b, err := beliefFromData(state)
	if err != nil {
		return err
	}
	next := belief.ApplyEvidence(belief.SettleDecay(b, at), false, at)
	_, err = s.run.CommitState(tenantID, runID, state.ID, CommitStateInput{
		ExpectedRevision: state.Revision,
		Data:             beliefToData(next),
		IdempotencyKey:   idempotencyKey,
		Reason:           "事实重复送达，证据增强",
		Source:           beliefSourceTag,
	})
	if errors.Is(err, rundomain.ErrConflict) {
		// A concurrent write already settled this delivery.
		return nil
	}
	return err
}

// List returns the agent's own beliefs in the run, optionally filtered by
// factRef. forgotten beliefs are excluded by default; use listBeliefs with
// includeForgotten=true to audit them. Lazy decay is settled on read.
func (s *BeliefService) List(tenantID, runID string, agentID uint64, factRef string) ([]Belief, error) {
	return s.listBeliefs(tenantID, runID, agentID, factRef, false)
}

func (s *BeliefService) listBeliefs(tenantID, runID string, agentID uint64, factRef string, includeForgotten bool) ([]Belief, error) {
	states, err := s.run.States(tenantID, runID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]Belief, 0, len(states))
	for i := range states {
		st := &states[i]
		if st.Namespace != BeliefNamespace || st.SubjectType != beliefSubjectType {
			continue
		}
		ag, ref, ok := parseBeliefSubjectID(st.SubjectID)
		if !ok || ag != agentID {
			continue
		}
		if factRef != "" && ref != factRef {
			continue
		}
		b, err := beliefFromData(st)
		if err != nil {
			return nil, err
		}
		settled := belief.SettleDecay(b, now)
		if settled.Status == belief.StatusForgotten && !includeForgotten {
			continue
		}
		if settled != b {
			// Persist the lazy settlement; ignore races — the next read
			// settles again from the stored event time.
			_, _ = s.run.CommitState(tenantID, runID, st.ID, CommitStateInput{
				ExpectedRevision: st.Revision,
				Data:             beliefToData(settled),
				IdempotencyKey:   fmt.Sprintf("belief:decay:%d:%d", st.ID, st.Revision),
				Reason:           "读取时惰性结算衰减",
				Source:           beliefSourceTag,
			})
		}
		out = append(out, Belief{StateID: st.ID, FactRef: settled.FactRef, Status: settled.Status, Confidence: settled.Confidence, Source: settled.Source, Statement: settled.Statement, LastEventAt: settled.LastEventAt, Revision: st.Revision})
	}
	return out, nil
}

// AddEvidence applies one evidence event to the agent's existing belief
// about factRef per the pure pack rules. A fact the agent never received has
// no row, so adding evidence to it fails instead of fabricating a belief.
func (s *BeliefService) AddEvidence(tenantID, runID string, agentID uint64, factRef string, contradict bool, at time.Time, idempotencyKey string) error {
	factRef = strings.TrimSpace(factRef)
	if err := validateBeliefEvent(factRef, at, idempotencyKey); err != nil {
		return err
	}
	state, err := s.findBeliefState(tenantID, runID, beliefSubjectID(agentID, factRef))
	if err != nil {
		return err
	}
	if state == nil {
		return fmt.Errorf("该事实尚未送达，无法更新看法")
	}
	b, err := beliefFromData(state)
	if err != nil {
		return err
	}
	next := belief.ApplyEvidence(belief.SettleDecay(b, at), contradict, at)
	reason := "证据确认，信心增强"
	if contradict {
		reason = "矛盾证据到达，看法降档"
	}
	_, err = s.run.CommitState(tenantID, runID, state.ID, CommitStateInput{
		ExpectedRevision: state.Revision,
		Data:             beliefToData(next),
		IdempotencyKey:   idempotencyKey,
		Reason:           reason,
		Source:           beliefSourceTag,
	})
	return err
}

// Claim records an agent's self-initiated claim. It creates a claim-scoped
// belief row (source=claim) keyed by a deterministic factRef derived from the
// parent fact and statement, and returns that factRef. Replays return the
// same factRef without side effects.
func (s *BeliefService) Claim(tenantID, runID string, agentID uint64, factRef string, statement string, at time.Time, idempotencyKey string) (string, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return "", fmt.Errorf("声称内容不能为空")
	}
	if err := validateBeliefEvent(factRef, at, idempotencyKey); err != nil {
		return "", err
	}
	if err := s.ensureBeliefSchema(tenantID); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(factRef + "\n" + statement))
	newFactRef := "claim:" + hex.EncodeToString(sum[:])[:16]
	subjectID := beliefSubjectID(agentID, newFactRef)
	state, err := s.findBeliefState(tenantID, runID, subjectID)
	if err != nil {
		return "", err
	}
	if state != nil {
		return newFactRef, nil
	}
	b := belief.NewClaim(newFactRef, statement, at)
	_, err = s.run.InitializeState(tenantID, runID, InitializeStateInput{
		Namespace:      BeliefNamespace,
		SchemaName:     BeliefSchemaName,
		SchemaVersion:  BeliefSchemaVersion,
		SubjectType:    beliefSubjectType,
		SubjectID:      subjectID,
		Data:           beliefToData(b),
		IdempotencyKey: idempotencyKey,
		Reason:         "提交声称",
		Source:         beliefSourceTag,
	})
	if errors.Is(err, rundomain.ErrDuplicate) {
		return newFactRef, nil
	}
	if err != nil {
		return "", err
	}
	return newFactRef, nil
}

// Disputes lists factRefs held with materially different stances by ≥2 agents
// in the run (the "两个角色对同一丑闻认知不同" admin view). forgotten beliefs
// are excluded: the holder no longer actively holds the stance.
func (s *BeliefService) Disputes(tenantID, runID string) ([]Dispute, error) {
	states, err := s.run.States(tenantID, runID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	holders := map[string][]DisputeEntry{}
	for i := range states {
		st := &states[i]
		if st.Namespace != BeliefNamespace || st.SubjectType != beliefSubjectType {
			continue
		}
		agentID, ref, ok := parseBeliefSubjectID(st.SubjectID)
		if !ok {
			continue
		}
		b, err := beliefFromData(st)
		if err != nil {
			return nil, err
		}
		settled := belief.SettleDecay(b, now)
		if settled.Status == belief.StatusForgotten {
			continue
		}
		holders[ref] = append(holders[ref], DisputeEntry{AgentID: agentID, Status: settled.Status, Confidence: settled.Confidence})
	}
	factRefs := make([]string, 0, len(holders))
	for ref := range holders {
		factRefs = append(factRefs, ref)
	}
	sort.Strings(factRefs)
	out := make([]Dispute, 0)
	for _, ref := range factRefs {
		entries := holders[ref]
		if len(entries) < 2 {
			continue
		}
		conflict := false
		for i := 0; i < len(entries) && !conflict; i++ {
			for j := i + 1; j < len(entries); j++ {
				a := belief.Belief{Status: entries[i].Status, Confidence: entries[i].Confidence}
				c := belief.Belief{Status: entries[j].Status, Confidence: entries[j].Confidence}
				if belief.MateriallyDifferent(a, c) {
					conflict = true
					break
				}
			}
		}
		if conflict {
			out = append(out, Dispute{FactRef: ref, Entries: entries})
		}
	}
	return out, nil
}
