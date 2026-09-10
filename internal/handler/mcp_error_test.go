package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------- issue #95 P2：mcp handler 边界分流回归 ----------

// setupMcpErrorTestDB builds an in-memory sqlite DB with the minimal
// mcp/agent tables and injects it into the database.DB global (same pattern
// as setupAgentErrorTestDB). Repos capture the global at construction, so
// the service must be built AFTER injection.
func setupMcpErrorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcp.McpServer{}, &agent.AgentConfig{}, &mcp.AgentMcpServer{}))

	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
	return db
}

// newMcpErrorRouter wires the production McpHandler against the real
// service with the tenant seeded like the JWT middleware does. Only the
// routes exercised by the error regressions are registered.
func newMcpErrorRouter(h *McpHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.GET("/api/v1/admin/mcps", h.List)
	r.GET("/api/v1/admin/mcps/:name", h.Get)
	r.POST("/api/v1/admin/mcps", h.Create)
	r.PUT("/api/v1/admin/mcps/:name", h.Update)
	return r
}

// TestMcpHandler_Get_NotFound404 锁定 Get 端点对未知 MCP 返回
// 404 + 中文用户面文案（service 层 gorm not-found 已按 ErrMcpNotFound
// 包装，handler 认 sentinel 即可；gorm 英文诊断不得泄漏到响应体）。
func TestMcpHandler_Get_NotFound404(t *testing.T) {
	setupMcpErrorTestDB(t)
	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/mcps/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "MCP 不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误")
}

// TestMcpHandler_List_InternalError500Neutral 锁定 List 端点遇到基础设施
// 故障（DB 关闭）→ 500 中性中文文案；英文包装（"list MCPs failed"）
// 只进服务端日志，不得外泄到响应体。
func TestMcpHandler_List_InternalError500Neutral(t *testing.T) {
	db := setupMcpErrorTestDB(t)

	// 关闭底层连接：ListAll 随即返回错误，触发 500 中性分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/mcps", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "list MCPs failed")
	require.NotContains(t, body, "database is closed")
	require.Contains(t, logBuf.String(), "list MCPs failed", "内部诊断必须进服务端日志")
}

// TestMcpHandler_ValidationError400 函数级锁定 respondMcpError 对
// *mcp.ValidationError → 400 完整链原文（不被 500 中性化吞掉）。
// 说明：Service.Create 的校验路径（validateMcpConfig / 存在性检查）目前
// 返回 plain error（未包 ValidationError），不存在能端到端触发 Create 400
// 的输入——因此 ValidationError → 400 分支以 responder 函数级单测覆盖，
// 端到端服务调用链另由 Update 内置 MCP（TestMcpHandler_Update_Builtin
// TransportType400）实测（brief 2c 二选一保障）。
func TestMcpHandler_ValidationError400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	respondMcpError(c, mcp.NewValidationErrorf("校验失败：参数不合法"))

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "校验失败：参数不合法")
	require.NotContains(t, body, "服务器内部错误")
}

// TestMcpHandler_Update_BuiltinTransportType400 锁定内置 MCP 修改
// transportType → 400 原文（service 层 Update 的 "内置 MCP 不可修改
// transportType" 已包 mcp.ValidationError，经真实 handler 调用链验证
// ValidationError → 400 分流，而非 500 中性）。
func TestMcpHandler_Update_BuiltinTransportType400(t *testing.T) {
	db := setupMcpErrorTestDB(t)
	// 直插内置 MCP 行：IsBuiltin 置真 + transportType=sse，随后的
	// transportType=http 更新即命中 service 层内置保护。
	builtin := &mcp.McpServer{
		Name:          "knowledge",
		TenantID:      chatTestTenant,
		Title:         "知识库检索",
		TransportType: "sse",
		URL:           "http://hub.example.com/api/v1/knowledge/mcp",
		IsBuiltin:     true,
		ProbeStatus:   "success",
	}
	require.NoError(t, db.Create(builtin).Error)

	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	body := `{"transportType":"http"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/mcps/knowledge", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "内置 MCP 不可修改 transportType")
	require.NotContains(t, respBody, "服务器内部错误")
}
