package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// createLegacyVendorPresetsForTest creates the vendor_presets table with raw
// DDL, mirroring the pre-split production schema (its model has since been
// removed in favor of ProviderSummary, whose unique index is also named
// uk_key). Index names are global in SQLite but table-scoped on MySQL, so we
// recreate the table here without the conflicting index.
func createLegacyVendorPresetsForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`
		CREATE TABLE vendor_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			`+"`key`"+` VARCHAR(64) NOT NULL,
			name VARCHAR(128) NOT NULL,
			description TEXT,
			description_en TEXT,
			protocol VARCHAR(16) NOT NULL,
			auth_style VARCHAR(16) NOT NULL,
			base_url VARCHAR(512),
			default_models TEXT,
			fields TEXT,
			icon_key VARCHAR(32),
			builtin BOOLEAN DEFAULT FALSE,
			locked_api_key TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
}

// TestMigrateProviderSplit_CopiesRowsFromLegacyVendorPresets verifies the
// INSERT INTO ... SELECT FROM vendor_presets SQL parses against the current
// provider_summaries schema (which no longer has default_models or type).
// Regression guard for Task 7 follow-up: the previous SQL listed
// default_models in both INSERT and SELECT, breaking stale upgrades.
func TestMigrateProviderSplit_CopiesRowsFromLegacyVendorPresets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}))
	createLegacyVendorPresetsForTest(t, db)

	// Populate vendor_presets (the real table still carries default_models).
	require.NoError(t, db.Exec(
		`INSERT INTO vendor_presets (`+"`key`"+`, name, protocol, auth_style, base_url, default_models, fields, icon_key, builtin, locked_api_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		"glm-cn", "GLM", "anthropic", "api_key", "http://x", `[{"modelId":"GLM-5"}]`, "[]", "", 0, "",
	).Error)

	require.NoError(t, migrateProviderSplit())

	var summaries []provider.ProviderSummary
	require.NoError(t, db.Find(&summaries).Error)
	require.Len(t, summaries, 1, "legacy row should be copied to provider_summaries")
	require.Equal(t, "glm-cn", summaries[0].Key)
	require.Equal(t, "GLM", summaries[0].Name)

	// Idempotent: second run must be a no-op because provider_summaries is
	// no longer empty.
	require.NoError(t, migrateProviderSplit())
	var count int64
	db.Model(&provider.ProviderSummary{}).Count(&count)
	require.Equal(t, int64(1), count)
}

// TestMigrateProviderSplit_NoopWhenNoLegacy ensures the migration does
// nothing when vendor_presets is empty.
func TestMigrateProviderSplit_NoopWhenNoLegacy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}))
	createLegacyVendorPresetsForTest(t, db)

	require.NoError(t, migrateProviderSplit())

	var count int64
	db.Model(&provider.ProviderSummary{}).Count(&count)
	require.Equal(t, int64(0), count)
}
