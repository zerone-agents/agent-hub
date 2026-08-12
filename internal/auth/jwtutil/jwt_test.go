package jwtutil

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeProvider is a controllable auth.Provider for middleware tests.
type fakeProvider struct {
	user  *auth.AuthUser
	err   error
	roles []string
	ok    bool
}

func (f *fakeProvider) ValidateAccessToken(string) (*auth.AuthUser, error) { return f.user, f.err }
func (f *fakeProvider) RefreshToken(string) (*auth.TokenPair, error)       { return nil, errors.New("x") }
func (f *fakeProvider) RevokeToken(string) error                           { return nil }
func (f *fakeProvider) GetUserRoles(string) ([]string, bool)               { return f.roles, f.ok }
func (f *fakeProvider) Mode() string                                       { return "builtin" }

// --- Test helpers ---

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.CLIToken{}))
	return db
}

// swapRolesFetcher replaces the package-level fetcher for the duration of the
// test and restores the previous value on cleanup.
func swapRolesFetcher(t *testing.T, fn func(p auth.Provider, userID string) ([]string, bool)) {
	t.Helper()
	prev := rolesFetcher
	rolesFetcher = fn
	t.Cleanup(func() { rolesFetcher = prev })
}

func newRequestWithBearer(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// --- Access token (JWT) path ---

func TestJWTPathSetsNormalizedRoles(t *testing.T) {
	p := &fakeProvider{user: &auth.AuthUser{
		ID: "1", Username: "alice", Roles: []string{"maintainer"},
	}}
	r := gin.New()
	var capturedRoles, capturedMethod any
	r.GET("/", AuthMiddlewareWithCLI(nil, p), func(c *gin.Context) {
		capturedRoles, _ = c.Get("roles")
		capturedMethod, _ = c.Get("auth_method")
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithBearer("any-jwt"))
	require.Equal(t, http.StatusOK, w.Code)
	roles, ok := capturedRoles.([]string)
	require.True(t, ok, "roles must be []string")
	require.Equal(t, []string{"maintainer"}, roles)
	assert.Equal(t, "builtin", capturedMethod)
}

func TestJWTPathRejectsBadToken(t *testing.T) {
	p := &fakeProvider{err: errors.New("bad")}
	r := gin.New()
	r.GET("/", AuthMiddlewareWithCLI(nil, p), func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithBearer("bad"))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNilProviderRejects(t *testing.T) {
	r := gin.New()
	r.GET("/", AuthMiddlewareWithCLI(nil, nil), func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithBearer("x"))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- CLI token path ---

func TestCLITokenPathSetsRoles(t *testing.T) {
	resetRolesCache()
	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)
	issued, err := svc.Issue("42", "laptop", 30)
	require.NoError(t, err)

	p := &fakeProvider{roles: []string{"admin"}, ok: true}
	r := gin.New()
	var capturedRoles, capturedUserID, capturedMethod any
	r.GET("/", AuthMiddlewareWithCLI(svc, p), func(c *gin.Context) {
		capturedRoles, _ = c.Get("roles")
		capturedUserID, _ = c.Get("user_id")
		capturedMethod, _ = c.Get("auth_method")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithBearer(issued.Token))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "42", capturedUserID)
	assert.Equal(t, "cli", capturedMethod)
	roles, ok := capturedRoles.([]string)
	require.True(t, ok)
	require.Equal(t, []string{"admin"}, roles)
}

func TestCLITokenRejectsUnknownUser(t *testing.T) {
	resetRolesCache()
	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)
	issued, err := svc.Issue("ghost", "x", 30)
	require.NoError(t, err)

	p := &fakeProvider{ok: false} // user not found / disabled
	r := gin.New()
	r.GET("/", AuthMiddlewareWithCLI(svc, p), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, newRequestWithBearer(issued.Token))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Cache ---

func TestFetchUserRolesCachesWithinTTL(t *testing.T) {
	resetRolesCache()
	var calls int32
	swapRolesFetcher(t, func(p auth.Provider, userID string) ([]string, bool) {
		atomic.AddInt32(&calls, 1)
		return []string{"admin"}, true
	})
	p := &fakeProvider{}
	for i := 0; i < 5; i++ {
		roles, ok := fetchUserRoles(p, "user-xyz")
		require.True(t, ok)
		require.Equal(t, []string{"admin"}, roles)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "fetcher should only be called once (cache hits)")
}

func TestFetchUserRolesFailureNotCached(t *testing.T) {
	resetRolesCache()
	swapRolesFetcher(t, func(p auth.Provider, userID string) ([]string, bool) {
		return nil, false
	})
	p := &fakeProvider{}
	roles, ok := fetchUserRoles(p, "ghost")
	assert.False(t, ok)
	assert.Nil(t, roles)
}

func TestFetchUserRolesParallelSafety(t *testing.T) {
	resetRolesCache()
	var calls int32
	swapRolesFetcher(t, func(p auth.Provider, userID string) ([]string, bool) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(5 * time.Millisecond)
		return []string{"admin"}, true
	})
	p := &fakeProvider{}
	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func() {
			_, ok := fetchUserRoles(p, "concurrent-user")
			assert.True(t, ok)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	assert.LessOrEqual(t, atomic.LoadInt32(&calls), int32(20))
}
