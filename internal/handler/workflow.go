package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

type WorkflowHandler struct{ service *services.WorkflowService }

func NewWorkflowHandler(s *services.WorkflowService) *WorkflowHandler {
	return &WorkflowHandler{service: s}
}
func (h *WorkflowHandler) write(c *gin.Context, data any, err error, created bool) {
	if err == nil {
		if created {
			respondCreated(c, data)
		} else {
			respondSuccess(c, data)
		}
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, services.ErrWorkflowNotFound) {
		status = 404
	} else if errors.Is(err, services.ErrWorkflowConflict) {
		status = 409
	}
	respondError(c, status, err.Error())
}
func (h *WorkflowHandler) Create(c *gin.Context) {
	var x struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	v, e := h.service.CreateDefinition(tenant.GetTenantID(c), x.Name, x.Description, actorID(c))
	h.write(c, v, e, true)
}
func (h *WorkflowHandler) List(c *gin.Context) {
	v, e := h.service.Definitions(tenant.GetTenantID(c))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) Get(c *gin.Context) {
	d, v, e := h.service.Definition(tenant.GetTenantID(c), c.Param("id"))
	if e != nil {
		h.write(c, nil, e, false)
		return
	}
	respondSuccess(c, gin.H{"workflow": d, "versions": v})
}
func (h *WorkflowHandler) CreateVersion(c *gin.Context) {
	var x services.CreateWorkflowVersionInput
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	x.CreatedBy = actorID(c)
	v, e := h.service.CreateVersion(tenant.GetTenantID(c), c.Param("id"), x)
	h.write(c, v, e, true)
}
func (h *WorkflowHandler) Publish(c *gin.Context) {
	v, e := h.service.PublishVersion(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) Start(c *gin.Context) {
	var x struct {
		RunID          string         `json:"runId"`
		Input          map[string]any `json:"input"`
		IdempotencyKey string         `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	v, e := h.service.StartExecution(tenant.GetTenantID(c), c.Param("id"), x.RunID, x.Input, x.IdempotencyKey, actorID(c))
	h.write(c, v, e, true)
}
func (h *WorkflowHandler) Executions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	v, e := h.service.Executions(tenant.GetTenantID(c), c.Query("workflowId"), c.Query("status"), limit)
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) Execution(c *gin.Context) {
	v, e := h.service.Execution(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) Audits(c *gin.Context) {
	v, e := h.service.Audits(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) CompleteStep(c *gin.Context) {
	var x struct {
		Output         map[string]any `json:"output"`
		IdempotencyKey string         `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	v, e := h.service.CompleteStep(tenant.GetTenantID(c), c.Param("id"), x.Output, x.IdempotencyKey, actorID(c))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) FailStep(c *gin.Context) {
	var x struct {
		Error          string `json:"error"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	v, e := h.service.FailStep(tenant.GetTenantID(c), c.Param("id"), x.Error, x.IdempotencyKey, actorID(c))
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) Decide(c *gin.Context) {
	var x struct {
		Decision       string         `json:"decision"`
		Reason         string         `json:"reason"`
		Conditions     map[string]any `json:"conditions"`
		IdempotencyKey string         `json:"idempotencyKey"`
	}
	if c.ShouldBindJSON(&x) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	v, e := h.service.DecideApproval(tenant.GetTenantID(c), c.Param("id"), actorID(c), x.Decision, x.Reason, x.Conditions, x.IdempotencyKey)
	h.write(c, v, e, false)
}
func (h *WorkflowHandler) ProcessTimeouts(c *gin.Context) {
	v, e := h.service.ProcessTimeouts(tenant.GetTenantID(c), c.Param("id"), actorID(c), time.Now().UTC())
	h.write(c, v, e, false)
}

func RegisterWorkflowRoutes(write, read *gin.RouterGroup, h *WorkflowHandler) {
	w, wr := write.Group("/workflows"), read.Group("/workflows")
	w.POST("", h.Create)
	wr.GET("", h.List)
	wr.GET("/:id", h.Get)
	w.POST("/:id/versions", h.CreateVersion)
	wv := write.Group("/workflow-versions")
	wv.POST("/:id/publish", h.Publish)
	wv.POST("/:id/executions", h.Start)
	we, wer := write.Group("/workflow-executions"), read.Group("/workflow-executions")
	wer.GET("", h.Executions)
	wer.GET("/:id", h.Execution)
	wer.GET("/:id/audit", h.Audits)
	we.POST("/:id/process-timeouts", h.ProcessTimeouts)
	ws := write.Group("/workflow-step-runs")
	ws.POST("/:id/complete", h.CompleteStep)
	ws.POST("/:id/fail", h.FailStep)
	write.POST("/workflow-approvals/:id/decisions", h.Decide)
}
