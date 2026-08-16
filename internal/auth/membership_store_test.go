package auth

import (
	"errors"
	"testing"
	"time"

	authdom "control-panel/internal/domain/auth"

	"gorm.io/gorm"
)

// newMembershipTestDB 打开 sqlite 内存库并迁移 UserIdentity 表。
func newMembershipTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newSQLite(t)
	if err := db.AutoMigrate(&authdom.UserIdentity{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// findIdentity 直接查库取记录，绕过被测接口，用于断言落库结果。
func findIdentity(t *testing.T, db *gorm.DB, provider, externalID string) *authdom.UserIdentity {
	t.Helper()
	var row authdom.UserIdentity
	if err := db.Where("provider = ? AND external_id = ?", provider, externalID).First(&row).Error; err != nil {
		t.Fatalf("find identity: %v", err)
	}
	return &row
}

func TestApplyDecisionCreatePending(t *testing.T) {
	// 无记录 + OpCreate(pending) → 建记录 role="" status=pending，快照写入
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	au := &AuthUser{ID: "cas-1", Username: "alice", DisplayName: "Alice",
		Email: "a@x.com", TenantID: "tenant-acme"}
	d := MembershipDecision{Role: "", Status: authdom.StatusPending, Op: OpCreate}

	before := time.Now()
	if err := store.ApplyDecision("casdoor", au, d); err != nil {
		t.Fatal(err)
	}

	row := findIdentity(t, db, "casdoor", "cas-1")
	if row.Role != "" {
		t.Fatalf("role should be empty (unassigned), got %q", row.Role)
	}
	if row.Status != authdom.StatusPending {
		t.Fatalf("status should be pending, got %q", row.Status)
	}
	// 快照字段取自 au
	if row.Username != "alice" || row.DisplayName != "Alice" ||
		row.Email != "a@x.com" || row.TenantID != "tenant-acme" {
		t.Fatalf("snapshot not written: %+v", row)
	}
	if row.LastLoginAt.Before(before) {
		t.Fatalf("last_login_at not set to now: %v", row.LastLoginAt)
	}
}

func TestApplyDecisionPromoteToAdmin(t *testing.T) {
	// 已有 pending 记录 + OpUpdate(admin/active) → role=admin status=active
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	au := &AuthUser{ID: "cas-2", Username: "bob", DisplayName: "Bob",
		Email: "b@x.com", TenantID: "tenant-acme"}
	// 先建 pending 记录
	if err := store.ApplyDecision("casdoor", au,
		MembershipDecision{Role: "", Status: authdom.StatusPending, Op: OpCreate}); err != nil {
		t.Fatal(err)
	}

	// 合成结果：提升为 admin/active
	d := MembershipDecision{Role: authdom.RoleAdmin, Status: authdom.StatusActive, Op: OpUpdate}
	if err := store.ApplyDecision("casdoor", au, d); err != nil {
		t.Fatal(err)
	}

	row := findIdentity(t, db, "casdoor", "cas-2")
	if row.Role != authdom.RoleAdmin || row.Status != authdom.StatusActive {
		t.Fatalf("expected admin/active, got %q/%q", row.Role, row.Status)
	}

	// 仍只有一条记录
	var count int64
	db.Model(&authdom.UserIdentity{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

func TestApplyDecisionNoneRefreshesSnapshotOnly(t *testing.T) {
	// 已有 member/active 记录 + OpNone + 新 displayName →
	// displayName/lastLoginAt 更新，role/status 不变
	db := newMembershipTestDB(t)
	if err := db.Create(&authdom.UserIdentity{
		Provider: "casdoor", ExternalID: "cas-3", TenantID: "tenant-acme",
		Username: "carol", DisplayName: "Carol", Email: "c@x.com",
		Role: authdom.RoleMember, Status: authdom.StatusActive,
		LastLoginAt: time.Now().Add(-time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	store := NewMembershipStore(db)

	au := &AuthUser{ID: "cas-3", Username: "carol", DisplayName: "Carol New",
		Email: "c@x.com", TenantID: "tenant-acme"}
	// OpNone 时 decision 里的 Role/Status 不应被采信（这里故意给不同值验证不变式）
	d := MembershipDecision{Role: authdom.RoleAdmin, Status: authdom.StatusPending, Op: OpNone}

	before := time.Now()
	if err := store.ApplyDecision("casdoor", au, d); err != nil {
		t.Fatal(err)
	}

	row := findIdentity(t, db, "casdoor", "cas-3")
	if row.DisplayName != "Carol New" {
		t.Fatalf("display_name not refreshed: %q", row.DisplayName)
	}
	if row.LastLoginAt.Before(before) {
		t.Fatalf("last_login_at not refreshed: %v", row.LastLoginAt)
	}
	// OpNone 绝不动 role/status
	if row.Role != authdom.RoleMember || row.Status != authdom.StatusActive {
		t.Fatalf("role/status must not change on OpNone, got %q/%q", row.Role, row.Status)
	}
}

func TestFindByExternalIDMissingReturnsNilNil(t *testing.T) {
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	rec, err := store.FindByExternalID("casdoor", "no-such-user")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if rec != nil {
		t.Fatalf("expected nil record, got %+v", rec)
	}
}

func TestFindByExternalIDFound(t *testing.T) {
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	au := &AuthUser{ID: "cas-4", Username: "dave", TenantID: "tenant-acme"}
	if err := store.ApplyDecision("casdoor", au,
		MembershipDecision{Role: "", Status: authdom.StatusPending, Op: OpCreate}); err != nil {
		t.Fatal(err)
	}

	rec, err := store.FindByExternalID("casdoor", "cas-4")
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.Username != "dave" {
		t.Fatalf("expected dave record, got %+v", rec)
	}
}

func TestListByTenant(t *testing.T) {
	// 3 条 tenant-a + 1 条 tenant-b → 只返回 tenant-a 的，按 id 升序（插入序）
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	seed := []authdom.UserIdentity{
		{Provider: "casdoor", ExternalID: "ext-1", TenantID: "tenant-a",
			Username: "alice", Role: authdom.RoleAdmin, Status: authdom.StatusActive},
		{Provider: "casdoor", ExternalID: "ext-2", TenantID: "tenant-a",
			Username: "bob", Role: authdom.RoleMember, Status: authdom.StatusActive},
		{Provider: "casdoor", ExternalID: "ext-3", TenantID: "tenant-a",
			Username: "carol", Role: "", Status: authdom.StatusPending},
		{Provider: "casdoor", ExternalID: "ext-4", TenantID: "tenant-b",
			Username: "dave", Role: authdom.RoleMember, Status: authdom.StatusActive},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	recs, err := store.ListByTenant("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3 (tenant-b excluded)", len(recs))
	}
	for i, want := range []string{"alice", "bob", "carol"} {
		if recs[i].Username != want {
			t.Fatalf("recs[%d].Username = %q, want %q（按 id 升序）", i, recs[i].Username, want)
		}
	}
}

func TestListByTenantEmpty(t *testing.T) {
	// 无记录的租户返回空列表 + nil error，不视为错误
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	recs, err := store.ListByTenant("no-such-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 0 {
		t.Fatalf("expected empty list, got %d records", len(recs))
	}
}

func TestSetRole(t *testing.T) {
	// pending/未分配记录 → SetRole(member/active)：只动 role/status 两列，
	// 快照字段与 last_login_at 不受影响
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	created := time.Now().Add(-time.Hour)
	if err := db.Create(&authdom.UserIdentity{
		Provider: "casdoor", ExternalID: "ext-9", TenantID: "tenant-acme",
		Username: "erin", DisplayName: "Erin", Email: "e@x.com",
		Role: "", Status: authdom.StatusPending, LastLoginAt: created,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := store.SetRole("casdoor", "ext-9", authdom.RoleMember, authdom.StatusActive); err != nil {
		t.Fatal(err)
	}

	row := findIdentity(t, db, "casdoor", "ext-9")
	if row.Role != authdom.RoleMember || row.Status != authdom.StatusActive {
		t.Fatalf("expected member/active, got %q/%q", row.Role, row.Status)
	}
	// 快照字段与登录时间不被触碰（容忍 sqlite 时间精度截断，但必须远早于 now）
	if row.Username != "erin" || row.DisplayName != "Erin" || row.TenantID != "tenant-acme" {
		t.Fatalf("snapshot must stay untouched: %+v", row)
	}
	if row.LastLoginAt.Sub(created).Abs() > 2*time.Second {
		t.Fatalf("last_login_at must not change, got %v want %v", row.LastLoginAt, created)
	}
}

func TestSetRoleMissing(t *testing.T) {
	// 记录不存在 → 明确报错（防御：调用方应先 FindByExternalID 校验）
	db := newMembershipTestDB(t)
	store := NewMembershipStore(db)

	err := store.SetRole("casdoor", "ghost", authdom.RoleMember, authdom.StatusActive)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("want gorm.ErrRecordNotFound, got %v", err)
	}
}
