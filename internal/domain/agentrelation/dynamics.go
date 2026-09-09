package agentrelation

import "math"

const (
	MinRelationshipScore = -100
	MaxRelationshipScore = 100
	DefaultEventSeverity = 1
	RelationshipRuleV1   = "v1"
)

// StanceForScore deliberately uses broad bands so ordinary interactions do
// not flip the visible stance on every turn. The numeric score remains
// available to the agent and UI for gradual changes inside a band.
func StanceForScore(score int) string {
	switch {
	case score >= 70:
		return "allied"
	case score >= 25:
		return "friendly"
	case score > -25:
		return "neutral"
	case score > -45:
		return "wary"
	case score > -70:
		return "competitive"
	default:
		return "hostile"
	}
}

// InitialScoreForStance preserves the old stance selector as a convenient
// starting condition for newly-created and migrated relationships.
func InitialScoreForStance(stance string) int {
	switch stance {
	case "allied":
		return 80
	case "friendly":
		return 40
	case "wary":
		return -30
	case "competitive":
		return -55
	case "hostile":
		return -80
	default:
		return 0
	}
}

// EventDelta returns a deterministic v1 score change. Severity is an ordinal
// 1..3 scale with bounded multipliers; the final cap prevents one subjective
// signal from taking a relationship from one extreme to the other.
func EventDelta(eventType string, severity int) (int, bool) {
	base, ok := RelationEventBaseDeltas[eventType]
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
	if delta > 30 {
		return 30, true
	}
	if delta < -40 {
		return -40, true
	}
	return delta, true
}

func ClampRelationshipScore(score int) int {
	if score < MinRelationshipScore {
		return MinRelationshipScore
	}
	if score > MaxRelationshipScore {
		return MaxRelationshipScore
	}
	return score
}
