package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupSubagentToolsTestDB spins up an in-memory DB with all tables touched by
// UpdateSubagents: agents, tools, agent_tools, agent_subagents. The tools table
// is pre-seeded with the three builtins so GetByName("Task") etc. succeed
// (mirrors production startup where SeedBuiltins runs first).
func setupSubagentToolsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Multiple domain models (AgentConfig.Name, Tool.Name, plus scene/mcp/skill
	// elsewhere) all declare `uniqueIndex:uk_name`. On SQLite the index
	// namespace is global, so any AutoMigrate that touches two of these tables
	// fails with "index uk_name already exists" (GORM also walks foreign-key
	// relations during AutoMigrate, so even migrating them one at a time can
	// trip the collision). To keep this test hermetic without touching the
	// shared production models, we create the four required tables via raw SQL
	// and use unique index names that don't collide. (Production uses Postgres,
	// where index names are per-table, so this is a test-only workaround.)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			content_hash VARCHAR(128) NOT NULL DEFAULT '',
			system_prompt TEXT NOT NULL DEFAULT '',
			permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto',
			max_turns INTEGER NOT NULL DEFAULT 50,
			title JSON,
			description JSON,
			icon VARCHAR(512) DEFAULT '',
			icon_name VARCHAR(64) DEFAULT '',
			icon_color VARCHAR(32) DEFAULT '',
			icon_bg_color VARCHAR(64) DEFAULT '',
			provider_id INTEGER,
			model_id VARCHAR(64) DEFAULT '',
			model_selection_id VARCHAR(128) DEFAULT '',
			field_overrides TEXT,
			source VARCHAR(16) NOT NULL DEFAULT 'remote',
			desktop_enabled INTEGER NOT NULL DEFAULT 0,
			mobile_enabled INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER DEFAULT 0,
			group_name VARCHAR(64) DEFAULT '',
			max_session_turns INTEGER,
			runtime_port INTEGER DEFAULT 0,
			deployment_status VARCHAR(32) DEFAULT '',
			deployed_at DATETIME,
			runtime_token TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX agents_uk_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			title VARCHAR(128) DEFAULT '',
			description TEXT,
			is_default INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX tools_uk_name ON tools(name)`,
		`CREATE TABLE agent_tools (
			agent_id INTEGER NOT NULL,
			tool_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, tool_id)
		)`,
		`CREATE TABLE agent_subagents (
			agent_id INTEGER NOT NULL,
			subagent_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, subagent_id)
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	for _, bt := range builtinTools {
		require.NoError(t, db.Create(&agent.Tool{
			Name: bt.Name, Title: bt.Title, Description: bt.Description, IsDefault: bt.IsDefault,
		}).Error)
	}
	return db
}

// agentToolNames returns the sorted names of tools bound to the given agent.
func agentToolNames(t *testing.T, agentID uint64) []string {
	t.Helper()
	toolRepo := repository.NewToolRepository()
	names, err := toolRepo.GetToolsByAgent(agentID)
	require.NoError(t, err)
	return names
}

func TestUpdateSubagents_AttachesTaskAndMultiTaskWhenBindingSubagent(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	// Create parent + one subagent
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child1"}))

	svc := NewAgentService("test-encryption-key")
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{"child1"}))

	parent, err := agentRepo.GetByName("default", "parent")
	require.NoError(t, err)
	assert.Contains(t, agentToolNames(t, parent.ID), "Task")
	assert.Contains(t, agentToolNames(t, parent.ID), "MultiTask")
}

func TestUpdateSubagents_AttachesTaskAndMultiTaskWhenBindingMultipleSubagents(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child1"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child2"}))

	svc := NewAgentService("test-encryption-key")
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{"child1", "child2"}))

	parent, err := agentRepo.GetByName("default", "parent")
	require.NoError(t, err)
	got := agentToolNames(t, parent.ID)
	assert.Contains(t, got, "Task")
	assert.Contains(t, got, "MultiTask")
	// Idempotency: still exactly one row each (EnsureAgentToolBinding counts first)
	assert.Equal(t, 1, countOccurrences(got, "Task"))
	assert.Equal(t, 1, countOccurrences(got, "MultiTask"))
}

func TestUpdateSubagents_RemovesTaskAndMultiTaskWhenClearingSubagents(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child1"}))

	svc := NewAgentService("test-encryption-key")
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{"child1"})) // attach
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{}))         // clear

	parent, err := agentRepo.GetByName("default", "parent")
	require.NoError(t, err)
	got := agentToolNames(t, parent.ID)
	assert.NotContains(t, got, "Task")
	assert.NotContains(t, got, "MultiTask")
}

func TestUpdateSubagents_ReattachesAfterManualRemoval(t *testing.T) {
	// Q3=A behavior: user manually removes Task, then re-saves subagent list
	// → Task is re-attached automatically.
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child1"}))

	svc := NewAgentService("test-encryption-key")
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{"child1"}))

	// Simulate manual removal via the repository (bypassing UpdateSubagents).
	parent, err := agentRepo.GetByName("default", "parent")
	require.NoError(t, err)
	toolRepo := repository.NewToolRepository()
	taskTool, err := toolRepo.GetByName("Task")
	require.NoError(t, err)
	require.NoError(t, agentRepo.RemoveAgentToolBinding(parent.ID, taskTool.ID))
	assert.NotContains(t, agentToolNames(t, parent.ID), "Task")

	// Re-saving subagent list (even unchanged) must re-attach.
	require.NoError(t, svc.UpdateSubagents("default", "parent", []string{"child1"}))
	assert.Contains(t, agentToolNames(t, parent.ID), "Task")
}

func TestUpdateSubagents_RejectsSelfReferenceAndDoesNotTouchBindings(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))

	svc := NewAgentService("test-encryption-key")
	err := svc.UpdateSubagents("default", "parent", []string{"parent"})
	require.Error(t, err)

	parent, err := agentRepo.GetByName("default", "parent")
	require.NoError(t, err)
	assert.Empty(t, agentToolNames(t, parent.ID), "no tool binding should exist after a rejected update")
}

func TestBackfillSubagentToolBindings_AttachesToAgentsWithSubagents(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()

	// Pre-upgrade state: agent has subagents but no Task/MultiTask binding.
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent1"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent2"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "lonely"}))
	p1, _ := agentRepo.GetByName("default", "parent1")
	p2, _ := agentRepo.GetByName("default", "parent2")
	c, _ := agentRepo.GetByName("default", "child")
	require.NoError(t, agentRepo.ReplaceSubagents(p1.ID, []uint64{c.ID}))
	require.NoError(t, agentRepo.ReplaceSubagents(p2.ID, []uint64{c.ID}))

	svc := NewToolService()
	require.NoError(t, svc.BackfillSubagentToolBindings())

	for _, name := range []string{"parent1", "parent2"} {
		a, err := agentRepo.GetByName("default", name)
		require.NoError(t, err)
		got := agentToolNames(t, a.ID)
		assert.Contains(t, got, "Task", "%s should have Task backfilled", name)
		assert.Contains(t, got, "MultiTask", "%s should have MultiTask backfilled", name)
	}

	// Lonely agent (no subagents) must not have been touched.
	lonely, err := agentRepo.GetByName("default", "lonely")
	require.NoError(t, err)
	assert.Empty(t, agentToolNames(t, lonely.ID))
}

func TestBackfillSubagentToolBindings_IsIdempotent(t *testing.T) {
	setupSubagentToolsTestDB(t)
	agentRepo := repository.NewAgentRepository()
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
	require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child"}))
	p, _ := agentRepo.GetByName("default", "parent")
	c, _ := agentRepo.GetByName("default", "child")
	require.NoError(t, agentRepo.ReplaceSubagents(p.ID, []uint64{c.ID}))

	svc := NewToolService()
	require.NoError(t, svc.BackfillSubagentToolBindings())
	require.NoError(t, svc.BackfillSubagentToolBindings())

	got := agentToolNames(t, p.ID)
	assert.Equal(t, 1, countOccurrences(got, "Task"))
	assert.Equal(t, 1, countOccurrences(got, "MultiTask"))
}

func TestBackfillSubagentToolBindings_NoAgents(t *testing.T) {
	setupSubagentToolsTestDB(t)
	svc := NewToolService()
	// Empty database — must not error.
	require.NoError(t, svc.BackfillSubagentToolBindings())
}

func countOccurrences(slice []string, target string) int {
	n := 0
	for _, s := range slice {
		if s == target {
			n++
		}
	}
	return n
}
