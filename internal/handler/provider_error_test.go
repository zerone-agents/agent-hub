package handler

import (
	"bytes"
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

	t.Run("ValidationError → 400 原文用户面文案", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondProviderError(c, provider.NewValidationErrorf("name 不能为空"))
		require.Equal(t, http.StatusBadRequest, w.Code)
		body := w.Body.String()
		require.Contains(t, body, "name 不能为空")
		require.NotContains(t, body, "服务器内部错误")
	})
}

// ------ issue #95 P2 复申（review #5598101374）：Create/Update 400 分支分流 ------

// TestProviderHandler_Create_DBFailure500Neutral 锁定 Create 遇到基础设施故障
// （DB 关闭）→ 500 中性中文文案，英文化内部诊断（"check provider existence
// failed" 等）只进服务端日志，不泄漏到响应体。
func TestProviderHandler_Create_DBFailure500Neutral(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	svc := services.NewProviderService(providerSyncTestKey)
	h := NewProviderHandler(svc, &stubMultiRAGClient{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/providers", h.Create)

	body := `{"key": "glm-cn", "name": "GLM", "protocol": "anthropic", "authStyle": "api_key", "baseUrl": "https://open.bigmodel.cn/api/anthropic"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "服务器内部错误，请稍后重试")
	require.NotContains(t, respBody, "check provider existence failed")
	require.NotContains(t, respBody, "database is closed")
}

// TestProviderHandler_Create_ValidationKey400 锁定用户面校验错误仍走 400
// 原文中文（不被 500 中性化吞掉）。注意 name 为空会被 binding:"required"
// 在 ShouldBindJSON 层拦截（gin 错误），故用非法 key（含空格）命中
// service 层 validateIdentifier 校验路径。
func TestProviderHandler_Create_ValidationKey400(t *testing.T) {
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
	r.POST("/api/v1/admin/providers", h.Create)

	body := `{"key": "glm cn", "name": "GLM", "protocol": "anthropic", "authStyle": "api_key", "baseUrl": "https://open.bigmodel.cn/api/anthropic"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	require.Contains(t, w.Body.String(), "Provider key 标识只能包含字母、数字、点、下划线和横线")
	require.NotContains(t, w.Body.String(), "服务器内部错误")
}
