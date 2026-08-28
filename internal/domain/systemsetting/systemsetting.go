// Package systemsetting 提供系统级配置的持久化存储。当前用于持久化
// builtin 模式下自动生成的 JWT secret：未显式配置 AUTH_JWT_SECRET 时，
// secret 生成后存入数据库（而非容器文件系统或进程内存），重启、容器
// 重建、镜像升级均不丢失，登录态保持。
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

// SystemSetting 是通用的 key/value 系统配置表。Value 存敏感值时为密文
// 或随机串本身；本表行数极少，不做缓存。
type SystemSetting struct {
	Key   string `gorm:"primaryKey;column:setting_key;size:128"`
	Value string `gorm:"column:setting_value;not null"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// EnsureJWTSecret 返回生效的 JWT secret：
//   - provided 非空：原样返回（显式配置优先，ValidateAuth 另行校验长度）
//   - provided 为空：从 system_settings 读取既有的自动生成 secret；
//     不存在则生成 32 字节随机 secret 并落库（唯一键冲突 = 并发副本已
//     写入，重读返回既有值）
//
// 存量值过短（<32 字节，视为损坏/人为误写）时返回错误——静默接受会削弱
// token 签名强度。
func EnsureJWTSecret(db *gorm.DB, provided string) (string, error) {
	if provided != "" {
		return provided, nil
	}

	var existing SystemSetting
	err := db.Where("setting_key = ?", JWTSecretKey).First(&existing).Error
	if err == nil {
		if len(existing.Value) < 32 {
			return "", fmt.Errorf("system_settings 中 %s 的值过短（%d 字节），请修复该行或显式配置 AUTH_JWT_SECRET", JWTSecretKey, len(existing.Value))
		}
		return existing.Value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("read persisted JWT secret: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate JWT secret: %w", err)
	}
	secret := hex.EncodeToString(buf)

	created := SystemSetting{Key: JWTSecretKey, Value: secret}
	if err := db.Create(&created).Error; err != nil {
		// 并发副本竞争：重读既有值
		var raced SystemSetting
		if rerr := db.Where("setting_key = ?", JWTSecretKey).First(&raced).Error; rerr == nil && len(raced.Value) >= 32 {
			return raced.Value, nil
		}
		return "", fmt.Errorf("persist generated JWT secret: %w", err)
	}
	return secret, nil
}
