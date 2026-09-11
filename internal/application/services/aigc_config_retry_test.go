package services

import (
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func mysqlErr(n uint16) error { return &mysql.MySQLError{Number: n} }

func TestIsRetryableMySQLError(t *testing.T) {
	require.True(t, isRetryableMySQLError(mysqlErr(1062)))
	require.True(t, isRetryableMySQLError(mysqlErr(1213)))
	require.False(t, isRetryableMySQLError(mysqlErr(1205))) // 锁等待超时：快速失败（spec §3.3）
	require.False(t, isRetryableMySQLError(errors.New("other")))
}

func TestWithRetryRetriesDeadlockThenSucceeds(t *testing.T) {
	calls := 0
	backoffs := 0
	orig := aigcSaveBackoff
	aigcSaveBackoff = func() time.Duration { backoffs++; return 0 }
	t.Cleanup(func() { aigcSaveBackoff = orig })
	err := withRetry(func() error {
		calls++
		if calls < 3 {
			return mysqlErr(1213)
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls)    // 1213 → 完整事务重跑
	require.Equal(t, 2, backoffs) // 退避被调用
}

func TestWithRetryFailsFastOnLockTimeout(t *testing.T) {
	calls := 0
	err := withRetry(func() error { calls++; return mysqlErr(1205) })
	require.Error(t, err)
	require.Equal(t, 1, calls) // 1205 不重试
}

func TestWithRetryCapsAtThreeAttempts(t *testing.T) {
	calls := 0
	orig := aigcSaveBackoff
	aigcSaveBackoff = func() time.Duration { return 0 }
	t.Cleanup(func() { aigcSaveBackoff = orig })
	err := withRetry(func() error { calls++; return mysqlErr(1213) })
	require.Error(t, err)
	require.Equal(t, 3, calls) // 总尝试 ≤3
}
