package database

import (
	"fmt"
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupProviderTenantMigrationTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}))
	oldDB, oldBackfill := DB, backfillTenantID
	DB, backfillTenantID = db, "default"
	t.Cleanup(func() {
		DB, backfillTenantID = oldDB, oldBackfill
	})
	return db
}

func TestMigrateProvidersTenantIDSkipsIntentionalSharedRowsAfterLegacyIndexIsGone(t *testing.T) {
	db := setupProviderTenantMigrationTest(t)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		TenantID: "", Key: "bailian", Name: "Shared Bailian", Protocol: "openai", AuthStyle: "api_key",
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		TenantID: "default", Key: "bailian", Name: "Customized Bailian", Protocol: "openai", AuthStyle: "api_key",
	}).Error)

	require.NoError(t, migrateProvidersTenantID())

	var sharedCount int64
	require.NoError(t, db.Model(&provider.ProviderSummary{}).
		Where("tenant_id = '' AND `key` = ?", "bailian").Count(&sharedCount).Error)
	require.Equal(t, int64(1), sharedCount)
}

func TestMigrateProvidersTenantIDRunsOnceWhileLegacyIndexExists(t *testing.T) {
	db := setupProviderTenantMigrationTest(t)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX uk_key ON provider_summaries(`key`)").Error)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		TenantID: "", Key: "legacy", Name: "Legacy", Protocol: "openai", AuthStyle: "api_key",
	}).Error)

	require.NoError(t, migrateProvidersTenantID())
	require.False(t, db.Migrator().HasIndex(&provider.ProviderSummary{}, "uk_key"))

	var migrated provider.ProviderSummary
	require.NoError(t, db.Where("`key` = ?", "legacy").First(&migrated).Error)
	require.Equal(t, "default", migrated.TenantID)
}
