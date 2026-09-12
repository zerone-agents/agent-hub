package services

import (
	"errors"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/personality"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPersonalityServiceTest(t *testing.T) (*PersonalityService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &personality.Template{}, &personality.Version{}))
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous })
	return NewPersonalityService(), db
}

func TestPersonalityService_SeedsPerTenant(t *testing.T) {
	service, _ := setupPersonalityServiceTest(t)

	rowsA, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, rowsA, 4)
	require.True(t, rowsA[0].IsBuiltin)
	require.NotEmpty(t, rowsA[0].Prompt)
	for _, row := range rowsA {
		require.Nil(t, row.BehaviorProfile, "新初始化的人格只能以提示词为行为来源")
	}

	rowsB, err := service.List("org-b")
	require.NoError(t, err)
	require.Len(t, rowsB, 4)

	rowsAAgain, err := service.List("org-a")
	require.NoError(t, err)
	require.Len(t, rowsAAgain, 4, "重复读取不能重复写入系统人格")
}

func TestPersonalityService_VersionsAndAgentSnapshot(t *testing.T) {
	service, db := setupPersonalityServiceTest(t)
	created, err := service.Create("org-a", &CreatePersonalityInput{
		Name: "crisis-negotiator", Title: "危机谈判者", Description: "先降温，再交换条件", Prompt: "原稿 v1",
	})
	require.NoError(t, err)
	require.Equal(t, 1, created.CurrentVersion)

	agentRow := &agent.AgentConfig{
		Name: "negotiator", TenantID: "org-a", ContentHash: "hash", SystemPrompt: "负责谈判",
		PersonalityTemplateName: created.Name, PersonalityTemplateVersion: 1, PersonalityPrompt: created.Prompt,
	}
	require.NoError(t, db.Create(agentRow).Error)

	nextPrompt := "原稿 v2：证据不足时先试探对方底线"
	nextTitle := "高级危机谈判者"
	updated, err := service.Update("org-a", created.Name, &UpdatePersonalityInput{
		Title: &nextTitle, Prompt: &nextPrompt, ChangeNote: "补充不确定情境",
	})
	require.NoError(t, err)
	require.Equal(t, 2, updated.CurrentVersion)
	require.Len(t, updated.Versions, 2)
	require.Equal(t, "补充不确定情境", updated.Versions[0].ChangeNote)

	var stored agent.AgentConfig
	require.NoError(t, db.First(&stored, agentRow.ID).Error)
	require.Equal(t, "原稿 v1", stored.PersonalityPrompt, "模板升级不能静默改写 Agent 快照")
	require.Equal(t, 1, stored.PersonalityTemplateVersion)

	got, err := service.Get("org-a", created.Name)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.UsageCount)
}

func TestPersonalityService_NewVersionDoesNotCopyLegacyBehaviorProfile(t *testing.T) {
	service, db := setupPersonalityServiceTest(t)
	profile := agent.DefaultBehaviorProfile()
	row := &personality.Template{
		TenantID: "org-a", Name: "legacy", Title: "旧人格", Prompt: "原稿 v1",
		BehaviorProfile: &profile, CurrentVersion: 1, Enabled: true,
	}
	version := &personality.Version{Version: 1, Prompt: row.Prompt, BehaviorProfile: &profile}
	require.NoError(t, service.repo.Create("org-a", row, version))

	nextPrompt := "原稿 v2"
	updated, err := service.Update("org-a", row.Name, &UpdatePersonalityInput{Prompt: &nextPrompt})
	require.NoError(t, err)
	require.NotNil(t, updated.BehaviorProfile, "旧模板字段保留只读兼容")

	var latest personality.Version
	require.NoError(t, db.Where("template_id = ?", row.ID).Order("version DESC").First(&latest).Error)
	require.Nil(t, latest.BehaviorProfile, "新版本不能继续写入结构化滑杆")
}

func TestPersonalityService_BuiltinCannotBeDeleted(t *testing.T) {
	service, _ := setupPersonalityServiceTest(t)
	_, err := service.List("org-a")
	require.NoError(t, err)
	require.True(t, errors.Is(service.Delete("org-a", "steady-operator"), personality.ErrBuiltinDelete))
}
