package repository

import (
	"errors"
	"testing"

	"control-panel/internal/domain/mcp"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupMcpRepoTestDB 起 sqlite 内存库，mcp_servers 表用裸 SQL 建（与迁移后
// 生产 schema 对齐：tenant_id 列 + 复合唯一索引 uk_tenant_name）。
func setupMcpRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE mcp_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			title VARCHAR(128) NOT NULL,
			description TEXT,
			transport_type VARCHAR(16) NOT NULL,
			url VARCHAR(512),
			headers TEXT,
			retry_max_retries INTEGER,
			retry_timeout_ms INTEGER,
			is_builtin INTEGER NOT NULL DEFAULT 0,
			tools TEXT,
			probe_status VARCHAR(16) NOT NULL DEFAULT 'pending',
			last_probed_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tenant_name ON mcp_servers(tenant_id, name)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func seedMcpTenantData(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO mcp_servers (name, tenant_id, title, transport_type, is_builtin, probe_status)
		VALUES ('knowledge', '', '知识库检索', 'http', 1, 'success')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_servers (name, tenant_id, title, transport_type, is_builtin, probe_status)
		VALUES ('search-a', 'org-a', 'A 搜索', 'http', 0, 'pending')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO mcp_servers (name, tenant_id, title, transport_type, is_builtin, probe_status)
		VALUES ('search-b', 'org-b', 'B 搜索', 'http', 0, 'pending')`).Error)
}

func TestMcpRepository_ListAll_TenantWithShared(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	list, err := repo.ListAll("org-a")
	require.NoError(t, err)
	require.Len(t, list, 2, "本租户行 + 共享内置行")
	names := []string{list[0].Name, list[1].Name}
	require.ElementsMatch(t, []string{"knowledge", "search-a"}, names)

	list, err = repo.ListAll("org-c")
	require.NoError(t, err)
	require.Len(t, list, 1, "无租户行时仅见共享内置行")
	require.Equal(t, "knowledge", list[0].Name)
}

func TestMcpRepository_GetByNameGetByID_TenantIsolation(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	got, err := repo.GetByName("org-a", "search-a")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	// 共享内置行对所有租户可读
	got, err = repo.GetByName("org-b", "knowledge")
	require.NoError(t, err)
	require.Equal(t, "", got.TenantID)

	_, err = repo.GetByName("org-a", "search-b")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)

	_, err = repo.GetByID("org-a", got.ID)
	require.NoError(t, err)

	bRow, err := repo.GetByName("org-b", "search-b")
	require.NoError(t, err)
	_, err = repo.GetByID("org-a", bRow.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户按 ID 读必须返回 ErrRecordNotFound, got %v", err)
}

func TestMcpRepository_SameNameAcrossTenants(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	require.NoError(t, repo.Create("org-a", &mcp.McpServer{Name: "dup", Title: "t", TransportType: "http"}))
	require.NoError(t, repo.Create("org-b", &mcp.McpServer{Name: "dup", Title: "t", TransportType: "http"}))

	// 内置共享行已占用 name 时，租户行仍可同名共存
	require.NoError(t, repo.Create("org-a", &mcp.McpServer{Name: "knowledge", Title: "t", TransportType: "http"}))

	err := repo.Create("org-a", &mcp.McpServer{Name: "dup", Title: "t", TransportType: "http"})
	require.Error(t, err, "同租户同名必须违反复合唯一索引")
}

func TestMcpRepository_Create_StampsTenant(t *testing.T) {
	setupMcpRepoTestDB(t)
	repo := NewMcpRepository()

	m := &mcp.McpServer{Name: "coder", Title: "t", TransportType: "http", TenantID: "forged"}
	require.NoError(t, repo.Create("org-a", m))
	require.Equal(t, "org-a", m.TenantID)
}

func TestMcpRepository_BuiltinWriteProtection(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	shared, err := repo.GetByName("org-a", "knowledge")
	require.NoError(t, err)

	shared.Title = "hijacked"
	err = repo.Update("org-a", shared)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "共享内置行修改必须被拒, got %v", err)

	err = repo.Delete("org-a", shared.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "共享内置行删除必须被拒, got %v", err)

	var title string
	require.NoError(t, db.Raw(`SELECT title FROM mcp_servers WHERE name = 'knowledge'`).Scan(&title).Error)
	require.Equal(t, "知识库检索", title, "共享行未被改动")

	// 系统路径（tenantID=''）刷新内置行元数据允许
	shared.Title = "知识库检索 v2"
	require.NoError(t, repo.Update("", shared))
	require.NoError(t, db.Raw(`SELECT title FROM mcp_servers WHERE name = 'knowledge'`).Scan(&title).Error)
	require.Equal(t, "知识库检索 v2", title)
}

func TestMcpRepository_UpdateDelete_CrossTenantRejected(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	own, err := repo.GetByName("org-a", "search-a")
	require.NoError(t, err)

	own.Title = "stolen"
	err = repo.Update("org-b", own)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户写必须被拒, got %v", err)

	err = repo.Delete("org-b", own.ID)
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户删除必须被拒, got %v", err)

	own.Title = "renamed"
	require.NoError(t, repo.Update("org-a", own))
	require.NoError(t, repo.Delete("org-a", own.ID))

	var cnt int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM mcp_servers WHERE name = 'search-a'`).Scan(&cnt).Error)
	require.Equal(t, int64(0), cnt)
}

func TestMcpRepository_ExistsByName_TenantIsolation(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	exists, err := repo.ExistsByName("org-a", "search-b")
	require.NoError(t, err)
	require.False(t, exists, "跨租户 ExistsByName 必须为 false")

	exists, err = repo.ExistsByName("org-c", "knowledge")
	require.NoError(t, err)
	require.True(t, exists, "共享内置行对所有租户可见")
}

func TestMcpRepository_CountByIDs_GetMcpsByIDs_TenantIsolation(t *testing.T) {
	db := setupMcpRepoTestDB(t)
	seedMcpTenantData(t, db)
	repo := NewMcpRepository()

	shared, err := repo.GetByName("org-a", "knowledge")
	require.NoError(t, err)
	own, err := repo.GetByName("org-a", "search-a")
	require.NoError(t, err)
	bRow, err := repo.GetByName("org-b", "search-b")
	require.NoError(t, err)

	count, err := repo.CountByIDs("org-a", []uint64{shared.ID, own.ID, bRow.ID})
	require.NoError(t, err)
	require.Equal(t, int64(2), count, "共享 + 本租户计入，他租户不计入")

	items, err := repo.GetMcpsByIDs("org-a", []uint64{shared.ID, own.ID, bRow.ID})
	require.NoError(t, err)
	require.Len(t, items, 2)
}
