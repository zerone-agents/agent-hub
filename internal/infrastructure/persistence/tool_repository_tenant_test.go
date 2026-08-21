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

// setupToolRepoTestDB 起 sqlite 内存库，tools 表用裸 SQL 建（与迁移后的
// 生产 schema 对齐：tenant_id 列 + 复合唯一索引 uk_tenant_name），
// 模式同 agent_repository_test.go。
func setupToolRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			title VARCHAR(128),
			description TEXT,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tenant_name ON tools(tenant_id, name)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func seedToolTenantData(t *testing.T, db *gorm.DB) {
	t.Helper()
	// 共享内置行 + 两个租户各一行
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('Skill', '', '内置')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('web-a', 'org-a', 'A 工具')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title) VALUES ('web-b', 'org-b', 'B 工具')`).Error)
}

func TestToolRepository_ListAll_TenantWithShared(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	list, err := repo.ListAll("org-a")
	require.NoError(t, err)
	require.Len(t, list, 2, "本租户行 + 共享内置行")
	names := []string{list[0].Name, list[1].Name}
	require.ElementsMatch(t, []string{"Skill", "web-a"}, names)

	list, err = repo.ListAll("org-c")
	require.NoError(t, err)
	require.Len(t, list, 1, "无租户行时仅见共享内置行")
	require.Equal(t, "Skill", list[0].Name)
}

func TestToolRepository_GetByName_TenantIsolation(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	got, err := repo.GetByName("org-a", "web-a")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	// 共享内置行对所有租户可读
	got, err = repo.GetByName("org-b", "Skill")
	require.NoError(t, err)
	require.Equal(t, "", got.TenantID)

	// 跨租户不可见
	_, err = repo.GetByName("org-a", "web-b")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)
}

func TestToolRepository_SameNameAcrossTenants(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	require.NoError(t, repo.Create("org-a", &agent.Tool{Name: "dup"}))
	require.NoError(t, repo.Create("org-b", &agent.Tool{Name: "dup"}))

	// 内置共享行已占用 name 时，租户行仍可同名共存
	require.NoError(t, repo.Create("org-a", &agent.Tool{Name: "Skill"}))

	err := repo.Create("org-a", &agent.Tool{Name: "dup"})
	require.Error(t, err, "同租户同名必须违反复合唯一索引")
}

func TestToolRepository_Create_StampsTenant(t *testing.T) {
	setupToolRepoTestDB(t)
	repo := NewToolRepository()

	t2 := &agent.Tool{Name: "coder", TenantID: "forged"}
	require.NoError(t, repo.Create("org-a", t2))
	require.Equal(t, "org-a", t2.TenantID)
}

func TestToolRepository_BuiltinWriteProtection(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	shared, err := repo.GetByName("org-a", "Skill")
	require.NoError(t, err)

	shared.Title = "hijacked"
	err = repo.Update("org-a", shared)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "共享内置行修改必须被拒, got %v", err)

	err = repo.Delete("org-a", shared.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "共享内置行删除必须被拒, got %v", err)

	var title string
	require.NoError(t, db.Raw(`SELECT title FROM tools WHERE name = 'Skill'`).Scan(&title).Error)
	require.Equal(t, "内置", title, "共享行未被改动")
}

func TestToolRepository_UpdateDelete_CrossTenantRejected(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	own, err := repo.GetByName("org-a", "web-a")
	require.NoError(t, err)

	own.Title = "stolen"
	err = repo.Update("org-b", own)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户写必须被拒, got %v", err)

	err = repo.Delete("org-b", own.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户删除必须被拒, got %v", err)

	var cnt int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM tools WHERE name = 'web-a'`).Scan(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响目标行")

	// 同租户写生效
	own.Title = "renamed"
	require.NoError(t, repo.Update("org-a", own))
	var renamed string
	require.NoError(t, db.Raw(`SELECT title FROM tools WHERE name = 'web-a'`).Scan(&renamed).Error)
	require.Equal(t, "renamed", renamed)
}

func TestToolRepository_ExistsByName_TenantIsolation(t *testing.T) {
	db := setupToolRepoTestDB(t)
	seedToolTenantData(t, db)
	repo := NewToolRepository()

	exists, err := repo.ExistsByName("org-a", "web-a")
	require.NoError(t, err)
	require.True(t, exists)

	exists, err = repo.ExistsByName("org-a", "web-b")
	require.NoError(t, err)
	require.False(t, exists, "跨租户 ExistsByName 必须为 false")

	exists, err = repo.ExistsByName("org-c", "Skill")
	require.NoError(t, err)
	require.True(t, exists, "共享内置行对所有租户可见")
}
