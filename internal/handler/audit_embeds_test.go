package handler

// Task 12 埋点回归：provider legacy 迁移（reveal_key / runtime_config 经
// Recorder legacy formatter 双写）+ agent / cli_token / aigc / sync_multirag
// 落库断言。fixture 复用同包既有模式（auditTestSqlite 包装器、
// stubMultiRAGClient、fakeModelSource、chatTestTenant）。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/aigc"
	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/provider"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/middleware"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// openAuditEmbedDB 打开 datetime(6) 包装的内存库（复用同包 audit_test.go 的
// auditTestSqlite——审计行 First 全列扫描含 CreatedAt，须去掉 MySQL 精度后缀）。
func openAuditEmbedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(auditTestSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	return db
}

// newHandlerTestAuditRecorder 供既有 fixture 构造器迁移用：真实 recorder 落到
// 自带内存 audit_logs，不与 fixture 业务表混库（既有测试不断言审计行）。
func newHandlerTestAuditRecorder(t *testing.T) *services.AuditRecorder {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	return services.NewAuditRecorder(repository.NewAuditRepository(db))
}

func countAuditByAction(t *testing.T, db *gorm.DB, action audit.Action) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&audit.Log{}).Where("action = ?", action).Count(&n).Error)
	return n
}

func fetchAuditByAction(t *testing.T, db *gorm.DB, action audit.Action) *audit.Log {
	t.Helper()
	var row audit.Log
	require.NoError(t, db.Where("action = ?", action).First(&row).Error)
	return &row
}

// ── provider.reveal_key：legacy 双写（stdout 逐字重放 + DB 落库）──────────

// TestAuditRevealKeyPersistsAndStdout 复用 provider_reveal_test.go 的 fixture
// （RequireAdmin + provider 行；该 fixture 已迁移为注入真实 recorder + tenant），
// 追加断言：成功后 audit_logs 出现 action=provider.reveal_key 行（spec §8）。
func TestAuditRevealKeyPersistsAndStdout(t *testing.T) {
	router, auditLog, db := setupProviderRevealRouter(t, "sk-audit-persist-001")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/reveal-key", nil)
	req.Header.Set("X-Test-Admin", "true")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionRevealKey))
	row := fetchAuditByAction(t, db, audit.ActionRevealKey)
	require.Equal(t, audit.CatProvider, row.Category)
	require.Equal(t, audit.TargetProvider, row.TargetType)
	require.Equal(t, "1", row.TargetID)
	require.Contains(t, auditLog.String(), "[AUDIT] provider API key revealed")
	require.NotContains(t, auditLog.String(), "sk-audit-persist-001")
}

// ── agent.deploy：真实 AgentDeployerService + 假 deployer HTTP 服务 ─────────

// setupAuditDeployEnv 构造 DeployAgent 审计环境：agent/provider 种子 +
// httptest deployer（能力探针回 v3.1 sentinel 400、create 回 200 幂等、
// GET 回 404）+ 真实 recorder（agent_deployer_handler_test.go 的 fakeDeployer
// 是 handler 镜像 seam，进不了真实 AgentHandler，故按
// agent_delete_failclosed_test.go 的 httptest deployer 模式落地）。
func setupAuditDeployEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(
		&agent.AgentConfig{}, &agent.AgentSubagent{},
		&agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{},
		&agent.Tool{}, &agent.AgentTool{},
		&skill.Skill{}, &agent.AgentSkill{},
		&mcp.McpServer{}, &mcp.AgentMcpServer{},
		&agent.AgentKnowledgeDataset{}, &agent.DeploymentSnapshot{},
		&provider.ProviderSummary{}, &provider.ProviderModel{}, &provider.ProviderAttribute{},
		&audit.Log{},
	))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	providerID := uint64(1)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID: 1, Key: "glm-cn", Name: "GLM", Protocol: "anthropic", AuthStyle: "api_key",
		BaseURL: "https://open.bigmodel.cn/api/anthropic",
	}).Error)
	require.NoError(t, db.Create(&agent.AgentConfig{
		Name: "deploy-me", TenantID: chatTestTenant, SystemPrompt: "p", ContentHash: "h",
		ProviderID: &providerID, ModelID: "glm-5",
	}).Error)

	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// resolveRuntimeToken 的 GetAgent 探测：本地无 token 且要求无容器。
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"error":"agent not found"}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "hub-capability-probe") {
			// SupportsDeploymentKey 的 v3.1+ sentinel（client.go 钉死契约）。
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"success":false,"error":"deploymentKey is required"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"agentName":"deploy-me","status":"running"}}`))
	}))
	t.Cleanup(deployerSrv.Close)

	deployerSvc := services.NewAgentDeployerService(services.AgentDeployerConfig{
		Client:        deployer.NewClient(deployerSrv.URL, "unused"),
		AuthMode:      services.ModeBuiltin,
		EncryptionKey: providerSyncTestKey,
	})
	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewAgentHandler(services.NewAgentService("", ""), deployerSvc, recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", chatTestTenant)
		c.Set("user_id", "user-1")
		c.Set("user_name", "Ada")
	})
	r.POST("/api/v1/admin/agents/:name/deploy", h.DeployAgent)
	return r, db
}

// TestAuditDeployAgent：DeployAgent 成功后落 agent.deploy 行（name 同时作
// TargetID/TargetName——路由参数无独立数字 id）。
func TestAuditDeployAgent(t *testing.T) {
	r, db := setupAuditDeployEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents/deploy-me/deploy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionDeploy))
	row := fetchAuditByAction(t, db, audit.ActionDeploy)
	require.Equal(t, audit.CatAgent, row.Category)
	require.Equal(t, audit.TargetAgent, row.TargetType)
	require.Equal(t, "deploy-me", row.TargetID)
	require.Equal(t, "deploy-me", row.TargetName)
	require.Equal(t, audit.StatusSuccess, row.Status)
}

// ── cli_token.issue / cli_token.revoke ─────────────────────────────────────

func setupAuditCliTokenEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&authdom.CLIToken{}, &audit.Log{}))
	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewCLITokenHandler(services.NewCLITokenService(db), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Set("user_name", "Ada")
		c.Set("tenant_id", "t1")
	})
	r.POST("/api/v1/user/cli-tokens", h.Issue)
	r.DELETE("/api/v1/user/cli-tokens/:id", h.Revoke)
	return r, db
}

// TestAuditCliTokenIssueRevoke：issue 记 token 名不记值（spec §3），revoke 记
// 数字 id；并断言 Category=token 关闭 T1 categoryOf 分支的覆盖债（Ledger
// 延后项：cli_token.* 前缀与 category 名不同，是唯一显式映射分支）。
func TestAuditCliTokenIssueRevoke(t *testing.T) {
	r, db := setupAuditCliTokenEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/cli-tokens", strings.NewReader(`{"name":"ci-token"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())

	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionCliTokenIssue))
	issueRow := fetchAuditByAction(t, db, audit.ActionCliTokenIssue)
	require.Equal(t, audit.CatToken, issueRow.Category)
	require.Equal(t, audit.TargetToken, issueRow.TargetType)
	require.Equal(t, "", issueRow.TargetID) // 不记值：明文 token 绝不落审计行
	require.Equal(t, "ci-token", issueRow.TargetName)

	var ids []uint64
	require.NoError(t, db.Model(&authdom.CLIToken{}).Where("name = ?", "ci-token").Pluck("id", &ids).Error)
	require.Len(t, ids, 1)
	idStr := strconv.FormatUint(ids[0], 10)

	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/user/cli-tokens/"+idStr, nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code, "body=%s", rec2.Body.String())

	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionCliTokenRevoke))
	revokeRow := fetchAuditByAction(t, db, audit.ActionCliTokenRevoke)
	require.Equal(t, audit.CatToken, revokeRow.Category)
	require.Equal(t, idStr, revokeRow.TargetID)
}

// ── aigc.save / aigc.rotate_key / aigc.delete ──────────────────────────────

func setupAuditAigcEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}, &audit.Log{}))
	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewAigcConfigHandler(services.NewAigcConfigService(db, aigcHandlerTestEncKey, fakeModelSource{}), recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-Admin") == "true" {
			c.Set("roles", []string{"admin"})
			c.Set("user_id", "user-1")
			c.Set("tenant_id", "default") // 模拟 tenant 注入中间件（builtin 恒 default）
		}
	})
	g := r.Group("/api/v1/admin/aigc", middleware.RequireAdmin())
	g.PUT("/config", h.Save)
	g.POST("/config/rotate-key", h.RotateKey)
	g.DELETE("/config", h.Delete)
	return r, db
}

// TestAuditAigcSaveDeleteRotateKey：Save 走 receipt（create 路径 ChangedFields
// 全集进 Detail）；rotate_key / delete 各落一行。
func TestAuditAigcSaveDeleteRotateKey(t *testing.T) {
	r, db := setupAuditAigcEnv(t)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Admin", "true")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := do(http.MethodPut, "/api/v1/admin/aigc/config", `{"uscc":"91320118MAK93FC72D","companyName":"南京审计测试有限公司"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionAigcSave))
	saveRow := fetchAuditByAction(t, db, audit.ActionAigcSave)
	require.Equal(t, audit.CatAigc, saveRow.Category)
	require.Equal(t, audit.TargetAigcConfig, saveRow.TargetType)
	require.Contains(t, saveRow.Detail, "uscc")
	require.Contains(t, saveRow.Detail, "companyName")

	rec = do(http.MethodPost, "/api/v1/admin/aigc/config/rotate-key", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionAigcRotateKey))
	rotateRow := fetchAuditByAction(t, db, audit.ActionAigcRotateKey)
	require.Equal(t, audit.CatAigc, rotateRow.Category)
	require.Equal(t, "default", rotateRow.TargetID) // aigc 配置以租户为主键

	rec = do(http.MethodDelete, "/api/v1/admin/aigc/config", "")
	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionAigcDelete))
	deleteRow := fetchAuditByAction(t, db, audit.ActionAigcDelete)
	require.Equal(t, audit.CatAigc, deleteRow.Category)
}

// ── provider.sync_multirag ──────────────────────────────────────────────────

// TestAuditSyncMultirag：同步成功后落 provider.sync_multirag 行（复用
// provider_sync_test.go 的种子形态 + stubMultiRAGClient）。
func TestAuditSyncMultirag(t *testing.T) {
	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderModel{}, &audit.Log{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	encrypted, err := provider.Encrypt("sk-test-glm-key", providerSyncTestKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&provider.ProviderSummary{
		ID: 1, Key: "glm-cn", Name: "GLM", Protocol: "anthropic", AuthStyle: "api_key",
		BaseURL: "https://open.bigmodel.cn/api/anthropic", LockedAPIKey: encrypted,
	}).Error)
	require.NoError(t, db.Create(&provider.ProviderModel{
		ProviderID: 1, SelectionID: "GLM-5-Turbo", ModelID: "GLM-5-Turbo",
		DisplayName: "GLM-5-Turbo", ModelType: "llm", ContextWindow: 200000,
	}).Error)

	recorder := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := NewProviderHandler(services.NewProviderService(providerSyncTestKey), &stubMultiRAGClient{}, recorder)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", "t1")
		c.Set("user_id", "user-1")
		c.Set("user_name", "Ada")
	})
	r.POST("/api/v1/admin/providers/:id/sync-multirag", h.SyncToMultiRAG)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/1/sync-multirag", strings.NewReader(`{"verifyOnly":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	require.EqualValues(t, 1, countAuditByAction(t, db, audit.ActionSyncMultirag))
	row := fetchAuditByAction(t, db, audit.ActionSyncMultirag)
	require.Equal(t, audit.CatProvider, row.Category)
	require.Equal(t, audit.TargetProvider, row.TargetType)
	require.Equal(t, "1", row.TargetID)
}
