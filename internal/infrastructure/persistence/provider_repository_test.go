package repository

import (
	"testing"

	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupProviderRepoTestDB 起 sqlite 内存库并替换 database.DB 包级变量
// （与 agent_repository_test.go 同一基建模式）。裸 SQL 建三张表，schema 与
// 迁移后的生产对齐：provider_summaries 带 tenant_id + 复合唯一索引
// uk_tenant_key；子表不加 tenant 列（归属经主表校验）。
func setupProviderRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE provider_summaries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tenant_id VARCHAR(64) NOT NULL DEFAULT '',
			` + "`key`" + ` VARCHAR(64) NOT NULL,
			name VARCHAR(128) NOT NULL,
			description TEXT,
			description_en TEXT,
			protocol VARCHAR(16) NOT NULL,
			auth_style VARCHAR(16) NOT NULL,
			base_url VARCHAR(512),
			fields TEXT,
			icon_key VARCHAR(32),
			builtin INTEGER DEFAULT 0,
			locked_api_key TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_tenant_key ON provider_summaries(tenant_id, ` + "`key`" + `)`,
		`CREATE TABLE provider_attributes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			attr_key VARCHAR(64) NOT NULL,
			attr_type VARCHAR(16) NOT NULL,
			attr_value TEXT,
			UNIQUE (provider_id, attr_key)
		)`,
		`CREATE TABLE provider_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL,
			selection_id VARCHAR(128) NOT NULL,
			model_id VARCHAR(128) NOT NULL,
			display_name VARCHAR(256),
			model_type VARCHAR(16) NOT NULL,
			context_window INTEGER,
			efforts TEXT,
			aigc_code VARCHAR(4) DEFAULT '',
			status VARCHAR(8) DEFAULT '1',
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			UNIQUE (provider_id, selection_id)
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

func seedProvider(t *testing.T, db *gorm.DB, tenantID, key, lockedAPIKey string) *provider.ProviderSummary {
	t.Helper()
	p := &provider.ProviderSummary{
		TenantID:     tenantID,
		Key:          key,
		Name:         "Provider " + key,
		Protocol:     "openai",
		AuthStyle:    "api_key",
		LockedAPIKey: lockedAPIKey,
	}
	require.NoError(t, db.Create(p).Error)
	return p
}

// TestProviderRepository_CompositeUniqueIndex_SameKeyAcrossTenants 验证
// uk_tenant_key 复合索引允许两个租户各自持有同 key 的 provider。
func TestProviderRepository_CompositeUniqueIndex_SameKeyAcrossTenants(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	seedProvider(t, db, "tenant-a", "openai", "")
	seedProvider(t, db, "tenant-b", "openai", "") // 不再触发全局唯一冲突
}

// TestProviderRepository_GetByID_CrossTenantNotFound 跨租户 GetByID 必须
// 返回 gorm.ErrRecordNotFound（不暴露存在性）。
func TestProviderRepository_GetByID_CrossTenantNotFound(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	b := seedProvider(t, db, "tenant-b", "openai", "enc-b")

	repo := NewProviderRepository()
	_, err := repo.GetByID("tenant-a", b.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestProviderRepository_GetByID_SharedVisibleToAll 共享行（tenant_id=”，
// 种子模板）对所有租户可读——copy-on-write 语义的读路径。
func TestProviderRepository_GetByID_SharedVisibleToAll(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	shared := seedProvider(t, db, "", "builtin-ocr", "")

	repo := NewProviderRepository()
	for _, tenantID := range []string{"tenant-a", "tenant-b"} {
		got, err := repo.GetByID(tenantID, shared.ID)
		require.NoError(t, err, tenantID)
		require.Equal(t, "builtin-ocr", got.Key)
	}
}

// TestProviderRepository_ListAll_TenantIsolationAndKeyLeak 租户 A 的 ListAll
// 不能出现租户 B 的 provider——provider_summaries 含加密 API key，跨租户
// 泄露等于密钥泄露。断言 B 的密文字段与 key 均不出现在 A 的结果里。
func TestProviderRepository_ListAll_TenantIsolationAndKeyLeak(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	seedProvider(t, db, "tenant-a", "a-provider", "enc-a")
	seedProvider(t, db, "tenant-b", "b-provider", "enc-super-secret-b")
	seedProvider(t, db, "", "shared-seed", "enc-shared")

	repo := NewProviderRepository()
	items, err := repo.ListAll("tenant-a")
	require.NoError(t, err)

	keys := map[string]string{} // key -> locked api key
	for _, p := range items {
		keys[p.Key] = p.LockedAPIKey
	}
	require.Contains(t, keys, "a-provider")
	require.Contains(t, keys, "shared-seed") // 共享种子可见
	require.NotContains(t, keys, "b-provider")
	require.NotContains(t, keys, "enc-super-secret-b") // 密钥字段不外泄
	require.Equal(t, "enc-shared", keys["shared-seed"])
}

// TestProviderRepository_ExistsByKey_TenantScope 同租户或共享行命中才算
// exists；其他租户的同 key 行不算（允许本租户创建同 key provider）。
func TestProviderRepository_ExistsByKey_TenantScope(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	seedProvider(t, db, "tenant-b", "openai", "")
	seedProvider(t, db, "", "shared", "")

	repo := NewProviderRepository()
	exists, err := repo.ExistsByKey("tenant-a", "openai")
	require.NoError(t, err)
	require.False(t, exists) // B 的同 key 行不阻塞 A 创建

	exists, err = repo.ExistsByKey("tenant-a", "shared")
	require.NoError(t, err)
	require.True(t, exists) // 共享行全局唯一 key，仍阻塞

	exists, err = repo.ExistsByKey("tenant-b", "openai")
	require.NoError(t, err)
	require.True(t, exists)
}

// TestProviderRepository_Count_Unscoped Count 只服务启动期 SeedIfEmpty
// （表全空才播种共享种子），跨租户全表计数。
func TestProviderRepository_Count_Unscoped(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	seedProvider(t, db, "tenant-a", "a", "")
	seedProvider(t, db, "tenant-b", "b", "")

	repo := NewProviderRepository()
	count, err := repo.Count()
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

// TestProviderRepository_SubtableWrite_CrossTenantRejected 子表写路径前置
// mustOwnProvider：租户 A 不能改/删租户 B 或共享 provider 的属性与模型。
func TestProviderRepository_SubtableWrite_CrossTenantRejected(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	b := seedProvider(t, db, "tenant-b", "openai", "")
	shared := seedProvider(t, db, "", "shared", "")

	repo := NewProviderRepository()

	err := repo.SetAttributes("tenant-a", b.ID, map[string]provider.AttrValue{
		"k": {Type: "string", Value: "v"},
	})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	err = repo.ReplaceModels("tenant-a", b.ID, []provider.ProviderModel{{ModelID: "m", SelectionID: "s", ModelType: "llm"}})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	err = repo.DeleteModel("tenant-a", b.ID, "whatever")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 共享行同样不可写（copy-on-write：写共享模板前必须先复制为本租户行）
	err = repo.SetAttributes("tenant-a", shared.ID, map[string]provider.AttrValue{
		"k": {Type: "string", Value: "v"},
	})
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestProviderRepository_SubtableRead_CrossTenantRejected 子表读路径同样
// 校验归属（可读自己 + 共享，不可读他租户）。
func TestProviderRepository_SubtableRead_CrossTenantRejected(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	b := seedProvider(t, db, "tenant-b", "openai", "")
	shared := seedProvider(t, db, "", "shared", "")
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: b.ID, SelectionID: "b-sel", ModelID: "b-model", ModelType: "llm",
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: shared.ID, SelectionID: "sh-sel", ModelID: "sh-model", ModelType: "llm",
	}).Error)

	repo := NewProviderRepository()

	_, err := repo.ListModels("tenant-a", b.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	_, err = repo.GetAttributes("tenant-a", b.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 共享行可读
	models, err := repo.ListModels("tenant-a", shared.ID)
	require.NoError(t, err)
	require.Len(t, models, 1)
}

// TestProviderRepository_ListAllModels_TenantScope ListAllModels 只返回
// 本租户 + 共享 provider 的模型行。
func TestProviderRepository_ListAllModels_TenantScope(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	a := seedProvider(t, db, "tenant-a", "a", "")
	b := seedProvider(t, db, "tenant-b", "b", "")
	shared := seedProvider(t, db, "", "shared", "")
	for _, p := range []*provider.ProviderSummary{a, b, shared} {
		require.NoError(t, db.Create(&provider.ProviderModel{
			ProviderID: p.ID, SelectionID: "sel", ModelID: "m-" + p.Key, ModelType: "llm",
		}).Error)
	}

	repo := NewProviderRepository()
	rows, err := repo.ListAllModels("tenant-a")
	require.NoError(t, err)
	modelIDs := map[string]bool{}
	for _, r := range rows {
		modelIDs[r.ModelID] = true
	}
	require.True(t, modelIDs["m-a"])
	require.True(t, modelIDs["m-shared"])
	require.False(t, modelIDs["m-b"])
}

// TestProviderRepository_Delete_CrossTenantRejected 跨租户删除被拒；
// 本租户删除级联清掉子表行。
func TestProviderRepository_Delete_CrossTenantRejected(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	a := seedProvider(t, db, "tenant-a", "a", "")
	shared := seedProvider(t, db, "", "shared", "")
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: a.ID, SelectionID: "sel", ModelID: "m", ModelType: "llm",
	}).Error)

	repo := NewProviderRepository()
	require.ErrorIs(t, repo.Delete("tenant-b", a.ID), gorm.ErrRecordNotFound)
	require.ErrorIs(t, repo.Delete("tenant-a", shared.ID), gorm.ErrRecordNotFound) // 共享模板不可删

	require.NoError(t, repo.Delete("tenant-a", a.ID))
	var n int64
	require.NoError(t, db.Model(&provider.ProviderModel{}).Where("provider_id = ?", a.ID).Count(&n).Error)
	require.Zero(t, n)
}

// TestProviderRepository_CopyForTenant copy-on-write：把共享模板完整复制
// 为本租户行（summary + attrs + models），原共享行不动。
func TestProviderRepository_CopyForTenant(t *testing.T) {
	db := setupProviderRepoTestDB(t)
	shared := seedProvider(t, db, "", "shared", "")
	require.NoError(t, db.Create(&provider.ProviderAttribute{
		ProviderID: shared.ID, AttrKey: "region", AttrType: "string", AttrValue: "cn",
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: shared.ID, SelectionID: "sel", ModelID: "m", ModelType: "llm", Status: "1",
	}).Error)

	repo := NewProviderRepository()
	copied, err := repo.CopyForTenant("tenant-a", shared.ID)
	require.NoError(t, err)
	require.NotZero(t, copied.ID)
	require.Equal(t, "tenant-a", copied.TenantID)
	require.Equal(t, shared.Key, copied.Key)
	require.Equal(t, shared.LockedAPIKey, copied.LockedAPIKey)

	attrs, err := repo.GetAttributes("tenant-a", copied.ID)
	require.NoError(t, err)
	require.Equal(t, "cn", attrs["region"].Value)

	models, err := repo.ListModels("tenant-a", copied.ID)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "m", models[0].ModelID)

	// 原共享行原封不动
	orig, err := repo.GetByID("tenant-b", shared.ID)
	require.NoError(t, err)
	require.Equal(t, "", orig.TenantID)

	// 非共享来源（他租户行）不可复制
	b := seedProvider(t, db, "tenant-b", "b", "")
	_, err = repo.CopyForTenant("tenant-a", b.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
