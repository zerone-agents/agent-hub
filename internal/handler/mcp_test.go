package handler

import (
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

func setupMcpHandlerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcp.McpServer{}, &agent.AgentConfig{}, &mcp.AgentMcpServer{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	h := NewMcpHandler(services.NewMcpService("test-key"))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	r.DELETE("/api/v1/admin/mcps/:name", h.Delete)
	return r
}

func TestMcpHandler_DeleteConflict409(t *testing.T) {
	r := setupMcpHandlerRouter(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "tenant-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, database.GetDB().Create(srv).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "tenant-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	require.NoError(t, database.GetDB().Create(&mcp.AgentMcpServer{AgentID: a.ID, McpServerID: srv.ID}).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/mcps/fs", nil))
	require.Equal(t, http.StatusConflict, resp.Code)
	require.Contains(t, resp.Body.String(), `"agents":["bot"]`)
	require.Contains(t, resp.Body.String(), `"foreign":false`)
}

func TestMcpHandler_DeleteConflict409_CrossTenantNoLeak(t *testing.T) {
	r := setupMcpHandlerRouter(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "tenant-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, database.GetDB().Create(srv).Error)
	fb := &agent.AgentConfig{Name: "sneaky", TenantID: "tenant-b", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(fb).Error)
	require.NoError(t, database.GetDB().Create(&mcp.AgentMcpServer{AgentID: fb.ID, McpServerID: srv.ID}).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/mcps/fs", nil))
	require.Equal(t, http.StatusConflict, resp.Code)
	require.NotContains(t, resp.Body.String(), "sneaky")
	require.Contains(t, resp.Body.String(), `"foreign":true`)
}

func TestMcpHandler_DeleteUnboundOK(t *testing.T) {
	r := setupMcpHandlerRouter(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "tenant-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, database.GetDB().Create(srv).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/mcps/fs", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), "MCP 已删除")
}