package auth

import (
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

func TestNormalizeCasdoorUser(t *testing.T) {
	u := &casdoorsdk.User{
		Id:          "u-123",
		Name:        "alice",
		Email:       "a@b.c",
		DisplayName: "Alice",
		Avatar:      "https://x/y.png",
		Owner:       "acme",
		Roles:       []*casdoorsdk.Role{{Name: "agents-admin"}},
	}
	au := NormalizeCasdoorUser(u)
	if au.ID != "u-123" || au.Username != "alice" || au.DisplayName != "Alice" {
		t.Fatalf("unexpected: %+v", au)
	}
	if len(au.Roles) != 1 || au.Roles[0] != "admin" {
		t.Fatalf("roles = %v", au.Roles)
	}
	if au.TenantID != "acme" {
		t.Fatalf("TenantID = %q, want casdoor owner %q", au.TenantID, "acme")
	}
}

func TestCasdoorProviderMode(t *testing.T) {
	p := NewCasdoorProvider(nil, "")
	if p.Mode() != "casdoor" {
		t.Fatalf("mode = %q", p.Mode())
	}
	if _, err := p.RefreshToken(""); err == nil {
		t.Fatal("empty refresh must error")
	}
}

func TestCasdoorProviderNormalizeUser_StrictMapping(t *testing.T) {
	p := NewCasdoorProvider(map[string]string{
		"admin":  "agent-hub-admin",
		"member": "agent-hub-member",
	}, "")
	u := &casdoorsdk.User{Id: "id1", Name: "alice", Owner: "tenant-acme",
		Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}}}
	got, err := p.NormalizeUser(u)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.TenantID != "tenant-acme" {
		t.Fatalf("tenant %q", got.TenantID)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Fatalf("roles %v", got.Roles)
	}

	// unmatched role, no default -> error
	u2 := &casdoorsdk.User{Id: "id2", Name: "bob", Owner: "tenant-acme",
		Roles: []*casdoorsdk.Role{{Name: "random"}}}
	if _, err := p.NormalizeUser(u2); err == nil {
		t.Fatal("expected ErrNoMatchedRole")
	}
}

func TestCasdoorProviderNormalizeUser_LegacyFallback(t *testing.T) {
	p := NewCasdoorProvider(nil, "") // no mapping -> legacy substring behavior
	u := &casdoorsdk.User{Id: "id1", Name: "alice", Owner: "org1",
		Roles: []*casdoorsdk.Role{{Name: "site-admin"}}}
	got, err := p.NormalizeUser(u)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Roles[0] != "admin" {
		t.Fatalf("legacy fallback broken: %v", got.Roles)
	}
}
