package repository

import (
	"os"
	"testing"

	"control-panel/internal/domain/audit"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// MAX(id) > MaxInt64 的无损捕获（PR #150 审查 P1）：SQLite INTEGER 为有符号
// int64 无法表达该值——按 spec §8 测试分工，真 MySQL 集成用例 env 提供 DSN
// 时运行、缺省跳过（与 aigc_config_mysql_test.go 同款门控）。
func TestAuditMaxIDUnsignedMySQL(t *testing.T) {
	dsn := os.Getenv("AUDIT_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("AUDIT_MYSQL_TEST_DSN not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	require.NoError(t, db.Exec("DELETE FROM audit_logs WHERE tenant_id = ?", "maxid-test").Error)
	t.Cleanup(func() { _ = db.Exec("DELETE FROM audit_logs WHERE tenant_id = ?", "maxid-test").Error })

	big := uint64(1)<<63 + 42 // > MaxInt64：int64 中转在此扫描失败/溢出
	l := &audit.Log{TenantID: "maxid-test", Category: audit.CatAuth, Action: audit.ActionLogin,
		TargetType: audit.TargetSystem, Status: audit.StatusSuccess}
	l.ID = big
	require.NoError(t, db.Create(l).Error)

	repo := NewAuditRepository(db)
	max, err := repo.MaxID("maxid-test")
	require.NoError(t, err)
	require.Equal(t, big, max)
}
