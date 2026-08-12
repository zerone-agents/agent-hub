package config

import (
	"strings"
	"testing"
)

func TestAuthConfigDefaults(t *testing.T) {
	t.Setenv("AUTH_MODE", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Mode != "builtin" {
		t.Fatalf("default mode = %q, want builtin", cfg.Auth.Mode)
	}
}

func TestValidateAuthBuiltinRequiresSecret(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{Mode: "builtin", JWTSecret: ""}}
	if err := cfg.ValidateAuth(); err == nil {
		t.Fatal("want error for empty secret")
	}
	cfg.Auth.JWTSecret = "short"
	if err := cfg.ValidateAuth(); err == nil {
		t.Fatal("want error for <32-byte secret")
	}
	cfg.Auth.JWTSecret = strings.Repeat("a", 32)
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateAuthCasdoorSkipsSecret(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{Mode: "casdoor"}}
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("casdoor mode must not require jwt_secret: %v", err)
	}
}

func TestValidateAuthRejectsUnknownMode(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{Mode: "ldap"}}
	if err := cfg.ValidateAuth(); err == nil {
		t.Fatal("want error for unknown mode")
	}
}
