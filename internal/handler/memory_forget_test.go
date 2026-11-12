package handler

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"control-panel/internal/memory"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryNativeForgetPermissionLifecycleAndAudit(t *testing.T) {
	router, db, scope, native, store := memoryManagementFixture(t)
	path := "/api/v1/admin/memory/native-messages/9007199254740994/forget"
	body := map[string]any{"scope": scope}
	result := memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "maintainer", body)
	row := memoryAdminData[memory.NativeMessage](t, result)
	require.NotEmpty(t, row.ForgottenAt)
	require.False(t, row.Status)
	require.Equal(t, 1, native.forgetCalls)
	require.Equal(t, "9007199254740994", native.id)
	require.Equal(t, "memory.admin.native.forget", string(store.rows[0].Action))
	require.NotContains(t, store.rows[0].Detail, "PRIVATE_SYNTHETIC_CHAT_BODY")
	require.NotContains(t, store.rows[0].Detail, scope.UserID)
	for _, tc := range []struct {
		hub, role string
		status    int
	}{
		{"delegated-hub", "member", 403}, {"foreign-hub", "admin", 403}, {"", "admin", 401},
	} {
		result = memoryAdminHTTP(t, router, "POST", path, tc.hub, tc.role, body)
		require.Equal(t, tc.status, result.Code)
		require.Equal(t, 1, native.forgetCalls)
	}
	result = memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "admin", map[string]any{"scope": scope, "memoryId": "forged"})
	require.Equal(t, 400, result.Code)
	result = memoryAdminHTTP(t, router, "POST", "/api/v1/admin/memory/native-messages/01/forget", "delegated-hub", "admin", body)
	require.Equal(t, 400, result.Code)
	native.statusErr = fmt.Errorf("%w: PRIVATE_PROVIDER_SECRET", memory.ErrIndexUnknown)
	result = memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "admin", body)
	require.Equal(t, 503, result.Code)
	require.NotContains(t, result.Body.String(), "PRIVATE_PROVIDER_SECRET")
	require.NoError(t, db.Table("memory_lifecycles").Where("state = ?", "active").Update("state", "frozen").Error)
	result = memoryAdminHTTP(t, router, "POST", path, "delegated-hub", "admin", body)
	require.Equal(t, 403, result.Code)
	require.Equal(t, 2, native.forgetCalls)
}

func TestDisabledMemoryReturnsJSONInsteadOfRedirect(t *testing.T) {
	testUnavailableMemoryRoutes(t, RegisterDisabledMemoryAdminRoutes, "memory_disabled")
}

func TestUnconfiguredMemoryReturnsJSONInsteadOfRedirect(t *testing.T) {
	testUnavailableMemoryRoutes(t, RegisterUnconfiguredMemoryAdminRoutes, "memory_not_configured")
}

func testUnavailableMemoryRoutes(t *testing.T, register func(*gin.RouterGroup), code string) {
	t.Helper()
	router := gin.New()
	group := router.Group("/api/v1/admin", func(c *gin.Context) {
		c.Set("roles", []string{c.GetHeader("X-Test-Role")})
	})
	register(group)
	router.NoRoute(func(c *gin.Context) { c.Redirect(302, "/static") })
	for _, tc := range []struct {
		role   string
		status int
	}{{"admin", 503}, {"maintainer", 503}, {"member", 403}, {"", 403}} {
		req := httptest.NewRequest("GET", "/api/v1/admin/memory/applications", nil)
		req.Header.Set("X-Test-Role", tc.role)
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		require.Equal(t, tc.status, result.Code)
		require.Empty(t, result.Header().Get("Location"))
		require.Contains(t, result.Header().Get("Content-Type"), "application/json")
		if tc.status == 503 {
			require.Contains(t, result.Body.String(), code)
		}
	}
}
