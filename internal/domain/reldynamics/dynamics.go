// Package reldynamics owns the io.zerone.relationship-dynamics capability
// vocabulary: per-run, directed attitudes between two agents. The legacy
// agent_relations.relationship_score column stays frozen read-only; this pack
// keeps its own score projection inside RunState only.
package reldynamics

import (
	"fmt"
	"math"
	"strings"
)

const (
	// Namespace is the capability namespace this pack owns.
	Namespace = "io.zerone.relationship-dynamics"
	// SchemaName is the v1 attitude state schema name.
	SchemaName = "relation-attitude"
	// SchemaVersion locks the v1 vocabulary for replayability.
	SchemaVersion = "v1"
	// SubjectType is the run-state subject type for a relation pair.
	SubjectType = "relation"

	MinScore = -100
	MaxScore = 100

	// single-event caps
	MaxEventDelta = 30
	MinEventDelta = -40

	// hostile gate threshold: scores strictly below this require confirmation
	// for assign-like actions.
	HostileThreshold = -60
)

// EventBaseDeltas is the pack-owned v1 vocabulary. It is semantically close to
// the legacy agentrelation.RelationEventBaseDeltas compatibility table but is
// owned (and version-locked) by this capability pack. trusted and threatened
// complete the betrayal/trust axis used by the H6 acceptance scenario.
var EventBaseDeltas = map[string]int{
	"betrayed":       -35,
	"trusted":        +12,
	"threatened":     -15,
	"aided":          +15,
	"task_completed": +10,
	"insulted":       -20,
	"cooperated":     +8,
}

// EventTypes lists the locked vocabulary in stable order (useful for tests
// and schema documentation).
func EventTypes() []string {
	types := make([]string, 0, len(EventBaseDeltas))
	for t := range EventBaseDeltas {
		types = append(types, t)
	}
	return types
}

// EventDelta returns the deterministic v1 score change for an event at the
// given severity ordinal (1..3 with multipliers 1 / 1.5 / 2). The second
// return reports whether the event type exists in the locked vocabulary.
func EventDelta(eventType string, severity int) (int, bool) {
	base, ok := EventBaseDeltas[eventType]
	if !ok {
		return 0, false
	}
	multiplier := 1.0
	switch severity {
	case 2:
		multiplier = 1.5
	case 3:
		multiplier = 2
	}
	delta := int(math.Round(float64(base) * multiplier))
	if delta > MaxEventDelta {
		return MaxEventDelta, true
	}
	if delta < MinEventDelta {
		return MinEventDelta, true
	}
	return delta, true
}

// ClampScore clamps the attitude score to the -100..100 range.
func ClampScore(score int) int {
	if score < MinScore {
		return MinScore
	}
	if score > MaxScore {
		return MaxScore
	}
	return score
}

// StanceForScore projects a score onto the v1 stance bands. Boundary values
// belong to the higher (warmer) band: -60 is wary, -20 is neutral, 20 is
// neutral, 60 is friendly.
func StanceForScore(score int) string {
	switch {
	case score < -60:
		return "hostile"
	case score < -20:
		return "wary"
	case score <= 20:
		return "neutral"
	case score <= 60:
		return "friendly"
	default:
		return "allied"
	}
}

// StanceLabelCN maps a stance to its fixed Chinese label used in narration.
var StanceLabelCN = map[string]string{
	"hostile":  "敌对",
	"wary":     "警惕",
	"neutral":  "中立",
	"friendly": "友好",
	"allied":   "同盟",
}

// Narration renders the deterministic threshold sentence for prompt
// composition, e.g. "你对 X 的态度：警惕（-40）". The agent name is supplied by
// the caller so this package stays free of lookup dependencies.
func Narration(agentName string, score int) string {
	label, ok := StanceLabelCN[StanceForScore(score)]
	if !ok {
		label = StanceLabelCN["neutral"]
	}
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = "对方"
	}
	return fmt.Sprintf("你对 %s 的态度：%s（%d）", name, label, score)
}

// AssignLikeActions are actions a counterpart may initiate toward the attitude
// holder that amount to delegating or requesting work. Under hostility they
// require the target's explicit confirmation. The set deliberately reuses the
// relation AllowedActions vocabulary from agentrelation.Actions.
var AssignLikeActions = map[string]struct{}{
	"assign":    {},
	"consult":   {},
	"submit":    {},
	"review":    {},
	"challenge": {},
	"handoff":   {},
	"escalate":  {},
	"invite":    {},
}

// UnaffectedActions are pure information-flow actions that hostility never
// blocks; they stay allowed without confirmation.
var UnaffectedActions = map[string]struct{}{
	"inform": {},
	"report": {},
}

// GateAction is the pure influence hook consulted by Core message/task paths.
// It never rewrites organization mechanics: it only decides whether an action
// the counterpart initiates needs the target's confirmation. When score is
// below the hostile threshold, assign-like actions require confirmation;
// inform-style actions are unaffected. Unknown actions are conservatively
// treated as confirm-required under hostility.
func GateAction(score int, action string) (allowed bool, needConfirm bool) {
	action = strings.ToLower(strings.TrimSpace(action))
	if _, ok := UnaffectedActions[action]; ok {
		return true, false
	}
	if score < HostileThreshold {
		if _, ok := AssignLikeActions[action]; ok {
			return true, true
		}
		// unknown action under hostility: allow but demand confirmation
		return true, true
	}
	return true, false
}

// SubjectID builds the stable per-run relation-pair key: the directed pair
// "fromAgentID:toAgentID" (attitude holder first, counterpart second).
// Decimal uint64 formatting keeps the key short, deterministic and free of any
// database-seeded relation row IDs, so it survives relation-row rewrites and
// works even when no agent_relations row exists for the pair.
func SubjectID(fromAgentID, toAgentID uint64) string {
	return fmt.Sprintf("%d:%d", fromAgentID, toAgentID)
}

// ParseSubjectID splits a pair key back into its two agent IDs.
func ParseSubjectID(subjectID string) (fromAgentID, toAgentID uint64, err error) {
	if _, err := fmt.Sscanf(subjectID, "%d:%d", &fromAgentID, &toAgentID); err != nil {
		return 0, 0, fmt.Errorf("invalid relation subject id: %s", subjectID)
	}
	return fromAgentID, toAgentID, nil
}
