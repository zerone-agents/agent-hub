package database

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestMySQLMigrationSmoke 在真实 MySQL 上冒烟完整迁移链（CI 只有 sqlite，
// 多租户 Phase 3 终审 checklist 要求 MySQL 冒烟）。默认跳过；设置
// TEST_MYSQL_DSN（如 root:pass@tcp(127.0.0.1:3307)/hub?parseTime=true）
// 后生效。流程：旧 schema（无 tenant_id）→ 真实 AutoMigrate 完整链 →
// 断言回填/索引切换 → 再跑一次断言幂等。
func TestMySQLMigrationSmoke(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 未设置，跳过 MySQL 冒烟")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// 干净起点：DROP 旧库重建（该 DSN 必须指向专用冒烟库）
	require.NoError(t, db.Exec("DROP DATABASE IF EXISTS hub_smoke").Error)
	require.NoError(t, db.Exec("CREATE DATABASE hub_smoke").Error)
	require.NoError(t, db.Exec("USE hub_smoke").Error)

	// 旧 schema（Phase 3 之前）：核心三表缺 tenant_id，agents 带旧全局唯一索引
	for _, stmt := range []string{
		"CREATE TABLE agents (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, content_hash VARCHAR(128) NOT NULL, system_prompt LONGTEXT NOT NULL, permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto', max_turns INT NOT NULL DEFAULT 50, source VARCHAR(16) NOT NULL DEFAULT 'remote', desktop_enabled TINYINT(1) NOT NULL DEFAULT 0, mobile_enabled TINYINT(1) NOT NULL DEFAULT 0, created_at DATETIME(3), updated_at DATETIME(3))",
		"CREATE UNIQUE INDEX uk_name ON agents(name)",
		"CREATE TABLE cloud_sessions (user_id VARCHAR(255) NOT NULL, id VARCHAR(255) NOT NULL, title VARCHAR(512), created_at DATETIME(3), updated_at DATETIME(3), PRIMARY KEY (user_id, id))",
		"CREATE TABLE cloud_messages (user_id VARCHAR(255) NOT NULL, id VARCHAR(255) NOT NULL, session_id VARCHAR(255) NOT NULL, role VARCHAR(50), content LONGTEXT, created_at DATETIME(3), PRIMARY KEY (user_id, id, session_id))",
		"CREATE TABLE vendor_presets (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, marker VARCHAR(64))", // 待 DROP 的 legacy 表（保持空表：migrateProviderSplit 空表早退，不触发 salvage 列匹配）
	} {
		require.NoError(t, db.Exec(stmt).Error, stmt)
	}
	require.NoError(t, db.Exec("INSERT INTO agents (name, content_hash, system_prompt) VALUES ('legacy-agent', 'h1', 'p1')").Error)
	require.NoError(t, db.Exec("INSERT INTO cloud_sessions (user_id, id, title) VALUES ('u-legacy', 's-1', 'legacy')").Error)
	require.NoError(t, db.Exec("INSERT INTO cloud_messages (user_id, id, session_id, role, content) VALUES ('u-legacy', 'm-1', 's-1', 'user', 'hi')").Error)

	// 第一次：真实完整迁移链（casdoor 场景，backfill=org 名）
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() { DB, backfillTenantID = oldDB, oldBackfill })
	DB = db
	require.NoError(t, AutoMigrate("org-smoke"))

	// 存量回填断言（C-1 守护：不能滞留 'default'）
	for table, key := range map[string]string{
		"agents":         "name = 'legacy-agent'",
		"cloud_sessions": "id = 's-1'",
		"cloud_messages": "id = 'm-1'",
	} {
		var tenantID string
		require.NoError(t, db.Raw("SELECT tenant_id FROM "+table+" WHERE "+key).Scan(&tenantID).Error)
		require.Equal(t, "org-smoke", tenantID, fmt.Sprintf("%s 存量行必须回填为 org-smoke", table))
	}

	// 索引切换断言：旧 uk_name 已删，新复合唯一存在
	var idxCount int
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='hub_smoke' AND TABLE_NAME='agents' AND INDEX_NAME='uk_name'").Scan(&idxCount).Error)
	require.Zero(t, idxCount, "agents 旧全局唯一索引 uk_name 必须被删除")
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='hub_smoke' AND TABLE_NAME='agents' AND INDEX_NAME='uk_agents_tenant_name'").Scan(&idxCount).Error)
	require.NotZero(t, idxCount, "agents 新复合唯一索引必须存在")

	// vendor_presets 已 DROP
	require.False(t, db.Migrator().HasTable("vendor_presets"), "vendor_presets 必须被 DROP")

	// 幂等冒烟：第二次完整链 no-op 不报错
	require.NoError(t, AutoMigrate("org-smoke"))
}
