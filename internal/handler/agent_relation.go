package handler

import (
	"errors"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type AgentRelationHandler struct {
	service *services.AgentRelationService
}

func NewAgentRelationHandler(service *services.AgentRelationService) *AgentRelationHandler {
	return &AgentRelationHandler{service: service}
}

func (h *AgentRelationHandler) List(c *gin.Context) {
	relations, err := h.service.List(tenant.GetTenantID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": relations})
}

func (h *AgentRelationHandler) ListEvents(c *gin.Context) {
	id, ok := parseRelationID(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	events, err := h.service.Events(tenant.GetTenantID(c), id, limit)
	if err != nil {
		writeAgentRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": events})
}

type createAgentRelationReq struct {
	SourceAgentID            uint64   `json:"sourceAgentId" binding:"required"`
	TargetAgentID            uint64   `json:"targetAgentId" binding:"required"`
	Scope                    string   `json:"scope"`
	RelationType             string   `json:"relationType" binding:"required"`
	RelationTypeTemplateName string   `json:"relationTypeTemplateName"`
	Stance                   string   `json:"stance"`
	AllowedActions           []string `json:"allowedActions" binding:"required"`
	ContextPolicy            string   `json:"contextPolicy"`
	DeliveryPolicy           string   `json:"deliveryPolicy"`
	Constraint               string   `json:"constraint"`
	Enabled                  *bool    `json:"enabled"`
	Bidirectional            bool     `json:"bidirectional"`
}

func (h *AgentRelationHandler) Create(c *gin.Context) {
	var req createAgentRelationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	relations, err := h.service.Create(tenant.GetTenantID(c), &services.CreateAgentRelationInput{
		SourceAgentID:            req.SourceAgentID,
		TargetAgentID:            req.TargetAgentID,
		Scope:                    req.Scope,
		RelationType:             req.RelationType,
		RelationTypeTemplateName: req.RelationTypeTemplateName,
		Stance:                   req.Stance,
		AllowedActions:           req.AllowedActions,
		ContextPolicy:            req.ContextPolicy,
		DeliveryPolicy:           req.DeliveryPolicy,
		Constraint:               req.Constraint,
		Enabled:                  enabled,
		Bidirectional:            req.Bidirectional,
	})
	if err != nil {
		writeAgentRelationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": relations})
}

type updateAgentRelationReq struct {
	Scope                    *string   `json:"scope"`
	RelationType             *string   `json:"relationType"`
	RelationTypeTemplateName *string   `json:"relationTypeTemplateName"`
	Stance                   *string   `json:"stance"`
	AllowedActions           *[]string `json:"allowedActions"`
	ContextPolicy            *string   `json:"contextPolicy"`
	DeliveryPolicy           *string   `json:"deliveryPolicy"`
	Constraint               *string   `json:"constraint"`
	Enabled                  *bool     `json:"enabled"`
}

type recordAgentRelationEventReq struct {
	EventType      string `json:"eventType" binding:"required"`
	Severity       int    `json:"severity"`
	Reason         string `json:"reason"`
	Visibility     string `json:"visibility"`
	SourceKind     string `json:"sourceKind"`
	SourceID       string `json:"sourceId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

func (h *AgentRelationHandler) RecordEvent(c *gin.Context) {
	id, ok := parseRelationID(c)
	if !ok {
		return
	}
	var req recordAgentRelationEventReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	result, err := h.service.RecordEvent(tenant.GetTenantID(c), id, &services.RecordAgentRelationEventInput{
		EventType: req.EventType, Severity: req.Severity, Reason: req.Reason,
		Visibility: req.Visibility, SourceKind: req.SourceKind, SourceID: req.SourceID,
		IdempotencyKey: req.IdempotencyKey,
	}, "admin", "")
	if err != nil {
		writeAgentRelationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}

func (h *AgentRelationHandler) Update(c *gin.Context) {
	id, ok := parseRelationID(c)
	if !ok {
		return
	}
	var req updateAgentRelationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	relation, err := h.service.Update(tenant.GetTenantID(c), id, &services.UpdateAgentRelationInput{
		Scope:                    req.Scope,
		RelationType:             req.RelationType,
		RelationTypeTemplateName: req.RelationTypeTemplateName,
		Stance:                   req.Stance,
		AllowedActions:           req.AllowedActions,
		ContextPolicy:            req.ContextPolicy,
		DeliveryPolicy:           req.DeliveryPolicy,
		Constraint:               req.Constraint,
		Enabled:                  req.Enabled,
	})
	if err != nil {
		writeAgentRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": relation})
}

func (h *AgentRelationHandler) Delete(c *gin.Context) {
	id, ok := parseRelationID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(tenant.GetTenantID(c), id); err != nil {
		writeAgentRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Agent 关系已删除"})
}

func parseRelationID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "关系 ID 格式无效"})
		return 0, false
	}
	return id, true
}

func writeAgentRelationError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, agentrelation.ErrNotFound) || errors.Is(err, agentrelation.ErrAgentNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, agentrelation.ErrAlreadyExists) {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"success": false, "error": err.Error()})
}
