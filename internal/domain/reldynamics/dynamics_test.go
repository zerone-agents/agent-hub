package reldynamics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventDeltaVocabularyAndSeverity(t *testing.T) {
	cases := []struct {
		event    string
		severity int
		want     int
	}{
		{"betrayed", 1, -35},
		{"betrayed", 2, -40}, // -52.5 already below the -40 cap
		{"betrayed", 3, -40}, // -70 capped at -40
		{"trusted", 1, 12},
		{"threatened", 1, -15},
		{"aided", 1, 15},
		{"aided", 2, 23}, // 22.5 rounds half away from zero
		{"task_completed", 1, 10},
		{"insulted", 1, -20},
		{"cooperated", 1, 8},
		{"cooperated", 3, 16}, // 8*2 (severity 3 multiplier is 2)
	}
	for _, c := range cases {
		got, ok := EventDelta(c.event, c.severity)
		require.True(t, ok, c.event)
		require.Equal(t, c.want, got, "%s severity %d", c.event, c.severity)
	}
	_, ok := EventDelta("nonexistent", 1)
	require.False(t, ok)
}

func TestEventDeltaCapsAndClamp(t *testing.T) {
	// caps
	got, _ := EventDelta("betrayed", 3)
	require.Equal(t, MinEventDelta, got)
	got, _ = EventDelta("aided", 3)
	require.Equal(t, MaxEventDelta, got)
	// clamp
	require.Equal(t, MinScore, ClampScore(-250))
	require.Equal(t, MaxScore, ClampScore(250))
	require.Equal(t, 0, ClampScore(0))
}

func TestStanceBandsV1(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{-100, "hostile"},
		{-61, "hostile"},
		{-60, "wary"}, // boundary belongs to the warmer band
		{-40, "wary"},
		{-21, "wary"},
		{-20, "neutral"},
		{0, "neutral"},
		{20, "neutral"},
		{21, "friendly"},
		{60, "friendly"},
		{61, "allied"},
		{100, "allied"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, StanceForScore(c.score), "score %d", c.score)
	}
}

func TestNarrationSentence(t *testing.T) {
	require.Equal(t, "你对 阿伟 的态度：警惕（-40）", Narration("阿伟", -40))
	require.Equal(t, "你对 对方 的态度：中立（0）", Narration("  ", 0))
	require.Equal(t, "你对 对方 的态度：敌对（-80）", Narration("", -80))
}

func TestGateActionMatrix(t *testing.T) {
	// hostile score: assign-like actions need confirmation
	allowed, confirm := GateAction(-80, "assign")
	require.True(t, allowed)
	require.True(t, confirm)
	for _, action := range []string{"consult", "submit", "review", "challenge", "handoff", "escalate", "invite"} {
		allowed, confirm = GateAction(-80, action)
		require.True(t, allowed, action)
		require.True(t, confirm, action)
	}
	// inform/report unaffected even under hostility
	for _, action := range []string{"inform", "report"} {
		allowed, confirm = GateAction(-80, action)
		require.True(t, allowed, action)
		require.False(t, confirm, action)
	}
	// non-hostile scores: everything passes without confirmation
	for _, score := range []int{-60, -40, 0, 40, 80} {
		allowed, confirm = GateAction(score, "assign")
		require.True(t, allowed)
		require.False(t, confirm)
	}
	// unknown action under hostility is conservatively confirm-required
	allowed, confirm = GateAction(-80, "unknown_action")
	require.True(t, allowed)
	require.True(t, confirm)
	// case-insensitive
	_, confirm = GateAction(-80, "Assign")
	require.True(t, confirm)
}

func TestSubjectIDRoundTrip(t *testing.T) {
	require.Equal(t, "7:42", SubjectID(7, 42))
	from, to, err := ParseSubjectID("7:42")
	require.NoError(t, err)
	require.Equal(t, uint64(7), from)
	require.Equal(t, uint64(42), to)
	_, _, err = ParseSubjectID("bad")
	require.Error(t, err)
}
