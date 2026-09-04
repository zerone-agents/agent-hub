package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMcpServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&mcp.McpServer{}, &agent.AgentConfig{}, &mcp.AgentMcpServer{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMcpService_Delete_BlocksBound(t *testing.T) {
	db := setupMcpServiceTestDB(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "acme", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, db.Create(srv).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: a.ID, McpServerID: srv.ID}).Error)

	svc := &McpService{repo: repository.NewMcpRepositoryWithDB(db)}
	err := svc.Delete("acme", "fs")
	var inUse *agent.McpInUseError
	require.ErrorAs(t, err, &inUse)
	require.Equal(t, []string{"bot"}, inUse.Agents)
	require.False(t, inUse.Foreign)
	var cnt int64
	require.NoError(t, db.Model(&mcp.McpServer{}).Where("name = ?", "fs").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)
}

func TestMcpService_Delete_BlocksForeignOnly(t *testing.T) {
	db := setupMcpServiceTestDB(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "acme", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, db.Create(srv).Error)
	fb := &agent.AgentConfig{Name: "sneaky", TenantID: "other", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(fb).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: fb.ID, McpServerID: srv.ID}).Error)

	svc := &McpService{repo: repository.NewMcpRepositoryWithDB(db)}
	err := svc.Delete("acme", "fs")
	var inUse *agent.McpInUseError
	require.ErrorAs(t, err, &inUse)
	require.Empty(t, inUse.Agents)
	require.True(t, inUse.Foreign)
}

func TestMcpService_Delete_UnboundOK(t *testing.T) {
	db := setupMcpServiceTestDB(t)
	srv := &mcp.McpServer{Name: "fs", TenantID: "acme", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, db.Create(srv).Error)

	svc := &McpService{repo: repository.NewMcpRepositoryWithDB(db)}
	require.NoError(t, svc.Delete("acme", "fs"))
	var cnt int64
	require.NoError(t, db.Model(&mcp.McpServer{}).Where("name = ?", "fs").Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
}

func TestMcpService_Delete_BuiltinStillBlocked(t *testing.T) {
	db := setupMcpServiceTestDB(t)
	srv := &mcp.McpServer{Name: "knowledge", TenantID: "", TransportType: "sse", URL: "", IsBuiltin: true}
	require.NoError(t, db.Create(srv).Error)

	svc := &McpService{repo: repository.NewMcpRepositoryWithDB(db)}
	err := svc.Delete("acme", "knowledge")
	require.Contains(t, err.Error(), "是内置服务，不可删除")
}
