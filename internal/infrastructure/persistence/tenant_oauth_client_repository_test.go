package repository

import (
	"errors"
	"strings"
	"testing"

	authdom "control-panel/internal/domain/auth"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTenantOAuthClientDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&authdom.TenantOAuthClient{}); err != nil {
		t.Fatal(err)
	}
	orig := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = orig })
	return db
}

func TestTenantOAuthClientUpsertAndFind(t *testing.T) {
	setupTenantOAuthClientDB(t)
	repo := NewTenantOAuthClientRepository()

	// 未注册 org 返回 (nil, nil)
	got, err := repo.Find("org-a")
	if err != nil || got != nil {
		t.Fatalf("Find unregistered: got (%v, %v), want (nil, nil)", got, err)
	}

	// 首次 Upsert
	if err := repo.Upsert("org-a", "client-1", "secret-enc-1", "cert-enc-1", true); err != nil {
		t.Fatal(err)
	}
	row, err := repo.Find("org-a")
	if err != nil || row == nil {
		t.Fatalf("Find after upsert: got (%v, %v)", row, err)
	}
	if row.ClientID != "client-1" || row.ClientSecretEnc != "secret-enc-1" || row.CertEnc != "cert-enc-1" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.DefaultKey == nil || *row.DefaultKey != authdom.DefaultKeySentinel {
		t.Fatalf("org-a should be default (sentinel): %+v", row.DefaultKey)
	}
	firstCreatedAt := row.CreatedAt
	if firstCreatedAt.IsZero() {
		t.Fatal("CreatedAt after first upsert should be non-zero")
	}
	if n, err := repo.Count(); err != nil || n != 1 {
		t.Fatalf("Count: got (%d, %v), want 1", n, err)
	}

	// 二次 Upsert 同 org = 更新
	if err := repo.Upsert("org-a", "client-2", "secret-enc-2", "", false); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.Count(); n != 1 {
		t.Fatalf("Count after re-upsert: got %d, want 1", n)
	}
	row, err = repo.Find("org-a")
	if err != nil || row.ClientID != "client-2" {
		t.Fatalf("Find after re-upsert: got (%+v, %v)", row, err)
	}
	// 更新已有行不得覆写 created_at
	if !row.CreatedAt.Equal(firstCreatedAt) {
		t.Fatalf("CreatedAt should be preserved on update: got %v, want %v", row.CreatedAt, firstCreatedAt)
	}
	// org-a 仍是 default（isDefault=false 不应摘掉 default 标记，因它是唯一行）
	if d, _ := repo.FindDefault(); d == nil || d.Org != "org-a" {
		t.Fatalf("FindDefault after re-upsert: got %+v, want org-a", d)
	}
}

func TestTenantOAuthClientUpsertFirstRowAutoDefault(t *testing.T) {
	setupTenantOAuthClientDB(t)
	repo := NewTenantOAuthClientRepository()

	// 空表 + isDefault=false → 事务内自动提升为 default（原 handler 层逻辑移入）。
	if err := repo.Upsert("org-a", "cid", "sec", "", false); err != nil {
		t.Fatal(err)
	}
	d, err := repo.FindDefault()
	if err != nil || d == nil || d.Org != "org-a" {
		t.Fatalf("FindDefault: got (%v, %v), want org-a", d, err)
	}
	// default_key 的值必须是哨兵常量，不是 org 名（唯一索引兜底的前提）。
	row, _ := repo.Find("org-a")
	if row.DefaultKey == nil || *row.DefaultKey != authdom.DefaultKeySentinel {
		t.Fatalf("default_key should be sentinel %q, got %v", authdom.DefaultKeySentinel, row.DefaultKey)
	}
}

func TestIsDefaultKeyConflict(t *testing.T) {
	cases := map[string]bool{
		// sqlite（glebarez）格式
		"UNIQUE constraint failed: tenant_oauth_clients.default_key": true,
		// MySQL 格式（错误链文本含索引名）
		"Error 1062: Duplicate entry 'default' for key 'tenant_oauth_clients.uk_default_key'": true,
		// 主键（org）冲突不含 "default_key"，不得误判
		"UNIQUE constraint failed: tenant_oauth_clients.org":                        false,
		"Error 1062: Duplicate entry 'orga' for key 'tenant_oauth_clients.PRIMARY'": false,
		"some other error": false,
	}
	for msg, want := range cases {
		if got := isDefaultKeyConflict(errors.New(msg)); got != want {
			t.Fatalf("isDefaultKeyConflict(%q) = %v, want %v", msg, got, want)
		}
	}
	if isDefaultKeyConflict(nil) {
		t.Fatal("nil error should not match")
	}
}

func TestTenantOAuthClientDefaultGuards(t *testing.T) {
	setupTenantOAuthClientDB(t)
	repo := NewTenantOAuthClientRepository()

	if err := repo.Upsert("org-a", "c-a", "s", "", true); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert("org-b", "c-b", "s", "", false); err != nil {
		t.Fatal(err)
	}

	// 两个 org 存在时，把 default 行降级 → ErrDefaultRequired
	err := repo.Upsert("org-a", "c-a2", "s", "", false)
	if err == nil || !strings.Contains(err.Error(), "default 租户必须存在且唯一") {
		t.Fatalf("demoting default with other rows should fail, got: %v", err)
	}

	// 删除 default 行且删后仍有其他行 → ErrDefaultRequired
	err = repo.Delete("org-a")
	if err == nil || !strings.Contains(err.Error(), "default 租户必须存在且唯一") {
		t.Fatalf("deleting default with other rows should fail, got: %v", err)
	}
	if n, _ := repo.Count(); n != 2 {
		t.Fatalf("Count after guarded delete: got %d, want 2", n)
	}

	// 切换 default 到 org-b
	if err := repo.Upsert("org-b", "c-b", "s", "", true); err != nil {
		t.Fatal(err)
	}
	d, err := repo.FindDefault()
	if err != nil || d == nil || d.Org != "org-b" {
		t.Fatalf("FindDefault after switch: got (%+v, %v)", d, err)
	}
	// List 派生信息：DefaultKey != nil 标记 default
	all, err := repo.List()
	if err != nil || len(all) != 2 {
		t.Fatalf("List: got (%v, %v)", all, err)
	}
	defaults := 0
	for _, r := range all {
		if r.DefaultKey != nil {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("List should contain exactly one default, got %d", defaults)
	}

	// 现在删 org-a（非 default）应成功；再删 org-b（最后一行，即使 default 也允许）应成功
	if err := repo.Delete("org-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete("org-b"); err != nil {
		t.Fatalf("deleting last row (default) should succeed, got: %v", err)
	}
	if n, _ := repo.Count(); n != 0 {
		t.Fatalf("Count after deletes: got %d, want 0", n)
	}
	if d, err := repo.FindDefault(); err != nil || d != nil {
		t.Fatalf("FindDefault on empty table: got (%v, %v), want (nil, nil)", d, err)
	}
}
