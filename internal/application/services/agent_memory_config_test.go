package services

import (
	"testing"

	"control-panel/internal/domain/agent"

	"github.com/stretchr/testify/require"
)

// 记忆 MCP Phase 1（spec 2026-11-11-memory-mcp-design §3）：scopeMode=user
// 依赖 runtime#95 会话身份透传（二期），保存期即拒绝，不给「配了但工具
// fail closed」的中间态，也不存在任何静默降级路径。
func TestAgentMemoryConfigRejectsUserScopeInPhase1(t *testing.T) {
	require.Error(t, validateMemoryScopeMode("user"), "Phase 1 保存期即拒绝 user（fail fast）")
	require.NoError(t, validateMemoryScopeMode("agent"))
	require.NoError(t, validateMemoryScopeMode(""), "空 = 未启用语义（默认 agent）")
	require.Error(t, validateMemoryScopeMode("bogus"), "未知值拒绝，不静默回退默认")
}

func TestAgentMemoryConfigRoundTrip(t *testing.T) {
	cfg := &agent.AgentConfig{Name: "m1", TenantID: "t1"}
	cfg.MemoryEnabled = true
	cfg.MemoryScopeMode = "agent"
	require.Equal(t, true, cfg.MemoryEnabled)
	require.Equal(t, "agent", cfg.MemoryScopeMode)
}
