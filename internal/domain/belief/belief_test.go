package belief

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var testBase = time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

func TestNewFromDeliverySeedsKnownAtInitialConfidence(t *testing.T) {
	b := NewFromDelivery("fact:scandal", testBase)
	require.Equal(t, "fact:scandal", b.FactRef)
	require.Equal(t, StatusKnown, b.Status)
	require.Equal(t, InitialDeliveryConfidence, b.Confidence)
	require.Equal(t, SourceDelivery, b.Source)
	require.Equal(t, testBase, b.LastEventAt)
}

func TestConfirmingEvidenceRaisesConfidenceAndPromotesStatus(t *testing.T) {
	b := NewFromDelivery("fact:a", testBase)
	at := testBase.Add(time.Hour)
	b = ApplyEvidence(b, false, at)
	require.Equal(t, StatusBelieved, b.Status)
	require.Equal(t, InitialDeliveryConfidence+ConfirmDelta, b.Confidence)
	require.Equal(t, at, b.LastEventAt)
}

func TestContradictingEvidenceDowngradesLadder(t *testing.T) {
	b := NewFromDelivery("fact:a", testBase)
	b = ApplyEvidence(b, true, testBase.Add(time.Hour))
	require.Equal(t, StatusDoubted, b.Status)
	require.Equal(t, InitialDeliveryConfidence+ContradictDelta, b.Confidence)
	b = ApplyEvidence(b, true, testBase.Add(2*time.Hour))
	require.Equal(t, StatusDisputed, b.Status)
	// disputed is absorbing: further contradiction keeps the status.
	b = ApplyEvidence(b, true, testBase.Add(3*time.Hour))
	require.Equal(t, StatusDisputed, b.Status)
	require.Equal(t, 0, b.Confidence)
}

func TestConfidenceDroppingBelowThresholdForgetsOnlyViaDecay(t *testing.T) {
	// Contradiction alone never forgets: it only walks the ladder, even
	// down to confidence 0.
	b := NewFromDelivery("fact:a", testBase)
	b = ApplyEvidence(b, true, testBase.Add(time.Hour))
	require.Equal(t, StatusDoubted, b.Status)
	b = ApplyEvidence(b, true, testBase.Add(2*time.Hour))
	require.Equal(t, StatusDisputed, b.Status)
	require.Equal(t, 0, b.Confidence)
	// forgotten is reached only when time decay settles confidence below 20.
	b = SettleDecay(b, testBase.Add(3*24*time.Hour))
	require.Equal(t, StatusForgotten, b.Status)
	require.Equal(t, 0, b.Confidence)
}

func TestConfirmingEvidenceAfterForgottenReconstructsBelief(t *testing.T) {
	// Decay 50 → 15 < 20 over seven days → forgotten.
	b := NewFromDelivery("fact:a", testBase)
	b = SettleDecay(b, testBase.Add(7*24*time.Hour))
	require.Equal(t, StatusForgotten, b.Status)
	// 15 + 10 = 25 ≥ 20 → the belief is rebuilt as believed.
	b = ApplyEvidence(b, false, testBase.Add(7*24*time.Hour+time.Hour))
	require.Equal(t, StatusBelieved, b.Status)
	require.Equal(t, 25, b.Confidence)
}

func TestSettleDecayIsPureFunctionOfEventTimes(t *testing.T) {
	b := NewFromDelivery("fact:a", testBase)
	// Two days at DecayPerDay → 50-10 = 40, still remembered.
	settled := SettleDecay(b, testBase.Add(48*time.Hour))
	require.Equal(t, InitialDeliveryConfidence-2*DecayPerDay, settled.Confidence)
	require.Equal(t, StatusKnown, settled.Status)
	// Seven days → 50-35 = 15 < 20 → forgotten.
	settled = SettleDecay(b, testBase.Add(7*24*time.Hour))
	require.Equal(t, StatusForgotten, settled.Status)
	// Earlier read time is a no-op.
	require.Equal(t, b, SettleDecay(b, testBase.Add(-time.Hour)))
	require.Equal(t, b, SettleDecay(b, testBase.Add(12*time.Hour)))
}

func TestEventTimeOnlyMovesForward(t *testing.T) {
	b := NewFromDelivery("fact:a", testBase.Add(2*time.Hour))
	b = ApplyEvidence(b, true, testBase)
	require.Equal(t, testBase.Add(2*time.Hour), b.LastEventAt)
}

func TestMateriallyDifferent(t *testing.T) {
	a := NewFromDelivery("fact:a", testBase)
	b := a
	require.False(t, MateriallyDifferent(a, b))
	b = ApplyEvidence(b, false, testBase.Add(time.Hour))
	require.True(t, MateriallyDifferent(a, b), "status ladder differs")
	c := Belief{Status: StatusBelieved, Confidence: 71, LastEventAt: testBase}
	require.True(t, MateriallyDifferent(b, c), "confidence gap >= 10")
	d := Belief{Status: StatusBelieved, Confidence: 66, LastEventAt: testBase}
	require.False(t, MateriallyDifferent(b, d), "confidence gap < 10 with same status")
}
