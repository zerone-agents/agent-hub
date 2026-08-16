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

func TestLoadConfig_CasdoorRoleMappingEnv(t *testing.T) {
	t.Setenv("AUTH_MODE", "casdoor")
	t.Setenv("CASDOOR_ROLE_MAPPING", "admin=agent-hub-admin, maintainer=agent-hub-maintainer ,member=agent-hub-member")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := map[string]string{
		"admin":      "agent-hub-admin",
		"maintainer": "agent-hub-maintainer",
		"member":     "agent-hub-member",
	}
	if len(cfg.Casdoor.RoleMapping) != len(want) {
		t.Fatalf("mapping = %v, want %v", cfg.Casdoor.RoleMapping, want)
	}
	for k, v := range want {
		if cfg.Casdoor.RoleMapping[k] != v {
			t.Fatalf("mapping[%q] = %q, want %q", k, cfg.Casdoor.RoleMapping[k], v)
		}
	}
}

func TestLoadConfig_CasdoorRoleMappingEnvInvalid(t *testing.T) {
	for _, raw := range []string{"admin", "admin=", "=x", "admin=a,,member=b"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("AUTH_MODE", "casdoor")
			t.Setenv("CASDOOR_ROLE_MAPPING", raw)
			if _, err := LoadConfig(); err == nil {
				t.Fatalf("want error for %q", raw)
			}
		})
	}
}

func TestValidateAuth_CasdoorRoleMapping(t *testing.T) {
	c := &Config{}
	c.Auth.Mode = "casdoor"
	// valid keys pass
	c.Casdoor.RoleMapping = map[string]string{"admin": "a", "member": "m"}
	if err := c.ValidateAuth(); err != nil {
		t.Fatalf("valid mapping rejected: %v", err)
	}
	// unknown hub role key rejected
	c.Casdoor.RoleMapping = map[string]string{"superuser": "a"}
	if err := c.ValidateAuth(); err == nil {
		t.Fatal("unknown mapping key accepted")
	}
	// duplicate casdoor role values rejected
	c.Casdoor.RoleMapping = map[string]string{"admin": "same", "member": "same"}
	if err := c.ValidateAuth(); err == nil {
		t.Fatal("duplicate mapping values accepted")
	}
}

func TestValidateAuth_CasdoorDefaultRole(t *testing.T) {
	c := &Config{}
	c.Auth.Mode = "casdoor"
	c.Casdoor.DefaultRole = "member"
	if err := c.ValidateAuth(); err != nil {
		t.Fatalf("valid default_role rejected: %v", err)
	}
	c.Casdoor.DefaultRole = "superuser"
	if err := c.ValidateAuth(); err == nil {
		t.Fatal("invalid default_role accepted")
	}
	c.Casdoor.DefaultRole = ""
	if err := c.ValidateAuth(); err != nil {
		t.Fatalf("empty default_role must be allowed: %v", err)
	}
}
