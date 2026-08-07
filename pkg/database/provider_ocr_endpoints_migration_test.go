package database

import (
	"testing"

	"control-panel/internal/domain/provider"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateProviderOcrEndpoints_BackfillsMinerU(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db

	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	// Seed a MinerU provider with no models (pre-fix state).
	require.NoError(t, db.Create(&provider.ProviderSummary{
		Key: "mineru", Name: "MinerU", Protocol: "mineru", AuthStyle: "api_key",
	}).Error)

	require.NoError(t, migrateProviderOcrEndpointsBackfill())

	var models []provider.ProviderModel
	require.NoError(t, db.Find(&models).Error)
	require.Len(t, models, 1)
	require.Equal(t, "mineru", models[0].ModelID)
	require.Equal(t, "MinerU", models[0].DisplayName)
	require.Equal(t, "ocr", models[0].ModelType)
	require.Equal(t, "1", models[0].Status)
}

func TestMigrateProviderOcrEndpoints_BackfillsPaddleOCR(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	require.NoError(t, db.Create(&provider.ProviderSummary{
		Key: "paddleocr", Name: "PaddleOCR", Protocol: "paddleocr", AuthStyle: "no_auth",
	}).Error)

	require.NoError(t, migrateProviderOcrEndpointsBackfill())

	var count int64
	db.Model(&provider.ProviderModel{}).Count(&count)
	require.Equal(t, int64(1), count)
}

func TestMigrateProviderOcrEndpoints_SkipsNonEndpointProviders(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	// GLM provider has no models but protocol is anthropic — should NOT be touched.
	require.NoError(t, db.Create(&provider.ProviderSummary{
		Key: "glm", Name: "GLM", Protocol: "anthropic", AuthStyle: "api_key",
	}).Error)

	require.NoError(t, migrateProviderOcrEndpointsBackfill())

	var count int64
	db.Model(&provider.ProviderModel{}).Count(&count)
	require.Equal(t, int64(0), count, "non-OCR-endpoint providers must not be touched")
}

func TestMigrateProviderOcrEndpoints_Idempotent(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	require.NoError(t, db.Create(&provider.ProviderSummary{
		Key: "mineru", Name: "MinerU", Protocol: "mineru", AuthStyle: "api_key",
	}).Error)

	require.NoError(t, migrateProviderOcrEndpointsBackfill())
	require.NoError(t, migrateProviderOcrEndpointsBackfill())

	var count int64
	db.Model(&provider.ProviderModel{}).Count(&count)
	require.Equal(t, int64(1), count, "second run must not duplicate the default model")
}

func TestMigrateProviderOcrEndpoints_RespectsExistingModels(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = db
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	// MinerU already has a user-configured model — backfill must not overwrite or duplicate.
	require.NoError(t, db.Create(&provider.ProviderSummary{
		Key: "mineru", Name: "MinerU", Protocol: "mineru", AuthStyle: "api_key",
	}).Error)
	var providerID uint64 = 1
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: providerID, SelectionID: "custom-1", ModelID: "custom-model",
		DisplayName: "Custom", ModelType: "ocr", Status: "1",
	}).Error)

	require.NoError(t, migrateProviderOcrEndpointsBackfill())

	var models []provider.ProviderModel
	require.NoError(t, db.Find(&models).Error)
	require.Len(t, models, 1, "existing model must remain; no default inserted")
	require.Equal(t, "custom-model", models[0].ModelID)
}
