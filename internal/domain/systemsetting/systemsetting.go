// Package systemsetting 提供系统级配置的持久化存储。当前用于持久化两类
// 启动时自动生成的 secret：builtin 模式下未显式配置 AUTH_JWT_SECRET 时
// 的 JWT secret（重启、容器重建、镜像升级均不丢失，登录态保持），以及
// 未显式配置 KNOWLEDGE_CAPABILITY_SECRET 时的 knowledge capability 签名
// secret（issue #111 重开：与 provider 凭证加密密钥解耦的独立 secret）。
package systemsetting

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// JWTSecretKey 是 JWT secret 在 settings 表中的键名。
const JWTSecretKey = "jwt_secret"

// KnowledgeCapabilitySecretKey 是 knowledge capability 签名 secret 在
// settings 表中的键名（issue #111 重开）。
const KnowledgeCapabilitySecretKey = "knowledge_capability_secret"

// SystemSetting 是通用的 key/value 系统配置表。Value 存敏感值时为密文
// 或随机串本身；本表行数极少，不做缓存。
type SystemSetting struct {
	Key   string `gorm:"primaryKey;column:setting_key;size:128"`
	Value string `gorm:"column:setting_value;not null"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// secretSpec 参数化 ensureSecret 骨架中随 secret 种类而异的字段。
type secretSpec struct {
	key    string // system_settings 键名
	env    string // 显式配置的环境变量名（错误提示指向它）
	label  string // wrapped error 中的语义名
	minLen int    // 显式值与存量值的最小字节数
	genLen int    // 自动生成的随机字节数（hex 编码后为 2×genLen 字符，恒 ≥ minLen）
}

var jwtSecretSpec = secretSpec{
	key:    JWTSecretKey,
	env:    "AUTH_JWT_SECRET",
	label:  "JWT secret",
	minLen: 32,
	genLen: 32,
}

var knowledgeCapabilitySecretSpec = secretSpec{
	key:    KnowledgeCapabilitySecretKey,
	env:    "KNOWLEDGE_CAPABILITY_SECRET",
	label:  "knowledge capability secret",
	minLen: 32,
	genLen: 32,
}

// EnsureJWTSecret 返回生效的 JWT secret：
//   - provided 非空：至少 32 字节方可原样返回（显式配置优先，不写库；
//     与 builtin 模式 ValidateAuth 的下限一致）
//   - provided 为空：从 system_settings 读取既有的自动生成 secret；
//     不存在则生成 32 字节随机 secret 并落库（唯一键冲突 = 并发副本已
//     写入，重读返回既有值）
//
// 显式值与存量值过短（<32 字节，视为配置错误/人为误写）时返回错误——
// 静默接受会削弱 token 签名强度。
func EnsureJWTSecret(db *gorm.DB, provided string) (string, error) {
	return ensureSecret(db, provided, jwtSecretSpec)
}

// EnsureKnowledgeCapabilitySecret 返回生效的 knowledge capability 签名
// secret（issue #111 重开修复：该 secret 与 provider 凭证加密密钥是不同的
// 安全关注点，不得复用）：
//   - provided 非空：至少 32 字节方可原样返回（显式配置
//     KNOWLEDGE_CAPABILITY_SECRET 优先，不写库）——短显式值直接拒绝启动
//     （PR #118 二轮 review：显式值须过与自动值相同的强度校验，否则弱密钥
//     签发的全部 HMAC capability 可被离线穷举）
//   - provided 为空：从 system_settings 读取既有的自动生成 secret；
//     不存在则生成 32 字节随机 secret 并落库（唯一键冲突 = 并发副本已
//     写入，重读返回既有值）
//
// 存量值过短（<32 字节，视为损坏/人为误写）时返回错误——静默接受会削弱
// capability 签名强度。签发侧（AgentDeployerService）与验证侧
// （AgentService）必须注入同一值：空密钥环境下已部署的 capability 不会
// 静默失效，secret 轮换前签发的 capability 验签必然失败（fail-closed）。
func EnsureKnowledgeCapabilitySecret(db *gorm.DB, provided string) (string, error) {
	return ensureSecret(db, provided, knowledgeCapabilitySecretSpec)
}

// ensureSecret 承载两个公开入口的共享骨架（PR #118 二轮 review 去重）：
// 读 DB → 无则用 provided（强度校验）或随机生成（落库）→ 并发插入冲突后
// 重读。长度一律按字节计（len）而非字符数。
func ensureSecret(db *gorm.DB, provided string, spec secretSpec) (string, error) {
	if provided != "" {
		if len(provided) < spec.minLen {
			return "", fmt.Errorf("%s 长度不足：需至少 %d 字节，当前 %d 字节", spec.env, spec.minLen, len(provided))
		}
		return provided, nil
	}

	var existing SystemSetting
	err := db.Where("setting_key = ?", spec.key).First(&existing).Error
	if err == nil {
		if len(existing.Value) < spec.minLen {
			return "", fmt.Errorf("system_settings 中 %s 的值过短（%d 字节），请修复该行或显式配置 %s", spec.key, len(existing.Value), spec.env)
		}
		return existing.Value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("read persisted %s: %w", spec.label, err)
	}

	buf := make([]byte, spec.genLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate %s: %w", spec.label, err)
	}
	secret := hex.EncodeToString(buf)

	created := SystemSetting{Key: spec.key, Value: secret}
	if err := db.Create(&created).Error; err != nil {
		// 并发副本竞争：重读既有值
		var raced SystemSetting
		if rerr := db.Where("setting_key = ?", spec.key).First(&raced).Error; rerr == nil && len(raced.Value) >= spec.minLen {
			return raced.Value, nil
		}
		return "", fmt.Errorf("persist generated %s: %w", spec.label, err)
	}
	return secret, nil
}
