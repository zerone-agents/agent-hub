package database

import (
	"control-panel/internal/domain/provider"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestMultiRAGBindingMigrationDoesNotAdoptOrReplayLegacyModels(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))
	require.NoError(t, db.Create(&provider.ProviderSummary{ID: 1, Key: "legacy", Name: "Legacy", Protocol: "openai", AuthStyle: "api_key"}).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{ProviderID: 1, SelectionID: "same", ModelID: "same", ModelType: "embedding"}).Error)
	require.NoError(t, db.AutoMigrate(&provider.MultiRAGSyncBinding{}))
	var count int64
	require.NoError(t, db.Model(&provider.MultiRAGSyncBinding{}).Count(&count).Error)
	require.Zero(t, count)
	pending := provider.MultiRAGSyncBinding{ProviderID: 1, Target: "scope", InstanceName: "reserved-before-dispatch", State: "creating", ConfigHash: "hash", Lease: "owned"}
	require.NoError(t, db.Create(&pending).Error)
	require.NoError(t, db.AutoMigrate(&provider.MultiRAGSyncBinding{}))
	var stored provider.MultiRAGSyncBinding
	require.NoError(t, db.First(&stored).Error)
	require.Equal(t, pending, stored)
	for _, column := range []string{"version", "lease", "state", "config_hash", "target", "instance_id"} {
		require.True(t, db.Migrator().HasColumn(&provider.MultiRAGSyncBinding{}, column))
	}
}
