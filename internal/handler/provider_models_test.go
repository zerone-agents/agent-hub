package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// providerModelsTestKey matches providerServiceTestEncryptionKey so the
// service can decrypt any locked keys we seed (none in this file).
const providerModelsTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// setupProviderModelsRouter spins up an in-memory sqlite DB with one
// provider (id=1) and one model row (selectionId="m1") and registers the
// three new per-model endpoints. Returns the router and the underlying
// service so tests can poke the repo directly when needed.
func setupProviderModelsRouter(t *testing.T) *gin.Engine {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:        1,
		Key:       "test-provider",
		Name:      "Test Provider",
		Protocol:  "openai",
		AuthStyle: "api_key",
		BaseURL:   "http://test.example.com",
	}).Error)

	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: 1, SelectionID: "m1", ModelID: "m1",
		DisplayName: "M1", ModelType: "llm",
		ContextWindow: 8192, Status: "1", SortOrder: 0,
	}).Error)

	gin.SetMode(gin.TestMode)
	h := NewProviderHandler(services.NewProviderService(providerModelsTestKey), nil)
	router := gin.New()
	router.POST("/api/v1/admin/providers/:id/models", h.AddModel)
	router.PATCH("/api/v1/admin/providers/:id/models/:selectionId", h.UpdateModel)
	router.DELETE("/api/v1/admin/providers/:id/models/:selectionId", h.DeleteModel)
	return router
}

func TestProviderHandler_AddModel_ReturnsUpdatedDTO(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"modelId":       "gpt-4o",
		"displayName":   "GPT-4o",
		"modelType":     "llm",
		"contextWindow": 128000,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	models := resp.Data["defaultModels"].([]interface{})
	require.Len(t, models, 2, "DTO should include the pre-existing model plus the new one")

	var sawNew bool
	for _, raw := range models {
		m := raw.(map[string]interface{})
		if m["modelId"] == "gpt-4o" {
			sawNew = true
			require.Equal(t, "gpt-4o", m["selectionId"])
			require.Equal(t, "llm", m["modelType"])
			require.Equal(t, float64(128000), m["contextWindow"])
		}
	}
	require.True(t, sawNew, "added model not present in DTO")
}

func TestProviderHandler_AddModel_RejectsInvalidModelType(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"modelId":   "weird",
		"modelType": "bogus",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProviderHandler_AddModel_RejectsMissingModelID(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"modelType": "llm",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProviderHandler_AddModel_UnknownProviderReturns404(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"modelId":   "x",
		"modelType": "llm",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/999/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProviderHandler_UpdateModel_PatchesDisplayName(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"displayName": "M1 Renamed",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/providers/1/models/m1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	models := resp.Data["defaultModels"].([]interface{})
	var hit map[string]interface{}
	for _, raw := range models {
		m := raw.(map[string]interface{})
		if m["selectionId"] == "m1" {
			hit = m
		}
	}
	require.NotNil(t, hit, "updated model missing from DTO")
	require.Equal(t, "M1 Renamed", hit["displayName"])
}

func TestProviderHandler_UpdateModel_UnknownSelectionIDReturns404(t *testing.T) {
	router := setupProviderModelsRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"displayName": "ghost",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/providers/1/models/ghost", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestProviderHandler_DeleteModel_RemovesRow(t *testing.T) {
	router := setupProviderModelsRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/providers/1/models/m1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	// Verify the model is actually gone via a fresh service lookup.
	svc := services.NewProviderService(providerModelsTestKey)
	p, err := svc.GetByID("", 1)
	require.NoError(t, err)
	require.Empty(t, p.DefaultModels(), "deleted model should not be returned by GetByID")
}

func TestProviderHandler_DeleteModel_UnknownSelectionIDReturns404(t *testing.T) {
	router := setupProviderModelsRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/providers/1/models/ghost", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
