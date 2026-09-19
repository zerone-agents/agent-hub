package handler

import (
	"strconv"

	"control-panel/internal/application/services"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type ToolResultHandler struct{ service *services.ToolResultService }

func NewToolResultHandler(service *services.ToolResultService) *ToolResultHandler {
	return &ToolResultHandler{service: service}
}

type toolResultRequest struct {
	ToolName       string               `json:"toolName"`
	ActorType      string               `json:"actorType"`
	ActorID        string               `json:"actorId"`
	CorrelationID  string               `json:"correlationId"`
	CausationID    string               `json:"causationId"`
	RootEventID    string               `json:"rootEventId"`
	IdempotencyKey string               `json:"idempotencyKey"`
	ToolResult     rundomain.ToolResult `json:"toolResult"`
	Reason         string               `json:"reason,omitempty"`
}

func (r toolResultRequest) serviceInput() services.CommitToolResultInput {
	return services.CommitToolResultInput{ToolName: r.ToolName, ActorType: r.ActorType, ActorID: r.ActorID, CorrelationID: r.CorrelationID, CausationID: r.CausationID, RootEventID: r.RootEventID, IdempotencyKey: r.IdempotencyKey, Result: r.ToolResult}
}

func (h *ToolResultHandler) Validate(c *gin.Context) {
	var req toolResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	result, err := h.service.Validate(tenant.GetTenantID(c), c.Param("id"), req.ToolResult)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *ToolResultHandler) Commit(c *gin.Context) {
	var req toolResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	result, err := h.service.Commit(tenant.GetTenantID(c), c.Param("id"), req.serviceInput())
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, result)
}

func (h *ToolResultHandler) Reject(c *gin.Context) {
	var req toolResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	result, err := h.service.Reject(tenant.GetTenantID(c), c.Param("id"), req.serviceInput(), req.Reason)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, result)
}

func (h *ToolResultHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := h.service.List(tenant.GetTenantID(c), c.Param("id"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}

func (h *ToolResultHandler) Get(c *gin.Context) {
	row, err := h.service.Get(tenant.GetTenantID(c), c.Param("id"), c.Param("toolResultId"))
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, row)
}
