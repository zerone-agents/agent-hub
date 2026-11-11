package systemsetting

import "gorm.io/gorm"

const MemoryCapabilitySecretKey = "memory_capability_secret"

var memoryCapabilitySecretSpec = secretSpec{
	key:    MemoryCapabilitySecretKey,
	env:    "MEMORY_CAPABILITY_SECRET",
	label:  "memory capability secret",
	minLen: 32,
	genLen: 32,
}

// EnsureMemoryCapabilitySecret 与 knowledge capability secret 同机制
// （spec docs/superpowers/specs/2026-11-11-memory-mcp-design.md §4）：显式
// 配置优先，否则启动时随机生成并持久化；与其他用途的 secret 解耦，
// 泄露面互不影响。
func EnsureMemoryCapabilitySecret(db *gorm.DB, provided string) (string, error) {
	return ensureSecret(db, provided, memoryCapabilitySecretSpec)
}
