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
		Name:            "profiled-agent",
		TenantID:        "tenant-a",
		ContentHash:     "sha256:test",
		SystemPrompt:    "base identity",
		BehaviorProfile: &profile,
	}
	require.NoError(t, db.Create(created).Error)

	var loaded agent.AgentConfig
	require.NoError(t, db.First(&loaded, created.ID).Error)
	require.NotNil(t, loaded.BehaviorProfile)
	require.Equal(t, profile, *loaded.BehaviorProfile)
}
