package systemsetting

import "gorm.io/gorm"

const AgentMemoryWrappingKey = "agent_memory_wrapping_key"

var agentMemoryWrappingKeySpec = secretSpec{
	key:    AgentMemoryWrappingKey,
	env:    "AGENT_MEMORY_WRAPPING_KEY",
	label:  "agent memory wrapping key",
	minLen: 64,
	genLen: 32,
}

// EnsureAgentMemoryWrappingKey 为记忆 MCP 账本 payload 加密供给 wrapping
// key（spec 2026-11-11-memory-mcp-design §6）：显式配置（64 hex = 32 字节）
// 优先，否则启动生成并持久化。与业务应用记忆的 wrapping key 互相独立——
// 两个账本域的密钥泄露面互不影响（格式合法性由
// memory.NewSyntheticKeyWrapper 在装配处校验）。
func EnsureAgentMemoryWrappingKey(db *gorm.DB, provided string) (string, error) {
	return ensureSecret(db, provided, agentMemoryWrappingKeySpec)
}
