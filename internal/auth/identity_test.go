package auth

import (
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"

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

func TestUpsertIdentityInsertThenRefresh(t *testing.T) {
	db := newSQLite(t)
	if err := db.AutoMigrate(&authdom.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	au := &AuthUser{ID: "cas-1", Username: "alice", DisplayName: "Alice", Email: "a@x.com",
		Roles: []string{"admin"}, TenantID: "tenant-acme"}
	if err := UpsertIdentity(db, "casdoor", au); err != nil {
		t.Fatal(err)
	}
	var row authdom.UserIdentity
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.TenantID != "tenant-acme" || row.Role != "admin" || row.ExternalID != "cas-1" {
		t.Fatalf("bad row: %+v", row)
	}
	firstLogin := row.LastLoginAt

	// second login: same external id, role changed -> update not insert
	time.Sleep(10 * time.Millisecond)
	au.Roles = []string{"member"}
	if err := UpsertIdentity(db, "casdoor", au); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&authdom.UserIdentity{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
	db.First(&row)
	if row.Role != "member" {
		t.Fatalf("role snapshot not refreshed: %q", row.Role)
	}
	if !row.LastLoginAt.After(firstLogin) {
		t.Fatal("last_login_at not refreshed")
	}
}
