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
	// TenantID is "default" for builtin mode; the casdoor organization name
	// otherwise.
	TenantID string
}

// Provider abstracts the authentication backend. Exactly one Provider is
// assembled at startup based on auth.mode, then injected into the auth
// middleware and route registration.
//
// 不含 RevokeToken：casdoor 无撤销能力（上游 casdoor#1574：不引入非标准
// 自造 API），且该成员已无生产调用方——builtin 的撤销经 handler 侧窄接口
// （auth_builtin.go builtinTokenProvider）暴露，casdoor 登出为客户端语义
// （见 handler.Logout / audit spec v20）。未来上游提供 RFC7009 撤销时再评估纳入。
type Provider interface {
	// ValidateAccessToken parses and verifies an access token, returning the
	// normalized user. Disabled/unknown users must error.
	ValidateAccessToken(token string) (*AuthUser, error)
	// RefreshToken rotates a refresh token, returning a fresh token pair.
	RefreshToken(refreshToken string) (*TokenPair, error)
	// GetUserIdentity looks up a user's current normalized identity. Used by
	// the CLI-token middleware path. The bool is false when the user is
	// unknown or disabled.
	GetUserIdentity(userID string) (*AuthUser, bool)
	// Mode reports the provider identifier ("builtin" | "casdoor").
	Mode() string
}
