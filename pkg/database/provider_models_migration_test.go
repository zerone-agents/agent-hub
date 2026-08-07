package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// addLegacyColumnsForTest re-adds the columns that Task 7 dropped, so that
// migrateProviderModels() can be exercised against a pre-drop schema.
func addLegacyColumnsForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN type VARCHAR(16)").Error)
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN default_models TEXT").Error)
}

// insertLegacySummary inserts a provider_summaries row with the legacy
// columns populated. Used by tests that exercise migrateProviderModels()
// (which reads those columns via the internal legacyProviderSummary).
func insertLegacySummary(t *testing.T, db *gorm.DB, key, protocol, ptype, defaultModels string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO provider_summaries (`+"`key`"+`, name, protocol, type, auth_style, base_url, default_models, fields, icon_key, builtin, locked_api_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, '[]', '', 0, '', datetime('now'), datetime('now'))`,
		key, key, protocol, ptype, "api_key", "http://x", defaultModels,
	).Error)
}

func TestMigrateProviderModels_BackfillsFromJSON(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))
	addLegacyColumnsForTest(t, db)

	insertLegacySummary(t, db, "glm-cn", "anthropic", "llm",
		`[{"selectionId":"glm-5","modelId":"GLM-5","displayName":"GLM-5","contextWindow":200000},{"selectionId":"glm-4","modelId":"GLM-4","displayName":"GLM-4","contextWindow":128000}]`)

	require.NoError(t, migrateProviderModels())

	var rows []provider.ProviderModel
	require.NoError(t, db.Order("sort_order").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, "GLM-5", rows[0].ModelID)
	require.Equal(t, "llm", rows[0].ModelType, "model_type should fall back to provider.type when JSON omits it")
	require.Equal(t, "1", rows[0].Status)
}

func TestMigrateProviderModels_Idempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))
	addLegacyColumnsForTest(t, db)

	insertLegacySummary(t, db, "glm-cn", "anthropic", "llm",
		`[{"selectionId":"a","modelId":"A","displayName":"A","contextWindow":1000}]`)

	require.NoError(t, migrateProviderModels())
	require.NoError(t, migrateProviderModels())

	var count int64
	db.Model(&provider.ProviderModel{}).Count(&count)
	require.Equal(t, int64(1), count, "second migration run must be a no-op")
}

func TestMigrateProviderModels_PrefersModelTypeFromJSON(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))
	addLegacyColumnsForTest(t, db)

	insertLegacySummary(t, db, "openai", "openai", "llm",
		`[{"selectionId":"emb","modelId":"text-embedding-3","displayName":"Emb","contextWindow":0,"modelType":"embedding"}]`)

	require.NoError(t, migrateProviderModels())

	var m provider.ProviderModel
	require.NoError(t, db.First(&m).Error)
	require.Equal(t, "embedding", m.ModelType, "model_type from JSON wins over provider.type fallback")
}

// TestMigrateProviderModels_SkipsWhenLegacyColumnsGone verifies the
// migration is a no-op on fresh post-Task-7 schemas where the legacy
// columns have already been dropped (so the backfill has nothing to read).
func TestMigrateProviderModels_SkipsWhenLegacyColumnsGone(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	// No addLegacyColumnsForTest: fresh schema has no legacy columns.
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	require.NoError(t, migrateProviderModels())

	var count int64
	db.Model(&provider.ProviderModel{}).Count(&count)
	require.Equal(t, int64(0), count, "no backfill when default_models column is gone")
}

func TestMigrateProviderModels_EffortsColumnExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, migrateProviderModels())
	require.True(t, db.Migrator().HasColumn(&provider.ProviderModel{}, "efforts"))
}
