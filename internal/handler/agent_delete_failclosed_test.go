package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	"control-panel/pkg/database"
)

// setupDeleteRouter 建一个含单个 agent "victim" 的内存库 + 指向 mock deployer
// 的生产 Delete 路由（tenant 注入方式与 JWT 中间件一致，pending artifacts 测试同款）。
func setupDeleteRouter(t *testing.T, deployerHandler http.HandlerFunc) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&agent.AgentConfig{}, &agent.AgentSubagent{},
		&agent.Tool{}, &agent.AgentTool{},
		&skill.Skill{}, &agent.AgentSkill{},
		&mcp.McpServer{}, &mcp.AgentMcpServer{},
		&agent.AgentKnowledgeDataset{}, &agent.DeploymentSnapshot{},
	))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	require.NoError(t, db.Create(&agent.AgentConfig{Name: "victim", TenantID: chatTestTenant, SystemPrompt: "p", ContentHash: "h"}).Error)

	deployerSrv := httptest.NewServer(deployerHandler)
	t.Cleanup(deployerSrv.Close)

	h := NewAgentHandler(
		services.NewAgentService("", ""),
		services.NewAgentDeployerService(services.AgentDeployerConfig{
			Client:   deployer.NewClient(deployerSrv.URL, "unused"),
			AuthMode: services.ModeBuiltin,
		}),
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.DELETE("/api/v1/admin/agents/:name", h.Delete)
	return r, db
}

func doDelete(r *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/agents/victim", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func agentExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("name = ?", "victim").Count(&count).Error)
	return count > 0
}

// deployer 5xx（状态不可确认）→ 502，且不删除配置。
func TestDelete_DeployerUnavailable_BlocksDelete(t *testing.T) {
	r, db := setupDeleteRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"error":"boom"}`))
	})

	rec := doDelete(r)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "无法确认部署状态")
	require.True(t, agentExists(t, db), "状态不可确认时不得删除 Agent 配置")
}

// deployer 404（确认未部署）→ 放行删除。
func TestDelete_Deployer404_AllowsDelete(t *testing.T) {
	r, db := setupDeleteRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"success":false,"error":"agent not found"}`))
	})

	rec := doDelete(r)
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, agentExists(t, db), "确认未部署后应完成删除")
}

// 活跃部署（running）→ 409，且不删除配置（既有行为回归保护）。
func TestDelete_ActiveDeployment_Conflict(t *testing.T) {
	r, db := setupDeleteRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"agentName":"victim","status":"running","health":"healthy","hostPort":3000}}`))
	})

	rec := doDelete(r)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.True(t, agentExists(t, db), "有活跃部署时不得删除 Agent 配置")
}
