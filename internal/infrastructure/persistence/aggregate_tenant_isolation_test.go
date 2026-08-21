package repository

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedSameNameAgents 造两个租户各一个同名 agent，返回 (orgA的agent, orgB的agent)。
func seedSameNameAgents(t *testing.T, db *gorm.DB, name string) (a, b *agent.AgentConfig) {
	t.Helper()
	a = &agent.AgentConfig{Name: name, TenantID: "org-a"}
	b = &agent.AgentConfig{Name: name, TenantID: "org-b"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	return a, b
}

func TestToolRepository_GetAllAgentTools_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewToolRepository()

	// 两租户同名 agent，各挂不同 tool（tool 为平台级资源，可同名）
	a, b := seedSameNameAgents(t, db, "coder")
	require.NoError(t, db.Exec(`INSERT INTO tools (name) VALUES ('tool-a'), ('tool-b')`).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: a.ID, ToolID: 1}).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: b.ID, ToolID: 2}).Error)

	// 同名 agent 跨租户时，按 name 聚合的 map 不能混入他租户的绑定
	aMap, err := repo.GetAllAgentTools("org-a")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"tool-a"}}, aMap)

	bMap, err := repo.GetAllAgentTools("org-b")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"tool-b"}}, bMap)
}

func TestSkillRepository_GetAllAgentSkills_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewSkillRepository()

	a, b := seedSameNameAgents(t, db, "coder")
	require.NoError(t, db.Exec(`INSERT INTO skills (name) VALUES ('skill-a'), ('skill-b')`).Error)
	require.NoError(t, db.Create(&agent.AgentSkill{AgentID: a.ID, SkillID: 1}).Error)
	require.NoError(t, db.Create(&agent.AgentSkill{AgentID: b.ID, SkillID: 2}).Error)

	aMap, err := repo.GetAllAgentSkills("org-a")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"skill-a"}}, aMap)

	bMap, err := repo.GetAllAgentSkills("org-b")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"skill-b"}}, bMap)
}

func TestMcpRepository_GetAllAgentMcpNames_TenantIsolation(t *testing.T) {
	db := setupAgentRepoTestDB(t)
	repo := NewMcpRepository()

	a, b := seedSameNameAgents(t, db, "coder")
	require.NoError(t, db.Exec(`INSERT INTO mcp_servers (name) VALUES ('mcp-a'), ('mcp-b')`).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: a.ID, McpServerID: 1}).Error)
	require.NoError(t, db.Create(&mcp.AgentMcpServer{AgentID: b.ID, McpServerID: 2}).Error)

	aMap, err := repo.GetAllAgentMcpNames("org-a")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"mcp-a"}}, aMap)

	bMap, err := repo.GetAllAgentMcpNames("org-b")
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"coder": {"mcp-b"}}, bMap)
}
