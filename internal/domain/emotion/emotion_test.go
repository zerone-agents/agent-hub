package emotion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventDeltaVocabularyAndCaps(t *testing.T) {
	delta, ok := EventDelta("praised", 1)
	require.True(t, ok)
	require.Equal(t, 12, delta)

	// severity 2 → ×1.5，severity 3 → ×2
	delta, _ = EventDelta("praised", 2)
	require.Equal(t, 18, delta)
	delta, _ = EventDelta("praised", 3)
	require.Equal(t, 24, delta)

	// 单事件上限 +30 / -40
	delta, _ = EventDelta("trusted", 3) // 15×2=30
	require.Equal(t, 30, delta)
	delta, _ = EventDelta("betrayed", 2) // -30×1.5=-45 → -40
	require.Equal(t, -40, delta)

	_, ok = EventDelta("not_a_event", 1)
	require.False(t, ok)
}

func TestApplyEventClampsIntensityAndProjectsMood(t *testing.T) {
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	s := DefaultState("")

	// 连续高强度正面事件：+24×3 = 72，mood 取事件指向
	var err error
	for i := 0; i < 3; i++ {
		s, err = ApplyEvent(s, "praised", 3, at)
		require.NoError(t, err)
	}
	require.Equal(t, 72, s.Intensity)
	require.Equal(t, MoodElated, s.Mood)

	// 叠加舒适事件：+12 → 84，mood 取 comforted 的指向（calm）
	s, err = ApplyEvent(s, "comforted", 1, at)
	require.NoError(t, err)
	require.Equal(t, MoodCalm, s.Mood)
	require.Equal(t, 84, s.Intensity)

	_, err = ApplyEvent(s, "bogus", 1, at)
	require.ErrorIs(t, err, ErrUnknownEventType)
}

func TestSettleDecayIsPureFunctionOfTimes(t *testing.T) {
	from := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	s := DefaultState("")
	s.Intensity = 80
	s.Mood = MoodAngry
	s.UpdatedAt = from

	to := from.Add(48 * time.Hour) // 2 天 × 20 = 40
	settled := SettleDecay(s, from, to)
	require.Equal(t, 40, settled.Intensity)
	require.Equal(t, to, settled.UpdatedAt)
	require.Equal(t, MoodAngry, settled.Mood, "未回落到平静档时 mood 保持")

	further := settled.UpdatedAt.Add(36 * time.Hour) // 再 1.5 天 × 20 = 30
	settled = SettleDecay(settled, settled.UpdatedAt, further)
	require.Equal(t, 10, settled.Intensity)
	require.Equal(t, MoodCalm, settled.Mood, "回到平静档后 mood 归位基线")

	// to 不晚于 from：原样返回
	again := SettleDecay(settled, further, further.Add(-time.Hour))
	require.Equal(t, settled.Intensity, again.Intensity)

	// 同一输入重放结果一致（纯函数）
	replay := SettleDecay(s, from, to)
	require.Equal(t, 40, replay.Intensity)
}

func TestNarrationForUsesFixedPatterns(t *testing.T) {
	require.Contains(t, NarrationFor(MoodCalm, 10), "情绪平稳")
	require.Contains(t, NarrationFor(MoodWary, 30), "些许波动")
	require.Contains(t, NarrationFor(MoodTense, 55), "显著起伏")
	require.Contains(t, NarrationFor(MoodAngry, 70), "情绪激动")
	require.Contains(t, NarrationFor(MoodGrieving, 90), "主导了你的行为")
	require.Contains(t, NarrationFor(MoodElated, 40), "心情为振奋")
}

func TestStateMapRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	s := DefaultState("")
	next, err := ApplyEvent(s, "betrayed", 2, at)
	require.NoError(t, err)

	parsed, err := StateFromData(next.ToMap())
	require.NoError(t, err)
	require.Equal(t, next.Mood, parsed.Mood)
	require.Equal(t, next.Intensity, parsed.Intensity)
	require.Equal(t, next.Baseline, parsed.Baseline)
	require.Equal(t, next.DecayPerDay, parsed.DecayPerDay)
	require.Equal(t, next.Narration, parsed.Narration)
	require.True(t, next.UpdatedAt.Equal(parsed.UpdatedAt))
}

func TestDefaultStateBaselineFallback(t *testing.T) {
	s := DefaultState("")
	require.Equal(t, MoodCalm, s.Baseline)
	require.Equal(t, MoodCalm, s.Mood)
	require.Equal(t, 0, s.Intensity)
	require.Equal(t, DefaultDecayPerDay, s.DecayPerDay)
	require.NotEmpty(t, s.Narration)

	custom := DefaultState(MoodWary)
	require.Equal(t, MoodWary, custom.Baseline)
}
