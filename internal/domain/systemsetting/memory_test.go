package systemsetting

import (
	"context"
	"strings"
	"testing"

	"control-panel/internal/memory"
	"control-panel/internal/memoryapp"
	"github.com/stretchr/testify/require"
)

func TestMemoryWrappingKeyPersistenceAndEncryption(t *testing.T) {
	db := newTestDB(t)
	key, err := EnsureMemoryWrappingKey(db, "")
	require.NoError(t, err)
	wrapper, err := memory.NewSyntheticKeyWrapper(key)
	require.NoError(t, err)
	ciphertext, err := wrapper.Wrap(context.Background(), []byte("synthetic lifecycle key"), []byte("scope"))
	require.NoError(t, err)
	second, err := EnsureMemoryWrappingKey(db, "")
	require.NoError(t, err)
	require.Equal(t, key, second, "restart must preserve the wrapping key")
	restored, err := memory.NewSyntheticKeyWrapper(second)
	require.NoError(t, err)
	plaintext, err := restored.Unwrap(context.Background(), ciphertext, []byte("scope"))
	require.NoError(t, err)
	require.Equal(t, "synthetic lifecycle key", string(plaintext))
	explicit := strings.Repeat("ab", 32)
	got, err := EnsureMemoryWrappingKey(db, explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, got, "legacy external keys keep precedence")
	got, err = EnsureMemoryWrappingKey(db, "")
	require.NoError(t, err)
	require.Equal(t, key, got, "explicit keys must not replace the persisted key")
}

func TestMemoryWrappingKeyRejectsCorruptionAndLegacyKeyLoss(t *testing.T) {
	for _, invalid := range []string{"short", strings.Repeat("z", 64), strings.Repeat("a", 66)} {
		db := newTestDB(t)
		_, err := EnsureMemoryWrappingKey(db, invalid)
		require.Error(t, err)
		require.NoError(t, db.Create(&SystemSetting{Key: MemoryWrappingKey, Value: invalid}).Error)
		_, err = EnsureMemoryWrappingKey(db, "")
		require.Error(t, err)
	}
	for _, table := range []string{"memory_lifecycles", "memory_applications"} {
		t.Run(table, func(t *testing.T) {
			db := newTestDB(t)
			require.NoError(t, memory.Migrate(db))
			require.NoError(t, memoryapp.Migrate(db))
			column := "wrapped_key"
			if table == "memory_applications" {
				column = "token_cipher"
			}
			require.NoError(t, db.Table(table).Create(map[string]any{"id": "legacy", column: []byte("legacy ciphertext")}).Error)
			_, err := EnsureMemoryWrappingKey(db, "")
			require.ErrorContains(t, err, "original MEMORY_WRAPPING_KEY")
			var count int64
			require.NoError(t, db.Model(&SystemSetting{}).Count(&count).Error)
			require.Zero(t, count)
			_, err = EnsureMemoryWrappingKey(db, strings.Repeat("ab", 32))
			require.NoError(t, err)
		})
	}
}
