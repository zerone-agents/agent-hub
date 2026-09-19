// Package belief implements the pure domain rules of the io.zerone.belief
// capability pack (H6 pack 2). A belief is the holder's stance toward one
// delivered fact; every transition is a pure function of (previous state,
// event, event time) so a given event sequence always replays identically.
package belief

import "time"

const (
	StatusKnown     = "known"
	StatusBelieved  = "believed"
	StatusDoubted   = "doubted"
	StatusDisputed  = "disputed"
	StatusForgotten = "forgotten"
)

const (
	SourceDelivery    = "delivery"
	SourceObservation = "observation"
	SourceClaim       = "claim"
)

const (
	MinConfidence = 0
	MaxConfidence = 100
	// ForgottenThreshold mirrors the contract: confidence below 20 decays
	// the belief to forgotten. forgotten is not deletion — the audit trail
	// in run_state_changes can replay that the agent once knew.
	ForgottenThreshold = 20
	// DecayPerDay is the v1 lazy decay rate settled on read.
	DecayPerDay = 5
	// ConfirmDelta raises confidence when confirming evidence (a repeated
	// delivery of the same fact) arrives.
	ConfirmDelta = 10
	// ContradictDelta lowers confidence when contradicting evidence arrives.
	ContradictDelta = -25
	// InitialDeliveryConfidence seeds a belief the first time a fact is
	// delivered to its holder.
	InitialDeliveryConfidence = 50
	// ClaimConfidence seeds a self-initiated claim; the claimant believes
	// what they claim.
	ClaimConfidence = 70
	// MateriallyDifferentDelta is the minimum confidence gap that counts as
	// a dispute when statuses already match.
	MateriallyDifferentDelta = 10
)

// Belief is the v1 record for one (agent, factRef) pair.
type Belief struct {
	FactRef     string
	Status      string
	Confidence  int
	Source      string
	Statement   string
	LastEventAt time.Time
}

// NewFromDelivery builds the initial belief for a fact reaching its holder
// for the first time. Beliefs only ever exist for delivered facts, so an
// unknown fact can never be injected into an agent's state by construction.
func NewFromDelivery(factRef string, at time.Time) Belief {
	return Belief{FactRef: factRef, Status: StatusKnown, Confidence: InitialDeliveryConfidence, Source: SourceDelivery, LastEventAt: at}
}

// NewClaim builds the belief row backing a self-initiated claim.
func NewClaim(factRef, statement string, at time.Time) Belief {
	return Belief{FactRef: factRef, Status: StatusBelieved, Confidence: ClaimConfidence, Source: SourceClaim, Statement: statement, LastEventAt: at}
}

func ClampConfidence(c int) int {
	if c < MinConfidence {
		return MinConfidence
	}
	if c > MaxConfidence {
		return MaxConfidence
	}
	return c
}

func ValidStatus(s string) bool {
	switch s {
	case StatusKnown, StatusBelieved, StatusDoubted, StatusDisputed, StatusForgotten:
		return true
	}
	return false
}

func ValidSource(s string) bool {
	switch s {
	case SourceDelivery, SourceObservation, SourceClaim:
		return true
	}
	return false
}

// downgrade maps the current status one step down the ladder when
// contradicting evidence arrives: known|believed → doubted → disputed.
// disputed is absorbing (both sides' evidence stays in the audit trail) and
// forgotten stays forgotten until confirming evidence rebuilds confidence.
func downgrade(status string) string {
	switch status {
	case StatusDisputed, StatusForgotten:
		return status
	case StatusDoubted:
		return StatusDisputed
	default: // known | believed
		return StatusDoubted
	}
}

func advanceLastEvent(b Belief, at time.Time) Belief {
	if at.After(b.LastEventAt) {
		b.LastEventAt = at
	}
	return b
}

// ApplyEvidence is the deterministic evidence-update rule. contradict=true
// applies the downgrading ladder and a confidence drop; contradict=false is
// confirming evidence and raises confidence. Event time only moves forward.
// The ladder never reaches forgotten directly — forgotten is only settled
// by time decay (SettleDecay), so contradicting evidence keeps the stance
// auditable down to confidence 0.
func ApplyEvidence(b Belief, contradict bool, at time.Time) Belief {
	b = advanceLastEvent(b, at)
	if contradict {
		b.Status = downgrade(b.Status)
		b.Confidence = ClampConfidence(b.Confidence + ContradictDelta)
		return b
	}
	b.Confidence = ClampConfidence(b.Confidence + ConfirmDelta)
	if b.Status == StatusKnown || b.Status == StatusForgotten {
		b.Status = StatusBelieved
	}
	return b
}

// SettleDecay lazily settles decay from the recorded event time to
// toEventTime. It depends only on recorded event times, never on the read
// wall clock, so replaying the same event sequence yields the same state.
func SettleDecay(b Belief, toEventTime time.Time) Belief {
	if !toEventTime.After(b.LastEventAt) {
		return b
	}
	days := int(toEventTime.Sub(b.LastEventAt).Hours() / 24)
	if days <= 0 {
		return b
	}
	b.Confidence = ClampConfidence(b.Confidence - days*DecayPerDay)
	if b.Confidence < ForgottenThreshold {
		b.Status = StatusForgotten
	}
	b.LastEventAt = toEventTime
	return b
}

// MateriallyDifferent reports whether two holders of the same factRef hold
// stances that should surface as a dispute: different status, or a
// confidence gap of at least MateriallyDifferentDelta.
func MateriallyDifferent(a, b Belief) bool {
	if a.Status != b.Status {
		return true
	}
	d := a.Confidence - b.Confidence
	if d < 0 {
		d = -d
	}
	return d >= MateriallyDifferentDelta
}
