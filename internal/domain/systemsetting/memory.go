package systemsetting

import (
	"encoding/hex"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const MemoryWrappingKey = "memory_wrapping_key"

// EnsureMemoryWrappingKey shares the persisted-secret mechanism, with a
// separate key and strict AES-256 encoding. Legacy external keys remain valid.
func EnsureMemoryWrappingKey(db *gorm.DB, provided string) (string, error) {
	if provided == "" {
		var existing SystemSetting
		err := db.Where("setting_key = ?", MemoryWrappingKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Never silently replace a missing legacy key with a new random key.
			for _, target := range []struct{ table, column string }{
				{"memory_lifecycles", "wrapped_key"},
				{"memory_applications", "token_cipher"},
			} {
				if !db.Migrator().HasTable(target.table) {
					continue
				}
				var count int64
				if err := db.Table(target.table).Where(target.column + " IS NOT NULL AND LENGTH(" + target.column + ") > 0").Count(&count).Error; err != nil {
					return "", fmt.Errorf("check existing memory encryption: %w", err)
				}
				if count > 0 {
					return "", fmt.Errorf("existing memory data requires the original MEMORY_WRAPPING_KEY; automatic key generation refused")
				}
			}
		} else if err != nil {
			return "", fmt.Errorf("read persisted memory wrapping key: %w", err)
		}
	}
	key, err := ensureSecret(db, provided, secretSpec{key: MemoryWrappingKey, env: "MEMORY_WRAPPING_KEY", label: "memory wrapping key", minLen: 64, genLen: 32})
	if err != nil {
		return "", err
	}
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("MEMORY_WRAPPING_KEY or its persisted value must be 64 hexadecimal characters")
	}
	return key, nil
}
