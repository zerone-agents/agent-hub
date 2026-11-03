package auth

import (
	"testing"

	authdom "control-panel/internal/domain/auth"
)

func TestSynthesizeMembership(t *testing.T) {
	cases := []struct {
		name            string
		isAdmin         bool
		rec             *authdom.UserIdentity // nil = 本地无记录
		isFirstInTenant bool                  // 租户无 active 成员（bootstrap，#238）
		wantRole        string
		wantStatus      string
		wantOp          MembershipOp
	}{
		{"org admin, no record", true, nil, false, "admin", "active", OpCreate},
		{"org admin, pending record promoted", true, &authdom.UserIdentity{Role: "", Status: "pending"}, false, "admin", "active", OpUpdate},
		{"org admin, member promoted", true, &authdom.UserIdentity{Role: "member", Status: "active"}, false, "admin", "active", OpUpdate},
		{"org admin, already admin", true, &authdom.UserIdentity{Role: "admin", Status: "active"}, false, "admin", "active", OpNone},
		{"not org admin, no record -> pending", false, nil, false, "", "pending", OpCreate},
		{"not org admin, admin revoked (bidirectional sync)", false, &authdom.UserIdentity{Role: "admin", Status: "active"}, false, "", "pending", OpUpdate},
		{"not org admin, member unchanged", false, &authdom.UserIdentity{Role: "member", Status: "active"}, false, "member", "active", OpNone},
		{"not org admin, maintainer unchanged", false, &authdom.UserIdentity{Role: "maintainer", Status: "active"}, false, "maintainer", "active", OpNone},
		{"not org admin, illegal role value -> pending (decision 8)", false, &authdom.UserIdentity{Role: "superuser", Status: "active"}, false, "", "pending", OpUpdate},
		{"org admin, illegal role value -> admin wins", true, &authdom.UserIdentity{Role: "superuser", Status: "active"}, false, "admin", "active", OpUpdate},
		// --- issue #238 bootstrap ---
		{"bootstrap: first user, no record -> admin", false, nil, true, "admin", "active", OpCreate},
		{"bootstrap: first user, pending record promoted", false, &authdom.UserIdentity{Role: "", Status: "pending"}, true, "admin", "active", OpUpdate},
		{"bootstrap: second user (not first) -> pending", false, nil, false, "", "pending", OpCreate},
		{"bootstrap does not protect an existing admin from casdoor demotion", false, &authdom.UserIdentity{Role: "admin", Status: "active"}, true, "", "pending", OpUpdate},
		{"bootstrap never re-grants a member", false, &authdom.UserIdentity{Role: "member", Status: "active"}, true, "member", "active", OpNone},
		{"org admin wins over bootstrap (same outcome, isAdmin case first)", true, nil, true, "admin", "active", OpCreate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SynthesizeMembership(tc.isAdmin, tc.rec, tc.isFirstInTenant)
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
	got := SynthesizeMembership(false, rec, false)
	if got.Role != authdom.RoleGuest || got.Status != authdom.StatusActive || got.Op != OpNone {
		t.Fatalf("explicit guest should persist unchanged, got %+v", got)
	}
}
