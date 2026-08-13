package auth

import (
	"errors"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

// CasdoorProvider adapts the existing Casdoor OAuth flow (package-level
// functions in casdoor.go) to the Provider interface. Construction requires
// InitCasdoor to have run first so the package-level client is initialized.
type CasdoorProvider struct{}

// NewCasdoorProvider constructs a CasdoorProvider.
func NewCasdoorProvider() *CasdoorProvider { return &CasdoorProvider{} }

// Mode identifies this provider.
func (p *CasdoorProvider) Mode() string { return "casdoor" }

// NormalizeCasdoorUser converts a Casdoor user into the normalized AuthUser,
// mapping roles via NormalizeCasdoorRoles.
func NormalizeCasdoorUser(u *casdoorsdk.User) *AuthUser {
	return &AuthUser{
		ID:          u.Id,
		Username:    u.Name,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Avatar:      u.Avatar,
		Roles:       NormalizeCasdoorRoles(u.Roles),
	}
}

// ValidateAccessToken parses a Casdoor JWT and returns the normalized user.
func (p *CasdoorProvider) ValidateAccessToken(token string) (*AuthUser, error) {
	u, err := GetUserInfo(token)
	if err != nil {
		return nil, err
	}
	return NormalizeCasdoorUser(u), nil
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

// GetUserRoles looks up a user's current roles from Casdoor (used by the
// CLI-token middleware path). The bool is false when the user is unknown or
// the lookup fails.
func (p *CasdoorProvider) GetUserRoles(userID string) ([]string, bool) {
	c := GetClient()
	if c == nil {
		return nil, false
	}
	u, err := c.GetUserByUserId(userID)
	if err != nil || u == nil {
		return nil, false
	}
	return NormalizeCasdoorRoles(u.Roles), true
}
