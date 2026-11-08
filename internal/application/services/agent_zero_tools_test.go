package services

import (
	"context"
	"testing"

	"control-panel/internal/domain/agent"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
)

func TestZeroToolsDeployProjectionAndMountRejection(t *testing.T) {
	w := newGraphWorld()
	cfg := w.addAgent("tenant-a", "deer", func(c *agent.AgentConfig) { c.ZeroTools = true })
	s := w.service(t, "http://127.0.0.1:1")
	def, _, err := s.buildAgentDefinition(context.Background(), "tenant-a", cfg, definitionOpts{isRoot: true})
	require.NoError(t, err)
	require.True(t, def.ZeroTools)
	require.Empty(t, def.Tools)
	w.tools[cfg.ID] = []*agent.Tool{{Name: "Bash", Source: agent.ToolSourceBuiltin}}
	_, _, err = s.buildAgentDefinition(context.Background(), "tenant-a", cfg, definitionOpts{isRoot: true})
	require.ErrorContains(t, err, "零工具")
	require.Error(t, ValidateConfig(map[string]interface{}{"zeroTools": "true"}))
	require.NoError(t, ValidateConfig(map[string]interface{}{"zeroTools": true}))
}

func TestZeroToolsCreationAndClearingNeverReattachDefaultTools(t *testing.T) {
	db := setupToolTenantServiceTestDB(t)
	require.NoError(t, db.Exec(`INSERT INTO tools (name,tenant_id,is_default,source) VALUES ('Bash','',1,'builtin')`).Error)
	svc := NewAgentService("test-encryption-key", "")
	out, err := svc.CreateAgent("org-b", &CreateAgentInput{Name: "zero-agent", Config: map[string]interface{}{"systemPrompt": "test", "zeroTools": true}})
	require.NoError(t, err)
	require.Equal(t, true, out.Config["zeroTools"])
	require.Empty(t, out.Tools)
	toolSvc := NewToolService(nil)
	_, err = toolSvc.UpdateAgentTools("org-b", "zero-agent", []string{"Bash"})
	require.Error(t, err)
	tools, err := toolSvc.UpdateAgentTools("org-b", "zero-agent", nil)
	require.NoError(t, err)
	require.Empty(t, tools)
	cfg, err := repository.NewAgentRepository().GetByName("org-b", "zero-agent")
	require.NoError(t, err)
	require.True(t, cfg.ZeroTools)
}
