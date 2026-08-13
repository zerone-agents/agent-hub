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
		Roles:       []*casdoorsdk.Role{{Name: "agents-admin"}},
	}
	au := NormalizeCasdoorUser(u)
	if au.ID != "u-123" || au.Username != "alice" || au.DisplayName != "Alice" {
		t.Fatalf("unexpected: %+v", au)
	}
	if len(au.Roles) != 1 || au.Roles[0] != "admin" {
		t.Fatalf("roles = %v", au.Roles)
	}
}

func TestCasdoorProviderMode(t *testing.T) {
	p := NewCasdoorProvider()
	if p.Mode() != "casdoor" {
		t.Fatalf("mode = %q", p.Mode())
	}
	if _, err := p.RefreshToken(""); err == nil {
		t.Fatal("empty refresh must error")
	}
}
