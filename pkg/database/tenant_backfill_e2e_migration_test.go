package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLegacyAgentsChatDB 用旧 schema（无 tenant_id 列）建 agents/cloud_sessions/
// cloud_messages 三表并插入存量行。与手写 DEFAULT ” 的模拟不同（见
// tenant_backfill_e2e 测试守护的 C-1），这里刻意不建 tenant_id 列——让真实
// AutoMigrate 走 ADD COLUMN 路径，暴露 model tag default 值与回填条件的耦合。
func setupLegacyAgentsChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		// 旧 schema（Phase3 之前）：与 model 的差异仅在缺 tenant_id 列（及索引）。
		// NOT NULL 无 default 的列（name/content_hash/system_prompt 等）旧库
		// 本就存在，避免 sqlite 拒绝补加 NOT NULL 列的噪音。
		`CREATE TABLE agents (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64) NOT NULL, content_hash VARCHAR(128) NOT NULL, system_prompt TEXT NOT NULL, permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto', max_turns INTEGER NOT NULL DEFAULT 50, source VARCHAR(16) NOT NULL DEFAULT 'remote', desktop_enabled BOOLEAN NOT NULL DEFAULT false, mobile_enabled BOOLEAN NOT NULL DEFAULT false, created_at DATETIME, updated_at DATETIME)`,
		`CREATE UNIQUE INDEX uk_name ON agents(name)`, // 旧全局唯一索引
		`CREATE TABLE cloud_sessions (user_id VARCHAR(255) NOT NULL, id VARCHAR(255) NOT NULL, title VARCHAR(512), created_at DATETIME, updated_at DATETIME, PRIMARY KEY (user_id, id))`,
		`CREATE TABLE cloud_messages (user_id VARCHAR(255) NOT NULL, id VARCHAR(255) NOT NULL, session_id VARCHAR(255) NOT NULL, role VARCHAR(50), content TEXT, created_at DATETIME, PRIMARY KEY (user_id, id, session_id))`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO agents (name, content_hash, system_prompt) VALUES ('legacy-agent', 'h1', 'p1'), ('legacy-agent-2', 'h2', 'p2')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO cloud_sessions (user_id, id, title) VALUES ('u-legacy', 's-1', 'legacy-session')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO cloud_messages (user_id, id, session_id, role, content) VALUES ('u-legacy', 'm-1', 's-1', 'user', 'hello')`).Error)
	return db
}

// runAutoMigrateWithRestore 跑真实 AutoMigrate 完整迁移链，并守护包级 DB 变量。
func runAutoMigrateWithRestore(t *testing.T, db *gorm.DB, backfillTenant string) {
	t.Helper()
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() {
		DB, backfillTenantID = oldDB, oldBackfill
	})
	DB = db
	require.NoError(t, AutoMigrate(backfillTenant))
}

// TestTenantBackfillE2E_Casdoor 守护终审 C-1：model tag 若是 default:'default'，
// AutoMigrate 的 ADD COLUMN ... NOT NULL DEFAULT 'default' 会把存量行在加列
// 那一刻填成 'default'，BackfillTenantID 的 WHERE tenant_id = ” 永不命中，
// casdoor 模式（backfill=组织名）升级后存量数据全部落错租户。改 tag 为
// default:” 后，存量行加列时填 ”，回填正确指向启动租户。
func TestTenantBackfillE2E_Casdoor(t *testing.T) {
	db := setupLegacyAgentsChatDB(t)
	runAutoMigrateWithRestore(t, db, "org-x")

	for table, key := range map[string]string{
		"agents":         "name = 'legacy-agent'",
		"cloud_sessions": "id = 's-1'",
		"cloud_messages": "id = 'm-1'",
	} {
		var tenantID string
		require.NoError(t, db.Raw("SELECT tenant_id FROM "+table+" WHERE "+key).Scan(&tenantID).Error)
		require.Equal(t, "org-x", tenantID, table+" 存量行必须回填为启动租户 org-x，而非 'default'")
	}
}

// TestTenantBackfillE2E_Builtin builtin 模式：backfill 传空串回落 'default'，
// 存量行回填后应恰为 'default'（与旧行为等价）。
func TestTenantBackfillE2E_Builtin(t *testing.T) {
	db := setupLegacyAgentsChatDB(t)
	runAutoMigrateWithRestore(t, db, "")

	for table, key := range map[string]string{
		"agents":         "name = 'legacy-agent-2'",
		"cloud_sessions": "id = 's-1'",
		"cloud_messages": "id = 'm-1'",
	} {
		var tenantID string
		require.NoError(t, db.Raw("SELECT tenant_id FROM "+table+" WHERE "+key).Scan(&tenantID).Error)
		require.Equal(t, "default", tenantID, table+" 存量行在 builtin 模式必须回填为 'default'")
	}
}
