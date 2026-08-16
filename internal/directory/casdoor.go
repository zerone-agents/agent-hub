package directory

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// NewCasdoorDirectory constructs a CasdoorDirectory. roleMapping empty falls
// back to DefaultCasdoorRoleMapping.
func NewCasdoorDirectory(client UserClient, roleMapping map[string]string, defaultRole string) *CasdoorDirectory {
	if len(roleMapping) == 0 {
		roleMapping = DefaultCasdoorRoleMapping
	}
	return &CasdoorDirectory{client: client, roleMapping: roleMapping, defaultRole: defaultRole}
}

// ListUsers returns users of tenantID (casdoor organization) with normalized
// roles. Users with no mapped role get Role "" (display-only; the list must
// not fail because one user is unmapped).
//
// Note: casdoor's get-users API returns users WITHOUT their roles (upstream
// issue casdoor/casdoor#3688), so roles are resolved from the role objects'
// Users lists instead (get-roles).
func (d *CasdoorDirectory) ListUsers(tenantID string) ([]ManagedUser, error) {
	users, err := d.client.GetUsers()
	if err != nil {
		return nil, err
	}
	roles, err := d.client.GetRoles()
	if err != nil {
		return nil, err
	}
	// reverse index: user id (owner/name) -> casdoor roles holding that user
	userRoles := make(map[string][]*casdoorsdk.Role, len(users))
	for _, r := range roles {
		if r == nil {
			continue
		}
		for _, uid := range r.Users {
			userRoles[uid] = append(userRoles[uid], r)
		}
	}
	out := make([]ManagedUser, 0, len(users))
	for _, u := range users {
		if u == nil || u.Owner != tenantID {
			continue
		}
		u.Roles = userRoles[u.GetId()]
		out = append(out, d.toManagedUser(u))
	}
	return out, nil
}

// toManagedUser maps a casdoor user to the admin-UI projection.
func (d *CasdoorDirectory) toManagedUser(u *casdoorsdk.User) ManagedUser {
	role := ""
	if roles, err := NormalizeCasdoorRolesMapped(u.Roles, d.roleMapping, d.defaultRole); err == nil && len(roles) > 0 {
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

// getTenantUser fetches a user and verifies tenant ownership. SDK errors are
// propagated as-is (mapped to 502 by the handler); only a missing user or a
// cross-tenant user yields ErrUserNotFound (404).
func (d *CasdoorDirectory) getTenantUser(tenantID, userID string) (*casdoorsdk.User, error) {
	u, err := d.client.GetUserByUserId(userID)
	if err != nil {
		return nil, err
	}
	if u == nil || u.Owner != tenantID {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// UpdateRole swaps the user's mapped casdoor roles for the one corresponding
// to role, preserving any roles outside the mapping value set.
//
// Casdoor does not support changing a user's roles via update-user (roles are
// an extended, read-only field on the User API); role membership lives on the
// Role object's Users list, so we update those instead.
func (d *CasdoorDirectory) UpdateRole(tenantID, userID, role, actorID string) error {
	if userID == actorID {
		return ErrSelfOperation
	}
	casdoorName, ok := d.roleMapping[role]
	if !ok {
		return ErrInvalidRole
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return err
	}
	mappedValues := make(map[string]bool, len(d.roleMapping))
	for _, v := range d.roleMapping {
		mappedValues[v] = true
	}
	roles, err := d.client.GetRoles()
	if err != nil {
		return err
	}
	uid := u.GetId() // "owner/name", the form stored in Role.Users
	for _, r := range roles {
		if r == nil || !mappedValues[r.Name] {
			continue // only touch mapped roles; external roles stay untouched
		}
		has := false
		kept := r.Users[:0]
		for _, member := range r.Users {
			if member == uid {
				has = true
				continue
			}
			kept = append(kept, member)
		}
		want := r.Name == casdoorName
		if has == want {
			continue // already in the desired state
		}
		if want {
			r.Users = append(r.Users, uid)
		} else {
			r.Users = kept
		}
		ok, err := d.client.UpdateRoleForColumns(r, []string{"users"})
		if err != nil {
			return err
		}
		if !ok {
			return ErrUpdateRejected
		}
	}
	return nil
}

// SetDisabled sets the casdoor is_forbidden flag.
func (d *CasdoorDirectory) SetDisabled(tenantID, userID string, disabled bool, actorID string) error {
	if userID == actorID {
		return ErrSelfOperation
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return err
	}
	u.IsForbidden = disabled
	ok, err := d.client.UpdateUserForColumns(u, []string{"is_forbidden"})
	if err != nil {
		return err
	}
	if !ok {
		return ErrUpdateRejected
	}
	return nil
}

// ResetPassword sets a random password (casdoor hashes it server-side) and
// returns the plaintext exactly once.
func (d *CasdoorDirectory) ResetPassword(tenantID, userID, actorID string) (string, error) {
	if userID == actorID {
		return "", ErrSelfOperation
	}
	u, err := d.getTenantUser(tenantID, userID)
	if err != nil {
		return "", err
	}
	b := make([]byte, 12) // 24 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	plain := "Reset!" + hex.EncodeToString(b)
	u.Password = plain
	ok, err := d.client.UpdateUserForColumns(u, []string{"password"})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUpdateRejected
	}
	return plain, nil
}
