package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

type RunHandler struct{ service *services.RunService }

func NewRunHandler(service *services.RunService) *RunHandler { return &RunHandler{service: service} }

type createRunRequest struct {
	Name         string                        `json:"name"`
	Description  string                        `json:"description"`
	Metadata     map[string]any                `json:"metadata"`
	Capabilities []services.RunCapabilityInput `json:"capabilityBindings"`
}

func (h *RunHandler) Create(c *gin.Context) {
	var req createRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	r, err := h.service.Create(tenant.GetTenantID(c), services.CreateRunInput{Name: req.Name, Description: req.Description, Metadata: req.Metadata, CreatedBy: actorID(c), Capabilities: req.Capabilities})
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, r)
}
func (h *RunHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rows, err := h.service.List(tenant.GetTenantID(c), c.Query("status"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}
func (h *RunHandler) Get(c *gin.Context) {
	r, err := h.service.Get(tenant.GetTenantID(c), c.Param("id"))
	if err != nil {
		writeRunError(c, err)
		return
	}
	states, err := h.service.States(tenant.GetTenantID(c), r.ID)
	if err != nil {
		writeRunError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"run": r, "states": states}})
}
func (h *RunHandler) Transition(c *gin.Context) {
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	r, err := h.service.Transition(tenant.GetTenantID(c), c.Param("id"), req.Status)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, r)
}
func (h *RunHandler) AddAgent(c *gin.Context) {
	var req struct {
		AgentID uint64 `json:"agentId"`
		Role    string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AgentID == 0 {
		respondError(c, 400, "agentId is required")
		return
	}
	row, err := h.service.AddAgent(tenant.GetTenantID(c), c.Param("id"), req.AgentID, req.Role)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, row)
}

type schemaRequest struct {
	Namespace    string         `json:"namespace"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Schema       map[string]any `json:"schema"`
	ScopeTypes   []string       `json:"scopeTypes"`
	SubjectTypes []string       `json:"subjectTypes"`
}

func (h *RunHandler) RegisterSchema(c *gin.Context) {
	var req schemaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	row, err := h.service.RegisterStateSchema(tenant.GetTenantID(c), services.RegisterStateSchemaInput{Namespace: req.Namespace, Name: req.Name, Version: req.Version, Schema: req.Schema, ScopeTypes: req.ScopeTypes, SubjectTypes: req.SubjectTypes})
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, row)
}

type initializeStateRequest struct {
	Namespace      string         `json:"namespace"`
	SchemaName     string         `json:"schemaName"`
	SchemaVersion  string         `json:"schemaVersion"`
	SubjectType    string         `json:"subjectType"`
	SubjectID      string         `json:"subjectId"`
	Data           map[string]any `json:"data"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Reason         string         `json:"reason"`
	Source         string         `json:"source"`
}

func (h *RunHandler) InitializeState(c *gin.Context) {
	var req initializeStateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	row, err := h.service.InitializeState(tenant.GetTenantID(c), c.Param("id"), services.InitializeStateInput{Namespace: req.Namespace, SchemaName: req.SchemaName, SchemaVersion: req.SchemaVersion, SubjectType: req.SubjectType, SubjectID: req.SubjectID, Data: req.Data, IdempotencyKey: req.IdempotencyKey, Reason: req.Reason, Source: req.Source})
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, row)
}
func (h *RunHandler) CommitState(c *gin.Context) {
	sid, err := strconv.ParseUint(c.Param("stateId"), 10, 64)
	if err != nil {
		respondError(c, 400, "invalid state id")
		return
	}
	var req struct {
		ExpectedRevision uint64         `json:"expectedRevision"`
		Data             map[string]any `json:"data"`
		IdempotencyKey   string         `json:"idempotencyKey"`
		Reason           string         `json:"reason"`
		Source           string         `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	row, e := h.service.CommitState(tenant.GetTenantID(c), c.Param("id"), sid, services.CommitStateInput{ExpectedRevision: req.ExpectedRevision, Data: req.Data, IdempotencyKey: req.IdempotencyKey, Reason: req.Reason, Source: req.Source})
	if e != nil {
		writeRunError(c, e)
		return
	}
	respondSuccess(c, row)
}
func (h *RunHandler) StateChanges(c *gin.Context) {
	sid, _ := strconv.ParseUint(c.Query("stateId"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := h.service.StateChanges(tenant.GetTenantID(c), c.Param("id"), sid, limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}

type activityRequest struct {
	Kind       string         `json:"kind"`
	Status     string         `json:"status"`
	ActorType  string         `json:"actorType"`
	ActorID    string         `json:"actorId"`
	StepID     string         `json:"stepId"`
	Name       string         `json:"name"`
	Input      map[string]any `json:"input"`
	Output     map[string]any `json:"output"`
	Error      string         `json:"error"`
	OccurredAt *time.Time     `json:"occurredAt"`
}

func (h *RunHandler) AppendActivity(c *gin.Context) {
	var req activityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	var at time.Time
	if req.OccurredAt != nil {
		at = *req.OccurredAt
	}
	row, err := h.service.AppendActivity(tenant.GetTenantID(c), c.Param("id"), services.AppendRunActivityInput{Kind: req.Kind, Status: req.Status, ActorType: req.ActorType, ActorID: req.ActorID, StepID: req.StepID, Name: req.Name, Input: req.Input, Output: req.Output, Error: req.Error, OccurredAt: at})
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondCreated(c, row)
}
func (h *RunHandler) Activities(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	rows, err := h.service.ListActivities(tenant.GetTenantID(c), c.Param("id"), limit)
	if err != nil {
		writeRunError(c, err)
		return
	}
	respondSuccess(c, rows)
}
func actorID(c *gin.Context) string {
	for _, key := range []string{"user_id", "sub", "username"} {
		if v, ok := c.Get(key); ok {
			return fmt.Sprint(v)
		}
	}
	return ""
}
func writeRunError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, rundomain.ErrNotFound), errors.Is(err, rundomain.ErrStateNotFound), errors.Is(err, rundomain.ErrSchemaNotFound):
		status = 404
	case errors.Is(err, rundomain.ErrConflict), errors.Is(err, rundomain.ErrDuplicate):
		status = 409
	case errors.Is(err, rundomain.ErrFrozen), errors.Is(err, rundomain.ErrInvalidTransition):
		status = 422
	}
	respondError(c, status, err.Error())
}
