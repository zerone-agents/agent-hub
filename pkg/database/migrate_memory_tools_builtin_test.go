package database

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMemoryToolsMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&agent.Tool{}))
	return db
}

func TestMigrateMemoryToolsBuiltin_ConvertsCustomRows(t *testing.T) {
	db := setupMemoryToolsMigrationDB(t)

	// 生产行的两种形态：租户域 custom 行 + 共享域 custom 行。
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "acme", Source: agent.ToolSourceCustom,
		Title: "记忆管理", FileName: "memory.ts", FileURL: "tools/acme/memory.ts",
		FileHash: "abc123", FileSize: 42,
	}).Error)
	require.NoError(t, db.Create(&agent.Tool{
		Name: "MemorySearch", TenantID: "", Source: agent.ToolSourceCustom,
		Title: "记忆搜索",
	}).Error)

	require.NoError(t, migrateMemoryToolsBuiltin())

	var memory agent.Tool
	require.NoError(t, db.Where("name = ?", "Memory").First(&memory).Error)
	require.Equal(t, agent.ToolSourceBuiltin, memory.Source)
	require.Equal(t, "", memory.TenantID)
	require.Empty(t, memory.FileName)
	require.Empty(t, memory.FileURL)
	require.Empty(t, memory.FileHash)
	require.Zero(t, memory.FileSize)

	var search agent.Tool
	require.NoError(t, db.Where("name = ?", "MemorySearch").First(&search).Error)
	require.Equal(t, agent.ToolSourceBuiltin, search.Source)
	require.Equal(t, "", search.TenantID)

	// 每行只剩一条（归零共享而不是复制新行）。
	var count int64
	require.NoError(t, db.Model(&agent.Tool{}).Where("name = ?", "Memory").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestMigrateMemoryToolsBuiltin_LeavesSessionSearchAlone(t *testing.T) {
	db := setupMemoryToolsMigrationDB(t)

	require.NoError(t, db.Create(&agent.Tool{
		Name: "SessionSearch", TenantID: "acme", Source: agent.ToolSourceCustom,
		Title: "会话搜索",
	}).Error)
	// 已是 builtin 的行不被触碰。
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "", Source: agent.ToolSourceBuiltin,
	}).Error)

	require.NoError(t, migrateMemoryToolsBuiltin())

	var session agent.Tool
	require.NoError(t, db.Where("name = ?", "SessionSearch").First(&session).Error)
	require.Equal(t, agent.ToolSourceCustom, session.Source)
	require.Equal(t, "acme", session.TenantID)

	var memory agent.Tool
	require.NoError(t, db.Where("name = ?", "Memory").First(&memory).Error)
	require.Equal(t, agent.ToolSourceBuiltin, memory.Source)
}

func TestMigrateMemoryToolsBuiltin_Idempotent(t *testing.T) {
	db := setupMemoryToolsMigrationDB(t)

	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "acme", Source: agent.ToolSourceCustom,
	}).Error)

	require.NoError(t, migrateMemoryToolsBuiltin())
	require.NoError(t, migrateMemoryToolsBuiltin())

	var count int64
	require.NoError(t, db.Model(&agent.Tool{}).Where("name = ?", "Memory").Count(&count).Error)
	require.Equal(t, int64(1), count)
}
