package auth

import (
	"errors"
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

// ErrNoMatchedRole is returned when no casdoor role matches the mapping and
// no defaultRole fallback is configured.
var ErrNoMatchedRole = errors.New("no casdoor role matched role mapping")

// NormalizeCasdoorRolesMapped maps casdoor roles via an explicit mapping
// (agent-hub role -> casdoor role name). A user holding several mapped roles
// gets the single highest one: admin > maintainer > member. When nothing
// matches, defaultRole is used if non-empty; otherwise ErrNoMatchedRole.
func NormalizeCasdoorRolesMapped(roles []*casdoorsdk.Role, mapping map[string]string, defaultRole string) ([]string, error) {
	// reverse: casdoor role name -> agent-hub role
	rev := make(map[string]string, len(mapping))
	for hubRole, casdoorName := range mapping {
		rev[casdoorName] = hubRole
	}
	best := ""
	for _, r := range roles {
		if r == nil {
			continue
		}
		hubRole, ok := rev[r.Name]
		if !ok {
			continue
		}
		if rank(hubRole) > rank(best) {
			best = hubRole
		}
	}
	if best == "" {
		if defaultRole == "" {
			return nil, ErrNoMatchedRole
		}
		best = defaultRole
	}
	return []string{best}, nil
}

// rank orders roles by privilege; "" ranks lowest.
func rank(role string) int {
	switch role {
	case authdom.RoleAdmin:
		return 3
	case authdom.RoleMaintainer:
		return 2
	case authdom.RoleMember:
		return 1
	default:
		return 0
	}
}
