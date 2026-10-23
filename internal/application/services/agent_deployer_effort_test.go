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

// TestApplyUpdateConfig_EffortValidatesAgainstStoredBinding 锁定评审修复
// （issue #226）：AgentForm 提交的 config 不含 providerId/modelId 绑定，
// 白名单若只读裸 payload 会从不运行——声明外的 effort 静默存下、部署才报
// 错。applyUpdateConfig 必须在 effort key 存在而 payload 无 providerId 时
// 注入存量绑定再验，让保存侧白名单按「更新后终态绑定」真实执行。
func TestApplyUpdateConfig_EffortValidatesAgainstStoredBinding(t *testing.T) {
	providerID := uint64(1)

	t.Run("存量绑定下声明外 effort 保存被拒", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		cfg := &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-4"}
		payload := map[string]interface{}{"effort": "ultra", "systemPrompt": "p"}
		err := newSvc().applyUpdateConfig("t", cfg, &UpdateAgentInput{Config: &payload})
		require.Error(t, err)
		require.Contains(t, err.Error(), "effort 'ultra' 不在模型 glm-4 声明的候选列表内")
		// 拒绝路径不得半写：cfg.Effort 保持未设置
		require.Empty(t, cfg.Effort, "rejected update must not half-apply effort")
	})

	t.Run("存量绑定下声明内 effort 保存通过", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		cfg := &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-4"}
		payload := map[string]interface{}{"effort": "high", "systemPrompt": "p"}
		require.NoError(t, newSvc().applyUpdateConfig("t", cfg, &UpdateAgentInput{Config: &payload}))
		require.Equal(t, "high", cfg.Effort)
	})

	t.Run("payload 自带 providerId 时以 payload 为准", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		storedProvider := uint64(1)
		cfg := &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &storedProvider, ModelID: "glm-4"}
		payload := map[string]interface{}{
			"effort": "ultra", "systemPrompt": "p",
			"providerId": float64(1), "modelId": "glm-4",
		}
		err := newSvc().applyUpdateConfig("t", cfg, &UpdateAgentInput{Config: &payload})
		require.Error(t, err)
		require.Contains(t, err.Error(), "effort 'ultra' 不在模型 glm-4 声明的候选列表内")
	})

	t.Run("payload 无 effort 时行为零变化（无绑定也过）", func(t *testing.T) {
		setupEffortDeployDB(t, `["low","high"]`)
		cfg := &agent.AgentConfig{ID: 1, Name: "general"}
		payload := map[string]interface{}{"systemPrompt": "p"}
		require.NoError(t, newSvc().applyUpdateConfig("t", cfg, &UpdateAgentInput{Config: &payload}))
	})
}

// newSvc builds a bare AgentService for applyUpdateConfig-level tests; the
// effort path only touches database.DB (seeded by setupEffortDeployDB) and
// the payload/cfg pair, no repo/encryption dependencies on this branch.
func newSvc() *AgentService {
	return &AgentService{}
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

	t.Run("selectionId 优先（报错文案同源，评审建议①）", func(t *testing.T) {
		setupEffortDeployDB(t, `["low"]`)
		sel := &agent.AgentConfig{ModelID: "glm-4", ModelSelectionID: "sel-1", Effort: "ultra"}
		err := svc.validateEffortForDeploy(1, sel)
		require.Error(t, err)
		// 与保存侧一致：带 selectionId 的配置报错指向 selectionId 而非 modelId
		require.Contains(t, err.Error(), "不在模型 sel-1 声明的候选列表内")
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
