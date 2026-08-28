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
