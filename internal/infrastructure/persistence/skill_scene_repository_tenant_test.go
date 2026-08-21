package repository

import (
	"errors"
	"testing"

	"control-panel/internal/domain/scene"
	"control-panel/internal/domain/skill"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupSkillSceneTestDB 起 sqlite 内存库，skills/scenes 表用裸 SQL 建（与
// 迁移后生产 schema 对齐：tenant_id 列 + 复合唯一索引 uk_tenant_name）。
// scenes 带 agent_id 归属列；agents 仅两列供外键式引用。
func setupSkillSceneTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			type VARCHAR(32) NOT NULL DEFAULT 'expert',
			title VARCHAR(128) NOT NULL,
			title_en VARCHAR(128),
			description TEXT,
			description_en TEXT,
			url VARCHAR(512),
			file_hash VARCHAR(128),
			file_size INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tenant_name ON skills(tenant_id, name)`,
		`CREATE TABLE scenes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			agent_id INTEGER NOT NULL,
			title VARCHAR(128) NOT NULL,
			title_en VARCHAR(128),
			prompt TEXT NOT NULL,
			prompt_en TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tenant_name_scene ON scenes(tenant_id, name)`,
		`CREATE TABLE agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default'
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func seedSkillTenantData(t *testing.T, db *gorm.DB) {
	t.Helper()
	// skills 没有内置种子语义，直接造两租户数据
	require.NoError(t, db.Exec(`INSERT INTO skills (name, tenant_id, title) VALUES ('writer-a', 'org-a', 'A 技能')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO skills (name, tenant_id, title) VALUES ('writer-b', 'org-b', 'B 技能')`).Error)
}

func TestSkillRepository_TenantIsolation(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSkillTenantData(t, db)
	repo := NewSkillRepository()

	list, err := repo.ListAll("org-a")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "writer-a", list[0].Name)

	list, err = repo.ListByType("org-a", "expert")
	require.NoError(t, err)
	require.Len(t, list, 1)

	got, err := repo.GetByName("org-a", "writer-a")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	_, err = repo.GetByName("org-a", "writer-b")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)

	exists, err := repo.ExistsByName("org-a", "writer-b")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestSkillRepository_SameNameAcrossTenants(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSkillTenantData(t, db)
	repo := NewSkillRepository()

	require.NoError(t, repo.Create("org-a", &skill.Skill{Name: "dup", Title: "t"}))
	require.NoError(t, repo.Create("org-b", &skill.Skill{Name: "dup", Title: "t"}))

	err := repo.Create("org-a", &skill.Skill{Name: "dup", Title: "t"})
	require.Error(t, err, "同租户同名必须违反复合唯一索引")
}

func TestSkillRepository_WriteScopes(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSkillTenantData(t, db)
	repo := NewSkillRepository()

	own, err := repo.GetByName("org-a", "writer-a")
	require.NoError(t, err)

	own.Title = "stolen"
	err = repo.Update("org-b", own)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户写必须被拒, got %v", err)

	err = repo.Delete("org-b", own.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户删除必须被拒, got %v", err)

	sk := &skill.Skill{Name: "coder", Title: "t", TenantID: "forged"}
	require.NoError(t, repo.Create("org-a", sk))
	require.Equal(t, "org-a", sk.TenantID, "Create 必须强制盖章请求租户")
}

// TestSkillRepository_RejectsEmptyTenantID skills 没有系统写通道：空
// tenantID 的 Create/Update/Delete 必须显式拒绝，防止 mustOwn 的系统路径
// 放行 + 盖章 ” 把租户私有 skill 静默提升为全局共享行。
func TestSkillRepository_RejectsEmptyTenantID(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSkillTenantData(t, db)
	repo := NewSkillRepository()

	own, err := repo.GetByName("org-a", "writer-a")
	require.NoError(t, err)

	err = repo.Create("", &skill.Skill{Name: "ghost", Title: "t"})
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Create 必须返回 ErrTenantIDRequired, got %v", err)

	own.Title = "promoted"
	err = repo.Update("", own)
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Update 必须返回 ErrTenantIDRequired, got %v", err)

	err = repo.Delete("", own.ID)
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Delete 必须返回 ErrTenantIDRequired, got %v", err)

	// 行本身未被改动/提升
	got, err := repo.GetByName("org-a", "writer-a")
	require.NoError(t, err)
	require.Equal(t, "A 技能", got.Title)
	require.Equal(t, "org-a", got.TenantID, "租户行不得被盖章为共享")
	var ghostCount int64
	db.Raw(`SELECT COUNT(*) FROM skills WHERE name = 'ghost'`).Scan(&ghostCount)
	require.Equal(t, int64(0), ghostCount)
}

func seedSceneTenantData(t *testing.T, db *gorm.DB) (aScene, bScene uint64) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('bot-a', 'org-a')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id) VALUES ('bot-b', 'org-b')`).Error)
	var aID, bID uint64
	require.NoError(t, db.Raw(`SELECT id FROM agents WHERE name = 'bot-a'`).Scan(&aID).Error)
	require.NoError(t, db.Raw(`SELECT id FROM agents WHERE name = 'bot-b'`).Scan(&bID).Error)
	require.NoError(t, db.Exec(`INSERT INTO scenes (name, tenant_id, agent_id, title, prompt) VALUES ('chat-a', 'org-a', ?, 'A 场景', 'p')`, aID).Error)
	require.NoError(t, db.Exec(`INSERT INTO scenes (name, tenant_id, agent_id, title, prompt) VALUES ('chat-b', 'org-b', ?, 'B 场景', 'p')`, bID).Error)
	return aID, bID
}

func TestSceneRepository_TenantIsolation(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	aAgentID, _ := seedSceneTenantData(t, db)
	repo := NewSceneRepository()

	list, err := repo.ListAll("org-a")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "chat-a", list[0].Name)

	list, err = repo.ListByAgent("org-a", aAgentID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	got, err := repo.GetByName("org-a", "chat-a")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	_, err = repo.GetByName("org-a", "chat-b")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)

	exists, err := repo.ExistsByName("org-a", "chat-b")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestSceneRepository_SameNameAcrossTenants(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	aAgentID, _ := seedSceneTenantData(t, db)
	repo := NewSceneRepository()

	require.NoError(t, repo.Create("org-a", &scene.Scene{Name: "dup", AgentID: aAgentID, Title: "t", Prompt: "p"}))
	require.NoError(t, repo.Create("org-b", &scene.Scene{Name: "dup", AgentID: aAgentID, Title: "t", Prompt: "p"}))

	err := repo.Create("org-a", &scene.Scene{Name: "dup", AgentID: aAgentID, Title: "t", Prompt: "p"})
	require.Error(t, err, "同租户同名必须违反复合唯一索引")
}

func TestSceneRepository_WriteScopes(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSceneTenantData(t, db)
	repo := NewSceneRepository()

	own, err := repo.GetByName("org-a", "chat-a")
	require.NoError(t, err)

	own.Title = "stolen"
	err = repo.Update("org-b", own)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户写必须被拒, got %v", err)

	err = repo.Delete("org-b", own.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户删除必须被拒, got %v", err)

	sc := &scene.Scene{Name: "coder", AgentID: own.AgentID, Title: "t", Prompt: "p", TenantID: "forged"}
	require.NoError(t, repo.Create("org-a", sc))
	require.Equal(t, "org-a", sc.TenantID, "Create 必须强制盖章请求租户")
}

// TestSceneRepository_RejectsEmptyTenantID scenes 没有系统写通道：空
// tenantID 的 Create/Update/Delete 必须显式拒绝（同 skill，防止私有行被
// 盖章 ” 提升为全局共享行）。
func TestSceneRepository_RejectsEmptyTenantID(t *testing.T) {
	db := setupSkillSceneTestDB(t)
	seedSceneTenantData(t, db)
	repo := NewSceneRepository()

	own, err := repo.GetByName("org-a", "chat-a")
	require.NoError(t, err)

	err = repo.Create("", &scene.Scene{Name: "ghost", AgentID: own.AgentID, Title: "t", Prompt: "p"})
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Create 必须返回 ErrTenantIDRequired, got %v", err)

	own.Title = "promoted"
	err = repo.Update("", own)
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Update 必须返回 ErrTenantIDRequired, got %v", err)

	err = repo.Delete("", own.ID)
	require.True(t, errors.Is(err, ErrTenantIDRequired), "空租户 Delete 必须返回 ErrTenantIDRequired, got %v", err)

	got, err := repo.GetByName("org-a", "chat-a")
	require.NoError(t, err)
	require.Equal(t, "A 场景", got.Title)
	require.Equal(t, "org-a", got.TenantID, "租户行不得被盖章为共享")
	var ghostCount int64
	db.Raw(`SELECT COUNT(*) FROM scenes WHERE name = 'ghost'`).Scan(&ghostCount)
	require.Equal(t, int64(0), ghostCount)
}
