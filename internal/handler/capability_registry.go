package handler

import (
	"net/http"
	"strconv"
	"strings"

	"control-panel/internal/application/services"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type CapabilityRegistryHandler struct {
	service *services.CapabilityRegistryService
}

func NewCapabilityRegistryHandler(s *services.CapabilityRegistryService) *CapabilityRegistryHandler {
	return &CapabilityRegistryHandler{service: s}
}

func (h *CapabilityRegistryHandler) Register(c *gin.Context) {
	var req struct {
		ManifestYAML      string         `json:"manifestYAML"`
		ResourcesSnapshot map[string]any `json:"resourcesSnapshot"`
		SourceType        string         `json:"sourceType"`
		SourceRef         string         `json:"sourceRef"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	p, err := h.service.Register(tenant.GetTenantID(c), services.RegisterCapabilityInput{ManifestYAML: req.ManifestYAML, ResourcesSnapshot: req.ResourcesSnapshot, SourceType: req.SourceType, SourceRef: req.SourceRef})
	if err != nil {
		if err == rundomain.ErrDuplicate {
			respondError(c, 409, err.Error())
		} else {
			respondError(c, 400, err.Error())
		}
		return
	}
	respondCreated(c, p)
}

func (h *CapabilityRegistryHandler) Approve(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, 400, "invalid package id")
		return
	}
	var req struct {
		Permissions map[string]any `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, 400, err.Error())
		return
	}
	p, err := h.service.Approve(tenant.GetTenantID(c), id, actorID(c), req.Permissions)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, 404, "capability package not found")
		} else {
			respondError(c, 400, err.Error())
		}
		return
	}
	respondSuccess(c, p)
}

func (h *CapabilityRegistryHandler) Resources(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, 400, "invalid package id")
		return
	}
	rows, err := h.service.Resources(tenant.GetTenantID(c), id)
	if err != nil {
		respondError(c, 500, err.Error())
		return
	}
	respondSuccess(c, rows)
}
func (h *CapabilityRegistryHandler) List(c *gin.Context) {
	var enabled *bool
	if raw := strings.TrimSpace(c.Query("enabled")); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			respondError(c, 400, "enabled must be true or false")
			return
		}
		enabled = &v
	}
	rows, err := h.service.List(tenant.GetTenantID(c), enabled)
	if err != nil {
		respondError(c, 500, err.Error())
		return
	}
	respondSuccess(c, rows)
}
func (h *CapabilityRegistryHandler) SetEnabled(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		respondError(c, 400, "invalid package id")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		respondError(c, 400, "enabled is required")
		return
	}
	p, err := h.service.SetEnabled(tenant.GetTenantID(c), id, *req.Enabled)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "capability package not found")
		} else if err == services.ErrCapabilityApprovalRequired {
			respondError(c, http.StatusConflict, err.Error())
		} else {
			respondError(c, 500, err.Error())
		}
		return
	}
	respondSuccess(c, p)
}
