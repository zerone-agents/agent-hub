package auth

import (
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

func TestNormalizeCasdoorRoles(t *testing.T) {
	cases := []struct {
		name  string
		input []*casdoorsdk.Role
		want  []string
	}{
		{"admin", []*casdoorsdk.Role{{Name: "agents-admin"}}, []string{"admin"}},
		{"maintainer", []*casdoorsdk.Role{{Name: "ops-maintainer"}}, []string{"maintainer"}},
		{"plain user", []*casdoorsdk.Role{{Name: "user"}}, []string{"member"}},
		{"empty defaults member", nil, []string{"member"}},
		{"mixed dedup", []*casdoorsdk.Role{{Name: "agents-admin"}, {Name: "other-admin"}}, []string{"admin"}},
		{"nil entry skipped", []*casdoorsdk.Role{nil, {Name: "x-maintainer"}}, []string{"maintainer"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeCasdoorRoles(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

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
