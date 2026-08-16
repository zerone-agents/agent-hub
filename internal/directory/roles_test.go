package directory

import (
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// 本测试原位于 internal/auth/roles_test.go，随映射助手一并迁入本包（见 roles.go 顶部注释）。
func TestNormalizeCasdoorRolesMapped(t *testing.T) {
	mapping := map[string]string{
		"admin":      "agent-hub-admin",
		"maintainer": "agent-hub-maintainer",
		"member":     "agent-hub-member",
	}
	r := func(name string) *casdoorsdk.Role { return &casdoorsdk.Role{Name: name} }

	cases := []struct {
		name        string
		roles       []*casdoorsdk.Role
		defaultRole string
		want        []string
		wantErr     bool
	}{
		{"admin only", []*casdoorsdk.Role{r("agent-hub-admin")}, "", []string{"admin"}, false},
		{"highest wins", []*casdoorsdk.Role{r("agent-hub-member"), r("agent-hub-admin")}, "", []string{"admin"}, false},
		{"maintainer over member", []*casdoorsdk.Role{r("agent-hub-member"), r("agent-hub-maintainer")}, "", []string{"maintainer"}, false},
		{"unmapped with default", []*casdoorsdk.Role{r("other")}, "member", []string{"member"}, false},
		{"unmapped no default rejects", []*casdoorsdk.Role{r("other")}, "", nil, true},
		{"empty roles no default rejects", nil, "", nil, true},
		{"nil entries skipped", []*casdoorsdk.Role{nil, r("agent-hub-member")}, "", []string{"member"}, false},
		{"unmapped roles alongside mapped ignored", []*casdoorsdk.Role{r("random"), r("agent-hub-member")}, "", []string{"member"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCasdoorRolesMapped(tc.roles, mapping, tc.defaultRole)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) || got[0] != tc.want[0] {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
