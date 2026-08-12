package services

import (
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newInviteSvc(t *testing.T) *InviteService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&authdom.Invite{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewInviteService(db)
}

func TestInviteLifecycle(t *testing.T) {
	svc := newInviteSvc(t)
	res, err := svc.Create(authdom.RoleMember, "给张三", 1, 0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(res.Token) < 36 || res.Token[:4] != "inv_" {
		t.Fatalf("bad token: %q", res.Token)
	}
	if time.Until(res.ExpiresAt) < 6*24*time.Hour {
		t.Fatal("default ttl should be ~7d")
	}

	inv, err := svc.Validate(res.Token)
	if err != nil || inv.Role != authdom.RoleMember {
		t.Fatalf("validate: %v %+v", err, inv)
	}
	if _, err := svc.Validate("inv_nonexistent"); err != ErrInviteInvalid {
		t.Fatalf("want ErrInviteInvalid, got %v", err)
	}

	used, err := svc.Consume(res.Token)
	if err != nil || used.UsedAt == nil {
		t.Fatalf("consume: %v %+v", err, used)
	}
	if _, err := svc.Validate(res.Token); err != ErrInviteInvalid {
		t.Fatalf("used invite must be invalid, got %v", err)
	}
	if _, err := svc.Consume(res.Token); err != ErrInviteInvalid {
		t.Fatalf("double consume must fail, got %v", err)
	}
}

func TestInviteRevoke(t *testing.T) {
	svc := newInviteSvc(t)
	res, _ := svc.Create(authdom.RoleAdmin, "", 1, 0)
	list, _ := svc.List()
	if len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}
	if err := svc.Revoke(list[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.Validate(res.Token); err != ErrInviteInvalid {
		t.Fatalf("revoked invite must be invalid, got %v", err)
	}
}

func TestInviteExpired(t *testing.T) {
	svc := newInviteSvc(t)
	res, _ := svc.Create(authdom.RoleMember, "", 1, 1)
	svc.db.Model(&authdom.Invite{}).Where("token_hash = ?", HashToken(res.Token)).
		Update("expires_at", time.Now().Add(-time.Hour))
	if _, err := svc.Validate(res.Token); err != ErrInviteInvalid {
		t.Fatalf("expired invite must be invalid, got %v", err)
	}
}

func TestInviteTTLLimits(t *testing.T) {
	svc := newInviteSvc(t)
	if _, err := svc.Create(authdom.RoleMember, "", 1, 31); err == nil {
		t.Fatal("ttl > 30 must be rejected")
	}
	if _, err := svc.Create("owner", "", 1, 0); err == nil {
		t.Fatal("invalid role must be rejected")
	}
}
