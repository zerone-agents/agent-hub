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

func TestIsLockWaitTimeoutErr(t *testing.T) {
	require.True(t, isLockWaitTimeoutErr(mysqlErr(1205)))
	require.False(t, isLockWaitTimeoutErr(mysqlErr(1213)))
	require.False(t, isLockWaitTimeoutErr(errors.New("other")))
}

// 冲突类（1062/1213 重试耗尽、1205 快速失败）→ 中文哨兵，MySQL 英文原文
// 不得直出给用户（CONTRIBUTING 用户错误中文契约，终审 Important#1）。
func TestSaveConflictOrErr(t *testing.T) {
	for _, code := range []uint16{1062, 1213, 1205} {
		require.ErrorIs(t, saveConflictOrErr(mysqlErr(code)), ErrAigcSaveConflict, "code %d", code)
	}
	require.Equal(t, "保存冲突，请稍后重试", ErrAigcSaveConflict.Error())
	other := errors.New("boom")
	require.Same(t, other, saveConflictOrErr(other)) // 非冲突类原样透传（不吞错）
}
