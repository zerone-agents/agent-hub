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

// setupToolServiceTestDB spins up an in-memory sqlite DB with the tools table
// migrated, and points the package-global database.DB at it for the duration
// of the test. ToolRepository reads database.GetDB(), so this is sufficient.
func setupToolServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

func TestSeedBuiltins_CreatesSkillTaskMultiTask(t *testing.T) {
	setupToolServiceTestDB(t)
	svc := NewToolService(nil)

	require.NoError(t, svc.SeedBuiltins())

	toolRepo := repository.NewToolRepository()
	for _, name := range []string{"Skill", "Task", "MultiTask"} {
		t.Run(name, func(t *testing.T) {
			got, err := toolRepo.GetByName("", name)
			require.NoError(t, err)
			assert.Equal(t, name, got.Name)
			assert.False(t, got.IsDefault, "%s must be IsDefault=false so it does not auto-attach to every agent", name)
			assert.NotEmpty(t, got.Title, "%s must have a non-empty Title", name)
			assert.NotEmpty(t, got.Description, "%s must have a non-empty Description", name)
		})
	}
}

func TestSeedBuiltins_IsIdempotent(t *testing.T) {
	setupToolServiceTestDB(t)
	svc := NewToolService(nil)

	require.NoError(t, svc.SeedBuiltins())
	require.NoError(t, svc.SeedBuiltins()) // second run must not fail or duplicate

	toolRepo := repository.NewToolRepository()
	tools, err := toolRepo.ListAll("")
	require.NoError(t, err)
	assert.Len(t, tools, 3, "expected exactly Skill/Task/MultiTask after two SeedBuiltins runs")
}

// TestSeedBuiltins_PreservesExistingSkillWhenAddingTasks covers the production
// upgrade path: an existing deployment already has the "Skill" row (created by
// the previous version of SeedBuiltins); after upgrade, the new SeedBuiltins
// must leave Skill untouched and add Task + MultiTask alongside it.
func TestSeedBuiltins_PreservesExistingSkillWhenAddingTasks(t *testing.T) {
	db := setupToolServiceTestDB(t)
	// Simulate the pre-upgrade state: only Skill exists, with a custom title
	// that we can verify is NOT overwritten.
	require.NoError(t, db.Create(&agent.Tool{
		Name:        "Skill",
		Title:       "PRE-EXISTING TITLE",
		Description: "PRE-EXISTING DESC",
		IsDefault:   false,
	}).Error)

	svc := NewToolService(nil)
	require.NoError(t, svc.SeedBuiltins())

	toolRepo := repository.NewToolRepository()
	skill, err := toolRepo.GetByName("", "Skill")
	require.NoError(t, err)
	assert.Equal(t, "PRE-EXISTING TITLE", skill.Title, "existing Skill row must not be overwritten")

	for _, name := range []string{"Task", "MultiTask"} {
		got, err := toolRepo.GetByName("", name)
		require.NoError(t, err)
		assert.Equal(t, name, got.Name)
	}
}

// TestBackfillBuiltinDescriptionEn_FillsEmptyOnly 覆盖 issue #93 存量路径：
// 已存在的内置行 description_en 为空时回填预设英文；非空行不动；
// 用户自定义 title/description 绝不被覆盖；二次执行幂等。
func TestBackfillBuiltinDescriptionEn_FillsEmptyOnly(t *testing.T) {
	db := setupToolServiceTestDB(t)
	svc := NewToolService(nil)

	// 存量形态：Skill 行已存在（含用户改动过的自定义 title），description_en 为空
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Skill", Title: "我的技能", Description: "自定义中", Source: agent.ToolSourceBuiltin,
	}).Error)
	// Bash 行已存在且已有英文（模拟已补齐）——回填不得动它
	require.NoError(t, db.Create(&agent.Tool{
		Name: "Bash", Title: "执行命令", Description: "中文", DescriptionEn: "existing-en", Source: agent.ToolSourceBuiltin,
	}).Error)

	require.NoError(t, svc.BackfillBuiltinDescriptionEn())
	require.NoError(t, svc.BackfillBuiltinDescriptionEn()) // 幂等

	repo := repository.NewToolRepository()
	skill, err := repo.GetByName("", "Skill")
	require.NoError(t, err)
	assert.Equal(t, "我的技能", skill.Title, "回填不得覆盖用户自定义 title")
	assert.Equal(t, "自定义中", skill.Description, "回填不得覆盖 description")
	if skill.DescriptionEn == "" {
		t.Fatal("Skill 的 description_en 必须被回填")
	}

	// Bash 行英文已存在 → 保持原值
	bash, err := repo.GetByName("", "Bash")
	require.NoError(t, err)
	assert.Equal(t, "existing-en", bash.DescriptionEn, "非空 description_en 不得被覆盖")
}

// TestBackfillBuiltinDescriptionEn_CoversAllPresets 断言 18 条预设全部有英文。
func TestBackfillBuiltinDescriptionEn_CoversAllPresets(t *testing.T) {
	db := setupToolServiceTestDB(t)
	svc := NewToolService(nil)
	for _, p := range presetToolSpecs { // 与实现同包，直接引用
		require.NoError(t, db.Create(&agent.Tool{Name: p.tool.Name, Title: "t", Description: "d", Source: agent.ToolSourceBuiltin}).Error)
	}
	require.NoError(t, svc.BackfillBuiltinDescriptionEn())
	repo := repository.NewToolRepository()
	for _, p := range presetToolSpecs {
		got, err := repo.GetByName("", p.tool.Name)
		require.NoError(t, err)
		if got.DescriptionEn == "" {
			t.Errorf("%s 缺英文描述", p.tool.Name)
		}
	}
}
