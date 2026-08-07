package jwtutil

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware is the default Casdoor JWT middleware. CLI token support is
// disabled; use AuthMiddlewareWithCLI to also accept cli_* tokens.
func AuthMiddleware() gin.HandlerFunc {
	return AuthMiddlewareWithCLI(nil)
}

// AuthMiddlewareWithCLI returns middleware that accepts either:
//   - Casdoor JWT tokens (existing OAuth flow), or
//   - Opaque CLI tokens of the form cli_<hex> (looked up in the DB via svc).
//
// Passing nil svc disables the CLI path and behaves identically to
// AuthMiddleware.
//
// Per spec §8.1, CLI tokens are carriers only — the user's roles are looked up
// fresh from Casdoor (cached briefly for performance) so that admin users get
// admin permissions through their CLI tokens, and regular users do not.
func AuthMiddlewareWithCLI(cliSvc *services.CLITokenService) gin.HandlerFunc {
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
			if cliSvc == nil {
				unauthorized(c, "cli token support is disabled")
				return
			}
			record, err := cliSvc.Verify(tokenString)
			if err != nil {
				unauthorized(c, "invalid cli token: "+err.Error())
				return
			}
			// CLI tokens identify the user but carry no Casdoor profile data.
			// Per spec §8.1, fetch the user's roles from Casdoor so admin users
			// can use CLI tokens for admin operations. If the user no longer
			// exists in Casdoor (nil roles + lookup failed), reject the token.
			roles, ok := fetchUserRoles(record.UserID)
			if !ok {
				unauthorized(c, "cli token user not found in casdoor")
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

		// Default Casdoor JWT flow.
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			unauthorized(c, "invalid token: "+err.Error())
			return
		}

		c.Set("user_id", claims.Id)
		c.Set("user_name", claims.Name)
		c.Set("email", claims.Email)
		c.Set("display_name", claims.DisplayName)
		c.Set("org_id", claims.Owner)
		c.Set("avatar", claims.Avatar)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
		c.Set("auth_method", "casdoor")
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

// --- User-roles cache (spec §8.1: real-time roles, cached for performance) ---

const rolesCacheTTL = 5 * time.Minute

type rolesCacheEntry struct {
	roles     []*casdoorsdk.Role
	fetchedAt time.Time
}

var (
	rolesCache   = make(map[string]*rolesCacheEntry)
	rolesCacheMu sync.RWMutex

	// rolesFetcher is the function used to look up a user's roles from Casdoor.
	// Swappable in tests; defaults to the real Casdoor client call. Returns
	// (roles, true) on success, (nil, false) if the user is unknown or the
	// lookup failed.
	rolesFetcher func(userID string) ([]*casdoorsdk.Role, bool) = defaultRolesFetcher
)

// fetchUserRoles returns the cached roles for userID, or fetches them from
// Casdoor when the cache entry is missing or stale. The bool is false if the
// user could not be found in Casdoor (treated as revoked / invalid).
func fetchUserRoles(userID string) ([]*casdoorsdk.Role, bool) {
	rolesCacheMu.RLock()
	if entry, ok := rolesCache[userID]; ok && time.Since(entry.fetchedAt) < rolesCacheTTL {
		rolesCacheMu.RUnlock()
		return entry.roles, true
	}
	rolesCacheMu.RUnlock()

	roles, ok := rolesFetcher(userID)

	// Only cache positive results. On failure (missing user OR transient
	// Casdoor error), skip the cache write so the next request retries
	// immediately instead of locking valid users out for the cache TTL.
	if ok {
		rolesCacheMu.Lock()
		rolesCache[userID] = &rolesCacheEntry{roles: roles, fetchedAt: time.Now()}
		rolesCacheMu.Unlock()
	}
	return roles, ok
}

// defaultRolesFetcher looks up the user via the Casdoor SDK and returns their
// roles. Returns (nil, false) if the user is unknown or the call fails.
func defaultRolesFetcher(userID string) ([]*casdoorsdk.Role, bool) {
	client := auth.GetClient()
	if client == nil {
		return nil, false
	}
	user, err := client.GetUserByUserId(userID)
	if err != nil || user == nil {
		return nil, false
	}
	return user.Roles, true
}

// resetRolesCache is intended for test isolation; not used in production code.
func resetRolesCache() {
	rolesCacheMu.Lock()
	rolesCache = make(map[string]*rolesCacheEntry)
	rolesCacheMu.Unlock()
}
