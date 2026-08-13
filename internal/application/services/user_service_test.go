package services

import (
	"testing"

	authdom "control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newUserSvc(t *testing.T) (*UserService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewUserService(db), db
}

func TestSetupFlow(t *testing.T) {
	svc, _ := newUserSvc(t)
	if ok, _ := svc.Initialized(); ok {
		t.Fatal("fresh db must be uninitialized")
	}
	u, err := svc.CreateInitialAdmin("Passw0rd!")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if u.Username != "admin" || u.Role != authdom.RoleAdmin {
		t.Fatalf("unexpected: %+v", u)
	}
	if _, err := svc.CreateInitialAdmin("Passw0rd!"); err != ErrAlreadyInitialized {
		t.Fatalf("second setup must fail, got %v", err)
	}
}

func TestAuthenticate(t *testing.T) {
	svc, _ := newUserSvc(t)
	if _, err := svc.Create("alice", "abcd1234", "Alice", authdom.RoleMember); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Authenticate("alice", "wrong-pass"); err != ErrInvalidCredentials {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
	u, err := svc.Authenticate("alice", "abcd1234")
	if err != nil || u.Username != "alice" {
		t.Fatalf("auth: %v %+v", err, u)
	}
}

func TestAuthenticateLockout(t *testing.T) {
	svc, _ := newUserSvc(t)
	svc.Create("bob", "abcd1234", "", authdom.RoleMember)
	for i := 0; i < 5; i++ {
		svc.Authenticate("bob", "bad")
	}
	if _, err := svc.Authenticate("bob", "abcd1234"); err != ErrLocked {
		t.Fatalf("want ErrLocked, got %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	svc, _ := newUserSvc(t)
	if _, err := svc.Create("a", "abcd1234", "", authdom.RoleMember); err != ErrInvalidUsername {
		t.Fatalf("short username: %v", err)
	}
	if _, err := svc.Create("valid_name", "short", "", authdom.RoleMember); err != ErrWeakPassword {
		t.Fatalf("weak pwd: %v", err)
	}
	if _, err := svc.Create("valid_name", "abcd1234", "", "owner"); err == nil {
		t.Fatal("invalid role must fail")
	}
	svc.Create("taken", "abcd1234", "", authdom.RoleMember)
	if _, err := svc.Create("taken", "abcd1234", "", authdom.RoleMember); err != ErrUsernameTaken {
		t.Fatalf("dup: %v", err)
	}
}

func TestLastAdminProtection(t *testing.T) {
	svc, _ := newUserSvc(t)
	admin, _ := svc.CreateInitialAdmin("Passw0rd!")
	if err := svc.UpdateRole(admin.ID, admin.ID, authdom.RoleMember); err == nil {
		t.Fatal("self role change must fail")
	}
	if err := svc.SetStatus(admin.ID, admin.ID, authdom.StatusDisabled); err == nil {
		t.Fatal("self disable must fail")
	}
	second, _ := svc.Create("admin2", "abcd1234", "", authdom.RoleAdmin)
	// Two admins exist: demoting admin by another actor is allowed.
	if err := svc.UpdateRole(admin.ID, second.ID, authdom.RoleMember); err != nil {
		t.Fatalf("demote with two admins should pass: %v", err)
	}
	// Now admin is the only remaining admin and must not be demoted.
	if err := svc.UpdateRole(second.ID, admin.ID, authdom.RoleMaintainer); err != ErrLastAdmin {
		t.Fatalf("want ErrLastAdmin, got %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	svc, _ := newUserSvc(t)
	u, _ := svc.Create("carol", "abcd1234", "", authdom.RoleMember)
	if err := svc.ChangePassword(u.ID, "wrong", "newpass123"); err != ErrInvalidCredentials {
		t.Fatalf("old pwd check: %v", err)
	}
	if err := svc.ChangePassword(u.ID, "abcd1234", "newpass123"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if _, err := svc.Authenticate("carol", "newpass123"); err != nil {
		t.Fatalf("auth with new pwd: %v", err)
	}
}
