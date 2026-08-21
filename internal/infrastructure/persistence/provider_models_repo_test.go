package repository

import (
	"testing"

	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

func TestProviderRepository_ModelsCRUD(t *testing.T) {
	setupModelsTestDB(t)
	repo := NewProviderRepository()
	const tenant = "t1"

	// Setup: create a provider row
	summary := &provider.ProviderSummary{
		Key: "test", Name: "Test", Protocol: "openai", AuthStyle: "api_key", BaseURL: "http://x",
	}
	require.NoError(t, repo.Create(tenant, summary))
	require.NotZero(t, summary.ID)

	// Create model
	pm := &provider.ProviderModel{
		ProviderID: summary.ID, SelectionID: "m1", ModelID: "m1",
		DisplayName: "M1", ModelType: "llm", ContextWindow: 8192, Status: "1", SortOrder: 0,
	}
	require.NoError(t, repo.CreateModel(tenant, pm))
	require.NotZero(t, pm.ID)

	// ListModels
	models, err := repo.ListModels(tenant, summary.ID)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "m1", models[0].ModelID)

	// GetModelBySelectionID
	got, err := repo.GetModelBySelectionID(tenant, summary.ID, "m1")
	require.NoError(t, err)
	require.Equal(t, "M1", got.DisplayName)

	// UpdateModel
	got.DisplayName = "M1 Updated"
	require.NoError(t, repo.UpdateModel(tenant, got))
	updated, _ := repo.GetModelBySelectionID(tenant, summary.ID, "m1")
	require.Equal(t, "M1 Updated", updated.DisplayName)

	// ReplaceModels (full swap)
	replacement := []provider.ProviderModel{
		{ProviderID: summary.ID, SelectionID: "m1", ModelID: "m1", DisplayName: "M1 v2", ModelType: "llm", Status: "1"},
		{ProviderID: summary.ID, SelectionID: "m2", ModelID: "m2", DisplayName: "M2", ModelType: "embedding", Status: "1"},
	}
	require.NoError(t, repo.ReplaceModels(tenant, summary.ID, replacement))
	all, err := repo.ListModels(tenant, summary.ID)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// ListAllModels (cross-provider, tenant-scoped)
	allModels, err := repo.ListAllModels(tenant)
	require.NoError(t, err)
	require.Len(t, allModels, 2)

	// DeleteModel
	require.NoError(t, repo.DeleteModel(tenant, summary.ID, "m2"))
	after, _ := repo.ListModels(tenant, summary.ID)
	require.Len(t, after, 1)

	// CASCADE: delete provider should delete its models
	require.NoError(t, repo.Delete(tenant, summary.ID))
	cascade, _ := repo.ListModels(tenant, summary.ID)
	require.Empty(t, cascade)
}
