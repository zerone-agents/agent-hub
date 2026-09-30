package handler

// Issue #206：skill / tool / mcp 三个域的 create / update / delete
// 成功路径必须落审计行（接入前三域 handler 从未接线 AuditRecorder）。
// fixture 复用同包既有模式：toolUploaderMock（skill_test/tool_custom_test）、
// buildZip 式内存 zip（services/skill_validator_test 同构，本包自建副本）、
// 真实 AuditRecorder 落同一内存库（audit_embeds_test.go 的
// newHandlerTestAuditRecorder 用独立库，本文件需要断言审计行故同库）。

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/skill"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// buildAuditTestSkillZip 构造合法 skill zip（顶层目录包裹 SKILL.md，
// frontmatter 满足服务端校验器全部规则）。
func buildAuditTestSkillZip(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(name + "/SKILL.md")
	require.NoError(t, err)
	_, err = f.Write([]byte("---\nname: " + name + "\ndescription: audit test skill\n---\n# body\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

// auditTestActor 注入 JWT 中间件同款上下文（tenant + user）。
func auditTestActor() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("tenant_id", "tenant-a")
		c.Set("user_id", "user-1")
		c.Set("user_name", "Ada")
	}
}

// assertAuditRow 断言恰好一行指定 action 的审计行，且 category/targetType/
// targetName/status 全部正确。
func assertAuditRow(t *testing.T, db *gorm.DB, action audit.Action, cat audit.Category, tt audit.TargetType, targetName string) {
	t.Helper()
	require.EqualValues(t, 1, countAuditByAction(t, db, action))
	row := fetchAuditByAction(t, db, action)
	require.Equal(t, cat, row.Category)
	require.Equal(t, tt, row.TargetType)
	require.Equal(t, targetName, row.TargetName)
	require.Equal(t, audit.StatusSuccess, row.Status)
}

// ── skill.create / skill.update / skill.delete ──────────────────────────────

func setupAuditSkillEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&skill.Skill{}, &agent.AgentConfig{}, &agent.AgentSkill{}, &audit.Log{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewSkillHandler(services.NewSkillService(&toolUploaderMock{data: map[string][]byte{}}, "https://cdn.example.com"), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auditTestActor())
	r.POST("/api/v1/admin/skills", h.Create)
	r.PUT("/api/v1/admin/skills/:name", h.Update)
	r.DELETE("/api/v1/admin/skills/:name", h.Delete)
	return r, db
}

func TestAuditSkillCreateUpdateDelete(t *testing.T) {
	r, db := setupAuditSkillEnv(t)

	// create：multipart（name + 合法 zip 文件）
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("name", "audit-skill"))
	fw, err := mw.CreateFormFile("file", "audit-skill.zip")
	require.NoError(t, err)
	_, err = fw.Write(buildAuditTestSkillZip(t, "audit-skill"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/skills", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionSkillCreate, audit.CatSkill, audit.TargetSkill, "audit-skill")

	// update：multipart 仅字段（handler 对 file 走 FormFile，Content-Type 必须
	// 是 multipart/form-data，缺文件时返回 ErrMissingFile 被容忍）
	body.Reset()
	mw = multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("title", "审计技能"))
	require.NoError(t, mw.WriteField("description", "desc"))
	require.NoError(t, mw.Close())
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/skills/audit-skill", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionSkillUpdate, audit.CatSkill, audit.TargetSkill, "audit-skill")

	// delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/skills/audit-skill", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionSkillDelete, audit.CatSkill, audit.TargetSkill, "audit-skill")
}

// ── tool.create / tool.update / tool.delete ─────────────────────────────────

func setupAuditToolEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}, &audit.Log{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewToolHandler(services.NewToolService(&toolUploaderMock{data: map[string][]byte{}}), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auditTestActor())
	r.POST("/api/v1/admin/tools", h.Create)
	r.PUT("/api/v1/admin/tools/:name", h.Update)
	r.DELETE("/api/v1/admin/tools/:name", h.Delete)
	return r, db
}

func TestAuditToolCreateUpdateDelete(t *testing.T) {
	r, db := setupAuditToolEnv(t)

	// create：multipart（name/title + .ts 文件）
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("name", "auditTool"))
	require.NoError(t, mw.WriteField("title", "审计工具"))
	fw, err := mw.CreateFormFile("file", "auditTool.ts")
	require.NoError(t, err)
	_, err = fw.Write([]byte("export default { name: 'auditTool' }"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tools", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionToolCreate, audit.CatTool, audit.TargetTool, "auditTool")

	// update：JSON 元数据
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/tools/auditTool", strings.NewReader(`{"title":"审计工具 v2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionToolUpdate, audit.CatTool, audit.TargetTool, "auditTool")

	// delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tools/auditTool", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionToolDelete, audit.CatTool, audit.TargetTool, "auditTool")
}

// ── mcp.create / mcp.update / mcp.delete ────────────────────────────────────

func setupAuditMcpEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&mcp.McpServer{}, &agent.AgentConfig{}, &mcp.AgentMcpServer{}, &audit.Log{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewMcpHandler(services.NewMcpService("test-key"), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auditTestActor())
	r.POST("/api/v1/admin/mcps", h.Create)
	r.PUT("/api/v1/admin/mcps/:name", h.Update)
	r.DELETE("/api/v1/admin/mcps/:name", h.Delete)
	return r, db
}

func TestAuditMcpCreateUpdateDelete(t *testing.T) {
	r, db := setupAuditMcpEnv(t)

	// create：JSON（name/title/transportType 必填）
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/mcps",
		strings.NewReader(`{"name":"audit-mcp","title":"审计 MCP","transportType":"sse","url":"https://mcp.example.com/sse"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionMcpCreate, audit.CatMcp, audit.TargetMcp, "audit-mcp")

	// update：JSON
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/mcps/audit-mcp", strings.NewReader(`{"title":"审计 MCP v2"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionMcpUpdate, audit.CatMcp, audit.TargetMcp, "audit-mcp")

	// delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/mcps/audit-mcp", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionMcpDelete, audit.CatMcp, audit.TargetMcp, "audit-mcp")
}
