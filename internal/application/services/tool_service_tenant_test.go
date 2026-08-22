package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupToolTenantServiceTestDB 起 sqlite 内存库（裸 SQL，索引名避免 sqlite 全局
// 唯一约束冲突，模式同 setupSubagentToolsTestDB），额外补齐 AgentService.
// CreateAgent 末尾 GetAgent 链路所需的 skills / mcps / 知识库绑定表。
func setupToolTenantServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			content_hash VARCHAR(128) NOT NULL DEFAULT '',
			system_prompt TEXT NOT NULL DEFAULT '',
			permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto',
			max_turns INTEGER NOT NULL DEFAULT 50,
			title JSON,
			description JSON,
			icon VARCHAR(512) DEFAULT '',
			icon_name VARCHAR(64) DEFAULT '',
			icon_color VARCHAR(32) DEFAULT '',
			icon_bg_color VARCHAR(64) DEFAULT '',
			provider_id INTEGER,
			model_id VARCHAR(64) DEFAULT '',
			model_selection_id VARCHAR(128) DEFAULT '',
			field_overrides TEXT,
			source VARCHAR(16) NOT NULL DEFAULT 'remote',
			desktop_enabled INTEGER NOT NULL DEFAULT 0,
			mobile_enabled INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER DEFAULT 0,
			group_name VARCHAR(64) DEFAULT '',
			max_session_turns INTEGER,
			runtime_port INTEGER DEFAULT 0,
			deployment_status VARCHAR(32) DEFAULT '',
			deployed_at DATETIME,
			runtime_token TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX agents_tuk_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(128) DEFAULT '',
			description TEXT,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX tools_tuk_tenant_name ON tools(tenant_id, name)`,
		`CREATE TABLE agent_tools (
			agent_id INTEGER NOT NULL,
			tool_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, tool_id)
		)`,
		`CREATE TABLE agent_subagents (
			agent_id INTEGER NOT NULL,
			subagent_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, subagent_id)
		)`,
		`CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(128) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE agent_skills (
			agent_id INTEGER NOT NULL,
			skill_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, skill_id)
		)`,
		`CREATE TABLE mcp_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(128) NOT NULL
		)`,
		`CREATE TABLE agent_mcp_servers (
			agent_id INTEGER NOT NULL,
			mcp_server_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, mcp_server_id)
		)`,
		`CREATE TABLE agent_knowledge_datasets (
			agent_id INTEGER NOT NULL,
			dataset_id VARCHAR(128) NOT NULL DEFAULT '',
			created_at DATETIME
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

func newMinimalAgent(name string) *agent.AgentConfig {
	return &agent.AgentConfig{Name: name}
}

// TestToolService_CreateDefaultTool_TenantScoped：org-a 创建 is_default 工具时
// AddToolToAllAgents 只绑 org-a 的 agent。
func TestToolService_CreateDefaultTool_TenantScoped(t *testing.T) {
	db := setupToolTenantServiceTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("org-a", newMinimalAgent("a1")))
	require.NoError(t, agentRepo.Create("org-b", newMinimalAgent("b1")))

	svc := NewToolService()
	_, err := svc.Create("org-a", &CreateToolInput{Name: "a-def", IsDefault: true})
	require.NoError(t, err)

	var names []string
	require.NoError(t, db.Raw(`
		SELECT t.name FROM agent_tools at JOIN tools t ON t.id = at.tool_id
		JOIN agents a ON a.id = at.agent_id WHERE a.name = 'b1'
	`).Pluck("name", &names).Error)
	assert.Empty(t, names, "org-b agent 不得被 org-a 的默认工具绑定")

	names = nil
	require.NoError(t, db.Raw(`
		SELECT t.name FROM agent_tools at JOIN tools t ON t.id = at.tool_id
		JOIN agents a ON a.id = at.agent_id WHERE a.name = 'a1'
	`).Pluck("name", &names).Error)
	assert.Contains(t, names, "a-def", "org-a 自家 agent 应被绑定")
}

// TestToolService_UpdateAgentTools_TenantIsolation：org-b 保存空工具列表时，
// 自动合并的默认工具不得包含 org-a 的默认工具（可含共享默认行）。
func TestToolService_UpdateAgentTools_TenantIsolation(t *testing.T) {
	db := setupToolTenantServiceTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("org-b", newMinimalAgent("b1")))

	toolSvc := NewToolService()
	_, err := toolSvc.Create("org-a", &CreateToolInput{Name: "a-def", IsDefault: true})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('shared-def', '', 1)`).Error)

	require.NoError(t, toolSvc.UpdateAgentTools("org-b", "b1", []string{}))

	b1, err := agentRepo.GetByName("org-b", "b1")
	require.NoError(t, err)
	toolRepo := repository.NewToolRepository()
	got, err := toolRepo.GetToolsByAgent(b1.ID)
	require.NoError(t, err)
	assert.NotContains(t, got, "a-def", "org-b agent 的绑定不得包含 org-a 默认工具")
	assert.Contains(t, got, "shared-def", "共享默认行对所有租户可见")
}

// TestAgentService_CreateAgent_DoesNotBindOtherTenantDefaultTools：org-b 新建
// agent 时默认工具绑定只取 org-b 可见的默认工具。
func TestAgentService_CreateAgent_DoesNotBindOtherTenantDefaultTools(t *testing.T) {
	db := setupToolTenantServiceTestDB(t)

	toolSvc := NewToolService()
	_, err := toolSvc.Create("org-a", &CreateToolInput{Name: "a-def", IsDefault: true})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, is_default) VALUES ('shared-def', '', 1)`).Error)

	agentSvc := NewAgentService("test-encryption-key")
	_, err = agentSvc.CreateAgent("org-b", &CreateAgentInput{
		Name:   "b-new",
		Config: map[string]interface{}{"systemPrompt": "test"},
	})
	require.NoError(t, err)

	agentRepo := repository.NewAgentRepository()
	bNew, err := agentRepo.GetByName("org-b", "b-new")
	require.NoError(t, err)
	toolRepo := repository.NewToolRepository()
	got, err := toolRepo.GetToolsByAgent(bNew.ID)
	require.NoError(t, err)
	assert.NotContains(t, got, "a-def", "org-b 新建 agent 不得绑定 org-a 默认工具")
	assert.Contains(t, got, "shared-def", "共享默认行应被绑定")
}
