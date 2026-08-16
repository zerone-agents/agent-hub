package config

import (
	"bytes"
	"log"
	"os"
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

func TestDeprecatedRoleMappingEnvWarns(t *testing.T) {
	t.Setenv("AUTH_MODE", "casdoor")
	t.Setenv("CASDOOR_ROLE_MAPPING", "admin=agent-hub-admin")
	t.Setenv("CASDOOR_DEFAULT_ROLE", "member")

	// 捕获标准日志，断言废弃警告被记录
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// 废弃 env 不应导致加载失败（给线上留清理窗口）
	if _, err := LoadConfig(); err != nil {
		t.Fatalf("废弃 env 不应导致 LoadConfig 失败: %v", err)
	}

	out := buf.String()
	for _, name := range []string{"CASDOOR_ROLE_MAPPING", "CASDOOR_DEFAULT_ROLE"} {
		if !strings.Contains(out, name) || !strings.Contains(out, "已废弃") {
			t.Fatalf("未记录 %s 的废弃警告，日志输出: %q", name, out)
		}
	}
}
