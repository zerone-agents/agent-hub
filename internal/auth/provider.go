package auth

// TokenPair is the response of login/refresh for all providers.
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
}

// AuthUser is the normalized identity extracted from an access token.
// Roles are normalized to the builtin role strings
// ("admin" | "maintainer" | "member").
type AuthUser struct {
	ID          string
	Username    string
	Email       string
	DisplayName string
	Avatar      string
	Roles       []string
}

// Provider abstracts the authentication backend. Exactly one Provider is
// assembled at startup based on auth.mode, then injected into the auth
// middleware and route registration.
type Provider interface {
	// ValidateAccessToken parses and verifies an access token, returning the
	// normalized user. Disabled/unknown users must error.
	ValidateAccessToken(token string) (*AuthUser, error)
	// RefreshToken rotates a refresh token, returning a fresh token pair.
	RefreshToken(refreshToken string) (*TokenPair, error)
	// RevokeToken revokes a refresh token (logout). Missing tokens are not
	// errors (idempotent).
	RevokeToken(token string) error
	// GetUserRoles looks up a user's current normalized roles. Used by the
	// CLI-token middleware path. The bool is false when the user is unknown
	// or disabled.
	GetUserRoles(userID string) ([]string, bool)
	// Mode reports the provider identifier ("builtin" | "casdoor").
	Mode() string
}
