package subjectivememory

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClampImportance(t *testing.T) {
	require.Equal(t, 0, ClampImportance(-10))
	require.Equal(t, 0, ClampImportance(0))
	require.Equal(t, 55, ClampImportance(55))
	require.Equal(t, 100, ClampImportance(150))
}

func TestValidate(t *testing.T) {
	require.ErrorIs(t, Validate("", "解释"), ErrEmptyFactRef)
	require.ErrorIs(t, Validate("fact", "  "), ErrEmptyInterpretation)
	long := make([]rune, MaxInterpretationRunes+1)
	for i := range long {
		long[i] = '记'
	}
	require.ErrorIs(t, Validate("fact", string(long)), ErrInterpretationTooLong)
	require.NoError(t, Validate("fact", string(long[:MaxInterpretationRunes])))
}

func TestRecencyScoreDecay(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	require.InDelta(t, 100*math.Pow(0.5, 1.0/72.0), RecencyScore(at.Add(-time.Hour), at), 0.001)
	require.InDelta(t, 50, RecencyScore(at.Add(-3*24*time.Hour), at), 0.001)
	require.InDelta(t, 25, RecencyScore(at.Add(-6*24*time.Hour), at), 0.001)
	// Future event time clamps to full recency, never negative.
	require.InDelta(t, 100, RecencyScore(at.Add(time.Hour), at), 0.001)
}

func TestRelevanceScore(t *testing.T) {
	require.InDelta(t, 50, RelevanceScore("", "f", "i"), 0.001, "empty query is topic-neutral")
	require.InDelta(t, 100, RelevanceScore("budget overrun", "f", "the budget overrun shocked us"), 0.001)
	require.InDelta(t, 50, RelevanceScore("budget silent", "f", "the budget overrun shocked us"), 0.001)
	// duplicate query tokens count once
	require.InDelta(t, 100, RelevanceScore("budget budget", "f", "budget"), 0.001)
	// CJK substrings match inside a longer contiguous run
	require.InDelta(t, 100, RelevanceScore("背叛", "f", "那次背叛让我无法再信任对方"), 0.001)
}

func TestScoreFormula(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	e := MemoryEntry{
		ID:             "m1",
		Importance:     80,
		FactRef:        "fact-1",
		Interpretation: "budget overrun",
		RecordedAt:     at.Add(-24 * time.Hour),
	}
	// recency at 1 day = 100*0.5^(1/3) ≈ 79.37, relevance = 100
	got := Score(e, "budget", at)
	want := 0.5*80 + 0.3*RecencyScore(e.RecordedAt, at) + 0.2*100
	require.InDelta(t, want, got, 0.001)
	// exact weights per contract: importance dominates relevance dominates recency ordering
	lowImp := e
	lowImp.Importance = 0
	require.Less(t, Score(lowImp, "budget", at), got)
	noRel := e
	noRel.FactRef, noRel.Interpretation = "x", "y"
	require.Less(t, Score(noRel, "unrelated", at), got)
}

func TestSoftForgotten(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	old := MemoryEntry{ID: "m", Importance: 10, RecordedAt: at.Add(-10 * 24 * time.Hour), RecallCount: 0}
	require.True(t, SoftForgotten(old, at))
	// importance above the bar keeps it afloat even when old and unrecalled
	hot := old
	hot.Importance = 21
	require.False(t, SoftForgotten(hot, at))
	// recalled enough resurfaces it
	recalled := old
	recalled.RecallCount = 2
	require.False(t, SoftForgotten(recalled, at))
	// recent enough resurfaces it
	fresh := old
	fresh.RecordedAt = at.Add(-24 * time.Hour)
	require.False(t, SoftForgotten(fresh, at))
}

func TestRankOrderingAndLimit(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	old := MemoryEntry{ID: "old", Importance: 90, Interpretation: "critical but ancient", RecordedAt: at.Add(-100 * 24 * time.Hour), RecallCount: 0}
	freshHigh := MemoryEntry{ID: "fresh-high", Importance: 90, Interpretation: "recent and important", RecordedAt: at.Add(-time.Hour), RecallCount: 3}
	relevant := MemoryEntry{ID: "relevant", Importance: 60, Interpretation: "about the budget meeting", RecordedAt: at.Add(-2 * 24 * time.Hour), RecallCount: 1}
	sunk := MemoryEntry{ID: "sunk", Importance: 5, Interpretation: "trivial", RecordedAt: at.Add(-30 * 24 * time.Hour), RecallCount: 0}

	ranked := Rank([]MemoryEntry{sunk, relevant, old, freshHigh}, "budget", at, 5)
	require.Len(t, ranked, 3, "soft-forgotten entry must sink below the cutoff")
	require.Equal(t, "fresh-high", ranked[0].ID)
	require.Equal(t, "relevant", ranked[1].ID)
	require.Equal(t, "old", ranked[2].ID)

	limited := Rank([]MemoryEntry{sunk, relevant, old, freshHigh}, "budget", at, 2)
	require.Len(t, limited, 2)
	require.Equal(t, "fresh-high", limited[0].ID)

	// default limit kicks in on non-positive values
	defaulted := Rank([]MemoryEntry{sunk, relevant, old, freshHigh}, "budget", at, 0)
	require.Len(t, defaulted, 3)
}

func TestRankDeterministicTies(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	a := MemoryEntry{ID: "a", Importance: 50, RecordedAt: at, RecallCount: 0}
	b := MemoryEntry{ID: "b", Importance: 50, RecordedAt: at, RecallCount: 0}
	first := Rank([]MemoryEntry{b, a}, "", at, 10)
	second := Rank([]MemoryEntry{a, b}, "", at, 10)
	require.Equal(t, first, second, "same inputs must replay to the same order")
}
