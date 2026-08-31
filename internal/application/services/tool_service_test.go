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
