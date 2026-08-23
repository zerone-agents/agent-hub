package jwtutil

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// AuthMiddlewareWithCLI is the single auth middleware entry point. It accepts
// either a provider-validated access token or an opaque cli_<hex> CLI token
// (when cliSvc is non-nil). The provider abstracts builtin vs casdoor backends;
// passing nil provider rejects every request (used during bootstrap before a
// provider is assembled).
//
// Context keys set (unchanged names from the legacy Casdoor middleware):
//
//	user_id, user_name, email, display_name,
//	org_id (the TenantID: casdoor organization name, or "default" for builtin),
//	avatar,
//	roles ([]string, normalized to admin|maintainer|member),
//	tenant_id (via tenant.SetTenantID; "default" for builtin mode),
//	permissions, auth_method ("builtin" | "casdoor" | "cli").
func AuthMiddlewareWithCLI(cliSvc *services.CLITokenService, p auth.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			unauthorized(c, "authorization header not found")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			unauthorized(c, "invalid authorization header format")
			return
		}

		// CLI token branch: opaque strings of the form cli_<hex>.
		if isCLIToken(tokenString) {
			if cliSvc == nil || p == nil {
				unauthorized(c, "cli token support is disabled")
				return
			}
			record, err := cliSvc.Verify(tokenString)
			if err != nil {
				unauthorized(c, "invalid cli token: "+err.Error())
				return
			}
			// CLI tokens identify the user but carry no profile/role data, so
			// the identity is looked up fresh via the provider (cached briefly).
			// If the user no longer exists or is disabled, reject the token.
			identity, ok := fetchUserIdentity(p, record.UserID)
			if !ok {
				unauthorized(c, "cli token user not found")
				return
			}
			c.Set("user_id", record.UserID)
			c.Set("user_name", "")
			c.Set("email", "")
			c.Set("display_name", "")
			c.Set("org_id", identity.TenantID)
			c.Set("avatar", "")
			c.Set("roles", identity.Roles)
			tenant.SetTenantID(c, tenantOrDefault(identity.TenantID))
			c.Set("permissions", []string{})
			c.Set("auth_method", "cli")
			c.Next()
			return
		}

		if p == nil {
			unauthorized(c, "auth provider not configured")
			return
		}

		user, err := p.ValidateAccessToken(tokenString)
		if err != nil {
			unauthorized(c, "invalid token: "+err.Error())
			return
		}

		c.Set("user_id", user.ID)
		c.Set("user_name", user.Username)
		c.Set("email", user.Email)
		c.Set("display_name", user.DisplayName)
		c.Set("org_id", user.TenantID)
		c.Set("avatar", user.Avatar)
		c.Set("roles", user.Roles)
		tenant.SetTenantID(c, tenantOrDefault(user.TenantID))
		c.Set("permissions", []string{})
		c.Set("auth_method", p.Mode())
		c.Next()
	}
}

// isCLIToken returns true iff s has the cli_<hex> prefix. Length is checked to
// avoid false positives on very short strings.
func isCLIToken(s string) bool {
	return len(s) >= 4 && s[:4] == "cli_"
}

func unauthorized(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error":   msg,
	})
	c.Abort()
}

// --- User-identity cache (CLI token path) ---
//
// The identity (roles + tenant) is fetched via the provider on the first
// CLI-token request for a user and cached briefly for performance. Only
// positive results are cached, so a transient provider failure retries
// immediately instead of locking a valid user out for the TTL.

const identityCacheTTL = 5 * time.Minute

type identityCacheEntry struct {
	user      *auth.AuthUser
	fetchedAt time.Time
}

var (
	identityCache   = make(map[string]*identityCacheEntry)
	identityCacheMu sync.RWMutex

	// identityFetcher is the function used to look up a user's identity via
	// the provider. Swappable in tests; defaults to calling p.GetUserIdentity.
	identityFetcher func(p auth.Provider, userID string) (*auth.AuthUser, bool) = func(p auth.Provider, userID string) (*auth.AuthUser, bool) {
		return p.GetUserIdentity(userID)
	}
)

// fetchUserIdentity returns the cached identity for userID, or fetches it via
// the provider when the cache entry is missing or stale. The bool is false if
// the user could not be found / is disabled / lookup failed.
func fetchUserIdentity(p auth.Provider, userID string) (*auth.AuthUser, bool) {
	identityCacheMu.RLock()
	if entry, ok := identityCache[userID]; ok && time.Since(entry.fetchedAt) < identityCacheTTL {
		identityCacheMu.RUnlock()
		return entry.user, true
	}
	identityCacheMu.RUnlock()

	user, ok := identityFetcher(p, userID)
	// Only cache positive results. On failure, skip the cache write so the next
	// request retries immediately instead of locking valid users out.
	if ok {
		identityCacheMu.Lock()
		identityCache[userID] = &identityCacheEntry{user: user, fetchedAt: time.Now()}
		identityCacheMu.Unlock()
	}
	return user, ok
}

// resetIdentityCache is intended for test isolation; not used in production code.
func resetIdentityCache() {
	identityCacheMu.Lock()
	identityCache = make(map[string]*identityCacheEntry)
	identityCacheMu.Unlock()
}

// tenantOrDefault guards against providers returning an empty TenantID: the
// middleware is the last line of defense, so an empty tenant falls back to
// tenant.DefaultID.
func tenantOrDefault(id string) string {
	if id == "" {
		return tenant.DefaultID
	}
	return id
}
