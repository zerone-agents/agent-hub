package jwtutil

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"

	"github.com/gin-gonic/gin"
)

// AuthMiddlewareWithCLI is the single auth middleware entry point. It accepts
// either a provider-validated access token or an opaque cli_<hex> CLI token
// (when cliSvc is non-nil). The provider abstracts builtin vs casdoor backends;
// passing nil provider rejects every request (used during bootstrap before a
// provider is assembled).
//
// Context keys set (unchanged names from the legacy Casdoor middleware):
//   user_id, user_name, email, display_name, org_id, avatar,
//   roles ([]string, normalized to admin|maintainer|member),
//   permissions, auth_method ("builtin" | "casdoor" | "cli").
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
			// roles are looked up fresh via the provider (cached briefly). If
			// the user no longer exists or is disabled, reject the token.
			roles, ok := fetchUserRoles(p, record.UserID)
			if !ok {
				unauthorized(c, "cli token user not found")
				return
			}
			c.Set("user_id", record.UserID)
			c.Set("user_name", "")
			c.Set("email", "")
			c.Set("display_name", "")
			c.Set("org_id", "")
			c.Set("avatar", "")
			c.Set("roles", roles)
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
		c.Set("org_id", "")
		c.Set("avatar", user.Avatar)
		c.Set("roles", user.Roles)
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

// --- User-roles cache (CLI token path) ---
//
// Roles are fetched via the provider on the first CLI-token request for a user
// and cached briefly for performance. Only positive results are cached, so a
// transient provider failure retries immediately instead of locking a valid
// user out for the TTL.

const rolesCacheTTL = 5 * time.Minute

type rolesCacheEntry struct {
	roles     []string
	fetchedAt time.Time
}

var (
	rolesCache   = make(map[string]*rolesCacheEntry)
	rolesCacheMu sync.RWMutex

	// rolesFetcher is the function used to look up a user's roles via the
	// provider. Swappable in tests; defaults to calling p.GetUserRoles.
	rolesFetcher func(p auth.Provider, userID string) ([]string, bool) = func(p auth.Provider, userID string) ([]string, bool) {
		return p.GetUserRoles(userID)
	}
)

// fetchUserRoles returns the cached roles for userID, or fetches them via the
// provider when the cache entry is missing or stale. The bool is false if the
// user could not be found / is disabled / lookup failed.
func fetchUserRoles(p auth.Provider, userID string) ([]string, bool) {
	rolesCacheMu.RLock()
	if entry, ok := rolesCache[userID]; ok && time.Since(entry.fetchedAt) < rolesCacheTTL {
		rolesCacheMu.RUnlock()
		return entry.roles, true
	}
	rolesCacheMu.RUnlock()

	roles, ok := rolesFetcher(p, userID)
	// Only cache positive results. On failure, skip the cache write so the next
	// request retries immediately instead of locking valid users out.
	if ok {
		rolesCacheMu.Lock()
		rolesCache[userID] = &rolesCacheEntry{roles: roles, fetchedAt: time.Now()}
		rolesCacheMu.Unlock()
	}
	return roles, ok
}

// resetRolesCache is intended for test isolation; not used in production code.
func resetRolesCache() {
	rolesCacheMu.Lock()
	rolesCache = make(map[string]*rolesCacheEntry)
	rolesCacheMu.Unlock()
}
