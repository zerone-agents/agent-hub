package database

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupToolsSourceDB 建新 schema（AutoMigrate 已含 source 列，default 'custom'）。
func setupToolsSourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}))
	return db
}

// runToolsSourceMigration 守护包级 DB 变量后执行 migrateToolsSource（模式同
// runAutoMigrateWithRestore，但只跑单个 migrate 函数）。
func runToolsSourceMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	old := DB
	t.Cleanup(func() { DB = old })
	DB = db
	require.NoError(t, migrateToolsSource())
}

func TestMigrateToolsSource_SharedPresetsBecomeBuiltin(t *testing.T) {
	db := setupToolsSourceDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, title, source, is_default, created_at, updated_at) VALUES
		('Bash', '', '执行命令', 'custom', false, datetime('now'), datetime('now')),
		('Skill', '', '技能加载', 'custom', false, datetime('now'), datetime('now')),
		('MyTool', 'acme', '自定义工具', 'custom', false, datetime('now'), datetime('now')),
		('Bash', 'acme', '租户与预设同名行', 'custom', false, datetime('now'), datetime('now'))`).Error)

	runToolsSourceMigration(t, db)

	var sharedBash agent.Tool
	require.NoError(t, db.Where("tenant_id = '' AND name = 'Bash'").First(&sharedBash).Error)
	require.Equal(t, agent.ToolSourceBuiltin, sharedBash.Source)

	var mine agent.Tool
	require.NoError(t, db.Where("tenant_id = 'acme' AND name = 'MyTool'").First(&mine).Error)
	require.Equal(t, agent.ToolSourceCustom, mine.Source)

	var tenantBash agent.Tool
	require.NoError(t, db.Where("tenant_id = 'acme' AND name = 'Bash'").First(&tenantBash).Error)
	require.Equal(t, agent.ToolSourceCustom, tenantBash.Source, "租户行即使与预设同名也保持 custom")
}

func TestMigrateToolsSource_CustomForcesIsDefaultFalse_KeepsBindings(t *testing.T) {
	db := setupToolsSourceDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, source, is_default, created_at, updated_at) VALUES
		('MyDefault', 'acme', 'custom', true, datetime('now'), datetime('now'))`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (name, tenant_id, content_hash, system_prompt, created_at, updated_at)
		VALUES ('bot', 'acme', 'h', 'p', datetime('now'), datetime('now'))`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agent_tools (agent_id, tool_id, created_at)
		SELECT a.id, t.id, datetime('now') FROM agents a, tools t WHERE a.name='bot' AND t.name='MyDefault'`).Error)

	runToolsSourceMigration(t, db)

	var tool agent.Tool
	require.NoError(t, db.Where("name = 'MyDefault'").First(&tool).Error)
	require.Equal(t, agent.ToolSourceCustom, tool.Source)
	require.False(t, tool.IsDefault, "custom 行必须强制 is_default=false")
	var count int64
	require.NoError(t, db.Table("agent_tools").Count(&count).Error)
	require.EqualValues(t, 1, count, "既有 agent_tools 关联必须保留")
}

func TestMigrateToolsSource_IdempotentAndFreshDB(t *testing.T) {
	db := setupToolsSourceDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name, tenant_id, source, created_at, updated_at) VALUES
		('Bash', '', 'builtin', datetime('now'), datetime('now')),
		('MyTool', 'acme', 'custom', datetime('now'), datetime('now'))`).Error)
	runToolsSourceMigration(t, db) // 第一次
	runToolsSourceMigration(t, db) // 第二次：幂等
	var n int64
	require.NoError(t, db.Model(&agent.Tool{}).Where("source NOT IN ?", []string{"builtin", "custom"}).Count(&n).Error)
	require.EqualValues(t, 0, n)
}
