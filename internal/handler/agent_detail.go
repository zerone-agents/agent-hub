package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/runtime"

	"github.com/gin-gonic/gin"
)

// AgentDetailService is the subset of *services.AgentChatService that
// AgentDetailHandler depends on. Declared as an interface (following the
// knowledge_mcp.go pattern) so tests can substitute a fake without
// constructing the full AgentChatService.
//
// *services.AgentChatService implicitly satisfies this interface.
type AgentDetailService interface {
	ResolveRuntime(tenantID, agentName string) (string, string, string, error)
	RuntimeClient() *runtime.Client
}

// AgentDetailHandler proxies GET /v1/agents/:agentId from runtime.
// It deliberately bypasses respondSuccess and writes the runtime JSON
// bytes verbatim, because the runtime's AgentDetail is already a stable
// API shape that should pass through unchanged.
type AgentDetailHandler struct {
	svc AgentDetailService
}

// NewAgentDetailHandler accepts a service that satisfies AgentDetailService.
// In production, pass *services.AgentChatService; it implicitly satisfies the
// interface. Tests pass a fake satisfying the same interface.
func NewAgentDetailHandler(svc AgentDetailService) *AgentDetailHandler {
	return &AgentDetailHandler{svc: svc}
}

// GetAgentDetail handles GET /api/v1/admin/agents/:name/detail.
// It resolves the agent's runtime container, fetches AgentDetail from
// runtime, and returns the JSON bytes verbatim.
func (h *AgentDetailHandler) GetAgentDetail(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))

	baseURL, apiKey, _, err := h.svc.ResolveRuntime(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// runtime 注册名为部署键（DeployKey(tenantID, name)），需用限定名寻址，裸名会 404。
	body, err := h.svc.RuntimeClient().GetAgentDetail(ctx, baseURL, services.DeployKey(tenant.GetTenantID(c), agentName), apiKey)
	if err != nil {
		// runtime client wraps non-2xx as "runtime returned HTTP %d: ..."
		// We map 404 specifically; other errors are 502 (unreachable or non-2xx).
		if strings.Contains(err.Error(), "HTTP 404") {
			respondError(c, http.StatusNotFound, "Agent not found in runtime")
			return
		}
		respondError(c, http.StatusBadGateway, "runtime unreachable: "+err.Error())
		return
	}

	// Naked passthrough: do NOT wrap in {success, data}. The runtime JSON
	// shape is the stable contract; re-wrapping would just bloat responses.
	c.Data(http.StatusOK, "application/json", body)
}
