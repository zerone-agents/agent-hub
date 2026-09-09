package handler

import (
	"errors"
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

// ---------- issue #95 P2：handler 边界 500 中性化回归 ----------

// TestProviderHandler_ListInternalError500Neutral 锁定 List 端点遇到
// 基础设施故障（DB 故障）→ 500 中性中文文案，英文内部诊断
// （service 层 "list providers failed" 包装）只进服务端日志，
// 不得外泄到响应体（issue #95 P2 外审发现）。
func TestProviderHandler_ListInternalError500Neutral(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	// 关闭底层连接：repo.ListAll 随即返回错误，触发 handler 500 分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	svc := services.NewProviderService(providerSyncTestKey)
	h := NewProviderHandler(svc, &stubMultiRAGClient{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/providers", h.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "list providers failed")
}

// TestProviderHandler_GetNotFound404 锁定领域 sentinel 仍走用户面 404
// 文案（不被 500 中性化吞掉）。
func TestProviderHandler_GetNotFound404(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	svc := services.NewProviderService(providerSyncTestKey)
	h := NewProviderHandler(svc, &stubMultiRAGClient{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/providers/:id", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/providers/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
}

// TestRespondProviderError_SentinelRouting 锁定 respondProviderError 的
// sentinel 路由与默认 500 中性行为（对齐 respondToolError 契约）。
func TestRespondProviderError_SentinelRouting(t *testing.T) {
	t.Run("ErrProviderNotFound → 404 用户面文案", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondProviderError(c, provider.ErrProviderNotFound)
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Contains(t, w.Body.String(), "provider not found")
	})

	t.Run("内部诊断 → 500 中性文案，英文不外泄", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondProviderError(c, errors.New("list providers failed: db connection refused"))
		require.Equal(t, http.StatusInternalServerError, w.Code)
		body := w.Body.String()
		require.Contains(t, body, "服务器内部错误，请稍后重试")
		require.NotContains(t, body, "db connection refused")
	})
}
