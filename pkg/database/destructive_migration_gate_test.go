package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// allowDestructiveMigrationsForTest 打开破坏性迁移开关并注册恢复。
// 需要真正验证删列/删表效果的用例必须先调它。
func allowDestructiveMigrationsForTest(t *testing.T) {
	t.Helper()
	old := destructiveMigrationsAllowed
	destructiveMigrationsAllowed = true
	t.Cleanup(func() { destructiveMigrationsAllowed = old })
}

func TestParseBoolEnv(t *testing.T) {
	for _, key := range []string{"1", "true", "TRUE", "yes", "Yes", "on", "ON", " true "} {
		t.Setenv(destructiveMigrationsEnv, key)
		require.True(t, parseBoolEnv(destructiveMigrationsEnv), "%q 应解析为 true", key)
	}
	for _, key := range []string{"", "0", "false", "no", "off", "enable", "2"} {
		t.Setenv(destructiveMigrationsEnv, key)
		require.False(t, parseBoolEnv(destructiveMigrationsEnv), "%q 应解析为 false", key)
	}
}

// 默认（未设 ALLOW_DESTRUCTIVE_MIGRATIONS）必须跳过删列，把不可逆操作
// 留给运维显式开启；跳过不是错误，启动照常继续。
func TestMigrateDropLegacyColumns_SkippedByDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	old := destructiveMigrationsAllowed
	destructiveMigrationsAllowed = false
	t.Cleanup(func() { destructiveMigrationsAllowed = old })

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN default_models TEXT").Error)
	require.NoError(t, db.Exec("ALTER TABLE provider_summaries ADD COLUMN type VARCHAR(16)").Error)

	require.NoError(t, migrateDropLegacyProviderColumns(), "默认跳过不是错误")

	migrator := db.Migrator()
	require.True(t, migrator.HasColumn(&provider.ProviderSummary{}, "default_models"), "未 opt-in 时不得删列")
	require.True(t, migrator.HasColumn(&provider.ProviderSummary{}, "type"), "未 opt-in 时不得删列")
}

// 未 opt-in 时同样不得删掉遗留表（DROP TABLE 比删列更不可逆）。
func TestMigrateDropVendorPresets_SkippedByDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	old := destructiveMigrationsAllowed
	destructiveMigrationsAllowed = false
	t.Cleanup(func() { destructiveMigrationsAllowed = old })

	createLegacyVendorPresetsForTest(t, db)
	require.True(t, db.Migrator().HasTable("vendor_presets"))

	require.NoError(t, migrateDropVendorPresets())
	require.True(t, db.Migrator().HasTable("vendor_presets"), "未 opt-in 时不得删表")
}

// 遗留租户域表/列同样受开关保护。
func TestMigrateDropLegacyTenantDomain_SkippedByDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	old := destructiveMigrationsAllowed
	destructiveMigrationsAllowed = false
	t.Cleanup(func() { destructiveMigrationsAllowed = old })

	require.NoError(t, db.Exec("CREATE TABLE tenants (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.True(t, db.Migrator().HasTable("tenants"))

	require.NoError(t, migrateDropLegacyTenantDomain())
	require.True(t, db.Migrator().HasTable("tenants"), "未 opt-in 时不得删表")
}
