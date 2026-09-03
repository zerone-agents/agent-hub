package systemsetting

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&SystemSetting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// 未显式配置时应生成 >=32 字节随机 secret 并落库；再次调用（模拟重启）
// 读回同一值——登录态跨重启保持。
func TestEnsureJWTSecretGeneratesAndPersists(t *testing.T) {
	db := newTestDB(t)

	first, err := EnsureJWTSecret(db, "")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if len(first) < 32 {
		t.Fatalf("generated secret too short: %d bytes", len(first))
	}

	second, err := EnsureJWTSecret(db, "")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if first != second {
		t.Fatal("expected persisted secret to be reused across restarts")
	}
}

// 显式配置的 secret 原样返回、不写库。
func TestEnsureJWTSecretKeepsExplicit(t *testing.T) {
	db := newTestDB(t)
	got, err := EnsureJWTSecret(db, strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got != strings.Repeat("a", 64) {
		t.Fatalf("explicit secret modified: %q", got)
	}
	var count int64
	db.Model(&SystemSetting{}).Count(&count)
	if count != 0 {
		t.Fatalf("explicit secret must not be persisted, got %d rows", count)
	}
}

// 库中存量值过短（损坏/误写）时拒绝启动，而非静默削弱签名强度。
func TestEnsureJWTSecretRejectsShortPersistedValue(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&SystemSetting{Key: JWTSecretKey, Value: "too-short"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureJWTSecret(db, ""); err == nil {
		t.Fatal("want error for short persisted secret")
	}
}

// 未显式配置时应生成 >=32 字节随机 capability secret 并落库；再次调用
// （模拟重启）读回同一值——已部署的 capability 签发/验证两侧跨重启保持
// 同源（issue #111 重开修复）。
func TestEnsureKnowledgeCapabilitySecretGeneratesAndPersists(t *testing.T) {
	db := newTestDB(t)

	first, err := EnsureKnowledgeCapabilitySecret(db, "")
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if len(first) < 32 {
		t.Fatalf("generated secret too short: %d bytes", len(first))
	}

	second, err := EnsureKnowledgeCapabilitySecret(db, "")
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if first != second {
		t.Fatal("expected persisted secret to be reused across restarts")
	}
}

// 显式配置的 capability secret 原样返回、不写库（与 AUTH_JWT_SECRET 对称）。
func TestEnsureKnowledgeCapabilitySecretKeepsExplicit(t *testing.T) {
	db := newTestDB(t)
	got, err := EnsureKnowledgeCapabilitySecret(db, strings.Repeat("a", 64))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got != strings.Repeat("a", 64) {
		t.Fatalf("explicit secret modified: %q", got)
	}
	var count int64
	db.Model(&SystemSetting{}).Count(&count)
	if count != 0 {
		t.Fatalf("explicit secret must not be persisted, got %d rows", count)
	}
}

// 库中存量值过短（损坏/误写）时拒绝，而非静默削弱签名强度。
func TestEnsureKnowledgeCapabilitySecretRejectsShortPersistedValue(t *testing.T) {
	db := newTestDB(t)
	if err := db.Create(&SystemSetting{Key: KnowledgeCapabilitySecretKey, Value: "too-short"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureKnowledgeCapabilitySecret(db, ""); err == nil {
		t.Fatal("want error for short persisted secret")
	}
}

// 显式配置的 capability secret 过短时必须拒绝（PR #118 二轮 review P1：
// 显式值须过与自动值相同的 ≥32 字节强度校验——否则
// KNOWLEDGE_CAPABILITY_SECRET=a 可正常启动并签发全部 HMAC capability，
// 签名密钥可被离线穷举）。校验先于任何 DB 访问，拒绝路径不得写库。
func TestEnsureKnowledgeCapabilitySecretRejectsShortExplicit(t *testing.T) {
	cases := []struct {
		name     string
		provided string
	}{
		{"1 字节", "a"},
		{"31 字节（差一字节）", strings.Repeat("a", 31)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			_, err := EnsureKnowledgeCapabilitySecret(db, tc.provided)
			if err == nil {
				t.Fatal("want error for short explicit capability secret")
			}
			if !strings.Contains(err.Error(), "KNOWLEDGE_CAPABILITY_SECRET") {
				t.Fatalf("error should name the env var to fix, got: %v", err)
			}
			var count int64
			db.Model(&SystemSetting{}).Count(&count)
			if count != 0 {
				t.Fatalf("rejected secret must not touch db, got %d rows", count)
			}
		})
	}
}

// 恰好 32 字节的显式 capability secret 通过且原样返回；长度按字节计
// （len()），16 个三字节字符 = 48 字节同样通过。
func TestEnsureKnowledgeCapabilitySecretAcceptsExactMinExplicit(t *testing.T) {
	for _, provided := range []string{strings.Repeat("a", 32), strings.Repeat("密", 16)} {
		db := newTestDB(t)
		got, err := EnsureKnowledgeCapabilitySecret(db, provided)
		if err != nil {
			t.Fatalf("ensure %q: %v", provided, err)
		}
		if got != provided {
			t.Fatalf("explicit secret modified: %q", got)
		}
		var count int64
		db.Model(&SystemSetting{}).Count(&count)
		if count != 0 {
			t.Fatalf("explicit secret must not be persisted, got %d rows", count)
		}
	}
}

// EnsureJWTSecret 的显式路径原先同样不校验长度（生产链路由 main.go 在
// builtin 模式下的 ValidateAuth 兜底，且仅在 JWTSecret 为空时才调用）。
// 抽取 ensureSecret 后统一执行 ≥32 字节校验，防御直接传非空 provided
// 的未来调用方。
func TestEnsureJWTSecretRejectsShortExplicit(t *testing.T) {
	cases := []struct {
		name     string
		provided string
	}{
		{"1 字节", "a"},
		{"31 字节（差一字节）", strings.Repeat("a", 31)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			_, err := EnsureJWTSecret(db, tc.provided)
			if err == nil {
				t.Fatal("want error for short explicit JWT secret")
			}
			if !strings.Contains(err.Error(), "AUTH_JWT_SECRET") {
				t.Fatalf("error should name the env var to fix, got: %v", err)
			}
			var count int64
			db.Model(&SystemSetting{}).Count(&count)
			if count != 0 {
				t.Fatalf("rejected secret must not touch db, got %d rows", count)
			}
		})
	}
}

// 恰好 32 字节的显式 JWT secret 通过且原样返回（与 builtin 模式
// ValidateAuth 的 32 字节下限一致）。
func TestEnsureJWTSecretAcceptsExactMinExplicit(t *testing.T) {
	for _, provided := range []string{strings.Repeat("a", 32), strings.Repeat("密", 16)} {
		db := newTestDB(t)
		got, err := EnsureJWTSecret(db, provided)
		if err != nil {
			t.Fatalf("ensure %q: %v", provided, err)
		}
		if got != provided {
			t.Fatalf("explicit secret modified: %q", got)
		}
		var count int64
		db.Model(&SystemSetting{}).Count(&count)
		if count != 0 {
			t.Fatalf("explicit secret must not be persisted, got %d rows", count)
		}
	}
}
