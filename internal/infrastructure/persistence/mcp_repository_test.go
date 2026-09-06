package repository

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupMcpScopedRepoTestDB 起 sqlite 内存库（mcp_servers + agents +
// agent_mcp_servers 全模型迁移，供绑定反查测试使用；避免与
// mcp_repository_tenant_test.go 的 setupMcpRepoTestDB 命名冲突）。
func setupMcpScopedRepoTestDB(t *testing.T) *gorm.DB {
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

// issue #123：反查按租户切分——own 只含请求租户名单（409 载荷），
// foreign 仅记他租户绑定事实，绝不携带他租户身份。
func TestMcpRepository_GetMcpBindingsScoped_TenantSplit(t *testing.T) {
	db := setupMcpScopedRepoTestDB(t)
	repo := NewMcpRepositoryWithDB(db)

	alpha := &agent.AgentConfig{Name: "alpha", TenantID: "org-a"}
	beta := &agent.AgentConfig{Name: "beta", TenantID: "org-b"}
	require.NoError(t, db.Create(alpha).Error)
	require.NoError(t, db.Create(beta).Error)
	srv := &mcp.McpServer{Name: "fs", TenantID: "org-a", TransportType: "sse", URL: "https://mcp.example.com/sse"}
	require.NoError(t, db.Create(srv).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: alpha.ID, McpServerID: srv.ID}).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: beta.ID, McpServerID: srv.ID}).Error)

	own, foreign, err := repo.GetMcpBindingsScoped("org-a", srv.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, own)
	require.True(t, foreign)

	srv2 := &mcp.McpServer{Name: "fs2", TenantID: "org-a", TransportType: "sse", URL: "https://mcp.example.com/sse2"}
	require.NoError(t, db.Create(srv2).Error)
	own2, foreign2, err := repo.GetMcpBindingsScoped("org-a", srv2.ID)
	require.NoError(t, err)
	require.Empty(t, own2)
	require.False(t, foreign2)
}
