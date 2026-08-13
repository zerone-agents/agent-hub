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
