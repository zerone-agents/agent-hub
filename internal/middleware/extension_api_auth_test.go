package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtensionAPIGuardDefaultDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := &fakeEnforcer{next: true}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Set(ExtensionIdentityContextKey, "io.example.ext")
		c.Next()
	}, ExtensionAPIGuard(enforcer))
	r.GET("/api/v1/admin/providers", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Empty(t, enforcer.snapshot())
}

func TestExtensionAPIGuardAgentWriteUsesGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := &fakeEnforcer{next: true}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Set(ExtensionIdentityContextKey, "io.example.ext")
		c.Next()
	}, ExtensionAPIGuard(enforcer))
	r.PUT("/api/v1/admin/agents/:name", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/worker", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []enforceCall{{
		TenantID: "tenant-a", ExtensionName: "io.example.ext", Permission: "agent",
		Action: "write", Resource: "worker", IP: "192.0.2.1",
	}}, enforcer.snapshot())
}

func TestExtensionAPIGuardHumanRequestUnaffected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforcer := &fakeEnforcer{next: false}
	r := gin.New()
	r.Use(ExtensionAPIGuard(enforcer))
	r.DELETE("/api/v1/admin/agents/:name", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/agents/worker", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Empty(t, enforcer.snapshot())
}

func TestVerifiedExtensionDoesNotNeedHumanBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Set(ExtensionIdentityContextKey, "io.example.ext")
		c.Next()
	}, JWTAuthWithCLIOrExtension(nil, nil))
	r.GET("/ok", func(c *gin.Context) {
		method, _ := c.Get("auth_method")
		roles, _ := c.Get("roles")
		c.JSON(http.StatusOK, gin.H{"method": method, "roles": roles})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"method":"extension"`)
	require.Contains(t, w.Body.String(), `"admin"`)
}
