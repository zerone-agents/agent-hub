package directory

import (
	"control-panel/internal/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// NewCasdoorDirectory constructs a CasdoorDirectory. roleMapping empty falls
// back to auth.DefaultCasdoorRoleMapping (same rule as CasdoorProvider).
func NewCasdoorDirectory(client UserClient, roleMapping map[string]string, defaultRole string) *CasdoorDirectory {
	if len(roleMapping) == 0 {
		roleMapping = auth.DefaultCasdoorRoleMapping
	}
	return &CasdoorDirectory{client: client, roleMapping: roleMapping, defaultRole: defaultRole}
}

// ListUsers returns users of tenantID (casdoor organization) with normalized
// roles. Users with no mapped role get Role "" (display-only; the list must
// not fail because one user is unmapped).
func (d *CasdoorDirectory) ListUsers(tenantID string) ([]ManagedUser, error) {
	users, err := d.client.GetUsers()
	if err != nil {
		return nil, err
	}
	out := make([]ManagedUser, 0, len(users))
	for _, u := range users {
		if u == nil || u.Owner != tenantID {
			continue
		}
		out = append(out, d.toManagedUser(u))
	}
	return out, nil
}

// toManagedUser maps a casdoor user to the admin-UI projection.
func (d *CasdoorDirectory) toManagedUser(u *casdoorsdk.User) ManagedUser {
	role := ""
	if roles, err := auth.NormalizeCasdoorRolesMapped(u.Roles, d.roleMapping, d.defaultRole); err == nil && len(roles) > 0 {
		role = roles[0]
	}
	status := "active"
	if u.IsForbidden {
		status = "disabled"
	}
	return ManagedUser{
		ID: u.Id, Username: u.Name, DisplayName: u.DisplayName,
		Email: u.Email, Role: role, Status: status, CreatedAt: u.CreatedTime,
	}
}
