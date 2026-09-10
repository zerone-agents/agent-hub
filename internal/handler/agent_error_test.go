package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------- issue #95 P2：agent handler 边界分流回归 ----------

// setupAgentErrorTestDB builds an in-memory sqlite DB with the minimal
// agent table and injects it into the database.DB global (same pattern as
// provider_error_test.go / provider_review3_test.go). Repos capture the
// global at construction, so the service must be built AFTER injection.
func setupAgentErrorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}))

	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
	return db
}

// newAgentErrorRouter wires the production AgentHandler against real
// services with the tenant seeded like the JWT middleware does. deployer
// service is intentionally nil: the error paths under test return before
// any ComputePendingArtifacts call, so it is never dereferenced.
func newAgentErrorRouter(h *AgentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.GET("/api/v1/agents", h.List)
	r.GET("/api/v1/agents/:name", h.Get)
	r.POST("/api/v1/admin/agents", h.Create)
	return r
}

// TestAgentHandler_Get_NotFound404 锁定 Get 端点对未知 agent 返回
// 404 + 中文用户面文案（领域 not-found；不再「所有错误一律 404」，
// 且 gorm 英文诊断 "record not found" 不得泄漏到响应体）。
func TestAgentHandler_Get_NotFound404(t *testing.T) {
	setupAgentErrorTestDB(t)
	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "Agent 不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误")
}

// TestAgentHandler_Create_InternalError500Neutral 锁定 Create 端点遇到
// 基础设施故障（DB 关闭）→ 500 中性中文文案；service 层英文包装
// （"check agent existence failed: database is closed"）只进服务端
// 日志，不得外泄到响应体（issue #95 P2 外审关注点）。
func TestAgentHandler_Create_InternalError500Neutral(t *testing.T) {
	db := setupAgentErrorTestDB(t)

	// 关闭底层连接：ExistsByName 随即返回错误，触发 500 中性分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"name":"builder-a","config":{"systemPrompt":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "服务器内部错误，请稍后重试")
	require.NotContains(t, respBody, "check agent existence failed")
	require.NotContains(t, respBody, "database is closed")
	require.Contains(t, logBuf.String(), "check agent existence failed", "内部诊断必须进服务端日志")
}

// TestAgentHandler_Create_Validation400 锁定用户面校验错误仍走 400
// 原文中文（不被 500 中性化吞掉）。name 非空可绕过 binding:"required"
// （gin 层只查空），但命中 service 层 ValidateAgentName 校验路径
// （非法标识：大写字母+下划线）。
func TestAgentHandler_Create_Validation400(t *testing.T) {
	setupAgentErrorTestDB(t)
	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"name":"Bad_Name","config":{"systemPrompt":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 标识只能包含小写字母、数字和连字符，必须以字母开头，连字符不能连续或出现在首尾")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_List_InternalError500Neutral 锁定 List（GetDesktopAgents）
// 端点遇到基础设施故障（DB 关闭）→ 500 中性中文文案，英文诊断
// （"list agents failed"）不得外泄到响应体。
func TestAgentHandler_List_InternalError500Neutral(t *testing.T) {
	db := setupAgentErrorTestDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "list agents failed")
	require.NotContains(t, body, "database is closed")
}
