package handler

import (
	"bytes"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/skill"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------- issue #95 P2：skill handler 边界分流回归 ----------

// setupSkillErrorTestDB builds an in-memory sqlite DB with the minimal
// skills/agents tables and injects it into the database.DB global (same
// pattern as agent_error_test.go). Agents is migrated alongside skills so the
// agent-skill binding endpoints hit a real "record not found" (not
// "no such table"). Repos capture the global at construction, so the
// service must be built AFTER injection.
func setupSkillErrorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&skill.Skill{}, &agent.AgentConfig{}))

	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
	return db
}

// newSkillErrorRouter wires the production SkillHandler against real
// services with the tenant seeded like the JWT middleware does. Only the
// routes exercised by the error-path tests are registered.
func newSkillErrorRouter(t *testing.T, h *SkillHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	r.GET("/api/v1/skills", h.ListPublic)
	r.GET("/api/v1/skills/:name", h.GetPublic)
	r.GET("/api/v1/skills/:name/download", h.Download)
	r.POST("/api/v1/admin/skills", h.Create)
	r.GET("/api/v1/admin/agents/:name/skills", h.GetAgentSkills)
	r.PUT("/api/v1/admin/agents/:name/skills", h.UpdateAgentSkills)
	return r
}

// newSkillErrorHandler builds the handler against real services with the
// in-memory DB already injected. The uploader mock is inert for every path
// under test (validation / not-found errors all precede OSS interaction).
func newSkillErrorHandler(t *testing.T) *SkillHandler {
	t.Helper()
	return NewSkillHandler(services.NewSkillService(
		&toolUploaderMock{data: map[string][]byte{}}, ""))
}

// buildSkillCreateMultipart builds a multipart create request body with the
// given name/type and a "file" part. File content is junk on purpose: both
// create error paths (type validation, duplicate name) are hit before zip
// validation. To guarantee that, pass the file content that would fail zip
// validation rather than a valid zip.
func buildSkillCreateMultipart(t *testing.T, name, skillType string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("name", name))
	if skillType != "" {
		require.NoError(t, w.WriteField("type", skillType))
	}
	fw, err := w.CreateFormFile("file", "skill.zip")
	require.NoError(t, err)
	_, err = fw.Write([]byte("not-a-real-zip"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

// TestSkillHandler_Get_NotFound404 锁定 Get 端点对未知技能返回
// 404 + 中文用户面 sentinel 文案（服务层把所有 repo 错误映射为
// ErrSkillNotFound，gorm 英文诊断 "record not found" 不得泄漏到响应体；
// 修前 handler 对任意服务错误一律 404，修后经 respondSkillError 分流）。
func TestSkillHandler_Get_NotFound404(t *testing.T) {
	setupSkillErrorTestDB(t)
	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "技能不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误")
}

// TestSkillHandler_List_InternalError500Neutral 锁定 List 端点遇到
// 基础设施故障（DB 关闭）→ 500 中性中文文案；service 层英文包装
// （"list skills failed: database is closed"）只进服务端日志，不得
// 外泄到响应体（issue #95 P2：内部诊断英文化后禁止透传给用户）。
func TestSkillHandler_List_InternalError500Neutral(t *testing.T) {
	db := setupSkillErrorTestDB(t)

	// 关闭底层连接：repo 查询随即返回错误，触发 500 中性分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "list skills failed", "英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "database is closed", "DB 细节不得泄漏到响应体")
	require.Contains(t, logBuf.String(), "list skills failed", "内部诊断必须进服务端日志")
}

// TestSkillHandler_Create_Validation400 锁定用户面校验错误（skillType
// 非法）仍走 400 原文中文（不被 500 中性化吞掉）。skillType 是 string
// 无 binding:"required"/oneof 约束，bind 层不拦截，"bogus" 直达
// service 层 ValidateSkillType → ValidationError（Task 1 包装）。
func TestSkillHandler_Create_Validation400(t *testing.T) {
	setupSkillErrorTestDB(t)
	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	body, contentType := buildSkillCreateMultipart(t, "valid-skill", "bogus")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/skills", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "技能类型必须是 expert 或 community")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestSkillHandler_Create_NameConflict400 锁定 Create 重名校验继续走 400
// 原文中文（CreateSkill 的 "技能 '%s' 已存在" 已包 ValidationError，不被
// 500 中性桶吞掉）。seed 一个同名 skill 行（无文件，URL 空即可）后再次
// POST 同名且 type=expert（先于 zip 校验命中 ExistsByName 重名）。
func TestSkillHandler_Create_NameConflict400(t *testing.T) {
	db := setupSkillErrorTestDB(t)
	require.NoError(t, db.Create(&skill.Skill{
		Name:     "dup-skill",
		TenantID: "tenant-a",
		Type:     "expert",
	}).Error)

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	body, contentType := buildSkillCreateMultipart(t, "dup-skill", "expert")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/skills", body)
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "技能 'dup-skill' 已存在")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestSkillHandler_GetAgentSkills_AgentNotFound404 锁定管理端点 GET admin
// agents/:name/skills 对未知 agent 返回 404「Agent 不存在」——service 层
// GetAgentSkills 已按 gorm 分叉为 agent.ErrAgentNotFound sentinel（此前裸
// 中文 fmt.Errorf 落 500 中性），respondSkillError 404 桶新增 agent 分支后
// 正确路由；gorm 英文诊断不得泄漏。
func TestSkillHandler_GetAgentSkills_AgentNotFound404(t *testing.T) {
	setupSkillErrorTestDB(t)
	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/ghost/skills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "Agent 不存在")
	require.NotContains(t, body, "record not found", "gorm 英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "服务器内部错误", "not-found 不得退化为 500 中性")
}

// TestSkillHandler_UpdateAgentSkills_AgentNotFound400 锁定绑定接口引用
// 未知 agent → 400 原文（用户可行动）：service 层 UpdateAgentSkills 的
// agent 不存在按 skill.ValidationError 包装（此前裸中文 fmt.Errorf 落 500
// 中性），handler 经 ValidationError 分支落 400 完整链。
func TestSkillHandler_UpdateAgentSkills_AgentNotFound400(t *testing.T) {
	setupSkillErrorTestDB(t)
	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	body := `{"skillNames":["ghost-skill"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost/skills", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Agent 'ghost' 不存在")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestSkillHandler_UpdateAgentSkills_SkillNotFound400 锁定绑定接口引用
// 未知技能 → 400 原文（场景 B 需要真实存在的 agent 越过第一步校验，命中
// Service 层技能存在性校验——skill.ValidationError，非 500 中性）。
func TestSkillHandler_UpdateAgentSkills_SkillNotFound400(t *testing.T) {
	db := setupSkillErrorTestDB(t)
	seed := &agent.AgentConfig{
		Name:         "existing-agent",
		TenantID:     "tenant-a",
		ContentHash:  "test-hash",
		SystemPrompt: "test-prompt",
	}
	require.NoError(t, db.Create(seed).Error)

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	body := `{"skillNames":["ghost-skill"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/existing-agent/skills", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
	respBody := w.Body.String()
	require.Contains(t, respBody, "Skill 'ghost-skill' 不存在")
	require.NotContains(t, respBody, "服务器内部错误")
}

// TestSkillHandler_Get_DBFailure500Neutral 锁定 Get 端点遇到基础设施故障
// （DB 关闭）→ 500 中性中文文案 + 服务端日志（替代修前的 404 伪装）：
// service 层 GetSkill 已按 gorm 分叉，非 not-found 错误走英文诊断
// "get skill %s failed"，handler 落 500 中性。用合法名直接越过路径参数
// 检查（GetPublic 无 binding 拦截），命中 service 层 DB 故障。
func TestSkillHandler_Get_DBFailure500Neutral(t *testing.T) {
	db := setupSkillErrorTestDB(t)

	// 关闭底层连接：GetByName 随即返回错误，触发 gorm 分叉的非 not-found 分支。
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	// 捕获服务端日志，锁定「完整错误链只在日志」。
	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/some-skill", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "服务器内部错误，请稍后重试")
	require.NotContains(t, body, "技能不存在", "DB 故障不得伪装为 404 not-found")
	require.NotContains(t, body, "get skill", "英文诊断不得泄漏到响应体")
	require.NotContains(t, body, "database is closed", "DB 细节不得泄漏到响应体")
	require.Contains(t, logBuf.String(), "get skill some-skill failed", "内部诊断必须进服务端日志")
}

// TestSkillHandler_Download_FileNotFound404 锁定 Download 端点对技能存在
// 但文件缺失（URL 为空）返回 404「技能文件不存在」（ErrSkillFileNotFound
// sentinel 并入 respondSkillError 后语义不倒退——原 handler 双判断分支
// 的等价回归）。
func TestSkillHandler_Download_FileNotFound404(t *testing.T) {
	db := setupSkillErrorTestDB(t)
	require.NoError(t, db.Create(&skill.Skill{
		Name:     "dl-skill",
		TenantID: "tenant-a",
		Type:     "expert",
	}).Error)

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills/dl-skill/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	require.Contains(t, body, "技能文件不存在")
	require.NotContains(t, body, "服务器内部错误")
}

// TestSkillHandler_UpdateAgentSkills_DBFailure500Neutral 锁定 review 裁决的
// fix：UpdateAgentSkills 的 DB 故障不得伪装为 400 中文「不存在」——gorm 分叉
// 后非 not-found 走英文诊断 → 500 中性 + 日志（对齐 #144 5db0991 的
// UpdateAgentMcps 模式）。
func TestSkillHandler_UpdateAgentSkills_DBFailure500Neutral(t *testing.T) {
	db := setupSkillErrorTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	h := newSkillErrorHandler(t)
	r := newSkillErrorRouter(t, h)

	body := `{"skillNames":["ghost-skill"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/ghost/skills", bytes.NewBufferString(body))
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
