package repository

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestIsFKConstraintError_MySQL1451(t *testing.T) {
	require.True(t, IsFKConstraintError(&mysql.MySQLError{Number: 1451}))
	// 经 %w 包装的错误链（service 层 "删除技能失败: %w" 形态）
	wrapped := fmt.Errorf("删除技能失败: %w", &mysql.MySQLError{Number: 1451})
	require.True(t, IsFKConstraintError(wrapped))
}

func TestIsFKConstraintError_SQLiteMessage(t *testing.T) {
	require.True(t, IsFKConstraintError(errors.New("FOREIGN KEY constraint failed")))
	wrapped := fmt.Errorf("删除 MCP 失败: %w", errors.New("FOREIGN KEY constraint failed"))
	require.True(t, IsFKConstraintError(wrapped))
}

func TestIsFKConstraintError_Other(t *testing.T) {
	require.False(t, IsFKConstraintError(errors.New("record not found")))
	require.False(t, IsFKConstraintError(&mysql.MySQLError{Number: 1062}))
	require.False(t, IsFKConstraintError(nil))
}
