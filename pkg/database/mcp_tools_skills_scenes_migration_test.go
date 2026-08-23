package database

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/scene"
	"control-panel/internal/domain/skill"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupLegacyBackfillDB 建四表 + agents（AutoMigrate 现行模型，天然无 uk_name
// 旧索引，即生产踩坑的 legacy 形态），并守护包级 DB/backfillTenantID 变量。
func setupLegacyBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&agent.Tool{}, &agent.AgentConfig{}, &skill.Skill{}, &mcp.McpServer{}, &scene.Scene{},
	))
	oldDB, oldBackfill := DB, backfillTenantID
	t.Cleanup(func() { DB, backfillTenantID = oldDB, oldBackfill })
	DB = db
	return db
}

func seedToolRow(t *testing.T, db *gorm.DB, name, tenantID string) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO tools (name, tenant_id, created_at, updated_at) VALUES (?, ?, '2026-01-01', '2026-01-01')", name, tenantID).Error)
}

func seedMcpRow(t *testing.T, db *gorm.DB, name string, isBuiltin bool) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO mcp_servers (name, tenant_id, title, transport_type, is_builtin, created_at, updated_at) VALUES (?, '', 'T', 'sse', ?, '2026-01-01', '2026-01-01')", name, isBuiltin).Error)
}

func seedSkillRow(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO skills (name, tenant_id, title, created_at, updated_at) VALUES (?, '', 'T', '2026-01-01', '2026-01-01')", name).Error)
}

func seedAgentRow(t *testing.T, db *gorm.DB, name, tenantID string) uint64 {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO agents (name, tenant_id, content_hash, system_prompt, created_at, updated_at) VALUES (?, ?, 'h', 'p', '2026-01-01', '2026-01-01')", name, tenantID).Error)
	var id uint64
	require.NoError(t, db.Raw("SELECT id FROM agents WHERE name = ?", name).Scan(&id).Error)
	return id
}

func seedSceneRow(t *testing.T, db *gorm.DB, name string, agentID uint64) {
	t.Helper()
	require.NoError(t, db.Exec("INSERT INTO scenes (name, tenant_id, agent_id, title, prompt, enabled, created_at, updated_at) VALUES (?, '', ?, 'T', 'p', 1, '2026-01-01', '2026-01-01')", name, agentID).Error)
}

type tenantRow struct {
	Name     string
	TenantID string
}

func tenantIDsByTable(t *testing.T, db *gorm.DB, table string) map[string]string {
	t.Helper()
	rows := []tenantRow{}
	require.NoError(t, db.Raw("SELECT name, tenant_id FROM `"+table+"` ORDER BY name").Scan(&rows).Error)
	out := map[string]string{}
	for _, r := range rows {
		out[r.Name] = r.TenantID
	}
	return out
}

// TestMigrateMcpToolsSkillsScenes_LegacyNoIndexBackfilled legacy 无 uk_name 索引
// 形态：四表存在 tenant_id=” 遗留行时必须触发回填（生产实锤：旧判据静默跳过）。
func TestMigrateMcpToolsSkillsScenes_LegacyNoIndexBackfilled(t *testing.T) {
	db := setupLegacyBackfillDB(t)
	// tools：预设名 + 自建名混合
	seedToolRow(t, db, "Skill", "")    // 预设（seedAlways）
	seedToolRow(t, db, "Bash", "")     // 预设（SeedIfEmpty）
	seedToolRow(t, db, "WebFetch", "") // 新预设
	seedToolRow(t, db, "my-tool", "")  // 自建
	// mcp_servers：内置 + 非内置
	seedMcpRow(t, db, "builtin-mcp", true)
	seedMcpRow(t, db, "my-mcp", false)
	// skills：纯自建
	seedSkillRow(t, db, "my-skill")
	// scenes：归属 zerone agent
	agentID := seedAgentRow(t, db, "my-agent", "zerone")
	seedSceneRow(t, db, "my-scene", agentID)
	backfillTenantID = "zerone"

	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())

	require.Equal(t, map[string]string{
		"Skill": "", "Bash": "", "WebFetch": "", "my-tool": "zerone",
	}, tenantIDsByTable(t, db, "tools"), "预设行归零共享，自建行归回填租户")
	require.Equal(t, map[string]string{
		"builtin-mcp": "", "my-mcp": "zerone",
	}, tenantIDsByTable(t, db, "mcp_servers"), "内置 mcp 归零共享，自建归回填租户")
	require.Equal(t, map[string]string{"my-skill": "zerone"}, tenantIDsByTable(t, db, "skills"))
	require.Equal(t, map[string]string{"my-scene": "zerone"}, tenantIDsByTable(t, db, "scenes"), "scenes 按 agents 主表回填")
}

// TestMigrateMcpToolsSkillsScenes_LegacyBackfillIdempotent 迁移后重跑状态不变。
func TestMigrateMcpToolsSkillsScenes_LegacyBackfillIdempotent(t *testing.T) {
	db := setupLegacyBackfillDB(t)
	seedToolRow(t, db, "my-tool", "")
	seedSkillRow(t, db, "my-skill")
	agentID := seedAgentRow(t, db, "my-agent", "zerone")
	seedSceneRow(t, db, "my-scene", agentID)
	seedMcpRow(t, db, "my-mcp", false)
	backfillTenantID = "zerone"

	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())

	snapshot := map[string]map[string]string{}
	for _, table := range []string{"tools", "skills", "scenes", "mcp_servers"} {
		snapshot[table] = tenantIDsByTable(t, db, table)
	}

	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())
	for _, table := range []string{"tools", "skills", "scenes", "mcp_servers"} {
		require.Equal(t, snapshot[table], tenantIDsByTable(t, db, table), "%s 二次迁移应 no-op", table)
	}
}

// TestMigrateMcpToolsSkillsScenes_NewInstallNoOp 新装库：backfill 为空（无法
// 推断）、tools 只有预设 ” 行、skills/scenes 无行 → no-op 不报错。
func TestMigrateMcpToolsSkillsScenes_NewInstallNoOp(t *testing.T) {
	db := setupLegacyBackfillDB(t)
	seedToolRow(t, db, "Skill", "")
	seedToolRow(t, db, "Bash", "")
	backfillTenantID = ""

	require.NoError(t, migrateMcpToolsSkillsScenesTenantID())
	require.Equal(t, map[string]string{"Skill": "", "Bash": ""}, tenantIDsByTable(t, db, "tools"))
}

// TestPresetToolNames_ExpandedTo18 预设清单 18 个且含 9 个新名字（防回退）。
func TestPresetToolNames_ExpandedTo18(t *testing.T) {
	require.Len(t, agent.PresetToolNames, 18)
	for _, name := range []string{
		"WebFetch", "WebSearch", "AskUserQuestion",
		"CronCreate", "CronDelete", "CronList",
		"Config", "TodoWrite", "FindTool",
	} {
		require.Contains(t, agent.PresetToolNames, name)
	}
}
