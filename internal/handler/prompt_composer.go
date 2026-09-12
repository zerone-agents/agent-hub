package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type PromptComposerHandler struct {
	service *services.PromptComposerService
}

func NewPromptComposerHandler(service *services.PromptComposerService) *PromptComposerHandler {
	return &PromptComposerHandler{service: service}
}

func (h *PromptComposerHandler) Compose(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("agentId"), 10, 64)
	if err != nil || agentID == 0 {
		respondError(c, http.StatusBadRequest, "invalid agent id")
		return
	}
	row, err := h.service.Compose(tenant.GetTenantID(c), c.Param("id"), agentID)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, row)
}

func (h *PromptComposerHandler) Latest(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("agentId"), 10, 64)
	if err != nil || agentID == 0 {
		respondError(c, http.StatusBadRequest, "invalid agent id")
		return
	}
	row, err := h.service.Latest(tenant.GetTenantID(c), c.Param("id"), agentID)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, row)
}
