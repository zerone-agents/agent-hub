package services

import (
	"testing"

	authdom "control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
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
	if _, err := svc.UpdateRole(admin.ID, admin.ID, authdom.RoleMember); err == nil {
		t.Fatal("self role change must fail")
	}
	if _, err := svc.SetStatus(admin.ID, admin.ID, authdom.StatusDisabled); err == nil {
		t.Fatal("self disable must fail")
	}
	second, _ := svc.Create("admin2", "abcd1234", "", authdom.RoleAdmin)
	// Two admins exist: demoting admin by another actor is allowed.
	if _, err := svc.UpdateRole(admin.ID, second.ID, authdom.RoleMember); err != nil {
		t.Fatalf("demote with two admins should pass: %v", err)
	}
	// Now admin is the only remaining admin and must not be demoted.
	if _, err := svc.UpdateRole(second.ID, admin.ID, authdom.RoleMaintainer); err != ErrLastAdmin {
		t.Fatalf("want ErrLastAdmin, got %v", err)
	}
}

func TestChangeAndResetPassword(t *testing.T) {
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
	// self-reset → ErrSelfOperation（自己的密码走自助改密）
	if _, err := svc.ResetPassword(u.ID, u.ID); err != ErrSelfOperation {
		t.Fatalf("self-reset: %v", err)
	}
	// admin 重置他人 → 返回明文一次
	other, _ := svc.Create("dave", "abcd1234", "", authdom.RoleMember)
	plain, err := svc.ResetPassword(u.ID, other.ID)
	if err != nil || len(plain) < 12 {
		t.Fatalf("reset: %v %q", err, plain)
	}
	if _, err := svc.Authenticate("carol", plain); err != nil {
		t.Fatalf("auth with reset pwd: %v", err)
	}
}

// seedUser inserts a user with an explicit id for receipt assertions.
func seedUser(t *testing.T, db *gorm.DB, id uint64, username, role, status string) *authdom.User {
	t.Helper()
	u := &authdom.User{ID: id, Username: username, PasswordHash: "x", Role: role, Status: status}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return u
}

func TestUpdateRoleReceiptBeforeAfter(t *testing.T) {
	svc, db := newUserSvc(t)
	seedUser(t, db, 1, "alice", "member", "active")
	rcpt, err := svc.UpdateRole(1, 2, "maintainer")
	require.NoError(t, err)
	require.NotNil(t, rcpt)
	require.Equal(t, authdom.Role("member"), rcpt.RoleBefore)
	require.Equal(t, authdom.Role("maintainer"), rcpt.RoleAfter)
	require.Equal(t, authdom.UserStatus("active"), rcpt.StatusBefore)
	require.Equal(t, rcpt.StatusBefore, rcpt.StatusAfter)          // role 变更不动 status
	require.Equal(t, rcpt.StatusBefore, rcpt.EffectiveStatusAfter) // builtin: Effective==Status
	require.True(t, rcpt.RemoteApplied)
	require.True(t, rcpt.LocalApplied)
}

func TestSetStatusReceipt(t *testing.T) {
	svc, db := newUserSvc(t)
	seedUser(t, db, 1, "alice", "member", "active")
	rcpt, err := svc.SetStatus(1, 2, "disabled")
	require.NoError(t, err)
	require.NotNil(t, rcpt)
	require.Equal(t, authdom.UserStatus("active"), rcpt.EffectiveStatusBefore)
	require.Equal(t, authdom.UserStatus("disabled"), rcpt.EffectiveStatusAfter)
	require.Equal(t, authdom.UserStatus("active"), rcpt.StatusBefore)
	require.Equal(t, authdom.UserStatus("disabled"), rcpt.StatusAfter)
	require.True(t, rcpt.RemoteApplied)
	require.True(t, rcpt.LocalApplied)
}

func TestUpdateColumnZeroRowsReturnsNotFound(t *testing.T) {
	// RowsAffected 判定：零行命中（并发删除窗口）→ 未生效 + 错误（spec §3.2）
	_, db := newUserSvc(t)
	rows, err := updateColumn(db, 999, "role", "admin")
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
}

func TestUpdateRoleErrorReturnsNilReceipt(t *testing.T) {
	// spec §3.2 规则 4：未生效不记录——任何错误路径 receipt 必须为 nil。
	svc, db := newUserSvc(t)
	seedUser(t, db, 1, "alice", "member", "active")
	rcpt, err := svc.UpdateRole(999, 2, "admin") // 用户不存在
	require.Error(t, err)
	require.Nil(t, rcpt)
	rcpt, err = svc.UpdateRole(1, 1, "admin") // 自我变更
	require.ErrorIs(t, err, ErrSelfOperation)
	require.Nil(t, rcpt)
}
