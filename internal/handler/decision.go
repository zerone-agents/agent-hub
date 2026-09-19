package handler

import (
	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type DecisionHandler struct{ service *services.DecisionService }

func NewDecisionHandler(s *services.DecisionService) *DecisionHandler {
	return &DecisionHandler{service: s}
}

type createDecisionRequest struct {
	GroupID         string                  `json:"groupId"`
	WorkflowRunID   string                  `json:"workflowRunId"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	QuorumPercent   int                     `json:"quorumPercent"`
	ApprovalPercent int                     `json:"approvalPercent"`
	TimeoutAction   string                  `json:"timeoutAction"`
	EscalateAgentID uint64                  `json:"escalateAgentId"`
	DeadlineAt      *time.Time              `json:"deadlineAt"`
	Electors        []services.ElectorInput `json:"electors"`
}

func (h *DecisionHandler) Create(c *gin.Context) {
	var req createDecisionRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.Create(tenant.GetTenantID(c), services.CreateDecisionInput{GroupID: req.GroupID, WorkflowRunID: req.WorkflowRunID, Title: req.Title, Description: req.Description, QuorumPercent: req.QuorumPercent, ApprovalPercent: req.ApprovalPercent, TimeoutAction: req.TimeoutAction, EscalateAgentID: req.EscalateAgentID, DeadlineAt: req.DeadlineAt, Electors: req.Electors}, actorID(c))
	h.write(c, row, err, true)
}
func (h *DecisionHandler) List(c *gin.Context) {
	rows, err := h.service.List(tenant.GetTenantID(c), c.Query("groupId"))
	h.write(c, rows, err, false)
}
func (h *DecisionHandler) Get(c *gin.Context) {
	row, err := h.service.Get(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, row, err, false)
}
func (h *DecisionHandler) Vote(c *gin.Context) {
	var req struct {
		AgentID uint64 `json:"agentId"`
		Choice  string `json:"choice"`
		Reason  string `json:"reason"`
	}
	if c.ShouldBindJSON(&req) != nil || req.AgentID == 0 {
		respondError(c, 400, "agentId and choice are required")
		return
	}
	row, err := h.service.CastVote(tenant.GetTenantID(c), c.Param("id"), req.AgentID, req.Choice, req.Reason, actorID(c))
	h.write(c, row, err, true)
}
func (h *DecisionHandler) Close(c *gin.Context) {
	row, err := h.service.Close(tenant.GetTenantID(c), c.Param("id"), actorID(c))
	h.write(c, row, err, false)
}
func (h *DecisionHandler) Timeout(c *gin.Context) {
	row, err := h.service.ApplyTimeout(tenant.GetTenantID(c), c.Param("id"), actorID(c))
	h.write(c, row, err, false)
}
func (h *DecisionHandler) Failure(c *gin.Context) {
	var req struct {
		Kind          string `json:"kind"`
		TargetAgentID uint64 `json:"targetAgentId"`
		Reason        string `json:"reason"`
	}
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.EscalateFailure(tenant.GetTenantID(c), c.Param("id"), req.Kind, req.TargetAgentID, req.Reason, actorID(c))
	h.write(c, row, err, false)
}
func (h *DecisionHandler) Audits(c *gin.Context) {
	rows, err := h.service.Audits(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}
func (h *DecisionHandler) write(c *gin.Context, data any, err error, created bool) {
	if err == nil {
		if created {
			respondCreated(c, data)
		} else {
			respondSuccess(c, data)
		}
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, services.ErrDecisionNotFound) {
		status = 404
	} else if errors.Is(err, services.ErrDecisionClosed) || errors.Is(err, services.ErrCollaborationConflict) {
		status = 409
	}
	respondError(c, status, err.Error())
}

func RegisterDecisionRoutes(write, read *gin.RouterGroup, h *DecisionHandler) {
	w, r := write.Group("/decisions"), read.Group("/decisions")
	w.POST("", h.Create)
	r.GET("", h.List)
	r.GET("/:id", h.Get)
	w.POST("/:id/votes", h.Vote)
	w.POST("/:id/close", h.Close)
	w.POST("/:id/timeout", h.Timeout)
	w.POST("/:id/failure", h.Failure)
	r.GET("/:id/audit", h.Audits)
}
