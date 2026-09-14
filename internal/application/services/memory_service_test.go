package services

import (
	"testing"
	"time"

	subjectivememory "control-panel/internal/domain/subjectivememory"

	"github.com/stretchr/testify/require"
)

func newMemoryTestService(t *testing.T) (*MemoryService, *RunService) {
	t.Helper()
	runService, _ := newRunTestService(t)
	return NewMemoryService(runService), runService
}

func TestMemoryServiceEnsureSchemasIdempotent(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	_, err := runService.Create("t1", CreateRunInput{Name: "one"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	require.NoError(t, svc.EnsureSchemas(), "EnsureSchemas must be idempotent")
}

func TestMemoryServiceRecordValidationClampAndIdempotency(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	r, err := runService.Create("t1", CreateRunInput{Name: "one"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	_, err = svc.Record("t1", r.ID, 7, "  ", "解释", 50, at, "k1")
	require.ErrorIs(t, err, subjectivememory.ErrEmptyFactRef)
	_, err = svc.Record("t1", r.ID, 7, "fact-1", "", 50, at, "k1")
	require.ErrorIs(t, err, subjectivememory.ErrEmptyInterpretation)
	_, err = svc.Record("t1", r.ID, 7, "fact-1", "解释", 50, at, "  ")
	require.ErrorContains(t, err, "幂等键")

	entry, err := svc.Record("t1", r.ID, 7, "fact-1", "对方在预算会议上公开质疑我", 150, at, "key-1")
	require.NoError(t, err)
	require.Equal(t, 100, entry.Importance, "importance must be clamped to 0..100")
	require.Equal(t, "fact-1", entry.FactRef)
	require.NotEmpty(t, entry.ID)

	replayed, err := svc.Record("t1", r.ID, 7, "fact-1", "ignored duplicate", 10, at, "key-1")
	require.NoError(t, err)
	require.Equal(t, entry.ID, replayed.ID, "duplicate idempotency key returns the existing entry")
	require.Equal(t, entry.Interpretation, replayed.Interpretation)
}

func TestMemoryServiceRecallRankingRecallCountAndSoftForget(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	r, err := runService.Create("t1", CreateRunInput{Name: "one"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	fresh, err := svc.Record("t1", r.ID, 7, "fact-a", "预算会议上的背叛让我警惕", 90, at.Add(-time.Hour), "k-fresh")
	require.NoError(t, err)
	relevant, err := svc.Record("t1", r.ID, 7, "fact-b", "预算超支的教训", 60, at.Add(-2*24*time.Hour), "k-relevant")
	require.NoError(t, err)
	_, err = svc.Record("t1", r.ID, 7, "fact-c", "一条微不足道的小事", 5, at.Add(-30*24*time.Hour), "k-trivial")
	require.NoError(t, err)

	got, err := svc.Recall("t1", r.ID, 7, "预算", 5, at)
	require.NoError(t, err)
	require.Len(t, got, 2, "soft-forgotten trivial memory must sink below the cutoff")
	require.Equal(t, fresh.ID, got[0].ID)
	require.Equal(t, relevant.ID, got[1].ID)

	// recall bumps recallCount exactly once per (memory, run) — idempotent key
	require.Equal(t, 1, got[0].RecallCount)
	again, err := svc.Recall("t1", r.ID, 7, "预算", 5, at)
	require.NoError(t, err)
	require.Equal(t, 1, again[0].RecallCount, "recallCount increment is idempotent within a run")

	// other agent's memories are never returned
	other, err := svc.Recall("t1", r.ID, 8, "预算", 5, at)
	require.NoError(t, err)
	require.Empty(t, other)
}

func TestMemoryServiceRecallDefaultLimit(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	r, err := runService.Create("t1", CreateRunInput{Name: "one"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	at := time.Now().UTC()
	for i := 0; i < 7; i++ {
		_, err := svc.Record("t1", r.ID, 7, "fact", "一条重要的共同记忆", 80, at.Add(-time.Duration(i)*time.Hour), string(rune('a'+i)))
		require.NoError(t, err)
	}
	got, err := svc.Recall("t1", r.ID, 7, "", 0, at)
	require.NoError(t, err)
	require.Len(t, got, subjectivememory.DefaultRecallLimit)
}

func TestMemoryServiceRunIsolation(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	runA, err := runService.Create("t1", CreateRunInput{Name: "run-a"})
	require.NoError(t, err)
	runB, err := runService.Create("t1", CreateRunInput{Name: "run-b"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	at := time.Now().UTC()

	_, err = svc.Record("t1", runA.ID, 7, "fact-a", "第一局的独特经历", 90, at, "key-a1")
	require.NoError(t, err)

	// same agent, different run: run A memories must never surface in run B
	got, err := svc.Recall("t1", runB.ID, 7, "", 5, at)
	require.NoError(t, err)
	require.Empty(t, got)

	// same idempotency key in another run is rejected, not replayed
	_, err = svc.Record("t1", runB.ID, 7, "fact-a", "第一局的独特经历", 90, at, "key-a1")
	require.Error(t, err)
}

func TestMemoryServiceTenantIsolation(t *testing.T) {
	svc, runService := newMemoryTestService(t)
	r, err := runService.Create("tenant-a", CreateRunInput{Name: "one"})
	require.NoError(t, err)
	require.NoError(t, svc.EnsureSchemas())
	at := time.Now().UTC()
	_, err = svc.Record("tenant-a", r.ID, 7, "fact-a", "租户 A 的秘密", 90, at, "key-a")
	require.NoError(t, err)

	got, err := svc.Recall("tenant-b", r.ID, 7, "", 5, at)
	require.NoError(t, err)
	require.Empty(t, got, "foreign tenant must never see this tenant's memories")
}
