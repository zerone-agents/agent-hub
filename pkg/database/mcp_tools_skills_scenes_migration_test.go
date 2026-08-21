package database

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLegacyTenantMigrationDB recreates the pre-Phase3 schema with raw DDL
// (同仓库其他迁移测试的风格：sqlite 索引名全局唯一，四个表不能都用
// uk_tenant_name，AutoMigrate 不可用)：tenant_id 列已存在（AutoMigrate 先
// 加列后跑数据迁移），tools 还带着旧的全局唯一索引 uk_name。
func setupLegacyTenantMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE mcp_servers (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', is_builtin INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE tools (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', title VARCHAR(128))`,
		`CREATE UNIQUE INDEX uk_name ON tools(name)`, // 旧全局唯一索引 → 迁移判据
		`CREATE TABLE skills (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '')`,
		`CREATE TABLE scenes (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', agent_id INTEGER)`,
		`CREATE TABLE agents (id INTEGER PRIMARY KEY AUTOINCREMENT, name VARCHAR(64), tenant_id VARCHAR(64) NOT NULL DEFAULT '')`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	return db
}

// TestMigrateMcpToolsSkillsScenes_Idempotent 守护 C1 回归：预设名单归零
// UPDATE 只能在 uk_name 仍存在（真正执行迁移）的那一次启动跑。重跑（uk_name
// 已删）时，与共享预设同名的租户行 ('org-a','Skill') 必须原封不动——旧实现
// 每次启动无条件按名单归零，会把租户私有行劫持进共享域（tenant_id != ” 的
// 写 scope 还会把它永久锁死不可删），甚至撞 uk_tenant_name 唯一索引导致
// 启动失败。
func TestMigrateMcpToolsSkillsScenes_Idempotent(t *testing.T) {
	db := setupLegacyTenantMigrationDB(t)
	DB = db

	// 迁移前状态：旧全局唯一索引 uk_name 下每个 name 至多一行。预设行
	// tenant_id 为空（将被 BackfillTenantID 回填为启动租户再归零）。
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('Skill', '', '旧内置'), ('Bash', '', '旧预设')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_servers (name, tenant_id, is_builtin) VALUES ('knowledge', '', 1)`).Error)

	// 第一次：真正执行迁移（回填 + 内置/预设行归零 + 删 uk_name）。
	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())

	var tenantID string
	require.NoError(t, db.Raw(`SELECT tenant_id FROM tools WHERE title = '旧内置'`).Scan(&tenantID).Error)
	require.Equal(t, "", tenantID, "旧内置行必须在迁移时归零为共享")
	require.NoError(t, db.Raw(`SELECT tenant_id FROM tools WHERE title = '旧预设'`).Scan(&tenantID).Error)
	require.Equal(t, "", tenantID, "旧预设行必须在迁移时归零为共享")
	require.NoError(t, db.Raw(`SELECT tenant_id FROM mcp_servers WHERE name = 'knowledge'`).Scan(&tenantID).Error)
	require.Equal(t, "", tenantID, "内置 MCP 行必须在迁移时归零为共享")
	require.False(t, db.Migrator().HasIndex(&agent.Tool{}, "uk_name"), "旧 uk_name 必须被删除")

	// 迁移后：租户创建与共享预设同名的行（uk_tenant_name 复合索引下合法）。
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('Skill', 'org-a', '租户自制'), ('Bash', 'org-a', '租户 Bash')`).Error)

	// 重跑两次：必须纯 no-op——不劫持租户行、不报错、不新增共享行。
	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())
	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())

	require.NoError(t, db.Raw(`SELECT tenant_id FROM tools WHERE title = '租户自制'`).Scan(&tenantID).Error)
	require.Equal(t, "org-a", tenantID, "重跑不得把租户私有行劫持进共享域")
	require.NoError(t, db.Raw(`SELECT tenant_id FROM tools WHERE title = '租户 Bash'`).Scan(&tenantID).Error)
	require.Equal(t, "org-a", tenantID, "重跑不得把租户私有行劫持进共享域")

	var count int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tools WHERE tenant_id = ''`).Scan(&count).Error)
	require.Equal(t, int64(2), count, "共享行集合在重跑后必须保持不变")
}

// TestMigrateMcpToolsSkillsScenes_NoopOnFreshDB 新库（从未有过 uk_name）必须
// 整体跳过回填/归零分支——AutoMigrate 建的已是 uk_tenant_name，Phase3 之后
// 写入的租户行（含与预设同名的）不能被任何 UPDATE 碰到。
func TestMigrateMcpToolsSkillsScenes_NoopOnFreshDB(t *testing.T) {
	db := setupLegacyTenantMigrationDB(t)
	DB = db
	// 去掉旧索引 = 模拟新库（AutoMigrate 只建复合索引，无 uk_name）。
	require.NoError(t, db.Exec(`DROP INDEX uk_name`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('Skill', 'org-a', '租户自制'), ('Skill', '', '共享')`).Error)

	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())

	var count int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tools WHERE tenant_id = 'org-a'`).Scan(&count).Error)
	require.Equal(t, int64(1), count, "新库上的租户行必须原封不动")
}
