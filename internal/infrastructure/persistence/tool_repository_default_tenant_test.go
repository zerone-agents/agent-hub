package repository

import (
	"testing"

	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupToolDefaultTestDB 起 sqlite 内存库，覆盖 isDefault 三条路径涉及的
// tools / agents / agent_tools 三表（裸 SQL，模式同 tool_repository_tenant_test.go）。
func setupToolDefaultTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(128),
			description TEXT,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tools_tenant_name ON tools(tenant_id, name)`,
		`CREATE TABLE agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_agents_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE agent_tools (
			agent_id INTEGER NOT NULL,
			tool_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, tool_id)
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func agentToolBindings(t *testing.T, db *gorm.DB, agentName string) []uint64 {
	t.Helper()
	var ids []uint64
	require.NoError(t, db.Raw(`
		SELECT at.tool_id FROM agent_tools at
		JOIN agents a ON a.id = at.agent_id
		WHERE a.name = ? AND a.tenant_id != ''
	`, agentName).Pluck("tool_id", &ids).Error)
	return ids
}

// TestToolRepository_AddToolToAllAgents_TenantScoped：org-a 的默认工具只能
// 绑定到 org-a 的 agent，不得跨租户泄漏到 org-b。
func TestToolRepository_AddToolToAllAgents_TenantScoped(t *testing.T) {
	db := setupToolDefaultTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('a-def', 'org-a', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('a1', 'org-a')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('a2', 'org-a')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('b1', 'org-b')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('b2', 'org-b')`).Error)

	var toolID uint64
	require.NoError(t, db.Raw(`SELECT id FROM tools WHERE name = 'a-def'`).Scan(&toolID).Error)

	repo := NewToolRepository()
	require.NoError(t, repo.AddToolToAllAgents("org-a", toolID))

	require.Len(t, agentToolBindings(t, db, "a1"), 1, "org-a agent 1 应被绑定")
	require.Len(t, agentToolBindings(t, db, "a2"), 1, "org-a agent 2 应被绑定")
	require.Empty(t, agentToolBindings(t, db, "b1"), "org-b agent 不得被跨租户绑定")
	require.Empty(t, agentToolBindings(t, db, "b2"), "org-b agent 不得被跨租户绑定")
}

// TestToolRepository_GetDefaultTools_TenantWithShared：默认工具查询 = 本租户
// is_default 行 + 共享 is_default 行；不含其他租户的默认行。
func TestToolRepository_GetDefaultTools_TenantWithShared(t *testing.T) {
	db := setupToolDefaultTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('a-def', 'org-a', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('b-def', 'org-b', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('shared-def', '', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('b-plain', 'org-b', 0)`).Error)

	repo := NewToolRepository()

	ids, err := repo.GetDefaultToolIDs("org-b")
	require.NoError(t, err)
	names, err := repo.GetDefaultToolNames("org-b")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"b-def", "shared-def"}, names, "org-b 默认工具 = 本租户 + 共享，不含 org-a")
	require.Len(t, ids, 2)

	names, err = repo.GetDefaultToolNames("org-a")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a-def", "shared-def"}, names, "org-a 默认工具 = 本租户 + 共享，不含 org-b")
}

// TestToolRepository_BindDefaultToolsToAgent_TenantScoped：新建 agent 的默认
// 工具绑定只取该租户可见的默认工具（本租户 + 共享）。
func TestToolRepository_BindDefaultToolsToAgent_TenantScoped(t *testing.T) {
	db := setupToolDefaultTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('a-def', 'org-a', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('shared-def', '', 1)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('b-new', 'org-b')`).Error)

	var agentID uint64
	require.NoError(t, db.Raw(`SELECT id FROM agents WHERE name = 'b-new'`).Scan(&agentID).Error)

	repo := NewToolRepository()
	require.NoError(t, repo.BindDefaultToolsToAgent("org-b", agentID))

	var names []string
	require.NoError(t, db.Raw(`
		SELECT t.name FROM agent_tools at JOIN tools t ON t.id = at.tool_id
		WHERE at.agent_id = ?
	`, agentID).Pluck("name", &names).Error)
	require.ElementsMatch(t, []string{"shared-def"}, names, "org-b agent 只绑共享默认工具，不得绑 org-a 默认工具")
}
