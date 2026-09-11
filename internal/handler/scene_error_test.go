package handler

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/scene"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------- issue #95 P2：scene handler 边界分流回归 ----------

// setupSceneErrorTestDB builds an in-memory sqlite DB with the minimal
// scene/agent tables and injects it into the database.DB global (same
// pattern as setupAgentErrorTestDB). Repos capture the global at
// construction, so the service must be built AFTER injection.
func setupSceneErrorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&scene.Scene{}, &agent.AgentConfig{}))

	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
	return db
}

// newSceneErrorRouter wires the production SceneHandler against the real
// service with the tenant seeded like the JWT middleware does. Only the
// routes exercised by the error regressions are registered.
func newSceneErrorRouter(h *SceneHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.GET("/api/v1/scenes", h.List)
	r.GET("/api/v1/scenes/:name", h.Get)
	r.POST("/api/v1/admin/scenes", h.Create)
	return r
}

// TestSceneHandler_Get_NotFound404 锁定 Get 端点对未知场景返回
// 404 + 中文用户面文案（ErrSceneNotFound sentinel；gorm 英文诊断
// 不得泄漏到响应体）。
func TestSceneHandler_Get_NotFound404(t *testing.T) {
	setupSceneErrorTestDB(t)
	h := NewSceneHandler(services.NewSceneService())
	r := newSceneErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "场景不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误")
}

// TestSceneHandler_List_InternalError500Neutral 锁定 List 端点遇到基础
// 设施故障（DB 关闭）→ 500 中性中文文案；英文包装（"list scenes failed"）
// 只进服务端日志，不得外泄到响应体。
func TestSceneHandler_List_InternalError500Neutral(t *testing.T) {
	db := setupSceneErrorTestDB(t)

	// 关闭底层连接：ListAll 随即返回错误，触发 500 中性分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := NewSceneHandler(services.NewSceneService())
	r := newSceneErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "list scenes failed")
	require.NotContains(t, body, "database is closed")
	require.Contains(t, logBuf.String(), "list scenes failed", "内部诊断必须进服务端日志")
}

// TestSceneHandler_Get_DBFailure500Neutral 锁定 Get 端点遇到基础设施
// 故障（DB 关闭）→ 500 中性中文文案 + 服务端日志（替代修前的 404 伪装）：
// service 层 GetScene 已按 gorm 分叉（非 not-found 不再被吞成
// ErrSceneNotFound -> 404），英文诊断 "get scene %s failed" 只进日志。
func TestSceneHandler_Get_DBFailure500Neutral(t *testing.T) {
	db := setupSceneErrorTestDB(t)

	// 关闭底层连接：GetByName 随即返回错误，触发 gorm 分叉的非 not-found 分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := NewSceneHandler(services.NewSceneService())
	r := newSceneErrorRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scenes/some-scene", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "场景不存在", "DB 故障不得伪装为 404 not-found")
	require.NotContains(t, body, "get scene", "英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "database is closed", "DB 细节不得泄漏到响应体")
	require.Contains(t, logBuf.String(), "get scene some-scene failed", "内部诊断必须进服务端日志")
}

// TestSceneHandler_Create_Validation400 锁定用户面校验错误仍走 400
// 原文中文（不被 500 中性化吞掉）。binding:"required" 只查非空串，
// 全空格 title 可绕过 gin 绑定但命中 service 层 ValidateSceneTitle
// （TrimSpace 后为空 → scene.ValidationError「场景名称不能为空」），
// 且按实际校验顺序 name 校验在前已通过、不会先触达 Agent 存在性检查。
func TestSceneHandler_Create_Validation400(t *testing.T) {
	setupSceneErrorTestDB(t)
	h := NewSceneHandler(services.NewSceneService())
	r := newSceneErrorRouter(h)

	body := `{"name":"myscene","agentId":1,"title":"  ","prompt":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scenes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "场景名称不能为空")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestSceneHandler_Create_AgentNotFound400 锁定 Create 关联不存在的
// Agent → 400 原文（ErrAgentNotFound 属用户可行动的关联校验冲突，非
// not-found 也非内部故障）。name/title/prompt 全部合法以通过前置校验，
// agents 表已迁移（不存在 → false 而非 no such table）。
func TestSceneHandler_Create_AgentNotFound400(t *testing.T) {
	setupSceneErrorTestDB(t)
	h := NewSceneHandler(services.NewSceneService())
	r := newSceneErrorRouter(h)

	body := `{"name":"myscene2","agentId":99999,"title":"My Scene","prompt":"hello world"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scenes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "关联的 Agent 不存在")
	require.NotContains(t, respBody, "服务器内部错误")
}
