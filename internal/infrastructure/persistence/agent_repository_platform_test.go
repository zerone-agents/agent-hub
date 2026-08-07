package repository

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPlatformTestDB creates an in-memory agents table matching the migrated
// production schema (desktop_enabled / mobile_enabled, no legacy enabled).
func setupPlatformTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(64) NOT NULL,
		content_hash VARCHAR(128) NOT NULL DEFAULT '',
		system_prompt TEXT NOT NULL DEFAULT '',
		permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto',
		max_turns INTEGER NOT NULL DEFAULT 50,
		title JSON,
		description JSON,
		icon VARCHAR(512) DEFAULT '',
		icon_name VARCHAR(64) DEFAULT '',
		icon_color VARCHAR(32) DEFAULT '',
		icon_bg_color VARCHAR(64) DEFAULT '',
		provider_id INTEGER,
		model_id VARCHAR(64) DEFAULT '',
		field_overrides TEXT,
		source VARCHAR(16) NOT NULL DEFAULT 'remote',
		desktop_enabled INTEGER NOT NULL DEFAULT 0,
		mobile_enabled INTEGER NOT NULL DEFAULT 0,
		is_default INTEGER DEFAULT 0,
		group_name VARCHAR(64) DEFAULT '',
		max_session_turns INTEGER,
		runtime_port INTEGER DEFAULT 0,
		deployment_status VARCHAR(32) DEFAULT '',
		deployed_at DATETIME,
		runtime_token TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`).Error)
	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	return db
}

func TestListForPlatform(t *testing.T) {
	db := setupPlatformTestDB(t)
	repo := NewAgentRepository()

	seed := []agent.AgentConfig{
		{Name: "both", DesktopEnabled: true, MobileEnabled: true},
		{Name: "desktop-only", DesktopEnabled: true},
		{Name: "mobile-only", MobileEnabled: true},
		{Name: "neither"},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}

	desktop, err := repo.ListForPlatform(agent.PlatformDesktop)
	require.NoError(t, err)
	require.Equal(t, []string{"both", "desktop-only"}, namesOf(desktop))

	mobile, err := repo.ListForPlatform(agent.PlatformMobile)
	require.NoError(t, err)
	require.Equal(t, []string{"both", "mobile-only"}, namesOf(mobile))

	// Empty platform string defaults to desktop (back-compat for existing clients).
	def, err := repo.ListForPlatform("")
	require.NoError(t, err)
	require.Equal(t, []string{"both", "desktop-only"}, namesOf(def))
}

func namesOf(configs []*agent.AgentConfig) []string {
	out := make([]string, 0, len(configs))
	for _, c := range configs {
		out = append(out, c.Name)
	}
	return out
}
