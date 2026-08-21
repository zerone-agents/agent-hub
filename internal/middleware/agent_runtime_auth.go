package middleware

import (
	"net/http"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

// agentRuntimeAuthRepository is the subset of the agent repository used by the
// middleware. It is unexported because the middleware is a function, not a
// struct; tests inject a mock via the package-level helper below.
type agentRuntimeAuthRepository interface {
	ListAllUnscoped() ([]*agent.AgentConfig, error)
}

// AgentRuntimeAuthMiddleware returns a gin middleware that authenticates
// requests by comparing the provided Bearer token against each agent's
// encrypted RuntimeToken.
func AgentRuntimeAuthMiddleware(encryptionKey string) gin.HandlerFunc {
	return agentRuntimeAuthMiddleware(encryptionKey, repository.NewAgentRepository())
}

func agentRuntimeAuthMiddleware(encryptionKey string, repo agentRuntimeAuthRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		const bearerPrefix = "Bearer "

		if !strings.HasPrefix(authHeader, bearerPrefix) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		token := strings.TrimSpace(authHeader[len(bearerPrefix):])
		if token == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// runtime token 本身即全局凭证，无法预知租户，显式跨租户全量；
		// 命中后把该行的 tenant_id 写入 context，供下游（knowledge MCP 链）使用。
		agents, err := repo.ListAllUnscoped()
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		for _, a := range agents {
			if a == nil || a.RuntimeToken == "" {
				continue
			}

			decrypted, err := provider.Decrypt(a.RuntimeToken, encryptionKey)
			if err != nil {
				continue
			}

			if decrypted == token {
				c.Set("agent", a)
				tenant.SetTenantID(c, a.TenantID)
				c.Next()
				return
			}
		}

		c.AbortWithStatus(http.StatusUnauthorized)
	}
}

// AgentFromContext retrieves the authenticated agent config from the gin context.
func AgentFromContext(c *gin.Context) (*agent.AgentConfig, bool) {
	v, exists := c.Get("agent")
	if !exists {
		return nil, false
	}
	cfg, ok := v.(*agent.AgentConfig)
	return cfg, ok
}
