package handler

import (
	"bytes"
	"encoding/json"
	"log"
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

const runtimeConfigTestEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// setupRuntimeConfigRouter seeds three providers (valid key / no key /
// corrupted key) and wires the runtime-config endpoint behind a stub auth
// middleware: requests with X-Test-User pass as an authenticated non-admin
// user, others are rejected with 401.
func setupRuntimeConfigRouter(t *testing.T) (*gin.Engine, *bytes.Buffer, string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	plainKey := "sk-runtime-secret-0001"
	encryptedKey, err := provider.Encrypt(plainKey, runtimeConfigTestEncryptionKey)
	require.NoError(t, err)

	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:           1,
		Key:          "with-key",
		Name:         "With Key",
		Protocol:     "openai",
		AuthStyle:    "api_key",
		BaseURL:      "https://api.example.com",
		LockedAPIKey: encryptedKey,
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:        2,
		Key:       "no-key",
		Name:      "No Key",
		Protocol:  "openai",
		AuthStyle: "api_key",
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:           3,
		Key:          "broken-key",
		Name:         "Broken Key",
		Protocol:     "openai",
		AuthStyle:    "api_key",
		LockedAPIKey: "enc:zz-not-valid-hex",
	}).Error)

	auditLog := &bytes.Buffer{}
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(auditLog)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	gin.SetMode(gin.TestMode)
	h := NewProviderHandler(services.NewProviderService(runtimeConfigTestEncryptionKey), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "unauthorized"})
			return
		}
		c.Set("user_id", "user-7")
		c.Set("user_name", "DesktopUser")
	})
	router.GET("/api/v1/providers/runtime-config", h.ListRuntimeConfig)
	return router, auditLog, plainKey
}

type runtimeConfigItem struct {
	ID           uint64 `json:"id"`
	Key          string `json:"key"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	APIKeyStatus string `json:"apiKeyStatus"`
}

func fetchRuntimeConfig(t *testing.T, router *gin.Engine, authenticated bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/runtime-config", nil)
	if authenticated {
		req.Header.Set("X-Test-User", "yes")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestListRuntimeConfig_ReturnsKeysAndDistinguishesStatuses(t *testing.T) {
	router, auditLog, plainKey := setupRuntimeConfigRouter(t)

	rec := fetchRuntimeConfig(t, router, true)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp struct {
		Success bool                `json:"success"`
		Data    []runtimeConfigItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 3)

	byKey := map[string]runtimeConfigItem{}
	for _, item := range resp.Data {
		byKey[item.Key] = item
	}

	require.Equal(t, plainKey, byKey["with-key"].APIKey)
	require.Equal(t, "ok", byKey["with-key"].APIKeyStatus)
	require.Equal(t, "https://api.example.com", byKey["with-key"].BaseURL)

	require.Empty(t, byKey["no-key"].APIKey)
	require.Equal(t, "none", byKey["no-key"].APIKeyStatus)

	require.Empty(t, byKey["broken-key"].APIKey)
	require.Equal(t, "unavailable", byKey["broken-key"].APIKeyStatus)

	require.Contains(t, auditLog.String(), "[AUDIT] provider runtime-config served")
	require.Contains(t, auditLog.String(), "user_id=user-7")
	require.Contains(t, auditLog.String(), "user_name=DesktopUser")
	require.NotContains(t, auditLog.String(), plainKey)
}

func TestListRuntimeConfig_RejectsUnauthenticated(t *testing.T) {
	router, _, _ := setupRuntimeConfigRouter(t)

	rec := fetchRuntimeConfig(t, router, false)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
