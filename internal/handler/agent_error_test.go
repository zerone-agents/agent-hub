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
	"control-panel/internal/domain/provider"
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
	r.PUT("/api/v1/admin/agents/:name/subagents", h.UpdateSubagents)
	r.PUT("/api/v1/admin/agents/:name/knowledge", h.UpdateAgentKnowledge)
	r.GET("/api/v1/admin/agents/:name/deploy", h.GetDeployment)
	r.POST("/api/v1/admin/agents/:name/probe", h.ProbeAgent)
	return r
}

// seedAgentRow writes a provider-independent Agent row directly so the test
// can construct name/N-delegation conflicts without going through the create
// pipeline (mirrors the direct AgentConfig seeding in
// agent_chat_runtime_addressing_test.go; table name is "agents").
func seedAgentRow(t *testing.T, db *gorm.DB, name string) agent.AgentConfig {
	t.Helper()
	row := agent.AgentConfig{
		Name:         name,
		TenantID:     chatTestTenant,
		ContentHash:  "seed-hash",
		SystemPrompt: "seed",
	}
	require.NoError(t, db.Create(&row).Error)
	return row
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

// TestAgentHandler_Create_NameConflict400 锁定 Create 重名校验继续走 400
// 原文中文（CreateAgent 的 "Agent '%s' 已存在" 已包 ValidationError，不被
// handler 500 中性桶吞掉）。seed 一个同名 Agent 行（provider 无关直插）后
// 再次 POST 同名。
func TestAgentHandler_Create_NameConflict400(t *testing.T) {
	db := setupAgentErrorTestDB(t)
	seedAgentRow(t, db, "builder-a")

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"name":"builder-a","config":{"systemPrompt":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 'builder-a' 已存在")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_UpdateSubagents_MainAgentNotFound400 锁定 UpdateSubagents
// 主 Agent 不存在的裸中文错误（UpdateSubagents 的 "Agent '%s' 不存在"）已包
// ValidationError → 400 原文，而非 500 中性文案。
func TestAgentHandler_UpdateSubagents_MainAgentNotFound400(t *testing.T) {
	setupAgentErrorTestDB(t)
	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"subagents":["stray-sub"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost-parent/subagents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 'ghost-parent' 不存在")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_UpdateSubagents_ParentMounted400 锁定一层委托规则原文：
// 已被其他 Agent 挂载的 Agent 不能再挂载子 Agent（issue #111 委托深度=1）。
// 前置：parent-a 挂载 worker-b（直插 agent_subagents 行），再 PUT
// worker-b 挂载 cand-c → 命中 "Agent %q 已被其他 Agent 挂载…" 400 回归。
func TestAgentHandler_UpdateSubagents_ParentMounted400(t *testing.T) {
	db := setupAgentErrorTestDB(t)
	require.NoError(t, db.AutoMigrate(&agent.AgentSubagent{}))
	parent := seedAgentRow(t, db, "parent-a")
	worker := seedAgentRow(t, db, "worker-b")
	seedAgentRow(t, db, "cand-c")
	require.NoError(t, db.Create(&agent.AgentSubagent{AgentID: parent.ID, SubagentID: worker.ID}).Error)

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"subagents":["cand-c"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/worker-b/subagents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, `Agent \"worker-b\" 已被其他 Agent 挂载，不能再挂载子 Agent（运行时仅支持一层委托）`)
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_UpdateSubagents_SubagentMounted400 锁定一层委托规则原文
// 的另一侧：自身已挂载子 Agent 的 Agent 不能再被挂载。前置：worker-y 已挂载
// child-z，PUT parent-x 挂载 worker-y → 命中
// "Agent %q 自身已挂载子 Agent…" 400 回归。
func TestAgentHandler_UpdateSubagents_SubagentMounted400(t *testing.T) {
	db := setupAgentErrorTestDB(t)
	require.NoError(t, db.AutoMigrate(&agent.AgentSubagent{}))
	seedAgentRow(t, db, "parent-x")
	worker := seedAgentRow(t, db, "worker-y")
	child := seedAgentRow(t, db, "child-z")
	require.NoError(t, db.Create(&agent.AgentSubagent{AgentID: worker.ID, SubagentID: child.ID}).Error)

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"subagents":["worker-y"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/parent-x/subagents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, `Agent \"worker-y\" 自身已挂载子 Agent，不能再被挂载（运行时仅支持一层委托）`)
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_UpdateAgentKnowledge_MissingBuiltinMcp500Neutral 锁定
// 外审 #5612003511 P2 复现场景：Agent 存在、内置 knowledge MCP 缺失时，
// gorm.ErrRecordNotFound 底链来自 builtin MCP 查找而非 Agent 本身——
// 修前 handler 双认 gorm 会误判 404「Agent 不存在」且吞掉诊断日志；修后
// 404 只认 Agent/Provider 专属 sentinel，builtin MCP 缺失落 500 中性 +
// 服务端日志（英文诊断 "builtin MCP 'knowledge' not found" 只在日志）。
func TestAgentHandler_UpdateAgentKnowledge_MissingBuiltinMcp500Neutral(t *testing.T) {
	db := setupAgentErrorTestDB(t)
	require.NoError(t, db.AutoMigrate(&agent.AgentKnowledgeDataset{}, &mcp.McpServer{}))
	seedAgentRow(t, db, "builder-a") // Agent 存在；内置 knowledge MCP 未配置

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"dataset_ids":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/builder-a/knowledge", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "服务器内部错误，请稍后重试")
	require.NotContains(t, respBody, "Agent 不存在", "builtin MCP 缺失不得误判为 Agent 不存在")
	require.NotContains(t, respBody, "builtin MCP")
	require.NotContains(t, respBody, "record not found")
	require.Contains(t, logBuf.String(), "builtin MCP 'knowledge' not found", "MCP 缺失诊断必须进服务端日志")
}

// TestAgentHandler_UpdateAgentKnowledge_AgentNotFound404 锁定知识库端点对
// 未知 agent 仍走 404 + Agent 专属 sentinel 文案（service 层 gorm not-found
// 已按 ErrAgentNotFound 包装，handler 认 sentinel 即可——不应落 500）。
func TestAgentHandler_UpdateAgentKnowledge_AgentNotFound404(t *testing.T) {
	setupAgentErrorTestDB(t)
	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"dataset_ids":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost-agent/knowledge", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 不存在")
	require.NotContains(t, respBody, "record not found", "gorm 英文诊断不得泄漏")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestAgentHandler_GetDeployment_AgentNotFound404 锁定外审 #5614465831 P2：
// GetDeployment 不存在的 Agent 必须是 404「Agent 不存在」（service 层
// GetStatus 的 not-found 已补 sentinel 包装），不得因 404 桶只认 sentinel
// 而回归为 500 中性。
func TestAgentHandler_GetDeployment_AgentNotFound404(t *testing.T) {
	setupAgentErrorTestDB(t)

	// deployer client 可置空：GetStatus 在 GetByName not-found 时提前返回，
	// 不会触达 client。
	deployerSvc := services.NewAgentDeployerService(services.AgentDeployerConfig{})
	h := NewAgentHandler(services.NewAgentService("", ""), deployerSvc)
	r := newAgentErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/ghost-agent/deploy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 不存在")
	require.NotContains(t, respBody, "server internal error", "不得回归 500 中性桶")
	require.NotContains(t, respBody, "record not found", "gorm 英文诊断不得泄漏")
}

// TestAgentHandler_ProbeAgent_ProviderNotFound404 锁定外审 #5614465831 P3：
// Probe 的 Provider 不存在必须返回中文用户面「Provider 不存在」，
// 不得直接透出 provider 域英文 sentinel "provider not found"。
func TestAgentHandler_ProbeAgent_ProviderNotFound404(t *testing.T) {
	db := setupAgentErrorTestDB(t)
	// provider_summaries 表必须存在：id 不存在才返回 gorm.ErrRecordNotFound
	// （表缺失会报 "no such table" 而非 not-found，走 500 桶）。
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}))
	seedAgentRow(t, db, "probe-me")

	h := NewAgentHandler(services.NewAgentService("", ""), nil)
	r := newAgentErrorRouter(h)

	body := `{"providerId":999999}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents/probe-me/probe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Provider 不存在")
	require.NotContains(t, respBody, "provider not found", "英文 sentinel 不得直达用户")
	require.NotContains(t, respBody, "服务器内部错误")
}
