package auth_test

import (
	"testing"

	"control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&auth.User{}, &auth.Invite{}, &auth.RefreshToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestUserTableConstraints(t *testing.T) {
	db := openTestDB(t)
	u := &auth.User{Username: "alice", PasswordHash: "x", Role: auth.RoleMember, Status: auth.StatusActive}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	dup := &auth.User{Username: "alice", PasswordHash: "y", Role: auth.RoleMember, Status: auth.StatusActive}
	if err := db.Create(dup).Error; err == nil {
		t.Fatal("want unique violation on username")
	}
}

func TestIsValidRole(t *testing.T) {
	for _, r := range []string{"admin", "maintainer", "member"} {
		if !auth.IsValidRole(r) {
			t.Fatalf("%s should be valid", r)
		}
	}
	if auth.IsValidRole("owner") {
		t.Fatal("owner should be invalid")
	}
}

func TestIsValidStatus(t *testing.T) {
	for _, s := range []string{auth.StatusActive, auth.StatusDisabled, auth.StatusPending} {
		if !auth.IsValidStatus(s) {
			t.Fatalf("%s should be valid", s)
		}
	}
	if auth.IsValidStatus("banned") {
		t.Fatal("banned should be invalid")
	}
}
