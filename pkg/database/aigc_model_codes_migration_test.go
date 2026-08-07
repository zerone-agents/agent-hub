package database

import (
	"encoding/json"
	"testing"

	"control-panel/internal/domain/aigc"
	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateAigcModelCodes_BackfillsFromBlobThenDropsColumn(t *testing.T) {
	db := openMigrationDbEmpty(t)
	seedAigcConfig(t, db, map[string]string{"glm-4.5": "0001", "gpt-4o": "0005"})
	seedProviderModel(t, db, 1, "glm-4.5", "")
	seedProviderModel(t, db, 1, "gpt-4o", "")
	seedProviderModel(t, db, 1, "claude-sonnet-4", "") // blob 未覆盖 → 递增 0006

	require.NoError(t, migrateAigcModelCodes(db))

	require.Equal(t, "0001", modelCode(t, db, "glm-4.5"))
	require.Equal(t, "0005", modelCode(t, db, "gpt-4o"))
	require.Equal(t, "0006", modelCode(t, db, "claude-sonnet-4"))
	require.False(t, db.Migrator().HasColumn("aigc_configs", "model_codes"))
}

func TestMigrateAigcModelCodes_CrossProviderReusesSameCode(t *testing.T) {
	db := openMigrationDbEmpty(t) // 无 aigc 配置 blob
	seedProviderModel(t, db, 1, "glm-4.5", "")
	seedProviderModel(t, db, 2, "glm-4.5", "")
	seedProviderModel(t, db, 1, "gpt-4o", "")

	require.NoError(t, migrateAigcModelCodes(db))

	require.Equal(t, "0001", modelCode(t, db, "glm-4.5"))
	require.Equal(t, "0001", modelCodeByProvider(t, db, 2, "glm-4.5"))
	require.Equal(t, "0002", modelCode(t, db, "gpt-4o"))
}

func TestMigrateAigcModelCodes_Idempotent(t *testing.T) {
	db := openMigrationDbEmpty(t)
	seedProviderModel(t, db, 1, "glm-4.5", "")
	require.NoError(t, migrateAigcModelCodes(db))
	before := modelCode(t, db, "glm-4.5")
	require.NoError(t, migrateAigcModelCodes(db)) // 二次运行不报错、不变更
	require.Equal(t, before, modelCode(t, db, "glm-4.5"))
}

func TestMigrateAigcModelCodes_NoOpWhenNoModels(t *testing.T) {
	db := openMigrationDbEmpty(t)
	seedAigcConfig(t, db, map[string]string{"glm-4.5": "0001"})
	require.NoError(t, migrateAigcModelCodes(db))
	require.False(t, db.Migrator().HasColumn("aigc_configs", "model_codes"))
}

// ── helpers ──

func openMigrationDbEmpty(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderModel{}))
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))
	// 旧 schema 中 aigc.Config 还有 model_codes 列；struct 已不含该字段，
	// 用原始 SQL 模拟旧 schema 以便迁移逻辑能 DROP 它。
	require.NoError(t, db.Exec("ALTER TABLE aigc_configs ADD COLUMN model_codes TEXT").Error)
	return db
}

func seedAigcConfig(t *testing.T, db *gorm.DB, codes map[string]string) {
	t.Helper()
	blob, _ := json.Marshal(codes)
	// aigc.Config struct 不含 ModelCodes 字段，用原始 SQL 写入该列
	require.NoError(t, db.Exec("INSERT INTO aigc_configs (id, uscc, company_name, content_producer, signing_key_encrypted, model_codes, created_at, updated_at) VALUES (1, 'TEST', 'T', 'X', '', ?, '2026-01-01', '2026-01-01')", string(blob)).Error)
}

func seedProviderModel(t *testing.T, db *gorm.DB, providerID uint64, modelID, code string) {
	t.Helper()
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID:  providerID,
		SelectionID: modelID, // SelectionID defaulting to modelID here is fine for these tests since each row is unique within a provider
		ModelID:     modelID,
		ModelType:   "llm",
		AigcCode:    code,
	}).Error)
}

func modelCode(t *testing.T, db *gorm.DB, modelID string) string {
	t.Helper()
	var row provider.ProviderModel
	require.NoError(t, db.Where("model_id = ?", modelID).First(&row).Error)
	return row.AigcCode
}

func modelCodeByProvider(t *testing.T, db *gorm.DB, providerID uint64, modelID string) string {
	t.Helper()
	var row provider.ProviderModel
	require.NoError(t, db.Where("provider_id = ? AND model_id = ?", providerID, modelID).First(&row).Error)
	return row.AigcCode
}
