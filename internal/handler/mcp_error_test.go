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
	r.GET("/api/v1/mcps", h.GetClientMcpsByAgent)
	r.GET("/api/v1/admin/agents/:name/mcps", h.GetAgentMcps)
	r.PUT("/api/v1/admin/agents/:name/mcps", h.UpdateAgentMcps)
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

// TestMcpHandler_Create_Validation400 升回端到端（brief 2c 原 Test 3）：
// service 层 validateMcpConfig 已包 *mcp.ValidationError（issue #95 batch 3a
// review P2：8 处用户可行动中文错误补包），POST 非法 transportType 经真实
// handler 调用链 → 400 完整链原文，而不是 500 中性。
func TestMcpHandler_Create_Validation400(t *testing.T) {
	setupMcpErrorTestDB(t)
	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	body := `{"name":"t-e2e","title":"端到端校验","url":"http://example.com/sse","transportType":"bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/mcps", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "transportType 必须是 sse / http 之一")
	require.NotContains(t, respBody, "服务器内部错误", "用户可行动的校验错误不得退化为 500 中性")
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

// TestMcpHandler_GetClientMcps_AgentNotFound404 公开端点回归锁（issue #95
// batch 3a review P2）：GetClientMcpsByAgent 对未知 agent 必须恢复 404
// 中文（service 层按 agent.ErrAgentNotFound sentinel 包装，handler 认
// sentinel），runtime 侧不得把「agent 不存在」误判为服务故障（500）。
func TestMcpHandler_GetClientMcps_AgentNotFound404(t *testing.T) {
	setupMcpErrorTestDB(t)
	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mcps?agent=ghost-agent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "Agent 不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误", "公开端点 not-found 不得退化为 500 中性")
}

// TestMcpHandler_GetAgentMcps_AgentNotFound404 管理端点回归锁（Task 1 review
// 明确要求）：GET admin agents/:name/mcps 对未知 agent 必须返回 404
// 「Agent 不存在」——service 层 GetAgentMcps 已按 gorm 分叉为
// agent.ErrAgentNotFound sentinel（此前裸中文 fmt.Errorf 落 500 中性），
// handler 认 sentinel；gorm 英文诊断不得泄漏。
func TestMcpHandler_GetAgentMcps_AgentNotFound404(t *testing.T) {
	setupMcpErrorTestDB(t)
	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/ghost/mcps", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "Agent 不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误", "not-found 不得退化为 500 中性")
}

// TestMcpHandler_UpdateAgentMcps_NotFound400 锁定绑定接口引用未知对象 →
// 400 原文（用户可行动）：service 层 UpdateAgentMcps 的 agent/mcp not-found
// 均包 *mcp.ValidationError，命令 handler 调用链验证非 500 中性。
func TestMcpHandler_UpdateAgentMcps_NotFound400(t *testing.T) {
	db := setupMcpErrorTestDB(t)
	// 场景 B 需要真实存在的 agent：seed 最小必填行（ContentHash/SystemPrompt
	// 无 DB 默认值）。
	seed := &agent.AgentConfig{
		Name:         "existing-agent",
		TenantID:     chatTestTenant,
		ContentHash:  "test-hash",
		SystemPrompt: "test-prompt",
	}
	require.NoError(t, db.Create(seed).Error)

	h := NewMcpHandler(services.NewMcpService("test-key"))
	r := newMcpErrorRouter(h)

	t.Run("agent not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost/mcps",
			bytes.NewBufferString(`{"mcpNames":["ghost-mcp"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
		body := w.Body.String()
		require.Contains(t, body, "Agent 'ghost' 不存在")
		require.NotContains(t, body, "服务器内部错误")
	})

	t.Run("mcp not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/existing-agent/mcps",
			bytes.NewBufferString(`{"mcpNames":["ghost-mcp"]}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
		body := w.Body.String()
		require.Contains(t, body, "MCP 'ghost-mcp' 不存在")
		require.NotContains(t, body, "服务器内部错误")
	})
}

// TestMcpHandler_UpdateAgentMcps_DBFailure500Neutral 锁定外审 #5635191918
// 发现 1：UpdateAgentMcps 的基础设施故障（DB 关闭）不得被误包为 400 用户
// 文案（"Agent/MCP 不存在"）——gorm 分叉后非 not-found 错误走英文诊断 →
// 500 中性 + 服务端日志，且 400 not-found 路径仍保留（上一测试锁定）。
func TestMcpHandler_UpdateAgentMcps_DBFailure500Neutral(t *testing.T) {
	db := setupMcpErrorTestDB(t)
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

	body := `{"mcpNames":["ghost-mcp"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost/mcps", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "服务器内部错误，请稍后重试")
	require.NotContains(t, respBody, "Agent 'ghost' 不存在", "DB 故障不得伪装为 400 用户文案")
	require.NotContains(t, respBody, "database is closed")
	require.Contains(t, logBuf.String(), "get agent ghost failed", "基础设施诊断必须进服务端日志")
}
