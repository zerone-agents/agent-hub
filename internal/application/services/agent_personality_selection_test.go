package services

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/stretchr/testify/require"
)

func TestAgentPersonalitySelectionUsesLibraryAsSourceOfTruth(t *testing.T) {
	personalityService, _ := setupPersonalityServiceTest(t)
	rows, err := personalityService.List("org-a")
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	var selectedPrompt string
	for _, row := range rows {
		if row.Name == "duty-whistleblower" {
			selectedPrompt = row.Prompt
		}
	}
	require.NotEmpty(t, selectedPrompt)

	service := NewAgentService("", "")
	cfg, err := service.prepareCreateConfig("org-a", &CreateAgentInput{
		Name: "reporter",
		Config: map[string]interface{}{
			"systemPrompt":               "负责审计",
			"personalityTemplateName":    "duty-whistleblower",
			"personalityTemplateVersion": float64(999),
			"personalityPrompt":          "客户端伪造的人格",
			"behaviorProfile":            behaviorProfileConfigMap(),
		},
	})
	require.NoError(t, err)
	require.Equal(t, "duty-whistleblower", cfg.PersonalityTemplateName)
	require.Equal(t, 1, cfg.PersonalityTemplateVersion)
	require.Equal(t, selectedPrompt, cfg.PersonalityPrompt)
	require.NotNil(t, cfg.BehaviorProfile)
	require.Equal(t, 95, cfg.BehaviorProfile.Whistleblowing)
}

func TestAgentPersonalitySelectionCannotBeEditedInline(t *testing.T) {
	personalityService, _ := setupPersonalityServiceTest(t)
	_, err := personalityService.List("org-a")
	require.NoError(t, err)

	profile := agent.DefaultBehaviorProfile()
	cfg := &agent.AgentConfig{
		PersonalityTemplateName:    "duty-whistleblower",
		PersonalityTemplateVersion: 1,
		PersonalityPrompt:          "人格库快照",
		BehaviorProfile:            &profile,
	}
	service := NewAgentService("", "")
	update := map[string]interface{}{
		"systemPrompt":               "更新职责",
		"personalityTemplateVersion": float64(999),
		"personalityPrompt":          "试图在 Agent 页面改人格",
		"behaviorProfile":            nil,
	}
	require.NoError(t, service.applyUpdateConfig("org-a", cfg, &UpdateAgentInput{Config: &update}))
	require.Equal(t, "更新职责", cfg.SystemPrompt)
	require.Equal(t, "duty-whistleblower", cfg.PersonalityTemplateName)
	require.Equal(t, 1, cfg.PersonalityTemplateVersion)
	require.Equal(t, "人格库快照", cfg.PersonalityPrompt)
	require.Same(t, &profile, cfg.BehaviorProfile)
}

func TestAgentPersonalitySelectionCanBeCleared(t *testing.T) {
	personalityService, _ := setupPersonalityServiceTest(t)
	_, err := personalityService.List("org-a")
	require.NoError(t, err)

	profile := agent.DefaultBehaviorProfile()
	cfg := &agent.AgentConfig{
		PersonalityTemplateName:    "steady-operator",
		PersonalityTemplateVersion: 1,
		PersonalityPrompt:          "旧人格",
		BehaviorProfile:            &profile,
	}
	service := NewAgentService("", "")
	update := map[string]interface{}{"personalityTemplateName": ""}
	require.NoError(t, service.applyUpdateConfig("org-a", cfg, &UpdateAgentInput{Config: &update}))
	require.Empty(t, cfg.PersonalityTemplateName)
	require.Zero(t, cfg.PersonalityTemplateVersion)
	require.Empty(t, cfg.PersonalityPrompt)
	require.Nil(t, cfg.BehaviorProfile)
}

func TestAgentPersonalitySelectionRejectsDisabledTemplate(t *testing.T) {
	personalityService, _ := setupPersonalityServiceTest(t)
	_, err := personalityService.List("org-a")
	require.NoError(t, err)
	enabled := false
	_, err = personalityService.Update("org-a", "steady-operator", &UpdatePersonalityInput{Enabled: &enabled})
	require.NoError(t, err)

	service := NewAgentService("", "")
	_, err = service.prepareCreateConfig("org-a", &CreateAgentInput{
		Name: "disabled-personality-agent",
		Config: map[string]interface{}{
			"systemPrompt":            "负责执行",
			"personalityTemplateName": "steady-operator",
		},
	})
	require.ErrorContains(t, err, "已停用")
}

func TestAgentWithDisabledPersonalityCanStillUpdateItsResponsibilities(t *testing.T) {
	personalityService, _ := setupPersonalityServiceTest(t)
	rows, err := personalityService.List("org-a")
	require.NoError(t, err)

	var prompt string
	for _, row := range rows {
		if row.Name == "steady-operator" {
			prompt = row.Prompt
		}
	}
	require.NotEmpty(t, prompt)
	enabled := false
	_, err = personalityService.Update("org-a", "steady-operator", &UpdatePersonalityInput{Enabled: &enabled})
	require.NoError(t, err)

	profile := agent.DefaultBehaviorProfile()
	cfg := &agent.AgentConfig{
		PersonalityTemplateName:    "steady-operator",
		PersonalityTemplateVersion: 1,
		PersonalityPrompt:          prompt,
		BehaviorProfile:            &profile,
	}
	service := NewAgentService("", "")
	update := map[string]interface{}{
		"systemPrompt":            "新的职责",
		"personalityTemplateName": "steady-operator",
	}
	require.NoError(t, service.applyUpdateConfig("org-a", cfg, &UpdateAgentInput{Config: &update}))
	require.Equal(t, "新的职责", cfg.SystemPrompt)
	require.Equal(t, prompt, cfg.PersonalityPrompt)
	require.Equal(t, 1, cfg.PersonalityTemplateVersion)
}
