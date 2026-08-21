package repository

import (
	"errors"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAgentRepoTestDB 起 sqlite 内存库并替换 database.DB 包级变量
// （与 chat_repository_test.go 同一基建模式）。agents 表用裸 SQL 建：
// 1) 与迁移后的生产 schema 对齐（tenant_id 列 + 复合唯一索引 uk_tenant_name）；
// 2) 避开 GORM AutoMigrate 在 sqlite 上沿外键漫游到其它 uk_name 表的问题
// （见 subagent_tools_test.go 的注释）。
func setupAgentRepoTestDB(t *testing.T) *gorm.DB {
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
		`CREATE UNIQUE INDEX uk_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE agent_subagents (
			agent_id INTEGER NOT NULL,
			subagent_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, subagent_id)
		)`,
		`CREATE TABLE agent_tools (
			agent_id INTEGER NOT NULL,
			tool_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, tool_id)
		)`,
		`CREATE TABLE agent_skills (
			agent_id INTEGER NOT NULL,
			skill_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, skill_id)
		)`,
		`CREATE TABLE agent_mcp_servers (
			agent_id INTEGER NOT NULL,
			mcp_server_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, mcp_server_id)
		)`,
		`CREATE TABLE agent_knowledge_datasets (
			agent_id INTEGER NOT NULL,
			dataset_id VARCHAR(64) NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, dataset_id)
		)`,
		// 聚合查询 JOIN 的对端表，只需 id/name 两列（插入走裸 SQL）
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL
		)`,
		`CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL
		)`,
		`CREATE TABLE mcp_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

// seedAgentTenantData 造两个租户各一个 agent（不同名），并给 org-a 的 agent
// 挂一条 subagent 关联行（供跨租户删除保护测试用）。
func seedAgentTenantData(t *testing.T, db *gorm.DB) (aAgent, bAgent *agent.AgentConfig) {
	t.Helper()
	aAgent = &agent.AgentConfig{Name: "coder-a", TenantID: "org-a", DesktopEnabled: true}
	bAgent = &agent.AgentConfig{Name: "coder-b", TenantID: "org-b", DesktopEnabled: true}
	require.NoError(t, db.Create(aAgent).Error)
	require.NoError(t, db.Create(bAgent).Error)
	require.NoError(t, db.Create(&agent.AgentSubagent{AgentID: aAgent.ID, SubagentID: aAgent.ID}).Error)
	return aAgent, bAgent
}

func TestAgentRepository_CompositeUniqueIndex_SameNameAcrossTenants(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewAgentRepository()

	// 复合索引 (tenant_id, name) 允许跨租户同名
	require.NoError(t, repo.Create("org-a", &agent.AgentConfig{Name: "coder"}))
	require.NoError(t, repo.Create("org-b", &agent.AgentConfig{Name: "coder"}))

	var count int64
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("name = ?", "coder").Count(&count).Error)
	require.Equal(t, int64(2), count)

	// 同租户同名仍冲突
	err := repo.Create("org-a", &agent.AgentConfig{Name: "coder"})
	require.Error(t, err, "同租户同名必须违反复合唯一索引")
}

func TestAgentRepository_GetByName_TenantIsolation(t *testing.T) {
	setupAgentRepoTestDB(t)
	repo := NewAgentRepository()
	require.NoError(t, repo.Create("org-a", &agent.AgentConfig{Name: "coder"}))
	require.NoError(t, repo.Create("org-b", &agent.AgentConfig{Name: "coder", ModelID: "b-model"}))

	got, err := repo.GetByName("org-a", "coder")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	got, err = repo.GetByName("org-b", "coder")
	require.NoError(t, err)
	require.Equal(t, "b-model", got.ModelID)

	_, err = repo.GetByName("org-c", "coder")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)
}

func TestAgentRepository_GetByID_CrossTenantNotFound(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	aAgent, _ := seedAgentTenantData(t, db)
	repo := NewAgentRepository()

	_, err := repo.GetByID("org-b", aAgent.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户按 ID 读必须返回 ErrRecordNotFound, got %v", err)

	got, err := repo.GetByID("org-a", aAgent.ID)
	require.NoError(t, err)
	require.Equal(t, "coder-a", got.Name)
}

func TestAgentRepository_ListAll_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	seedAgentTenantData(t, db)
	repo := NewAgentRepository()

	list, err := repo.ListAll("org-a")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "coder-a", list[0].Name)

	list, err = repo.ListAll("org-b")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "coder-b", list[0].Name)

	list, err = repo.ListAll("org-c")
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestAgentRepository_ListForPlatform_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	seedAgentTenantData(t, db)
	require.NoError(t, db.Create(&agent.AgentConfig{Name: "mobile-b", TenantID: "org-b", MobileEnabled: true}).Error)
	repo := NewAgentRepository()

	desktop, err := repo.ListForPlatform("org-a", agent.PlatformDesktop)
	require.NoError(t, err)
	require.Equal(t, []string{"coder-a"}, namesOf(desktop))

	desktop, err = repo.ListForPlatform("org-b", agent.PlatformDesktop)
	require.NoError(t, err)
	require.Equal(t, []string{"coder-b"}, namesOf(desktop))

	mobile, err := repo.ListForPlatform("org-b", agent.PlatformMobile)
	require.NoError(t, err)
	require.Equal(t, []string{"mobile-b"}, namesOf(mobile))

	mobile, err = repo.ListForPlatform("org-a", agent.PlatformMobile)
	require.NoError(t, err)
	require.Empty(t, mobile)
}

func TestAgentRepository_ExistsAndExistsByName_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	aAgent, _ := seedAgentTenantData(t, db)
	repo := NewAgentRepository()

	exists, err := repo.Exists("org-a", aAgent.ID)
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = repo.Exists("org-b", aAgent.ID)
	require.NoError(t, err)
	require.False(t, exists, "跨租户 Exists 必须为 false（不暴露存在性）")

	exists, err = repo.ExistsByName("org-a", "coder-a")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = repo.ExistsByName("org-b", "coder-a")
	require.NoError(t, err)
	require.False(t, exists, "跨租户 ExistsByName 必须为 false（不暴露存在性）")
}

func TestAgentRepository_ClearDefaultExcept_TenantScoped(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewAgentRepository()

	a1 := &agent.AgentConfig{Name: "a1", TenantID: "org-a", IsDefault: true}
	a2 := &agent.AgentConfig{Name: "a2", TenantID: "org-a", IsDefault: true}
	b1 := &agent.AgentConfig{Name: "b1", TenantID: "org-b", IsDefault: true}
	require.NoError(t, db.Create(a1).Error)
	require.NoError(t, db.Create(a2).Error)
	require.NoError(t, db.Create(b1).Error)

	require.NoError(t, repo.ClearDefaultExcept("org-a", a2.ID))

	require.NoError(t, db.First(a1, a1.ID).Error)
	require.False(t, a1.IsDefault, "本租户其它 agent 的 default 必须被清掉")
	require.NoError(t, db.First(a2, a2.ID).Error)
	require.True(t, a2.IsDefault, "保留项不受影响")
	require.NoError(t, db.First(b1, b1.ID).Error)
	require.True(t, b1.IsDefault, "跨租户的 default 标记不得被清")

	require.NoError(t, repo.ClearAllDefault("org-b"))
	require.NoError(t, db.First(b1, b1.ID).Error)
	require.False(t, b1.IsDefault)
	require.NoError(t, db.First(a2, a2.ID).Error)
	require.True(t, a2.IsDefault, "ClearAllDefault 也只清本租户")
}

func TestAgentRepository_Create_StampsTenant(t *testing.T) {
	setupAgentRepoTestDB(t)
	repo := NewAgentRepository()

	// 调用方传入的 TenantID 不可信，repository 必须强制盖章
	cfg := &agent.AgentConfig{Name: "coder", TenantID: "forged"}
	require.NoError(t, repo.Create("org-a", cfg))
	require.Equal(t, "org-a", cfg.TenantID)

	got, err := repo.GetByName("org-a", "coder")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	_, err = repo.GetByName("forged", "coder")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestAgentRepository_Update_CrossTenantRejected(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	aAgent, _ := seedAgentTenantData(t, db)
	repo := NewAgentRepository()

	// 跨租户 Update：必须先取到本租户行再改，直接拿他租户行写必须被拒
	stolen, err := repo.GetByID("org-a", aAgent.ID)
	require.NoError(t, err)
	stolen.ModelID = "hijacked"
	err = repo.Update("org-b", stolen)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户写必须返回 ErrRecordNotFound, got %v", err)

	// 原行未被改动
	require.NoError(t, db.First(aAgent, aAgent.ID).Error)
	require.NotEqual(t, "hijacked", aAgent.ModelID)

	// 同租户 Update 生效（盖章防伪造）
	stolen.ModelID = "new-model"
	stolen.TenantID = "forged"
	require.NoError(t, repo.Update("org-a", stolen))
	require.NoError(t, db.First(aAgent, aAgent.ID).Error)
	require.Equal(t, "new-model", aAgent.ModelID)
	require.Equal(t, "org-a", aAgent.TenantID)
}

func TestAgentRepository_Delete_CrossTenantRejected(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	aAgent, bAgent := seedAgentTenantData(t, db)
	repo := NewAgentRepository()

	// 跨租户删除：返回 ErrRecordNotFound（不暴露存在性），主行和关联行都不动
	err := repo.Delete("org-b", aAgent.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户删除必须返回 ErrRecordNotFound, got %v", err)

	var cnt int64
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("id = ?", aAgent.ID).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响目标行")
	require.NoError(t, db.Model(&agent.AgentSubagent{}).Where("agent_id = ?", aAgent.ID).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响关联行")

	// 同租户删除生效（主行 + 关联行级联）
	require.NoError(t, repo.Delete("org-a", aAgent.ID))
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("id = ?", aAgent.ID).Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
	require.NoError(t, db.Model(&agent.AgentSubagent{}).Where("agent_id = ?", aAgent.ID).Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)

	// org-b 的行不受影响
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("id = ?", bAgent.ID).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)
}

func TestAgentRepository_GetAllSubagents_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewAgentRepository()

	// 两租户都有名为 parent/child 的 agent（复合索引允许），各自建绑定
	aParent := &agent.AgentConfig{Name: "parent", TenantID: "org-a"}
	aChild := &agent.AgentConfig{Name: "child", TenantID: "org-a"}
	bParent := &agent.AgentConfig{Name: "parent", TenantID: "org-b"}
	bChild := &agent.AgentConfig{Name: "child", TenantID: "org-b"}
	require.NoError(t, db.Create(aParent).Error)
	require.NoError(t, db.Create(aChild).Error)
	require.NoError(t, db.Create(bParent).Error)
	require.NoError(t, db.Create(bChild).Error)
	require.NoError(t, db.Create(&agent.AgentSubagent{AgentID: aParent.ID, SubagentID: aChild.ID}).Error)
	require.NoError(t, db.Create(&agent.AgentSubagent{AgentID: bParent.ID, SubagentID: bChild.ID}).Error)

	// 同名 agent 跨租户时，按 name 聚合的 map 不能混入他租户的绑定
	aMap, err := repo.GetAllSubagents("org-a")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"parent": {"child"}}, aMap)

	bMap, err := repo.GetAllSubagents("org-b")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"parent": {"child"}}, bMap)
}

func TestAgentRepository_GetAllAgentKnowledgeDatasetIDs_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewAgentRepository()

	a := &agent.AgentConfig{Name: "coder", TenantID: "org-a"}
	b := &agent.AgentConfig{Name: "coder", TenantID: "org-b"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	require.NoError(t, db.Create(&agent.AgentKnowledgeDataset{AgentID: a.ID, DatasetID: "ds-a"}).Error)
	require.NoError(t, db.Create(&agent.AgentKnowledgeDataset{AgentID: b.ID, DatasetID: "ds-b"}).Error)

	aMap, err := repo.GetAllAgentKnowledgeDatasetIDs("org-a")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"ds-a"}}, aMap)

	bMap, err := repo.GetAllAgentKnowledgeDatasetIDs("org-b")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"ds-b"}}, bMap)
}
