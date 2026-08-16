package auth

import (
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

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

func TestCasdoorProviderNormalizeUser_DefaultMapping(t *testing.T) {
	p := NewCasdoorProvider(nil, "") // no mapping -> DefaultCasdoorRoleMapping
	u := &casdoorsdk.User{Id: "id1", Name: "alice", Owner: "org1",
		Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}}}
	got, err := p.NormalizeUser(u)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Fatalf("roles %v", got.Roles)
	}

	// role outside the default mapping, no defaultRole -> error
	u2 := &casdoorsdk.User{Id: "id2", Name: "bob", Owner: "org1",
		Roles: []*casdoorsdk.Role{{Name: "site-admin"}}}
	if _, err := p.NormalizeUser(u2); err == nil {
		t.Fatal("expected ErrNoMatchedRole for unmapped role")
	}
}

func TestCasdoorProviderNormalizeIdentity_ActiveUser(t *testing.T) {
	p := NewCasdoorProvider(nil, "")
	u := &casdoorsdk.User{Id: "id1", Name: "alice", Owner: "org1",
		Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}}}
	au, ok := p.normalizeIdentity(u)
	if !ok {
		t.Fatal("expected ok for active user")
	}
	if au == nil || au.ID != "id1" || au.TenantID != "org1" {
		t.Fatalf("unexpected identity %+v", au)
	}
}

func TestCasdoorProviderNormalizeIdentity_ForbiddenUser(t *testing.T) {
	p := NewCasdoorProvider(nil, "")
	u := &casdoorsdk.User{Id: "id2", Name: "bob", Owner: "org1", IsForbidden: true,
		Roles: []*casdoorsdk.Role{{Name: "agent-hub-admin"}}}
	if _, ok := p.normalizeIdentity(u); ok {
		t.Fatal("disabled (IsForbidden) casdoor user must be rejected")
	}
}

func TestCasdoorProviderNormalizeIdentity_NormalizeFailure(t *testing.T) {
	p := NewCasdoorProvider(nil, "")
	// unmapped role, no defaultRole -> NormalizeUser error -> not ok
	u := &casdoorsdk.User{Id: "id3", Name: "carol", Owner: "org1",
		Roles: []*casdoorsdk.Role{{Name: "site-admin"}}}
	if _, ok := p.normalizeIdentity(u); ok {
		t.Fatal("user whose roles fail normalization must be rejected")
	}
}
