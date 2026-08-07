package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateDropLegacyColumns_RemovesDefaultModelsAndType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	// Manually re-add legacy columns to simulate pre-migration state.
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN default_models TEXT").Error)
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN type VARCHAR(16)").Error)

	// Sanity-check: columns are present before migration.
	migrator := db.Migrator()
	require.True(t, migrator.HasColumn(&provider.ProviderSummary{}, "default_models"))
	require.True(t, migrator.HasColumn(&provider.ProviderSummary{}, "type"))

	require.NoError(t, migrateDropLegacyProviderColumns())

	// Verify columns are gone.
	cols, err := db.Migrator().ColumnTypes("provider_summaries")
	require.NoError(t, err)
	for _, c := range cols {
		name := c.Name()
		require.NotEqual(t, "default_models", name, "default_models column should be dropped")
		require.NotEqual(t, "type", name, "type column should be dropped")
	}
}

func TestMigrateDropLegacyColumns_Idempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	// No legacy columns to start with — AutoMigrate followed the new schema.
	// Running the drop migration twice must be a no-op (no error).
	require.NoError(t, migrateDropLegacyProviderColumns())
	require.NoError(t, migrateDropLegacyProviderColumns())
}
