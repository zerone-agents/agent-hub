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
	oldDB := DB
	t.Cleanup(func() { DB = oldDB })
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

func TestMigrateMemoryToolsBuiltin_SameNameMultiRowNoIndexCollision(t *testing.T) {
	db := setupMemoryToolsMigrationDB(t)

	// 自建多组织部署形态：两租户同名 custom 行在 uk(tenant_id,name) 下合法共存，
	// 再加一条共享域同名行——朴素归一会同时写 tenant_id='' 撞唯一索引。
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "acme", Source: agent.ToolSourceCustom, Title: "租户A的记忆",
	}).Error)
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "beta", Source: agent.ToolSourceCustom, Title: "租户B的记忆",
	}).Error)
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "", Source: agent.ToolSourceCustom, Title: "共享旧行",
	}).Error)

	// 不返回错误（不撞 uk_tools_tenant_name）。
	require.NoError(t, migrateMemoryToolsBuiltin())

	// 共享域恰好一行、source=builtin。
	var shared []agent.Tool
	require.NoError(t, db.Where("name = ? AND tenant_id = ''", "Memory").Find(&shared).Error)
	require.Len(t, shared, 1)
	require.Equal(t, agent.ToolSourceBuiltin, shared[0].Source)

	// 其余行原地保留为租户 custom（不被劫持、不删除——agent_tools FK RESTRICT）。
	var acme, beta agent.Tool
	require.NoError(t, db.Where("name = ? AND tenant_id = ?", "Memory", "acme").First(&acme).Error)
	require.NoError(t, db.Where("name = ? AND tenant_id = ?", "Memory", "beta").First(&beta).Error)
	if shared[0].ID != acme.ID {
		require.Equal(t, agent.ToolSourceCustom, acme.Source)
	}
	if shared[0].ID != beta.ID {
		require.Equal(t, agent.ToolSourceCustom, beta.Source)
	}
}

func TestMigrateMemoryToolsBuiltin_NormalizesMetadata(t *testing.T) {
	db := setupMemoryToolsMigrationDB(t)

	require.NoError(t, db.Create(&agent.Tool{
		Name: "Memory", TenantID: "acme", Source: agent.ToolSourceCustom,
		Title: "旧标题", Description: "旧描述", // DescriptionEn 缺失
	}).Error)

	require.NoError(t, migrateMemoryToolsBuiltin())

	var memory agent.Tool
	require.NoError(t, db.Where("name = ?", "Memory").First(&memory).Error)
	require.Equal(t, "长期记忆", memory.Title)
	require.NotEmpty(t, memory.Description)
	require.NotEmpty(t, memory.DescriptionEn)
}

// TestPresetToolNamesV1_FrozenForLegacyMigration v1 冻结名单是 18 个旧预设、
// 不含新扩名字——防止有人「顺手同步」回活名单，重新引入提前归零撞索引风险。
func TestPresetToolNamesV1_FrozenForLegacyMigration(t *testing.T) {
	require.Len(t, presetToolNamesV1, 18)
	require.NotContains(t, presetToolNamesV1, "Memory")
	require.NotContains(t, presetToolNamesV1, "MemorySearch")
	require.Contains(t, presetToolNamesV1, "FindTool")
}
