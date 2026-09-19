package middleware

import (
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/jwtutil"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// JWTAuthWithCLIOrExtension keeps browser/CLI authentication unchanged while
// allowing a verified extension credential to be the caller's sole identity.
// Extensions no longer need (and must not be given) a human/CLI bearer token.
func JWTAuthWithCLIOrExtension(cliSvc *services.CLITokenService, p auth.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		if name := ExtensionIdentityOf(c); name != "" {
			c.Set("user_id", "extension:"+name)
			c.Set("user_name", name)
			c.Set("display_name", name)
			c.Set("org_id", tenant.GetTenantID(c))
			c.Set("roles", []string{"admin"})
			c.Set("permissions", []string{})
			c.Set("auth_method", "extension")
			c.Next()
			return
		}
		jwtutil.Authenticate(c, cliSvc, p)
		if !c.IsAborted() {
			c.Next()
		}
	}
}

// ExtensionAPIGuard is the default-deny boundary for extension principals.
// Human and CLI requests are unaffected. An extension may only enter an
// explicitly mapped core API and every mapped request is checked against its
// manifest grant. This prevents an extension from dropping identity headers
// and reusing a human token: extension deployments no longer require that
// token, while their own credential can reach only this allow-list.
func ExtensionAPIGuard(enforcer ExtensionEnforcer) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := ExtensionIdentityOf(c)
		if name == "" {
			c.Next()
			return
		}
		permission, action, resource, ok := extensionRoutePermission(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "扩展不允许访问该平台接口"})
			return
		}
		if !enforcer.Enforce(tenant.GetTenantID(c), name, permission, action, resource, c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "扩展未被授予该权限"})
			return
		}
		c.Set("extension_authorized", true)
		c.Next()
	}
}

func extensionRoutePermission(c *gin.Context) (permission, action, resource string, ok bool) {
	path := c.FullPath()
	method := c.Request.Method
	action = "write"
	if method == http.MethodGet || method == http.MethodHead {
		action = "read"
	}
	resource = firstNonEmpty(c.Param("name"), c.Param("id"))

	switch {
	case strings.HasPrefix(path, "/api/v1/admin/agents"):
		return "agent", action, resource, true
	case strings.HasPrefix(path, "/api/v1/admin/tools"):
		return "tool", action, resource, true
	case strings.HasPrefix(path, "/api/v1/admin/mcps"):
		return "tool", action, resource, true
	case strings.HasPrefix(path, "/api/v1/admin/skills"):
		return "tool", action, resource, true
	case strings.HasPrefix(path, "/api/v1/admin/runs") && (strings.Contains(path, "/states") ||
		strings.Contains(path, "/state-changes") || strings.Contains(path, "/persona-state") ||
		strings.Contains(path, "/belief-disputes")):
		return "state", action, c.Param("id"), true
	case strings.HasPrefix(path, "/api/v1/admin/runs") && strings.Contains(path, "/events"):
		return "event", action, c.Param("id"), true
	case strings.HasPrefix(path, "/api/v1/admin/runs") && strings.Contains(path, "/tool-results"):
		return "tool", action, c.Param("id"), true
	case strings.HasPrefix(path, "/api/v1/admin/runs") && strings.Contains(path, "/agent-messages"):
		return "message", action, c.Param("id"), true
	case strings.HasPrefix(path, "/api/v1/admin/runs"):
		return "run", action, c.Param("id"), true
	case strings.HasPrefix(path, "/api/v1/admin/state-schemas"):
		return "state", action, resource, true
	case strings.HasPrefix(path, "/api/v1/admin/workflows"):
		return "workflow", action, resource, true
	}
	return "", "", "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
