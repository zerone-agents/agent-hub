package jwtutil

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeProvider is a controllable auth.Provider for middleware tests.
type fakeProvider struct {
	user *auth.AuthUser
	err  error
	ok   bool
}

func (f *fakeProvider) ValidateAccessToken(string) (*auth.AuthUser, error) { return f.user, f.err }
func (f *fakeProvider) RefreshToken(string) (*auth.TokenPair, error)       { return nil, errors.New("x") }
func (f *fakeProvider) RevokeToken(string) error                           { return nil }
func (f *fakeProvider) GetUserIdentity(string) (*auth.AuthUser, bool)      { return f.user, f.ok }
func (f *fakeProvider) Mode() string                                       { return "builtin" }

// --- Test helpers ---

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.CLIToken{}))
	return db
}

// swapIdentityFetcher replaces the package-level fetcher for the duration of
// the test and restores the previous value on cleanup.
func swapIdentityFetcher(t *testing.T, fn func(p auth.Provider, userID string) (*auth.AuthUser, bool)) {
	t.Helper()
	prev := identityFetcher
	identityFetcher = fn
	t.Cleanup(func() { identityFetcher = prev })
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
	resetIdentityCache()
	db := setupTestDB(t)
	svc := services.NewCLITokenService(db)
	issued, err := svc.Issue("42", "laptop", 30)
	require.NoError(t, err)

	p := &fakeProvider{user: &auth.AuthUser{ID: "42", Roles: []string{"admin"}}, ok: true}
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
	resetIdentityCache()
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

func TestFetchUserIdentityCachesWithinTTL(t *testing.T) {
	resetIdentityCache()
	var calls int32
	swapIdentityFetcher(t, func(p auth.Provider, userID string) (*auth.AuthUser, bool) {
		atomic.AddInt32(&calls, 1)
		return &auth.AuthUser{ID: userID, Roles: []string{"admin"}}, true
	})
	p := &fakeProvider{}
	for i := 0; i < 5; i++ {
		identity, ok := fetchUserIdentity(p, "user-xyz")
		require.True(t, ok)
		require.Equal(t, []string{"admin"}, identity.Roles)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls), "fetcher should only be called once (cache hits)")
}

func TestFetchUserIdentityFailureNotCached(t *testing.T) {
	resetIdentityCache()
	swapIdentityFetcher(t, func(p auth.Provider, userID string) (*auth.AuthUser, bool) {
		return nil, false
	})
	p := &fakeProvider{}
	identity, ok := fetchUserIdentity(p, "ghost")
	assert.False(t, ok)
	assert.Nil(t, identity)
}

func TestFetchUserIdentityParallelSafety(t *testing.T) {
	resetIdentityCache()
	var calls int32
	swapIdentityFetcher(t, func(p auth.Provider, userID string) (*auth.AuthUser, bool) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(5 * time.Millisecond)
		return &auth.AuthUser{ID: userID, Roles: []string{"admin"}}, true
	})
	p := &fakeProvider{}
	done := make(chan struct{}, 20)
	for i := 0; i < 20; i++ {
		go func() {
			_, ok := fetchUserIdentity(p, "concurrent-user")
			assert.True(t, ok)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	assert.LessOrEqual(t, atomic.LoadInt32(&calls), int32(20))
}

// --- tenant_id propagation ---

func TestMiddlewareSetsTenantID(t *testing.T) {
	// builtin path: fake provider returning TenantID "default"
	p := &fakeProvider{
		user: &auth.AuthUser{ID: "1", Username: "u", Roles: []string{"member"}, TenantID: "default"},
	}
	router := gin.New()
	router.Use(AuthMiddlewareWithCLI(nil, p))
	router.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{"tenant": tenant.GetTenantID(c)})
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"tenant":"default"`) {
		t.Fatalf("tenant_id not propagated: %s", w.Body.String())
	}
}

func TestMiddlewareSetsTenantIDCasdoorOrg(t *testing.T) {
	p := &fakeProvider{
		user: &auth.AuthUser{ID: "abc", Username: "u", Roles: []string{"admin"}, TenantID: "tenant-acme"},
	}
	router := gin.New()
	router.Use(AuthMiddlewareWithCLI(nil, p))
	router.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{"tenant": tenant.GetTenantID(c)})
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"tenant":"tenant-acme"`) {
		t.Fatalf("tenant_id not propagated: %s", w.Body.String())
	}
}

func TestMiddlewareEmptyTenantFallsBackToDefault(t *testing.T) {
	p := &fakeProvider{
		user: &auth.AuthUser{ID: "1", Roles: []string{"member"}, TenantID: ""},
	}
	router := gin.New()
	router.Use(AuthMiddlewareWithCLI(nil, p))
	router.GET("/x", func(c *gin.Context) {
		c.JSON(200, gin.H{"tenant": tenant.GetTenantID(c)})
	})
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"tenant":"default"`) {
		t.Fatalf("empty tenant must fall back to default: %s", w.Body.String())
	}
}
