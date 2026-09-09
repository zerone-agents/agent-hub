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

// ---------- issue #95 P2/P3 复申回归（review #5599426234）----------

// TestProviderHandler_SyncToMultiRAG_CClassNoModels400 锁定 C 类 provider
// （OCR/PaddleOCR 等）无模型时的配置提示仍走 400 原文（NewValidationErrorf
// 包了 "C 类 provider %s 没有 models"），不被 500 中性文案吞掉。
func TestProviderHandler_SyncToMultiRAG_CClassNoModels400(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	// PaddleOCR (C 类) 种子行：不配 default models → sync 时无模型可同步。
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID:        1,
		Key:       "paddleocr",
		Name:      "PaddleOCR",
		Protocol:  string(provider.ProtocolPaddleOCR),
		AuthStyle: string(provider.AuthStyleNoAuth),
	}).Error)

	svc := services.NewProviderService(providerSyncTestKey)
	h := NewProviderHandler(svc, &stubMultiRAGClient{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/providers/:id/sync-multirag", h.SyncToMultiRAG)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/sync-multirag", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "没有 models")
	require.NotContains(t, body, "服务器内部错误")
}

// TestProviderHandler_Create_BatchModelValidationKeepsIndex 锁定批量模型
// 校验错误保留外层 wrap 的索引与模型名（defaultModels[i] (modelID)）：
// respondProviderError 必须返回完整错误链 err.Error() 而非内层
// ValidationError.Error()。
func TestProviderHandler_Create_BatchModelValidationKeepsIndex(t *testing.T) {
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

	payload, _ := json.Marshal(map[string]any{
		"key":       "glm-cn",
		"name":      "GLM",
		"protocol":  "anthropic",
		"authStyle": "api_key",
		"baseUrl":   "https://open.bigmodel.cn/api/anthropic",
		"defaultModels": []map[string]any{
			{"modelId": "GLM-5", "modelType": "bogus"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "defaultModels[0] (GLM-5)")
	require.Contains(t, body, "type 不支持")
	require.NotContains(t, body, "服务器内部错误")
}
