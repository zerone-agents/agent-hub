package builtin

import (
	"strconv"
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/tenant"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const testSecret = "test-secret-test-secret-test-secret!!"

func newTestProvider(t *testing.T) (*Provider, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.User{}, &authdom.RefreshToken{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db, testSecret), db
}

func seedUser(t *testing.T, db *gorm.DB, username, role, status string) *authdom.User {
	t.Helper()
	u := &authdom.User{Username: username, PasswordHash: "x", Role: role, Status: status}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return u
}

func TestIssueAndValidateAccessToken(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "alice", authdom.RoleMaintainer, authdom.StatusActive)

	pair, err := p.IssueTokenPair(u)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if pair.ExpiresIn != 7200 {
		t.Fatalf("expiresIn = %d, want 7200", pair.ExpiresIn)
	}
	au, err := p.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if au.ID != strconv.FormatUint(u.ID, 10) || au.Username != "alice" {
		t.Fatalf("unexpected user: %+v", au)
	}
	if len(au.Roles) != 1 || au.Roles[0] != authdom.RoleMaintainer {
		t.Fatalf("roles = %v", au.Roles)
	}
	if au.TenantID != tenant.DefaultID {
		t.Fatalf("builtin TenantID = %q, want %q", au.TenantID, tenant.DefaultID)
	}
}

func TestValidateRejectsGarbage(t *testing.T) {
	p, _ := newTestProvider(t)
	if _, err := p.ValidateAccessToken("not-a-jwt"); err == nil {
		t.Fatal("want error")
	}
}

func TestValidateRejectsDisabledUser(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "zoe", authdom.RoleMember, authdom.StatusActive)
	pair, _ := p.IssueTokenPair(u)
	db.Model(&authdom.User{}).Where("id = ?", u.ID).Update("status", authdom.StatusDisabled)
	if _, err := p.ValidateAccessToken(pair.AccessToken); err == nil {
		t.Fatal("disabled user's access token must be rejected")
	}
}

func TestRefreshRotates(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "bob", authdom.RoleMember, authdom.StatusActive)
	pair, _ := p.IssueTokenPair(u)

	pair2, err := p.RefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	// Old refresh token must be invalidated after rotation.
	if _, err := p.RefreshToken(pair.RefreshToken); err == nil {
		t.Fatal("old refresh token must be invalid after rotation")
	}
	if _, err := p.ValidateAccessToken(pair2.AccessToken); err != nil {
		t.Fatalf("new access token invalid: %v", err)
	}
}

func TestRevokeAllForUser(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "carol", authdom.RoleAdmin, authdom.StatusActive)
	pair, _ := p.IssueTokenPair(u)
	if err := p.RevokeAllForUser(u.ID); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	if _, err := p.RefreshToken(pair.RefreshToken); err == nil {
		t.Fatal("refresh must fail after RevokeAllForUser")
	}
}

func TestGetUserIdentityDisabledUser(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "dave", authdom.RoleAdmin, authdom.StatusDisabled)
	if _, ok := p.GetUserIdentity(strconv.FormatUint(u.ID, 10)); ok {
		t.Fatal("disabled user must report not-found")
	}
}

func TestRefreshExpired(t *testing.T) {
	p, db := newTestProvider(t)
	u := seedUser(t, db, "erin", authdom.RoleMember, authdom.StatusActive)
	pair, _ := p.IssueTokenPair(u)
	// Force the refresh token to be expired.
	db.Model(&authdom.RefreshToken{}).Where("user_id = ?", u.ID).
		Update("expires_at", time.Now().Add(-time.Hour))
	if _, err := p.RefreshToken(pair.RefreshToken); err == nil {
		t.Fatal("expired refresh must fail")
	}
}

func TestRevokeMissingIsNoop(t *testing.T) {
	p, _ := newTestProvider(t)
	if err := p.RevokeToken("rt_doesnotexist"); err != nil {
		t.Fatalf("revoking a missing token must not error, got %v", err)
	}
	if err := p.RevokeToken(""); err != nil {
		t.Fatalf("revoking empty token must not error, got %v", err)
	}
}
