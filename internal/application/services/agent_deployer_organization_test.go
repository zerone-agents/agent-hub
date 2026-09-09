package services

import (
	"context"
	"fmt"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildAgentDefinitionAutoMountsOrganizationMcpForRelatedAgent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}))

	source := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	target := agent.AgentConfig{Name: "agent-b", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&source).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&agentrelation.AgentRelation{
		TenantID: "tenant-a", Scope: "speeding-hq", SourceAgentID: source.ID, TargetAgentID: target.ID,
		RelationType: "peer", Stance: "friendly", AllowedActions: []string{"consult"},
		ContextPolicy: "summary_only", DeliveryPolicy: "sync", Enabled: true,
	}).Error)

	agentRepo := &mockAgentRepo{
		getByNameFunc: func(_, name string) (*agent.AgentConfig, error) {
			if name == source.Name {
				return &source, nil
			}
			return &target, nil
		},
		updateFunc: func(string, *agent.AgentConfig) error { return nil },
	}
	mcpSvc := &mockMcpSvc{organization: &McpClientDTO{
		Name: "organization", Type: "http", URL: "https://hub.example.com/api/v1/organization/mcp",
		Headers: map[string]string{"Authorization": BuiltinOrganizationAuthHeader},
		Tools:   builtinOrganizationTools,
	}}
	service := &AgentDeployerService{
		agentRepo:    agentRepo,
		toolRepo:     &mockToolRepo{},
		skillRepo:    &mockSkillRepo{},
		mcpSvc:       mcpSvc,
		relationRepo: repository.NewAgentRelationRepositoryWithDB(db),
		knowledgeSvc: &mockKnowledgeSvc{},
	}

	definition, _, err := service.buildAgentDefinition(context.Background(), "tenant-a", &source, definitionOpts{isRoot: true})
	require.NoError(t, err)
	organization, ok := definition.McpServers["organization"]
	require.True(t, ok)
	require.Equal(t, "https://hub.example.com/api/v1/organization/mcp", organization.URL)
	require.Equal(t, BuiltinOrganizationAuthHeader, organization.Headers["Authorization"])
	require.Contains(t, definition.Tools, "mcp__organization__agent_send")
	require.Contains(t, definition.Tools, "mcp__organization__agent_relations")
}

func TestBuildAgentDefinitionSkipsOrganizationMcpWithoutEnabledRelation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &agentrelation.AgentRelation{}))

	standalone := agent.AgentConfig{Name: "standalone", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&standalone).Error)
	service := &AgentDeployerService{
		agentRepo: &mockAgentRepo{
			getByNameFunc: func(_, _ string) (*agent.AgentConfig, error) { return &standalone, nil },
			updateFunc:    func(string, *agent.AgentConfig) error { return nil },
		},
		toolRepo:     &mockToolRepo{},
		skillRepo:    &mockSkillRepo{},
		mcpSvc:       &mockMcpSvc{organization: &McpClientDTO{Name: "organization", Type: "http", URL: "https://hub.example.com/api/v1/organization/mcp"}},
		relationRepo: repository.NewAgentRelationRepositoryWithDB(db),
		knowledgeSvc: &mockKnowledgeSvc{},
	}

	definition, _, err := service.buildAgentDefinition(context.Background(), "tenant-a", &standalone, definitionOpts{isRoot: true})
	require.NoError(t, err)
	require.NotContains(t, definition.McpServers, "organization")
}
