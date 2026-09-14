package auth

import (
	"testing"

	authdom "control-panel/internal/domain/auth"
)

func TestSynthesizeMembership(t *testing.T) {
	cases := []struct {
		name       string
		isAdmin    bool
		rec        *authdom.UserIdentity // nil = 本地无记录
		wantRole   string
		wantStatus string
		wantOp     MembershipOp
	}{
		{"org admin, no record", true, nil, "admin", "active", OpCreate},
		{"org admin, pending record promoted", true, &authdom.UserIdentity{Role: "", Status: "pending"}, "admin", "active", OpUpdate},
		{"org admin, member promoted", true, &authdom.UserIdentity{Role: "member", Status: "active"}, "admin", "active", OpUpdate},
		{"org admin, already admin", true, &authdom.UserIdentity{Role: "admin", Status: "active"}, "admin", "active", OpNone},
		{"not org admin, no record -> pending", false, nil, "", "pending", OpCreate},
		{"not org admin, admin revoked (bidirectional sync)", false, &authdom.UserIdentity{Role: "admin", Status: "active"}, "", "pending", OpUpdate},
		{"not org admin, member unchanged", false, &authdom.UserIdentity{Role: "member", Status: "active"}, "member", "active", OpNone},
		{"not org admin, maintainer unchanged", false, &authdom.UserIdentity{Role: "maintainer", Status: "active"}, "maintainer", "active", OpNone},
		{"not org admin, illegal role value -> pending (decision 8)", false, &authdom.UserIdentity{Role: "superuser", Status: "active"}, "", "pending", OpUpdate},
		{"org admin, illegal role value -> admin wins", true, &authdom.UserIdentity{Role: "superuser", Status: "active"}, "admin", "active", OpUpdate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SynthesizeMembership(tc.isAdmin, tc.rec)
			if got.Role != tc.wantRole || got.Status != tc.wantStatus || got.Op != tc.wantOp {
				t.Fatalf("got %+v, want role=%q status=%q op=%d", got, tc.wantRole, tc.wantStatus, tc.wantOp)
			}
		})
	}
}

// TestSynthesizeMembership_ExplicitGuest 显式 guest 角色是合法角色：非 org-admin
// 用户携带 guest 角色时原样保留（OpNone），不被当作"未分配"清洗成 pending。
func TestSynthesizeMembership_ExplicitGuest(t *testing.T) {
	rec := &authdom.UserIdentity{Role: authdom.RoleGuest, Status: authdom.StatusActive}
	got := SynthesizeMembership(false, rec)
	if got.Role != authdom.RoleGuest || got.Status != authdom.StatusActive || got.Op != OpNone {
		t.Fatalf("explicit guest should persist unchanged, got %+v", got)
	}
}
