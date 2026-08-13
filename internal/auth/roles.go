package auth

import (
	"strings"

	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// NormalizeCasdoorRoles maps Casdoor roles to builtin role strings.
// A role name containing "admin" maps to admin; one containing "maintainer"
// maps to maintainer; anything else maps to member. Empty/nil input defaults
// to ["member"]. Results are deduplicated, preserving first-seen order; nil
// role entries are skipped.
func NormalizeCasdoorRoles(roles []*casdoorsdk.Role) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(roles))
	add := func(r string) {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range roles {
		if r == nil {
			continue
		}
		switch name := strings.ToLower(r.Name); {
		case strings.Contains(name, "admin"):
			add(authdom.RoleAdmin)
		case strings.Contains(name, "maintainer"):
			add(authdom.RoleMaintainer)
		default:
			add(authdom.RoleMember)
		}
	}
	if len(out) == 0 {
		out = append(out, authdom.RoleMember)
	}
	return out
}
