package handler

import (
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type RunAgentMessageReader interface {
	ListRun(tenantID, runID string, limit int) ([]*services.AgentMessageDTO, error)
}

type AgentMessageAdminHandler struct{ service RunAgentMessageReader }

func NewAgentMessageAdminHandler(service RunAgentMessageReader) *AgentMessageAdminHandler {
	return &AgentMessageAdminHandler{service: service}
}

// ListRun exposes the durable, ordered hop audit for product acceptance and
// incident review. Message bodies remain out of this summary DTO.
func (h *AgentMessageAdminHandler) ListRun(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := h.service.ListRun(tenant.GetTenantID(c), c.Param("id"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}
