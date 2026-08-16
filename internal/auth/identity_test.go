package auth

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newSQLite opens a fresh in-memory SQLite database for tests.
func newSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}
