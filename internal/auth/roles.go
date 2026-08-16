package auth

import (
	"errors"

	authdom "control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// DefaultCasdoorRoleMapping is the fallback role mapping used when
// CASDOOR_ROLE_MAPPING is not configured: strict matching against the
// conventional casdoor role names.
var DefaultCasdoorRoleMapping = map[string]string{
	authdom.RoleAdmin:      "agent-hub-admin",
	authdom.RoleMaintainer: "agent-hub-maintainer",
	authdom.RoleMember:     "agent-hub-member",
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
