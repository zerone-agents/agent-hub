package repository

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupToolCustomRepoDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}))
	return db
}

func TestGetToolRecordsByAgent_ReturnsFullRows(t *testing.T) {
	db := setupToolCustomRepoDB(t)
	custom := &agent.Tool{Name: "SayHello", TenantID: "acme", Source: agent.ToolSourceCustom,
		FileName: "say.ts", FileURL: "tools/acme/SayHello/abc.ts", FileHash: "abc", FileSize: 10}
	builtin := &agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}
	require.NoError(t, db.Create(custom).Error)
	require.NoError(t, db.Create(builtin).Error)
	acme := &agent.AgentConfig{Name: "bot", TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(acme).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: acme.ID, ToolID: custom.ID}).Error)
	require.NoError(t, db.Create(&agent.AgentTool{AgentID: acme.ID, ToolID: builtin.ID}).Error)

	repo := NewToolRepositoryWithDB(db)
	rows, err := repo.GetToolRecordsByAgent(acme.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	byName := map[string]*agent.Tool{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	require.Equal(t, "tools/acme/SayHello/abc.ts", byName["SayHello"].FileURL)
	require.Equal(t, agent.ToolSourceBuiltin, byName["Bash"].Source)
}

func TestGetAgentNamesByToolID_SortedNames(t *testing.T) {
	db := setupToolCustomRepoDB(t)
	tool := &agent.Tool{Name: "SayHello", TenantID: "acme", Source: agent.ToolSourceCustom}
	require.NoError(t, db.Create(tool).Error)
	for _, n := range []string{"zeta", "alpha", "mid"} {
		a := &agent.AgentConfig{Name: n, TenantID: "acme", ContentHash: "h", SystemPrompt: "p"}
		require.NoError(t, db.Create(a).Error)
		require.NoError(t, db.Create(&agent.AgentTool{AgentID: a.ID, ToolID: tool.ID}).Error)
	}
	otherTenant := &agent.AgentConfig{Name: "nope", TenantID: "other", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, db.Create(otherTenant).Error) // 未关联

	repo := NewToolRepositoryWithDB(db)
	names, err := repo.GetAgentNamesByToolID(tool.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "mid", "zeta"}, names)
}
