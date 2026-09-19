package handler

import (
	"errors"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

type CollaborationHandler struct {
	service *services.CollaborationService
}

func NewCollaborationHandler(s *services.CollaborationService) *CollaborationHandler {
	return &CollaborationHandler{service: s}
}

type groupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

func (h *CollaborationHandler) CreateGroup(c *gin.Context) {
	var req groupRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.CreateGroup(tenant.GetTenantID(c), req.Name, req.Description, req.Visibility, actorID(c))
	h.write(c, row, err, true)
}
func (h *CollaborationHandler) Groups(c *gin.Context) {
	rows, err := h.service.Groups(tenant.GetTenantID(c))
	h.write(c, rows, err, false)
}
func (h *CollaborationHandler) Group(c *gin.Context) {
	row, err := h.service.Group(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) UpdateGroup(c *gin.Context) {
	var req groupRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.UpdateGroup(tenant.GetTenantID(c), c.Param("id"), req.Name, req.Description, req.Visibility, actorID(c))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) DeleteGroup(c *gin.Context) {
	err := h.service.DeleteGroup(tenant.GetTenantID(c), c.Param("id"), actorID(c))
	if err != nil {
		h.write(c, nil, err, false)
		return
	}
	respondSuccess(c, gin.H{"deleted": true})
}

type memberRequest struct {
	AgentID uint64 `json:"agentId"`
	Role    string `json:"role"`
}

func (h *CollaborationHandler) Members(c *gin.Context) {
	rows, err := h.service.Members(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}
func (h *CollaborationHandler) AddMember(c *gin.Context) {
	var req memberRequest
	if c.ShouldBindJSON(&req) != nil || req.AgentID == 0 {
		respondError(c, 400, "agentId is required")
		return
	}
	row, err := h.service.AddMember(tenant.GetTenantID(c), c.Param("id"), req.AgentID, req.Role, actorID(c))
	h.write(c, row, err, true)
}
func (h *CollaborationHandler) UpdateMember(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("agentId"), 10, 64)
	if err != nil {
		respondError(c, 400, "invalid agentId")
		return
	}
	var req memberRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, e := h.service.UpdateMember(tenant.GetTenantID(c), c.Param("id"), id, req.Role, actorID(c))
	h.write(c, row, e, false)
}
func (h *CollaborationHandler) RemoveMember(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("agentId"), 10, 64)
	if err != nil {
		respondError(c, 400, "invalid agentId")
		return
	}
	e := h.service.RemoveMember(tenant.GetTenantID(c), c.Param("id"), id, actorID(c))
	if e != nil {
		h.write(c, nil, e, false)
		return
	}
	respondSuccess(c, gin.H{"deleted": true})
}
func (h *CollaborationHandler) Audits(c *gin.Context) {
	rows, err := h.service.MemberAudits(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}

type channelRequest struct {
	Name       string `json:"name"`
	Topic      string `json:"topic"`
	Visibility string `json:"visibility"`
}

func (h *CollaborationHandler) Channels(c *gin.Context) {
	rows, err := h.service.Channels(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}
func (h *CollaborationHandler) CreateChannel(c *gin.Context) {
	var req channelRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.CreateChannel(tenant.GetTenantID(c), c.Param("id"), req.Name, req.Topic, req.Visibility, actorID(c))
	h.write(c, row, err, true)
}
func (h *CollaborationHandler) Channel(c *gin.Context) {
	row, err := h.service.Channel(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) UpdateChannel(c *gin.Context) {
	var req channelRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.UpdateChannel(tenant.GetTenantID(c), c.Param("id"), req.Name, req.Topic, req.Visibility, actorID(c))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) DeleteChannel(c *gin.Context) {
	err := h.service.DeleteChannel(tenant.GetTenantID(c), c.Param("id"), actorID(c))
	if err != nil {
		h.write(c, nil, err, false)
		return
	}
	respondSuccess(c, gin.H{"deleted": true})
}
func (h *CollaborationHandler) Subscriptions(c *gin.Context) {
	rows, err := h.service.Subscribers(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}
func (h *CollaborationHandler) PutSubscription(c *gin.Context) {
	var req struct {
		AgentID uint64 `json:"agentId"`
		Mode    string `json:"mode"`
	}
	if c.ShouldBindJSON(&req) != nil || req.AgentID == 0 {
		respondError(c, 400, "agentId is required")
		return
	}
	row, err := h.service.PutSubscription(tenant.GetTenantID(c), c.Param("id"), req.AgentID, req.Mode, actorID(c))
	h.write(c, row, err, false)
}

type sessionRequest struct {
	Agenda              string   `json:"agenda"`
	HostAgentID         uint64   `json:"hostAgentId"`
	ParticipantAgentIDs []uint64 `json:"participantAgentIds"`
	Summary             string   `json:"summary"`
}

func (h *CollaborationHandler) Sessions(c *gin.Context) {
	rows, err := h.service.Sessions(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, rows, err, false)
}
func (h *CollaborationHandler) CreateSession(c *gin.Context) {
	var req sessionRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.CreateSession(tenant.GetTenantID(c), c.Param("id"), req.Agenda, req.HostAgentID, actorID(c), req.ParticipantAgentIDs...)
	h.write(c, row, err, true)
}
func (h *CollaborationHandler) Session(c *gin.Context) {
	row, err := h.service.Session(tenant.GetTenantID(c), c.Param("id"))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) StartSession(c *gin.Context) {
	row, err := h.service.StartSession(tenant.GetTenantID(c), c.Param("id"), actorID(c))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) CompleteSession(c *gin.Context) {
	var req sessionRequest
	if c.ShouldBindJSON(&req) != nil {
		respondError(c, 400, "invalid request")
		return
	}
	row, err := h.service.CompleteSession(tenant.GetTenantID(c), c.Param("id"), req.Summary, actorID(c))
	h.write(c, row, err, false)
}
func (h *CollaborationHandler) write(c *gin.Context, data any, err error, created bool) {
	if err == nil {
		if created {
			respondCreated(c, data)
		} else {
			respondSuccess(c, data)
		}
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, services.ErrCollaborationNotFound) {
		status = 404
	} else if errors.Is(err, services.ErrCollaborationConflict) {
		status = 409
	}
	respondError(c, status, err.Error())
}

func RegisterCollaborationRoutes(write, read *gin.RouterGroup, h *CollaborationHandler) {
	gr, grr := write.Group("/groups"), read.Group("/groups")
	gr.POST("", h.CreateGroup)
	grr.GET("", h.Groups)
	grr.GET("/:id", h.Group)
	gr.PUT("/:id", h.UpdateGroup)
	gr.DELETE("/:id", h.DeleteGroup)
	grr.GET("/:id/members", h.Members)
	gr.POST("/:id/members", h.AddMember)
	gr.PATCH("/:id/members/:agentId", h.UpdateMember)
	gr.DELETE("/:id/members/:agentId", h.RemoveMember)
	grr.GET("/:id/audit", h.Audits)
	grr.GET("/:id/channels", h.Channels)
	gr.POST("/:id/channels", h.CreateChannel)
	ch, chr := write.Group("/channels"), read.Group("/channels")
	chr.GET("/:id", h.Channel)
	ch.PUT("/:id", h.UpdateChannel)
	ch.DELETE("/:id", h.DeleteChannel)
	chr.GET("/:id/subscriptions", h.Subscriptions)
	ch.PUT("/:id/subscriptions", h.PutSubscription)
	chr.GET("/:id/sessions", h.Sessions)
	ch.POST("/:id/sessions", h.CreateSession)
	se, ser := write.Group("/sessions"), read.Group("/sessions")
	ser.GET("/:id", h.Session)
	se.POST("/:id/start", h.StartSession)
	se.POST("/:id/complete", h.CompleteSession)
}
