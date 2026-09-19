package repository

import (
	"fmt"
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAgentBehaviorProfileJSONRoundTrip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}))

	profile := agent.DefaultBehaviorProfile()
	created := &agent.AgentConfig{
		Name:                       "profiled-agent",
		TenantID:                   "tenant-a",
		ContentHash:                "sha256:test",
		SystemPrompt:               "base identity",
		BehaviorProfile:            &profile,
		PersonalityTemplateName:    "duty-whistleblower",
		PersonalityTemplateVersion: 3,
		PersonalityPrompt:          "证据充分且常规渠道失效时，你会越级报告。",
	}
	require.NoError(t, db.Create(created).Error)

	var loaded agent.AgentConfig
	require.NoError(t, db.First(&loaded, created.ID).Error)
	require.NotNil(t, loaded.BehaviorProfile)
	require.Equal(t, profile, *loaded.BehaviorProfile)
	require.Equal(t, "duty-whistleblower", loaded.PersonalityTemplateName)
	require.Equal(t, 3, loaded.PersonalityTemplateVersion)
	require.Contains(t, loaded.PersonalityPrompt, "常规渠道失效")
}
