package handler

import (
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type RunAgentMessageReader interface {
	ListRun(tenantID, runID string, limit int) ([]*services.AgentMessageDTO, error)
	MessageChainAdmin(tenantID, conversationID, rootMessageID string, limit int) (*services.AgentMessageChainDTO, error)
}

type ChannelAgentMessageReader interface {
	ListChannel(tenantID, channelID string, limit int) ([]*services.AgentMessageDTO, error)
}

// GetChain is the administrator view of the complete tenant-local causal
// chain. Tenant identity always comes from auth context, never query input.
func (h *AgentMessageAdminHandler) GetChain(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	chain, err := h.service.MessageChainAdmin(tenant.GetTenantID(c), c.Query("conversation_id"), c.Query("root_message_id"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, chain)
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

func (h *AgentMessageAdminHandler) ListChannel(c *gin.Context) {
	reader, ok := h.service.(ChannelAgentMessageReader)
	if !ok {
		c.JSON(501, gin.H{"error": "channel history is not enabled"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := reader.ListChannel(tenant.GetTenantID(c), c.Param("id"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}
