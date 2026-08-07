package handler

import (
	"bytes"
	"context"
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

const providerSyncTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// stubMultiRAGClient is a no-op MultiRAGClient used by handler tests to
// exercise dispatch logic without spinning up a real MultiRAG server.
// Infrastructure-layer tests (internal/infrastructure/multirag) cover the
// real HTTP behaviour; here we only need a non-nil client to satisfy the
// service's config check.
type stubMultiRAGClient struct {
	lastAddLLM *provider.AddLLMRequest
}

func (s *stubMultiRAGClient) AddLLM(ctx context.Context, payload provider.AddLLMRequest) (*provider.MultiRAGResponse, error) {
	s.lastAddLLM = &payload
	return &provider.MultiRAGResponse{HTTPStatusCode: 200, Success: true}, nil
}

// setupProviderSyncRouter spins up an in-memory sqlite DB and wires the
// sync-multirag endpoint. When seedProvider is true, a single GLM-shaped
// provider row (id=1) with an encrypted LockedAPIKey and one model is
// inserted so the handler can reach the MultiRAG dispatch stage.
func setupProviderSyncRouter(t *testing.T, seedProvider bool, client provider.MultiRAGClient) (*gin.Engine, *stubMultiRAGClient) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	if seedProvider {
		encrypted, err := provider.Encrypt("sk-test-glm-key", providerSyncTestKey)
		require.NoError(t, err)
		require.NoError(t, db.Create(&provider.ProviderSummary{
			ID:           1,
			Key:          "glm-cn",
			Name:         "GLM",
			Protocol:     "anthropic",
			AuthStyle:    "api_key",
			BaseURL:      "https://open.bigmodel.cn/api/anthropic",
			LockedAPIKey: encrypted,
		}).Error)
		require.NoError(t, db.Create(&provider.ProviderModel{
			ProviderID:    1,
			SelectionID:   "GLM-5-Turbo",
			ModelID:       "GLM-5-Turbo",
			DisplayName:   "GLM-5-Turbo",
			ModelType:     "llm",
			ContextWindow: 200000,
		}).Error)
	}

	svc := services.NewProviderService(providerSyncTestKey)
	stub, ok := client.(*stubMultiRAGClient)
	if !ok {
		stub = &stubMultiRAGClient{}
	}
	h := NewProviderHandler(svc, stub)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/providers/:id/sync-multirag", h.SyncToMultiRAG)
	return r, stub
}

func TestProviderHandler_SyncToMultiRAG_UnknownProviderReturns404(t *testing.T) {
	router, _ := setupProviderSyncRouter(t, false, &stubMultiRAGClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/999/sync-multirag", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

func TestProviderHandler_SyncToMultiRAG_NilClientReturns503(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:       1,
		Key:      "glm-cn",
		Name:     "GLM",
		Protocol: "anthropic",
		BaseURL:  "https://open.bigmodel.cn/api/anthropic",
	}).Error)

	svc := services.NewProviderService(providerSyncTestKey)
	h := NewProviderHandler(svc, nil) // nil client → server admin didn't configure MultiRAG.

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/providers/:id/sync-multirag", h.SyncToMultiRAG)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/sync-multirag", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code, "body=%s", w.Body.String())
}

func TestProviderHandler_SyncToMultiRAG_RejectsInvalidID(t *testing.T) {
	router, _ := setupProviderSyncRouter(t, false, &stubMultiRAGClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/abc/sync-multirag", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProviderHandler_SyncToMultiRAG_SuccessReturns200(t *testing.T) {
	router, stub := setupProviderSyncRouter(t, true, &stubMultiRAGClient{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/sync-multirag", bytes.NewBufferString(`{"verifyOnly":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	// SyncResult fields serialize with camelCase JSON tags. After LLM
	// consolidation, branded providers (glm-cn etc.) sync under the
	// generic "Anthropic" factory name.
	factory, _ := resp.Data["factoryName"].(string)
	require.Equal(t, "Anthropic", factory)

	// Service must decrypt the stored key and pass plaintext to MultiRAG.
	require.NotNil(t, stub.lastAddLLM)
	require.Equal(t, json.RawMessage(`"sk-test-glm-key"`), stub.lastAddLLM.APIKey, "service must decrypt key before dispatch")
	require.True(t, stub.lastAddLLM.Verify, "verifyOnly must propagate")
}
