package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/provider"
	"control-panel/internal/middleware"
	"control-panel/pkg/database"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const providerRevealTestEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setupProviderRevealRouter(t *testing.T, apiKey string) (*gin.Engine, *bytes.Buffer) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	encryptedKey, err := provider.Encrypt(apiKey, providerRevealTestEncryptionKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:           1,
		Key:          "test-provider",
		Name:         "Test Provider",
		Protocol:     "openai",
		AuthStyle:    "api_key",
		LockedAPIKey: encryptedKey,
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
	h := NewProviderHandler(services.NewProviderService(providerRevealTestEncryptionKey), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") == "true" {
			c.Set("roles", []*casdoorsdk.Role{{Name: "agents-admin"}})
			c.Set("user_id", "user-1")
			c.Set("user_name", "Ada")
		}
	})
	router.POST("/api/v1/admin/providers/:id/reveal-key", middleware.RequireAdmin(), h.RevealAPIKey)
	return router, auditLog
}

func TestRevealAPIKey_ReturnsPlaintextAndAuditsWithoutSecret(t *testing.T) {
	apiKey := "sk-test-super-secret-1234"
	router, auditLog := setupProviderRevealRouter(t, apiKey)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/reveal-key", nil)
	req.Header.Set("X-Test-Admin", "true")
	req.RemoteAddr = "203.0.113.7:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"success":true,"data":{"apiKey":"sk-test-super-secret-1234"}}`, rec.Body.String())
	require.Contains(t, auditLog.String(), "[AUDIT] provider API key revealed")
	require.Contains(t, auditLog.String(), "user_id=user-1")
	require.Contains(t, auditLog.String(), "user_name=Ada")
	require.Contains(t, auditLog.String(), "provider_id=1")
	require.Contains(t, auditLog.String(), "remote_ip=203.0.113.7")
	require.NotContains(t, auditLog.String(), apiKey)
}

func TestRevealAPIKey_RejectsNonAdmin(t *testing.T) {
	router, _ := setupProviderRevealRouter(t, "sk-test-super-secret-1234")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/reveal-key", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRevealAPIKey_ReturnsNotFoundForUnknownProvider(t *testing.T) {
	router, _ := setupProviderRevealRouter(t, "sk-test-super-secret-1234")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/99/reveal-key", nil)
	req.Header.Set("X-Test-Admin", "true")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
