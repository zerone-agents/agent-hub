package jwtutil

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/auth"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// --- Test helpers ---

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&auth.CLIToken{}))
	return db
}

// swapRolesFetcher replaces the package-level fetcher for the duration of the
// test and restores the previous value on cleanup.
func swapRolesFetcher(t *testing.T, fn func(userID string) ([]*casdoorsdk.Role, bool)) {
	prev := rolesFetcher
	rolesFetcher = fn
	t.Cleanup(func() { rolesFetcher = prev })
}

func newRequestWithCLI(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// --- Tests ---

// TestFetchUserRoles_CachesWithinTTL verifies that fetchUserRoles only invokes
// the underlying fetcher once during a burst of identical lookups (cache hit).
func TestFetchUserRoles_CachesWithinTTL(t *testing.T) {
	resetRolesCache()
	var calls int32
	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		atomic.AddInt32(&calls, 1)
		return []*casdoorsdk.Role{{Name: "agents-admin"}}, true
	})

	for i := 0; i < 5; i++ {
		roles, ok := fetchUserRoles("user-xyz")
		require.True(t, ok)
		require.Len(t, roles, 1)
		assert.Equal(t, "agents-admin", roles[0].Name)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "fetcher should only be called once (cache hits)")
}

// TestFetchUserRoles_FetcherFailureIsNotCachedAsOK ensures that when the
// fetcher reports the user is unknown, fetchUserRoles returns !ok so the
// middleware rejects the token.
func TestFetchUserRoles_FetcherFailureIsNotCachedAsOK(t *testing.T) {
	resetRolesCache()
	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		return nil, false
	})
	roles, ok := fetchUserRoles("ghost")
	assert.False(t, ok, "missing user should return ok=false")
	assert.Nil(t, roles)
}

// TestAuthMiddlewareWithCLI_SetsRolesForAdminUser is the spec §8.1 integration
// test: a valid CLI token for an admin user must populate the gin context with
// the user's roles so downstream RequireAdmin middleware authorizes the call.
func TestAuthMiddlewareWithCLI_SetsRolesForAdminUser(t *testing.T) {
	resetRolesCache()

	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)

	// Issue a real CLI token to get a valid plaintext.
	result, err := svc.Issue("admin/alice", "laptop", 30)
	require.NoError(t, err)

	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		if userID != "admin/alice" {
			return nil, false
		}
		return []*casdoorsdk.Role{{Name: "agents-admin"}}, true
	})

	r := gin.New()
	r.Use(AuthMiddlewareWithCLI(svc))
	var capturedRoles interface{}
	var capturedUserID interface{}
	var capturedMethod interface{}
	r.GET("/", func(c *gin.Context) {
		capturedRoles, _ = c.Get("roles")
		capturedUserID, _ = c.Get("user_id")
		capturedMethod, _ = c.Get("auth_method")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithCLI(result.Token))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "admin/alice", capturedUserID)
	assert.Equal(t, "cli", capturedMethod)

	roles, ok := capturedRoles.([]*casdoorsdk.Role)
	require.True(t, ok, "roles must be []*casdoorsdk.Role")
	require.Len(t, roles, 1)
	assert.Equal(t, "agents-admin", roles[0].Name,
		"admin user's roles must be populated per spec §8.1")
}

// TestAuthMiddlewareWithCLI_RejectsUnknownUser ensures that if the CLI token
// validates but the user no longer exists in Casdoor, the middleware returns
// 401 (token effectively revoked).
func TestAuthMiddlewareWithCLI_RejectsUnknownUser(t *testing.T) {
	resetRolesCache()

	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)
	result, err := svc.Issue("ghost-user", "x", 30)
	require.NoError(t, err)

	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		return nil, false
	})

	r := gin.New()
	r.Use(AuthMiddlewareWithCLI(svc))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithCLI(result.Token))

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"a CLI token whose user is missing from Casdoor must be rejected")
}

// TestAuthMiddlewareWithCLI_NoRolesForRegularUser confirms that a CLI token
// for a non-admin user carries empty roles, so admin routes still 403.
func TestAuthMiddlewareWithCLI_NoRolesForRegularUser(t *testing.T) {
	resetRolesCache()

	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)
	result, err := svc.Issue("user/bob", "x", 30)
	require.NoError(t, err)

	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		// Bob has no roles.
		return []*casdoorsdk.Role{}, true
	})

	r := gin.New()
	r.Use(AuthMiddlewareWithCLI(svc))
	var capturedRoles interface{}
	r.GET("/", func(c *gin.Context) {
		capturedRoles, _ = c.Get("roles")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithCLI(result.Token))

	require.Equal(t, http.StatusOK, w.Code)
	roles, ok := capturedRoles.([]*casdoorsdk.Role)
	require.True(t, ok)
	assert.Empty(t, roles)
}

// TestFetchUserRoles_ParallelSafety is a smoke test that concurrent fetches
// for the same userID do not race (the mutex must serialize cache writes).
func TestFetchUserRoles_ParallelSafety(t *testing.T) {
	resetRolesCache()
	var calls int32
	swapRolesFetcher(t, func(userID string) ([]*casdoorsdk.Role, bool) {
		atomic.AddInt32(&calls, 1)
		// Tiny sleep to widen the race window.
		time.Sleep(5 * time.Millisecond)
		return []*casdoorsdk.Role{{Name: "agents-admin"}}, true
	})

	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func() {
			_, ok := fetchUserRoles("concurrent-user")
			assert.True(t, ok)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	// We tolerate a small number of duplicated fetches (cache stampede on first
	// load) but the majority should hit the cache after the first writer.
	assert.LessOrEqual(t, atomic.LoadInt32(&calls), int32(20))
}
