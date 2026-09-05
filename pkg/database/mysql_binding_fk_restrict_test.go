package database

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestMySQLBindingFKRESTRICTUpgrade 存量 MySQL 升级迁移（#123 review P2）：
// 从旧 CASCADE schema（三张绑定表资源侧 FK 为 ON DELETE CASCADE，含存量
// 绑定数据）经真实 AutoMigrate 完整链后，断言：
//  1. 资源侧 FK DELETE_RULE=RESTRICT，agent 侧保持 CASCADE；
//  2. 绕过守卫直接删除资源被拒，资源与绑定行均保留（并发竞态闭环）；
//  3. 迁移幂等：重跑完整链 no-op，RESTRICT 保持。
//
// 默认跳过；设置 TEST_MYSQL_DSN 后生效（须指向专用测试 MySQL 实例）。
func TestMySQLBindingFKRESTRICTUpgrade(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 未设置，跳过 MySQL 迁移测试")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// 单连接：USE 语句生效于池内全部操作（信息查询均显式限定库名，双保险）
	sqlDB.SetMaxOpenConns(1)

	// 专用冒烟库
	require.NoError(t, db.Exec("DROP DATABASE IF EXISTS hub_fk_upgrade").Error)
	require.NoError(t, db.Exec("CREATE DATABASE hub_fk_upgrade").Error)
	require.NoError(t, db.Exec("USE hub_fk_upgrade").Error)

	// 旧 schema（存量多租户形态）：绑定表资源侧 FK 为 CASCADE + 存量绑定数据
	for _, stmt := range []string{
		"CREATE TABLE agents (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', content_hash VARCHAR(128) NOT NULL DEFAULT '', system_prompt LONGTEXT NOT NULL, created_at DATETIME(3), updated_at DATETIME(3))",
		"CREATE TABLE skills (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', type VARCHAR(32) NOT NULL DEFAULT 'expert', title VARCHAR(128) NOT NULL DEFAULT '', url VARCHAR(512), file_hash VARCHAR(128), file_size BIGINT, created_at DATETIME(3), updated_at DATETIME(3))",
		"CREATE TABLE mcp_servers (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', title VARCHAR(128) NOT NULL DEFAULT '', description TEXT, transport_type VARCHAR(16) NOT NULL DEFAULT 'http', url VARCHAR(512), headers TEXT, is_builtin TINYINT(1) NOT NULL DEFAULT 0, tools TEXT, probe_status VARCHAR(16) NOT NULL DEFAULT 'pending', created_at DATETIME(3), updated_at DATETIME(3))",
		"CREATE TABLE tools (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(64) NOT NULL, tenant_id VARCHAR(64) NOT NULL DEFAULT '', title VARCHAR(128) NOT NULL DEFAULT '', source VARCHAR(16) NOT NULL DEFAULT 'custom', file_url VARCHAR(512), file_hash VARCHAR(128), file_size BIGINT, created_at DATETIME(3), updated_at DATETIME(3))",
		`CREATE TABLE agent_skills (
			agent_id BIGINT UNSIGNED NOT NULL, skill_id BIGINT UNSIGNED NOT NULL, created_at DATETIME(3),
			PRIMARY KEY (agent_id, skill_id),
			CONSTRAINT fk_agent_skills_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
			CONSTRAINT fk_agent_skills_skill FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE)`,
		`CREATE TABLE agent_mcp_servers (
			agent_id BIGINT UNSIGNED NOT NULL, mcp_server_id BIGINT UNSIGNED NOT NULL, created_at DATETIME(3),
			PRIMARY KEY (agent_id, mcp_server_id),
			CONSTRAINT fk_agent_mcp_servers_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
			CONSTRAINT fk_agent_mcp_servers_mcp_server FOREIGN KEY (mcp_server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE)`,
		`CREATE TABLE agent_tools (
			agent_id BIGINT UNSIGNED NOT NULL, tool_id BIGINT UNSIGNED NOT NULL, created_at DATETIME(3),
			PRIMARY KEY (agent_id, tool_id),
			CONSTRAINT fk_agent_tools_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE,
			CONSTRAINT fk_agent_tools_tool FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE)`,
	} {
		require.NoError(t, db.Exec(stmt).Error, stmt)
	}
	require.NoError(t, db.Exec("INSERT INTO agents (name, tenant_id, system_prompt) VALUES ('bot', 'org-a', 'p')").Error)
	require.NoError(t, db.Exec("INSERT INTO skills (name, tenant_id) VALUES ('s1', 'org-a')").Error)
	require.NoError(t, db.Exec("INSERT INTO mcp_servers (name, tenant_id) VALUES ('fs', 'org-a')").Error)
	require.NoError(t, db.Exec("INSERT INTO tools (name, tenant_id) VALUES ('t1', 'org-a')").Error)
	require.NoError(t, db.Exec("INSERT INTO agent_skills (agent_id, skill_id) VALUES (1, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO agent_mcp_servers (agent_id, mcp_server_id) VALUES (1, 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO agent_tools (agent_id, tool_id) VALUES (1, 1)").Error)

	// 真实完整迁移链（等价旧库正常升级路径）
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() { DB, backfillTenantID = oldDB, oldBackfill })
	DB = db
	require.NoError(t, AutoMigrate("org-a"))

	deleteRule := func(table, col string) string {
		var rule string
		require.NoError(t, db.Raw(`SELECT rc.DELETE_RULE FROM information_schema.REFERENTIAL_CONSTRAINTS rc
			JOIN information_schema.KEY_COLUMN_USAGE kcu
			  ON kcu.CONSTRAINT_SCHEMA = rc.CONSTRAINT_SCHEMA AND kcu.CONSTRAINT_NAME = rc.CONSTRAINT_NAME
			WHERE rc.CONSTRAINT_SCHEMA = 'hub_fk_upgrade' AND kcu.TABLE_NAME = ? AND kcu.COLUMN_NAME = ?`, table, col).Scan(&rule).Error)
		return rule
	}

	// 断言 1：资源侧 → RESTRICT；agent 侧 → 保持 CASCADE
	for table, col := range map[string]string{
		"agent_skills":      "skill_id",
		"agent_mcp_servers": "mcp_server_id",
		"agent_tools":       "tool_id",
	} {
		require.Equal(t, "RESTRICT", deleteRule(table, col), fmt.Sprintf("%s.%s 必须迁为 RESTRICT", table, col))
	}
	for table := range map[string]string{"agent_skills": "", "agent_mcp_servers": "", "agent_tools": ""} {
		require.Equal(t, "CASCADE", deleteRule(table, "agent_id"), fmt.Sprintf("%s.agent_id 必须保持 CASCADE", table))
	}

	// 断言 2：绕过守卫直接删除被拒 + 资源与绑定行均保留（并发竞态闭环）
	err = db.Exec("DELETE FROM skills WHERE id = 1").Error
	require.Error(t, err, "存量数据下技能删除必须被 RESTRICT 拒绝")
	var skillCnt, skillBindCnt int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM skills WHERE id = 1").Scan(&skillCnt).Error)
	require.Equal(t, int64(1), skillCnt)
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM agent_skills WHERE skill_id = 1").Scan(&skillBindCnt).Error)
	require.Equal(t, int64(1), skillBindCnt)

	// 断言 3：幂等——重跑完整链 no-op，RESTRICT 保持
	require.NoError(t, AutoMigrate("org-a"))
	require.Equal(t, "RESTRICT", deleteRule("agent_skills", "skill_id"))
}
