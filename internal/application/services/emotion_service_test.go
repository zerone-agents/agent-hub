package services

import (
	"testing"
	"time"

	"control-panel/internal/domain/emotion"
	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
)

func newEmotionTestService(t *testing.T) (*EmotionService, *RunService) {
	t.Helper()
	rs, _ := newRunTestService(t)
	return NewEmotionService(rs), rs
}

// pinEmotionClock 把服务的"当前时间"钉在 now 上。Status 的惰性衰减结算
// 以该时间为准，不钉住的话断言值会随真实时间推移而漂移。
func pinEmotionClock(es *EmotionService, now time.Time) {
	es.now = func() time.Time { return now }
}

func TestEmotionServiceEnsureSchemasIdempotent(t *testing.T) {
	es, rs := newEmotionTestService(t)
	_, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)

	require.NoError(t, es.EnsureSchemas())
	require.NoError(t, es.EnsureSchemas(), "重复注册必须是幂等的")

	var count int64
	require.NoError(t, rs.db.Model(&rundomain.StateSchema{}).
		Where("tenant_id = ? AND namespace = ? AND name = ? AND version = ?", "t1", emotion.Namespace, emotion.SchemaName, emotion.SchemaVersion).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestEmotionServiceStatusDefaultsToBaseline(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)

	status, err := es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	require.Equal(t, emotion.MoodCalm, status.Mood)
	require.Equal(t, emotion.MoodCalm, status.Baseline)
	require.Equal(t, 0, status.Intensity)
	require.NotEmpty(t, status.Narration)
}

func TestEmotionServiceOnEventAppliesVocabularyAndIsIdempotent(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	// 时钟钉在事件时间上：Status 读取不做衰减，期望值不随真实时间改变。
	pinEmotionClock(es, at)

	require.NoError(t, es.OnEvent("t1", r.ID, 7, "praised", 3, at, "evt-1")) // +24
	status, err := es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	require.Equal(t, 24, status.Intensity)
	require.Equal(t, emotion.MoodElated, status.Mood)
	require.Contains(t, status.Narration, "心情为振奋")

	// 重复投递同一 idempotencyKey：无操作，不产生第二次效果
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "praised", 3, at, "evt-1"))
	status, err = es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	require.Equal(t, 24, status.Intensity)

	// 新事件正常叠加
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "insulted", 2, at.Add(time.Hour), "evt-2")) // -30 → -6
	status, err = es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	require.Equal(t, -6, status.Intensity)
	// |−6| ≤ 20 属平静档，叙述保持"情绪平稳"，但强度已为负向
	require.Equal(t, emotion.MoodCalm, status.Mood)
}

func TestEmotionServiceOnEventRejectsUnknownVocabulary(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)
	err = es.OnEvent("t1", r.ID, 7, "random_mood_swing", 1, time.Now(), "evt-x")
	require.ErrorIs(t, err, emotion.ErrUnknownEventType)
}

func TestEmotionServiceDecaySettlesByEventTime(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)
	// 注意：Status 读取时按"当前时间"结算衰减，因此事件时间必须以 now
	// 为锚，不能用写死的过去日期；时钟同样钉在 now 上保证可复现。
	now := time.Now().UTC()
	pinEmotionClock(es, now)
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "betrayed", 3, now.Add(-48*time.Hour), "evt-b")) // -40
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "threatened", 3, now, "evt-t"))
	// 48h × 20/day = 40 衰减后回到 0，再 -40 → 负面情绪重新生效
	status, err := es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	require.Equal(t, -40, status.Intensity)
	require.Equal(t, emotion.MoodTense, status.Mood)
}

func TestEmotionServiceDecayTowardBaselineAcrossReads(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)
	now := time.Now().UTC()
	at := now.Add(-72 * time.Hour) // 强度 40 在 3 天前应已衰减到 0
	pinEmotionClock(es, now)

	require.NoError(t, es.OnEvent("t1", r.ID, 7, "betrayed", 2, at, "evt-old")) // -40（×1.5 触顶 -40）
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "praised", 1, now, "evt-new"))
	status, err := es.Status("t1", r.ID, 7)
	require.NoError(t, err)
	// 旧事件已完全衰减，新事件 +12
	require.Equal(t, 12, status.Intensity)
}

func TestEmotionServiceRunIsolation(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r1, err := rs.Create("t1", CreateRunInput{Name: "run-one"})
	require.NoError(t, err)
	r2, err := rs.Create("t1", CreateRunInput{Name: "run-two"})
	require.NoError(t, err)
	at := time.Now().UTC()
	pinEmotionClock(es, at)

	require.NoError(t, es.OnEvent("t1", r1.ID, 7, "praised", 3, at, "evt-r1"))

	s1, err := es.Status("t1", r1.ID, 7)
	require.NoError(t, err)
	require.Equal(t, 24, s1.Intensity)

	// 第二局同一 Agent：互不可见，返回基线默认
	s2, err := es.Status("t1", r2.ID, 7)
	require.NoError(t, err)
	require.Equal(t, 0, s2.Intensity)
	require.Equal(t, emotion.MoodCalm, s2.Mood)
}

func TestEmotionServiceNarrationPersistedInState(t *testing.T) {
	es, rs := newEmotionTestService(t)
	r, err := rs.Create("t1", CreateRunInput{Name: "scene"})
	require.NoError(t, err)
	at := time.Now().UTC()
	pinEmotionClock(es, at)

	require.NoError(t, es.OnEvent("t1", r.ID, 7, "insulted", 3, at, "evt-i"))                 // -40
	require.NoError(t, es.OnEvent("t1", r.ID, 7, "insulted", 3, at.Add(time.Hour), "evt-i2")) // 再 -40 → 0

	rows, err := rs.States("t1", r.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, emotion.Namespace, rows[0].Namespace)
	require.Equal(t, emotion.SubjectType, rows[0].SubjectType)
	require.Equal(t, "7", rows[0].SubjectID)
	require.Contains(t, rows[0].Data["narration"], "心情为")
}
