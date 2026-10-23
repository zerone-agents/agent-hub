package services

import (
	"context"
	"testing"

	"control-panel/internal/domain/agent"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupEffortDeployDB seeds an in-memory provider_models table whose row for
// (provider 1, model glm-4) declares the given efforts JSON (issue #226).
func setupEffortDeployDB(t *testing.T, efforts string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&providerdomain.ProviderSummary{}, &providerdomain.ProviderModel{}))
	require.NoError(t, db.Create(&providerdomain.ProviderSummary{
		ID: 1, Key: "glm", Name: "GLM", Protocol: "anthropic",
		AuthStyle: "api_key", BaseURL: "http://x",
	}).Error)
	require.NoError(t, db.Create(&providerdomain.ProviderModel{
		ProviderID: 1, SelectionID: "sel-1", ModelID: "glm-4",
		ModelType: "llm", Efforts: efforts,
	}).Error)

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
}

// TestValidateEffortForDeploy 锁定下发兜底契约（issue #226）：按当前
// provider_models 行复验声明白名单；模型行不存在 fail-open（模型缺失由
// 部署链路自身报错，不伪装成 effort 问题）。
func TestValidateEffortForDeploy(t *testing.T) {
	svc := &AgentDeployerService{}
	cfg := func(effort string) *agent.AgentConfig {
		return &agent.AgentConfig{ModelID: "glm-4", Effort: effort}
	}

	t.Run("声明外拒绝", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		err := svc.validateEffortForDeploy(1, cfg("ultra"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "effort 'ultra' 不在模型 glm-4 声明的候选列表内")
	})

	t.Run("声明内放行", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		require.NoError(t, svc.validateEffortForDeploy(1, cfg("high")))
	})

	t.Run("无声明透传放行", func(t *testing.T) {
		setupEffortDeployDB(t, "")
		require.NoError(t, svc.validateEffortForDeploy(1, cfg("custom-value")))
	})

	t.Run("模型行不存在 fail-open", func(t *testing.T) {
		setupEffortDeployDB(t, `["low"]`)
		missing := &agent.AgentConfig{ModelID: "no-such-model", Effort: "ultra"}
		require.NoError(t, svc.validateEffortForDeploy(1, missing))
	})

	t.Run("selectionId 优先", func(t *testing.T) {
		setupEffortDeployDB(t, `["low"]`)
		sel := &agent.AgentConfig{ModelID: "glm-4", ModelSelectionID: "sel-1", Effort: "low"}
		require.NoError(t, svc.validateEffortForDeploy(1, sel))
	})
}

// TestBuildCreateRequest_EffortDeployGuard 锁定 buildCreateRequest 的兜底
// 接线（issue #226）：root 声明了声明外的 effort 时部署请求必须 fail-fast，
// 错误在构建期冒泡（对齐 deployer 400 前置拦截）。
func TestBuildCreateRequest_EffortDeployGuard(t *testing.T) {
	setupEffortDeployDB(t, `["low","high"]`)
	providerID := uint64(1)
	provider := buildReqTestProvider(t, providerID)
	agentRepo := &mockAgentRepo{getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
		return &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-4",
			SystemPrompt: "p", Effort: "ultra"}, nil
	}}
	providerSvc := &mockProviderSvc{
		getByIDFunc:   func(id uint64) (providerdomain.Provider, error) { return buildReqTestProvider(t, id), nil },
		getRawKeyFunc: func(id uint64) (string, error) { return "k", nil },
	}
	svc := newTestAgentDeployerService(t, "http://deployer.test", agentRepo, providerSvc)
	svc.toolRepo = &mockToolRepo{}
	svc.cdnHost = "https://cdn.example.com"
	cfg := &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-4",
		SystemPrompt: "p", Effort: "ultra"}

	_, err := svc.buildCreateRequest(context.Background(), "t", cfg, provider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "effort 'ultra' 不在模型 glm-4 声明的候选列表内")

	// 声明内值正常构建且 root 节点携带 effort。
	cfg.Effort = "high"
	req, err := svc.buildCreateRequest(context.Background(), "t", cfg, provider)
	require.NoError(t, err)
	var rootEffort string
	found := false
	for i := range req.Agents {
		if req.Agents[i].Name == "general" {
			rootEffort = req.Agents[i].Effort
			found = true
		}
	}
	require.True(t, found, "root definition must be present")
	require.Equal(t, "high", rootEffort)
}
