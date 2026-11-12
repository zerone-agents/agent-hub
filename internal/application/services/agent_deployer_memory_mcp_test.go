package services

import (
	"strings"
	"testing"

	"control-panel/internal/infrastructure/deployer"

	"github.com/stretchr/testify/require"
)

func memoryMcpAttachFixtureReq() *deployer.CreateAgentRequest {
	return &deployer.CreateAgentRequest{
		Agents: []deployer.AgentDefinition{
			{
				Name: "counselor",
				McpServers: map[string]deployer.McpServerConfig{
					"memory": {
						Type:    "http",
						URL:     "http://hub:8081/api/v1/memory/mcp",
						Headers: map[string]string{"Authorization": "Bearer " + agentRuntimeTokenPlaceholder},
					},
				},
			},
			{Name: "plain-assistant"}, // 无 memory 条目
		},
	}
}

func TestAttachMemoryCapabilitiesRoundTripAndBindings(t *testing.T) {
	secret := strings.Repeat("ab", 32)
	s := &AgentDeployerService{memoryCapabilitySecret: secret}
	req := memoryMcpAttachFixtureReq()
	require.NoError(t, s.attachMemoryCapabilities(req, "tenant-1", "runtime-token-1"))

	entry := req.Agents[0].McpServers["memory"]
	raw := entry.Headers["X-Agent-Capability"]
	require.NotEmpty(t, raw)
	require.True(t, strings.HasPrefix(raw, "v1."), "capability 为签名线格式")
	// 签名可验：同 agent + token 指纹通过，载荷断言正确（spec §4）
	verifier := NewMemoryCapabilityService(secret)
	payload, err := verifier.VerifyWithToken(raw, "counselor", "runtime-token-1")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", payload.TenantID)
	require.Equal(t, "agent", payload.ScopeMode)
	require.ElementsMatch(t, []string{"memory_remember", "memory_recall", "memory_list", "memory_update"}, payload.Tools)
	// token 轮换（指纹不符）拒绝
	_, err = verifier.VerifyWithToken(raw, "counselor", "rotated-token")
	require.ErrorIs(t, err, ErrMemoryCapabilityDenied)
	// 既有 Authorization 头不被覆盖（占位符已由 resolveMcpHeaders 先行替换）
	require.Equal(t, "Bearer "+agentRuntimeTokenPlaceholder, entry.Headers["Authorization"])
	// 无 memory 条目的节点不受影响
	_, has := req.Agents[1].McpServers["memory"]
	require.False(t, has)
}

func TestAttachMemoryCapabilitiesFailClosedOnMissingSecret(t *testing.T) {
	s := &AgentDeployerService{}
	req := memoryMcpAttachFixtureReq()
	err := s.attachMemoryCapabilities(req, "t", "tok")
	require.ErrorContains(t, err, "memory capability secret", "空 secret 拒绝签发（防伪造）")
}

func TestAttachMemoryCapabilitiesNoopWithoutMemoryEntry(t *testing.T) {
	s := &AgentDeployerService{}
	req := &deployer.CreateAgentRequest{Agents: []deployer.AgentDefinition{{Name: "a"}}}
	require.NoError(t, s.attachMemoryCapabilities(req, "t", "tok"), "无 memory 条目时 no-op（空 secret 也不报错）")
}

func TestMemoryMcpServerEntryShapeAndFailFast(t *testing.T) {
	_, err := memoryMcpServerEntry("counselor", "   ")
	require.ErrorContains(t, err, "MEMORY_MCP_URL", "开启记忆但未配置可达地址须 fail fast 并给出配置指引")
	entry, err := memoryMcpServerEntry("counselor", "http://hub:8081/api/v1/memory/mcp")
	require.NoError(t, err)
	require.Equal(t, "http", entry.Type)
	require.Equal(t, "http://hub:8081/api/v1/memory/mcp", entry.URL)
	require.Equal(t, "Bearer "+agentRuntimeTokenPlaceholder, entry.Headers["Authorization"], "认证头用运行时 token 占位符（Deploy 侧统一替换）")
}
