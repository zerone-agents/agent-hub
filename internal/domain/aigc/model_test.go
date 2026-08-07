package aigc

import (
	"testing"

	"control-panel/internal/domain/chat"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAutoMigrate_ConfigAndMessageAigcColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Config{}, &chat.Message{}))

	require.True(t, db.Migrator().HasTable(&Config{}))
	require.True(t, db.Migrator().HasColumn(&Config{}, "SigningKeyEncrypted"))
	require.True(t, db.Migrator().HasColumn(&chat.Message{}, "Aigc"))
	require.Equal(t, "aigc_configs", Config{}.TableName())
}
