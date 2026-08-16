package auth

import (
	"errors"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorProvider adapts the existing Casdoor OAuth flow (package-level
// functions in casdoor.go) to the Provider interface. Construction requires
// InitCasdoor to have run first so the package-level client is initialized.
type CasdoorProvider struct {
	roleMapping map[string]string
	defaultRole string
}

// NewCasdoorProvider constructs a CasdoorProvider. An empty roleMapping falls
// back to DefaultCasdoorRoleMapping (strict matching against the conventional
// agent-hub-* casdoor role names).
func NewCasdoorProvider(roleMapping map[string]string, defaultRole string) *CasdoorProvider {
	return &CasdoorProvider{roleMapping: roleMapping, defaultRole: defaultRole}
}

// Mode identifies this provider.
func (p *CasdoorProvider) Mode() string { return "casdoor" }

// NormalizeUser converts a casdoor user to AuthUser: TenantID from Owner
// (empty -> "default"); roles via strict role mapping (configured mapping or
// DefaultCasdoorRoleMapping).
func (p *CasdoorProvider) NormalizeUser(u *casdoorsdk.User) (*AuthUser, error) {
	mapping := p.roleMapping
	if len(mapping) == 0 {
		mapping = DefaultCasdoorRoleMapping
	}
	roles, err := NormalizeCasdoorRolesMapped(u.Roles, mapping, p.defaultRole)
	if err != nil {
		return nil, err
	}
	tenantID := u.Owner
	if tenantID == "" {
		tenantID = "default"
	}
	return &AuthUser{
		ID:          u.Id,
		Username:    u.Name,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Avatar:      u.Avatar,
		Roles:       roles,
		TenantID:    tenantID,
	}, nil
}

// ValidateAccessToken parses a Casdoor JWT and returns the normalized user.
func (p *CasdoorProvider) ValidateAccessToken(token string) (*AuthUser, error) {
	u, err := GetUserInfo(token)
	if err != nil {
		return nil, err
	}
	return p.NormalizeUser(u)
}

// RefreshToken exchanges a Casdoor refresh token for a fresh token pair.
func (p *CasdoorProvider) RefreshToken(refreshToken string) (*TokenPair, error) {
	if refreshToken == "" {
		return nil, errors.New("refresh token is empty")
	}
	resp, err := RefreshAccessToken(refreshToken)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresIn:    resp.ExpiresIn,
	}, nil
}

// RevokeToken revokes a Casdoor access or refresh token. The package-level
// RevokeToken is called by method dispatch (receiver-bound), so it does not
// shadow itself here.
func (p *CasdoorProvider) RevokeToken(token string) error {
	return RevokeToken(token)
}

// GetUserIdentity looks up a user's current normalized identity from Casdoor
// (used by the CLI-token middleware path). The bool is false when the user is
// unknown or the lookup fails.
func (p *CasdoorProvider) GetUserIdentity(userID string) (*AuthUser, bool) {
	c := GetClient()
	if c == nil {
		return nil, false
	}
	u, err := c.GetUserByUserId(userID)
	if err != nil || u == nil {
		return nil, false
	}
	au, err := p.NormalizeUser(u)
	if err != nil {
		return nil, false
	}
	return au, true
}
