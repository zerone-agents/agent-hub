// Package builtin implements auth.Provider backed by the local users table.
package builtin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"control-panel/internal/auth"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/tenant"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

const (
	accessTokenTTL  = 2 * time.Hour
	refreshTokenTTL = 30 * 24 * time.Hour
)

// Provider issues HS256 JWT access tokens plus opaque rotating refresh tokens,
// backed by the local users and refresh_tokens tables.
type Provider struct {
	db        *gorm.DB
	jwtSecret []byte
}

// New constructs a builtin provider. jwtSecret must be at least 32 bytes
// (enforced earlier by config.ValidateAuth).
func New(db *gorm.DB, jwtSecret string) *Provider {
	return &Provider{db: db, jwtSecret: []byte(jwtSecret)}
}

// Mode identifies this provider.
func (p *Provider) Mode() string { return "builtin" }

type accessTokenClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// IssueTokenPair creates a fresh access token + refresh token for user.
func (p *Provider) IssueTokenPair(user *authdom.User) (*auth.TokenPair, error) {
	now := time.Now()
	c := accessTokenClaims{
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(user.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenTTL)),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(p.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	refresh := "rt_" + hex.EncodeToString(b)
	rt := &authdom.RefreshToken{
		UserID:    user.ID,
		TokenHash: sha256Hex(refresh),
		ExpiresAt: now.Add(refreshTokenTTL),
	}
	if err := p.db.Create(rt).Error; err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &auth.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refresh,
		ExpiresIn:    int(accessTokenTTL / time.Second),
	}, nil
}

// ValidateAccessToken parses and verifies a JWT, then confirms the user is
// still active and re-reads the role from the DB so role/status changes take
// effect immediately (without an access-token blacklist).
func (p *Provider) ValidateAccessToken(tokenString string) (*auth.AuthUser, error) {
	var c accessTokenClaims
	_, err := jwt.ParseWithClaims(tokenString, &c, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return p.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	if err != nil {
		return nil, errors.New("invalid subject")
	}
	var user authdom.User
	if err := p.db.First(&user, id).Error; err != nil {
		return nil, errors.New("user not found")
	}
	if user.Status != authdom.StatusActive {
		return nil, errors.New("user disabled")
	}
	return &auth.AuthUser{
		ID:          c.Subject,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       []string{user.Role},
		TenantID:    tenant.DefaultID,
	}, nil
}

// RefreshToken rotates: validates the old refresh token, deletes it, and issues
// a brand-new pair. Reuse of an already-rotated token fails.
func (p *Provider) RefreshToken(refreshToken string) (*auth.TokenPair, error) {
	rt, err := p.lookupRefresh(refreshToken)
	if err != nil {
		return nil, err
	}
	var user authdom.User
	if err := p.db.First(&user, rt.UserID).Error; err != nil {
		return nil, errors.New("user not found")
	}
	if user.Status != authdom.StatusActive {
		return nil, errors.New("user disabled")
	}
	p.db.Delete(rt)
	return p.IssueTokenPair(&user)
}

// RevokeToken deletes a refresh token (logout). Missing tokens are not errors.
func (p *Provider) RevokeToken(refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	p.db.Where("token_hash = ?", sha256Hex(refreshToken)).Delete(&authdom.RefreshToken{})
	return nil
}

// RevokeAllForUser deletes every refresh token of the user — used on password
// change or disable to log the user out everywhere once the short-lived access
// token expires (and immediately on the next refresh attempt).
func (p *Provider) RevokeAllForUser(userID uint64) error {
	return p.db.Where("user_id = ?", userID).Delete(&authdom.RefreshToken{}).Error
}

// GetUserIdentity returns the user's current identity; false when unknown or
// disabled. Used by the CLI-token middleware path. Builtin mode is
// single-tenant, so TenantID is always tenant.DefaultID.
func (p *Provider) GetUserIdentity(userID string) (*auth.AuthUser, bool) {
	id, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return nil, false
	}
	var user authdom.User
	if err := p.db.First(&user, id).Error; err != nil {
		return nil, false
	}
	if user.Status != authdom.StatusActive {
		return nil, false
	}
	return &auth.AuthUser{
		ID:          userID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       []string{user.Role},
		TenantID:    tenant.DefaultID,
	}, true
}

func (p *Provider) lookupRefresh(plaintext string) (*authdom.RefreshToken, error) {
	var rt authdom.RefreshToken
	if err := p.db.Where("token_hash = ?", sha256Hex(plaintext)).First(&rt).Error; err != nil {
		return nil, errors.New("invalid refresh token")
	}
	if time.Now().After(rt.ExpiresAt) {
		p.db.Delete(&rt)
		return nil, errors.New("refresh token expired")
	}
	return &rt, nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
