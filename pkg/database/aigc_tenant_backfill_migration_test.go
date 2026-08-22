package database

import (
	"testing"

	"control-panel/internal/domain/aigc"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupAigcBackfillDB 建 aigc_configs 表（含 tenant_id 列与 uk_tenant_id 唯一
// 索引），并守护包级 DB/backfillTenantID 变量。
func setupAigcBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() {
		DB, backfillTenantID = oldDB, oldBackfill
	})
	DB = db
	return db
}

func seedAigcRow(t *testing.T, db *gorm.DB, id int, tenantID string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO aigc_configs (id, tenant_id, uscc, company_name, content_producer, signing_key_encrypted, created_at, updated_at) VALUES (?, ?, 'TEST', 'T', 'X', '', '2026-01-01', '2026-01-01')",
		id, tenantID,
	).Error)
}

func aigcTenantIDs(t *testing.T, db *gorm.DB) map[int]string {
	t.Helper()
	rows := []struct {
		ID       int
		TenantID string
	}{}
	require.NoError(t, db.Raw("SELECT id, tenant_id FROM aigc_configs ORDER BY id").Scan(&rows).Error)
	out := map[int]string{}
	for _, r := range rows {
		out[r.ID] = r.TenantID
	}
	return out
}

// TestMigrateAigcConfigs_BackfillLegacyRowToTenant ” 遗留行 + 回填租户无自有行
// → 归属回填租户（方案 A：遗留全局配置归属唯一/指定的租户）。
func TestMigrateAigcConfigs_BackfillLegacyRowToTenant(t *testing.T) {
	db := setupAigcBackfillDB(t)
	seedAigcRow(t, db, 1, "")
	backfillTenantID = "org-x"

	require.NoError(t, migrateAigcConfigsTenantID())
	require.Equal(t, map[int]string{1: "org-x"}, aigcTenantIDs(t, db))
}

// TestMigrateAigcConfigs_CollisionKeepsTenantRow ” 遗留行 + 回填租户已有自有行
// → 撞 uk_tenant_id 唯一索引：删除 ” 遆留行，保留租户自有（较新）行。
func TestMigrateAigcConfigs_CollisionKeepsTenantRow(t *testing.T) {
	db := setupAigcBackfillDB(t)
	seedAigcRow(t, db, 1, "")      // 升级遗留共享行
	seedAigcRow(t, db, 2, "org-x") // 租户已保存的自有行
	backfillTenantID = "org-x"

	require.NoError(t, migrateAigcConfigsTenantID())
	require.Equal(t, map[int]string{2: "org-x"}, aigcTenantIDs(t, db), "'' 遗留行应被删除，租户自有行保留")
}

// TestMigrateAigcConfigs_NoLegacyRowIsNoOp 无 ” 行 → no-op（幂等重启）。
func TestMigrateAigcConfigs_NoLegacyRowIsNoOp(t *testing.T) {
	db := setupAigcBackfillDB(t)
	seedAigcRow(t, db, 1, "org-x")
	seedAigcRow(t, db, 2, "org-y")
	backfillTenantID = "org-x"

	require.NoError(t, migrateAigcConfigsTenantID())
	require.NoError(t, migrateAigcConfigsTenantID()) // 幂等：重跑 no-op
	require.Equal(t, map[int]string{1: "org-x", 2: "org-y"}, aigcTenantIDs(t, db))
}

// TestMigrateAigcConfigs_EmptyBackfillWithLegacyRowErrors backfill 为空 + 有 ”
// 遗留行 → 指引性错误（与其他表一致的 CASDOOR_ORGANIZATION 逃生舱语义）。
func TestMigrateAigcConfigs_EmptyBackfillWithLegacyRowErrors(t *testing.T) {
	db := setupAigcBackfillDB(t)
	seedAigcRow(t, db, 1, "")
	backfillTenantID = ""

	err := migrateAigcConfigsTenantID()
	require.Error(t, err)
	require.Contains(t, err.Error(), "CASDOOR_ORGANIZATION", "报错必须指引配置 CASDOOR_ORGANIZATION")
}

// TestMigrateAigcConfigs_NullNormalizedToBackfill NULL 中间态先归一为 ” 再
// 走回填（异常中间态兜底，如手工改库/中断的半迁移）。正常 schema 列为
// NOT NULL，这里手工建可空列模拟异常库。
func TestMigrateAigcConfigs_NullNormalizedToBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE aigc_configs (id INTEGER PRIMARY KEY, tenant_id VARCHAR(64), uscc VARCHAR(32), company_name VARCHAR(128), content_producer VARCHAR(64), signing_key_encrypted TEXT, created_at DATETIME, updated_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX uk_tenant_id ON aigc_configs(tenant_id)`).Error)
	require.NoError(t, db.Exec("INSERT INTO aigc_configs (id, tenant_id, uscc, company_name, content_producer, signing_key_encrypted) VALUES (1, NULL, 'TEST', 'T', 'X', '')").Error)
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() { DB, backfillTenantID = oldDB, oldBackfill })
	DB = db
	backfillTenantID = "org-x"

	require.NoError(t, migrateAigcConfigsTenantID())
	require.Equal(t, map[int]string{1: "org-x"}, aigcTenantIDs(t, db))
}
