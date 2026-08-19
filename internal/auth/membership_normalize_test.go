package auth

import (
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"
)

// TestNormalizePendingRoles 锁定迁移级行为：存量 {role, pending} 旧行归一为
// {role:"", pending}；active 行不受影响；重复执行幂等。
func TestNormalizePendingRoles(t *testing.T) {
	db := newMembershipTestDB(t)
	oldLogin := time.Now().Add(-24 * time.Hour)

	seed := []authdom.UserIdentity{
		// 旧模型存量行：升级时 AutoMigrate 填 status=pending 但保留旧 role 快照
		{Provider: "casdoor", ExternalID: "ext-a", TenantID: "t",
			Username: "alice", Role: authdom.RoleMember, Status: authdom.StatusPending,
			LastLoginAt: oldLogin},
		{Provider: "casdoor", ExternalID: "ext-b", TenantID: "t",
			Username: "bob", Role: authdom.RoleMaintainer, Status: authdom.StatusPending},
		// 合法组合：active 行带角色，不应被触碰
		{Provider: "casdoor", ExternalID: "ext-c", TenantID: "t",
			Username: "carol", Role: authdom.RoleMember, Status: authdom.StatusActive},
		// 新模型 pending 行（role 本就为空），幂等重放场景
		{Provider: "casdoor", ExternalID: "ext-d", TenantID: "t",
			Username: "dave", Role: "", Status: authdom.StatusPending},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := NormalizePendingRoles(db); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	// 旧 pending 行：role 清空、status 及其他字段不变
	a := findIdentity(t, db, "casdoor", "ext-a")
	if a.Role != "" || a.Status != authdom.StatusPending {
		t.Fatalf("ext-a: want role=\"\"/pending, got %q/%q", a.Role, a.Status)
	}
	if a.Username != "alice" || a.TenantID != "t" {
		t.Fatalf("ext-a: snapshot must stay untouched: %+v", a)
	}
	if a.LastLoginAt.Sub(oldLogin).Abs() > 2*time.Second {
		t.Fatalf("ext-a: last_login_at must not change, got %v want %v", a.LastLoginAt, oldLogin)
	}
	b := findIdentity(t, db, "casdoor", "ext-b")
	if b.Role != "" || b.Status != authdom.StatusPending {
		t.Fatalf("ext-b: want role=\"\"/pending, got %q/%q", b.Role, b.Status)
	}

	// active 行不受影响
	c := findIdentity(t, db, "casdoor", "ext-c")
	if c.Role != authdom.RoleMember || c.Status != authdom.StatusActive {
		t.Fatalf("ext-c: active row must stay member/active, got %q/%q", c.Role, c.Status)
	}

	// 幂等：重复执行无副作用
	if err := NormalizePendingRoles(db); err != nil {
		t.Fatalf("second normalize: %v", err)
	}
	c = findIdentity(t, db, "casdoor", "ext-c")
	if c.Role != authdom.RoleMember || c.Status != authdom.StatusActive {
		t.Fatalf("idempotency broken: ext-c changed to %q/%q", c.Role, c.Status)
	}
	d := findIdentity(t, db, "casdoor", "ext-d")
	if d.Role != "" || d.Status != authdom.StatusPending {
		t.Fatalf("ext-d: new-model pending row must stay \"\"/pending, got %q/%q", d.Role, d.Status)
	}
}
