package provider

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProviderModelAutoMigrate_HasAigcCodeColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ProviderModel{}))
	require.True(t, db.Migrator().HasColumn(&ProviderModel{}, "AigcCode"))
}
