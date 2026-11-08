package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type cancelledWriteIndex struct {
	*logicalNativeIndex
	cancel     context.CancelFunc
	afterWrite func()
}

func (i *cancelledWriteIndex) Put(ctx context.Context, bank, scope, content string) (string, error) {
	if _, err := i.testIndex.Put(ctx, bank, scope, content); err != nil {
		return "", err
	}
	if i.afterWrite != nil {
		i.afterWrite()
	}
	i.cancel()
	return "", context.Canceled
}

func TestCancelledSyncPersistsUnknownAndReconcilesWithoutRewriting(t *testing.T) {
	service, scope, index := nativeFixture(t)
	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	service.cfg.Index = &cancelledWriteIndex{logicalNativeIndex: index, cancel: cancel}
	fact := publish(t, service, scope, input("我喜欢乌龙茶。"))
	require.ErrorIs(t, service.Sync(ctx, 100), context.Canceled)
	var job outboxRow
	require.NoError(t, service.db.First(&job, "record_id = ?", fact.ID).Error)
	require.Equal(t, "unknown", job.Status)
	require.Equal(t, 1, job.Attempts)
	require.NotNil(t, job.LeaseUntil)
	require.True(t, job.LeaseUntil.After(time.Now()), "recovery must persist without waiting for the five-minute crash lease")
	require.Equal(t, 1, index.puts)
	// A fresh attempt sees unknown and performs Find only. Its Put method still
	// fails, so successful confirmation also proves no write was replayed.
	require.NoError(t, service.Sync(context.Background(), 100))
	require.Equal(t, 1, index.puts)
	require.NoError(t, service.db.First(&job, "record_id = ?", fact.ID).Error)
	require.Equal(t, "done", job.Status)
	require.Equal(t, 2, job.Attempts)
	current, err := service.Get(testCtx, scope, fact.ID)
	require.NoError(t, err)
	require.Equal(t, "confirmed", current.SyncStatus)
}

func TestCancelledOldAttemptDoesNotOverwriteNewerProcessingAttempt(t *testing.T) {
	service, scope, index := nativeFixture(t)
	ctx, cancel := context.WithCancel(testCtx)
	defer cancel()
	fact := publish(t, service, scope, input("我喜欢黑咖啡。"))
	service.cfg.Index = &cancelledWriteIndex{logicalNativeIndex: index, cancel: cancel, afterWrite: func() {
		// Model a second process taking a newer lease while the original network
		// result arrives late. This fixture DB is isolated; no live outbox is used.
		result := service.db.Model(&outboxRow{}).Where("record_id = ? AND status = ? AND attempts = ?", fact.ID, "processing", 1).Updates(map[string]any{"attempts": 2, "lease_until": time.Now().Add(5 * time.Minute)})
		require.NoError(t, result.Error)
		require.EqualValues(t, 1, result.RowsAffected)
	}}
	require.ErrorIs(t, service.Sync(ctx, 100), context.Canceled)
	var job outboxRow
	require.NoError(t, service.db.First(&job, "record_id = ?", fact.ID).Error)
	require.Equal(t, 2, job.Attempts)
	require.Equal(t, "processing", job.Status, "the original attempt must not overwrite the newer lease with unknown")
	require.Equal(t, 1, index.puts)
}

func TestSyncDoesNotClaimAChangedAttemptFromItsStaleSnapshot(t *testing.T) {
	service, scope, index := nativeFixture(t)
	fact := publish(t, service, scope, input("我喜欢散步。"))
	require.NoError(t, service.db.Model(&outboxRow{}).Where("record_id = ?", fact.ID).Updates(map[string]any{"status": "unknown", "attempts": 1}).Error)
	advanced := false
	const callback = "test:advance-memory-attempt-after-snapshot"
	require.NoError(t, service.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		jobs, ok := tx.Statement.Dest.(*[]outboxRow)
		if !ok || len(*jobs) != 1 || advanced {
			return
		}
		advanced = true
		// Another process completed a later uncertain attempt after this worker's
		// SELECT. The old snapshot must not claim the now-current unknown state.
		result := service.db.Model(&outboxRow{}).Where("id = ? AND attempts = ?", (*jobs)[0].ID, 1).Update("attempts", 2)
		require.NoError(t, result.Error)
		require.EqualValues(t, 1, result.RowsAffected)
	}))
	defer service.db.Callback().Query().Remove(callback)
	require.NoError(t, service.Sync(testCtx, 100))
	require.True(t, advanced)
	var job outboxRow
	require.NoError(t, service.db.First(&job, "record_id = ?", fact.ID).Error)
	require.Equal(t, "unknown", job.Status)
	require.Equal(t, 2, job.Attempts)
	require.Zero(t, index.puts)
}
