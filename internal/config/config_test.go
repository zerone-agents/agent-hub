package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthConfigDefaults(t *testing.T) {
	t.Setenv("AUTH_MODE", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("AUTH_JWT_SECRET_FILE", filepath.Join(t.TempDir(), ".jwt-secret"))
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.Mode != "builtin" {
		t.Fatalf("default mode = %q, want builtin", cfg.Auth.Mode)
	}
}

// builtin 模式未配置 JWT secret 时 LoadConfig 应自动生成 >=32 字节的随机
// secret 并持久化到 AUTH_JWT_SECRET_FILE，再次加载读回同一值（重启不丢
// 登录态）。
func TestLoadConfigPersistsGeneratedJWTSecret(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "nested", ".jwt-secret")
	t.Setenv("AUTH_MODE", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("AUTH_JWT_SECRET_FILE", secretFile)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Auth.JWTSecret) < 32 {
		t.Fatalf("generated secret too short: %d bytes", len(cfg.Auth.JWTSecret))
	}
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("generated secret must pass validation: %v", err)
	}
	info, err := os.Stat(secretFile)
	if err != nil {
		t.Fatalf("secret file not persisted: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file perm = %o, want 600", info.Mode().Perm())
	}

	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if cfg.Auth.JWTSecret != cfg2.Auth.JWTSecret {
		t.Fatal("expected persisted secret to be reloaded across restarts")
	}
}

// secret 文件不可写时降级为进程内临时随机 secret（不报错，仅 warning），
// 两次加载得到不同值。
func TestLoadConfigEphemeralWhenSecretFileUnwritable(t *testing.T) {
	t.Setenv("AUTH_MODE", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("AUTH_JWT_SECRET_FILE", "/proc/nonexistent-dir/jwt-secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg2, err := LoadConfig()
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if len(cfg.Auth.JWTSecret) < 32 || len(cfg2.Auth.JWTSecret) < 32 {
		t.Fatal("ephemeral secret must still be >=32 bytes")
	}
	if cfg.Auth.JWTSecret == cfg2.Auth.JWTSecret {
		t.Fatal("expected distinct ephemeral secrets when persistence fails")
	}
}

// 显式配置的 secret 不被覆盖，且过短的显式值仍被 ValidateAuth 拒绝。
func TestLoadConfigKeepsExplicitJWTSecret(t *testing.T) {
	t.Setenv("AUTH_MODE", "")
	t.Setenv("AUTH_JWT_SECRET", "my-secret")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Auth.JWTSecret != "my-secret" {
		t.Fatalf("explicit secret overwritten: %q", cfg.Auth.JWTSecret)
	}
	if err := cfg.ValidateAuth(); err == nil {
		t.Fatal("want validation error for explicit short secret")
	}
}

func TestLoadOrCreateJWTSecret(t *testing.T) {
	dir := t.TempDir()

	// 已存在的合法文件：直接读回
	valid := filepath.Join(dir, "valid")
	existing := strings.Repeat("b", 64)
	if err := os.WriteFile(valid, []byte(existing+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadOrCreateJWTSecret(valid)
	if err != nil || got != existing {
		t.Fatalf("read persisted: got %q err %v", got, err)
	}

	// 内容过短的文件：拒绝（防止截断文件静默削弱签名）
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateJWTSecret(short); err == nil {
		t.Fatal("want error for short persisted secret")
	}

	// 空路径：报错（调用方降级 ephemeral）
	if _, err := loadOrCreateJWTSecret(""); err == nil {
		t.Fatal("want error for empty path")
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
	cfg := &Config{Auth: AuthConfig{Mode: "casdoor"}, Casdoor: CasdoorConfig{Organization: "zerone"}}
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("casdoor mode must not require jwt_secret: %v", err)
	}
}

// TestValidateAuthCasdoorOrganizationOptional：Organization 不再必填——它只是
// 存量数据回填目标的一次性显式覆盖（升级逃生舱），未配置时 AutoMigrate 从
// user_identities 自动推断。
func TestValidateAuthCasdoorOrganizationOptional(t *testing.T) {
	cfg := &Config{Auth: AuthConfig{Mode: "casdoor"}, Casdoor: CasdoorConfig{Organization: ""}}
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("casdoor mode must not require organization: %v", err)
	}
	cfg.Casdoor.Organization = "zerone"
	if err := cfg.ValidateAuth(); err != nil {
		t.Fatalf("unexpected: %v", err)
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
