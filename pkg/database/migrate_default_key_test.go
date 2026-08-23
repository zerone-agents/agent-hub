package database

import (
	"testing"

	authdom "control-panel/internal/domain/auth"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSentinelMigrationDB(t *testing.T, seed string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&authdom.TenantOAuthClient{}); err != nil {
		t.Fatal(err)
	}
	if seed != "" {
		if err := db.Exec(seed).Error; err != nil {
			t.Fatal(err)
		}
	}
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })
}

func TestMigrateTenantDefaultKeySentinel_NormalizesLegacyValue(t *testing.T) {
	// 存量形态：default 行的 default_key 存 org 名（历史版本写入）。
	setupSentinelMigrationDB(t, `INSERT INTO tenant_oauth_clients (org, client_id, client_secret_enc, cert_enc, default_key, created_at, updated_at)
		VALUES ('zerone', 'cid', 'sec', '', 'zerone', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	if err := migrateTenantDefaultKeySentinel(); err != nil {
		t.Fatal(err)
	}
	var row authdom.TenantOAuthClient
	if err := DB.Where("org = ?", "zerone").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.DefaultKey == nil || *row.DefaultKey != authdom.DefaultKeySentinel {
		t.Fatalf("default_key should be sentinel, got %v", row.DefaultKey)
	}
}

func TestMigrateTenantDefaultKeySentinel_HealsCorruption(t *testing.T) {
	// 竞态污染形态：两行 default（历史 bug 产物）→ 保留 MIN(org) 一行。
	setupSentinelMigrationDB(t, `INSERT INTO tenant_oauth_clients (org, client_id, client_secret_enc, cert_enc, default_key, created_at, updated_at)
		VALUES ('ayu', 'cid', 'sec', '', 'ayu', '2026-01-01 00:00:00', '2026-01-01 00:00:00'),
		       ('zerone', 'cid', 'sec', '', 'zerone', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	if err := migrateTenantDefaultKeySentinel(); err != nil {
		t.Fatal(err)
	}
	var n int64
	DB.Model(&authdom.TenantOAuthClient{}).Where("default_key IS NOT NULL").Count(&n)
	if n != 1 {
		t.Fatalf("want exactly 1 default row, got %d", n)
	}
	var row authdom.TenantOAuthClient
	if err := DB.Where("default_key IS NOT NULL").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Org != "ayu" { // MIN(org)：ayu < zerone
		t.Fatalf("should keep MIN(org) row, got %s", row.Org)
	}
}

func TestMigrateTenantDefaultKeySentinel_Idempotent(t *testing.T) {
	setupSentinelMigrationDB(t, `INSERT INTO tenant_oauth_clients (org, client_id, client_secret_enc, cert_enc, default_key, created_at, updated_at)
		VALUES ('zerone', 'cid', 'sec', '', 'default', '2026-01-01 00:00:00', '2026-01-01 00:00:00')`)
	if err := migrateTenantDefaultKeySentinel(); err != nil {
		t.Fatal(err)
	}
	if err := migrateTenantDefaultKeySentinel(); err != nil { // 重复跑
		t.Fatal(err)
	}
	var n int64
	DB.Model(&authdom.TenantOAuthClient{}).Where("default_key IS NOT NULL").Count(&n)
	if n != 1 {
		t.Fatalf("idempotent rerun broke invariant: %d", n)
	}
}
