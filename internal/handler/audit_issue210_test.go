package handler

// Issue #210：补齐 #206 遗留的三类审计盲区——
//   1. tool UploadFile（制品替换，实质性内容变更）
//   2. agent 绑定变更（UpdateAgentSkills / UpdateAgentTools / UpdateAgentMcps）
//   3. agent create / update
// 绑定变更归 agent 分类（agent.update_bindings），Detail 记录 kind + 新名单；
// agent.update 的 Detail 记录请求中出现的顶层字段名（不含值）。
// fixture 复用 audit_skill_tool_mcp_test.go 的 setupAuditToolEnv /
// auditTestActor / assertAuditRow。

import (
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

// ── tool.upload_file ────────────────────────────────────────────────────────

func TestAuditToolUploadFile(t *testing.T) {
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
	r.PUT("/api/v1/admin/tools/:name/file", h.UploadFile)

	// 先 create 一个工具（其审计行 action 不同，不干扰计数）
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	require.NoError(t, mw.WriteField("name", "uploadAudit"))
	fw, err := mw.CreateFormFile("file", "uploadAudit.ts")
	require.NoError(t, err)
	_, err = fw.Write([]byte("export default { name: 'uploadAudit' }"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tools", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())

	// 替换制品文件 → 必须落 tool.upload_file 行
	body.Reset()
	mw = multipart.NewWriter(&body)
	fw, err = mw.CreateFormFile("file", "uploadAudit.ts")
	require.NoError(t, err)
	_, err = fw.Write([]byte("export default { name: 'uploadAudit', version: 2 }"))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/tools/uploadAudit/file", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	assertAuditRow(t, db, audit.ActionToolUploadFile, audit.CatTool, audit.TargetTool, "uploadAudit")
}

// ── agent.update_bindings（skills / tools / mcps 三个端点）─────────────────

func setupAuditBindingsEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(
		&agent.AgentConfig{},
		&agent.Tool{}, &agent.AgentTool{},
		&skill.Skill{}, &agent.AgentSkill{},
		&mcp.McpServer{}, &mcp.AgentMcpServer{},
		&audit.Log{},
	))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	// 种子：一个 agent + 各一份可绑定资源；内置 Skill 工具行（共享行
	// tenant_id=''）是 UpdateAgentSkills 挂载/卸载 Skill tool 的前置
	require.NoError(t, db.Create(&agent.AgentConfig{
		Name: "bind-me", TenantID: "tenant-a", SystemPrompt: "p", ContentHash: "h",
	}).Error)
	require.NoError(t, db.Create(&skill.Skill{Name: "skill-x", TenantID: "tenant-a", Type: "expert"}).Error)
	require.NoError(t, db.Create(&agent.Tool{
		Name: "toolX", TenantID: "tenant-a", Source: "custom",
		FileName: "toolX.ts", FileURL: "tools/toolX.ts", FileHash: "h", FileSize: 1,
	}).Error)
	require.NoError(t, db.Create(&agent.Tool{Name: "Skill", TenantID: "", Source: "builtin"}).Error)
	require.NoError(t, db.Create(&mcp.McpServer{Name: "mcp-x", TenantID: "tenant-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}).Error)

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	skillH := NewSkillHandler(services.NewSkillService(&toolUploaderMock{data: map[string][]byte{}}, ""), recorder)
	toolH := NewToolHandler(services.NewToolService(&toolUploaderMock{data: map[string][]byte{}}), recorder)
	mcpH := NewMcpHandler(services.NewMcpService("test-key"), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auditTestActor())
	r.PUT("/api/v1/admin/agents/:name/skills", skillH.UpdateAgentSkills)
	r.PUT("/api/v1/admin/agents/:name/tools", toolH.UpdateAgentTools)
	r.PUT("/api/v1/admin/agents/:name/mcps", mcpH.UpdateAgentMcps)
	return r, db
}

func TestAuditAgentBindingUpdates(t *testing.T) {
	r, db := setupAuditBindingsEnv(t)

	put := func(path, body string) {
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "%s body=%s", path, rec.Body.String())
	}

	put("/api/v1/admin/agents/bind-me/skills", `{"skillNames":["skill-x"]}`)
	put("/api/v1/admin/agents/bind-me/tools", `{"toolNames":["toolX"]}`)
	put("/api/v1/admin/agents/bind-me/mcps", `{"mcpNames":["mcp-x"]}`)

	// 三次绑定变更 → 三行 agent.update_bindings，target 均为 agent 名
	require.EqualValues(t, 3, countAuditByAction(t, db, audit.ActionAgentUpdateBindings))
	var rows []audit.Log
	require.NoError(t, db.Where("action = ?", audit.ActionAgentUpdateBindings).Order("id").Find(&rows).Error)

	wantKinds := []string{"skill", "tool", "mcp"}
	wantNames := []string{"skill-x", "toolX", "mcp-x"}
	for i, row := range rows {
		require.Equal(t, audit.CatAgent, row.Category)
		require.Equal(t, audit.TargetAgent, row.TargetType)
		require.Equal(t, "bind-me", row.TargetName)
		require.Equal(t, audit.StatusSuccess, row.Status)
		require.Contains(t, row.Detail, `"kind":"`+wantKinds[i]+`"`)
		require.Contains(t, row.Detail, wantNames[i])
	}
}

// ── agent.create / agent.update ─────────────────────────────────────────────

func setupAuditAgentCrudEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(
		&agent.AgentConfig{}, &agent.AgentSubagent{},
		&agent.Tool{}, &agent.AgentTool{}, // CreateAgent 末尾 BindDefaultToolsToAgent 需要
		&skill.Skill{}, &agent.AgentSkill{}, // GetAgent 返回 DTO 时 join skills
		&mcp.McpServer{}, &mcp.AgentMcpServer{}, // 同上，join mcps
		&agent.AgentKnowledgeDataset{}, // 同上，join datasets
		&audit.Log{},
	))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	// Create/Update 不触 deployer，nil 即可（agent_error_test.go 同款取舍）
	h := NewAgentHandler(services.NewAgentService("", ""), nil, recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(auditTestActor())
	r.POST("/api/v1/admin/agents", h.Create)
	r.PUT("/api/v1/admin/agents/:name", h.Update)
	return r, db
}

func TestAuditAgentCreateUpdate(t *testing.T) {
	r, db := setupAuditAgentCrudEnv(t)

	// create
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents",
		strings.NewReader(`{"name":"audit-agent","config":{"systemPrompt":"p"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionAgentCreate, audit.CatAgent, audit.TargetAgent, "audit-agent")

	// update：开 desktopEnabled → Detail 记录变更字段名
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/audit-agent",
		strings.NewReader(`{"desktopEnabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	assertAuditRow(t, db, audit.ActionAgentUpdate, audit.CatAgent, audit.TargetAgent, "audit-agent")
	row := fetchAuditByAction(t, db, audit.ActionAgentUpdate)
	require.Contains(t, row.Detail, "desktopEnabled")
}
