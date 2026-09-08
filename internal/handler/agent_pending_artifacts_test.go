package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPendingArtifactTestDB builds an in-memory sqlite DB with every table the
// agent read paths (ListAdmin/Get/GetDeployment) touch, and seeds:
//   - "plain":   an agent with NO deployment snapshot (未部署)
//   - "general": an agent whose snapshot recorded tool "calc" with the old
//     hash while the current binding carries a new hash (待更新)
func setupPendingArtifactTestDB(t *testing.T) (*gorm.DB, *agent.AgentConfig, *agent.AgentConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&agent.AgentConfig{}, &agent.AgentSubagent{},
		&agent.Tool{}, &agent.AgentTool{},
		&skill.Skill{}, &agent.AgentSkill{},
		&mcp.McpServer{}, &mcp.AgentMcpServer{},
		&agent.AgentKnowledgeDataset{},
		&agent.DeploymentSnapshot{},
	))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	plain := &agent.AgentConfig{Name: "plain", TenantID: chatTestTenant, SystemPrompt: "p", ContentHash: "h-plain"}
	general := &agent.AgentConfig{Name: "general", TenantID: chatTestTenant, SystemPrompt: "p", ContentHash: "h-general"}
	require.NoError(t, db.Create(plain).Error)
	require.NoError(t, db.Create(general).Error)

	// custom && ready tool（四字段齐备 → ToolArtifactReady）挂载到 general
	tool := &agent.Tool{
		Name: "calc", TenantID: chatTestTenant, Source: agent.ToolSourceCustom,
		FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "newhash", FileSize: 10,
	}
	require.NoError(t, db.Create(tool).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: general.ID, ToolID: tool.ID}).Error)

	// 快照记录部署时的旧哈希 → calc 哈希不一致 → pending
	snapRepo := repository.NewDeploymentSnapshotRepositoryWithDB(db)
	require.NoError(t, snapRepo.Upsert(context.Background(), &agent.DeploymentSnapshot{
		AgentID: general.ID, TenantID: chatTestTenant, DeployedAt: time.Now(),
		ToolHashes: map[string]string{"calc": "oldhash"},
	}))
	return db, plain, general
}

// setupPendingArtifactRouter wires the production AgentHandler against the
// real services + seeded DB, with the tenant seeded exactly like the JWT
// middleware does (chat_handler_test 同款)。deployer client 指向不可达地址，
// GetStatus 因此走 not_found 分支（HTTP 200）——无需真实 deployer 即可验证
// GetDeployment 的 pendingArtifactUpdates 注入。
func setupPendingArtifactRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, _, _ := setupPendingArtifactTestDB(t)

	h := NewAgentHandler(
		services.NewAgentService("", ""),
		services.NewAgentDeployerService(services.AgentDeployerConfig{
			Client:   deployer.NewClient("http://127.0.0.1:1", "unused"),
			AuthMode: services.ModeBuiltin,
		}),
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.GET("/api/v1/admin/agents", h.ListAdmin)
	r.GET("/api/v1/agents/:name", h.Get)
	r.GET("/api/v1/admin/agents/:name/deploy", h.GetDeployment)
	return r, db
}

// 响应体断言辅助：data 下的 agent 条目需携带 pendingArtifactUpdates。
type pendingArtifactsAgentEntry struct {
	Name                   string                           `json:"name"`
	PendingArtifactUpdates *services.PendingArtifactUpdates `json:"pendingArtifactUpdates"`
}

// TestListAdmin_PendingArtifactUpdates (issue #86 Task 5 用例 A+B)：
// 无快照（未部署）agent → pendingArtifactUpdates 为 null；快照与当前绑定
// 存在哈希差异 → {"tools":["calc"],"skills":[]}。
func TestListAdmin_PendingArtifactUpdates(t *testing.T) {
	r, _ := setupPendingArtifactRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, `"pendingArtifactUpdates":null`)
	require.Contains(t, body, `"pendingArtifactUpdates":{"tools":["calc"],"skills":[]}`)

	var parsed struct {
		Data struct {
			Agents []pendingArtifactsAgentEntry `json:"agents"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &parsed))

	find := func(name string) *services.PendingArtifactUpdates {
		t.Helper()
		for _, a := range parsed.Data.Agents {
			if a.Name == name {
				return a.PendingArtifactUpdates
			}
		}
		t.Fatalf("agent %q not found in admin list response", name)
		return nil
	}

	require.Nil(t, find("plain"), "无快照（未部署）agent 的 pendingArtifactUpdates 应为 null")
	pending := find("general")
	require.NotNil(t, pending, "存在哈希差异的 agent 必须返回 pendingArtifactUpdates")
	require.Equal(t, []string{"calc"}, pending.Tools)
	require.Empty(t, pending.Skills)
}

// TestGetAgent_PendingArtifactUpdates (issue #86 Task 5)：Get 单查路径
// 注入同一字段，null 与差异两种形态齐备。
func TestGetAgent_PendingArtifactUpdates(t *testing.T) {
	r, _ := setupPendingArtifactRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/general", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"pendingArtifactUpdates":{"tools":["calc"],"skills":[]}`)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/plain", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"pendingArtifactUpdates":null`)
}

// TestGetDeployment_PendingArtifactUpdates (issue #86 Task 5)：
// 部署状态端点按 name 注入（ComputePendingArtifactsByName），同样覆盖
// null 与差异两种形态（deployer 不可达时 GetStatus 走 not_found 分支，200）。
func TestGetDeployment_PendingArtifactUpdates(t *testing.T) {
	r, _ := setupPendingArtifactRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/general/deploy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"pendingArtifactUpdates":{"tools":["calc"],"skills":[]}`)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/plain/deploy", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"pendingArtifactUpdates":null`)
}

// TestListAdmin_PendingArtifacts_ComputeFailureFailsOpen (issue #86 Task 5)：
// ComputePendingArtifacts 不可用（快照表被删 → 非 RecordNotFound 错误）时，
// 列表仍返回 200、字段回落 null，失败只进服务端日志——pending 是提示非授权，
// 绝不阻塞列表。
func TestListAdmin_PendingArtifacts_ComputeFailureFailsOpen(t *testing.T) {
	r, db := setupPendingArtifactRouter(t)
	require.NoError(t, db.Migrator().DropTable(&agent.DeploymentSnapshot{}))

	var logBuf bytes.Buffer
	oldOut := log.Writer()
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(oldOut) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "pending 计算失败不得阻塞列表")
	require.Contains(t, rec.Body.String(), `"pendingArtifactUpdates":null`)
	require.Contains(t, logBuf.String(), "compute pending artifacts", "失败原因必须进服务端日志")
}
