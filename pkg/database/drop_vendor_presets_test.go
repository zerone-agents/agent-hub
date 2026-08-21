package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMigrateDropVendorPresets_DropsTableIdempotently verifies that the
// migration drops an existing vendor_presets table and is a no-op when the
// table is already gone.
func TestMigrateDropVendorPresets_DropsTableIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	createLegacyVendorPresetsForTest(t, db)
	require.True(t, db.Migrator().HasTable("vendor_presets"))

	require.NoError(t, migrateDropVendorPresets())
	require.False(t, db.Migrator().HasTable("vendor_presets"), "vendor_presets should be dropped")

	// Second run must be a silent no-op.
	require.NoError(t, migrateDropVendorPresets())
	require.False(t, db.Migrator().HasTable("vendor_presets"))
}

// TestMigrateDropVendorPresets_NoTableIsNoOp verifies fresh databases (where
// vendor_presets was never created) pass through unharmed.
func TestMigrateDropVendorPresets_NoTableIsNoOp(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, migrateDropVendorPresets())
	require.False(t, db.Migrator().HasTable("vendor_presets"))
}

// TestMigrateProviderSplit_NoLegacyTable ensures the split migration tolerates
// a database where vendor_presets has already been dropped (fresh installs
// after the LegacyProvider model removal).
func TestMigrateProviderSplit_NoLegacyTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}))
	require.NoError(t, migrateProviderSplit())

	var count int64
	db.Model(&provider.ProviderSummary{}).Count(&count)
	require.Equal(t, int64(0), count)
}
