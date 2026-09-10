package handler

import (
	"errors"
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/personality"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type PersonalityHandler struct{ service *services.PersonalityService }

func NewPersonalityHandler(service *services.PersonalityService) *PersonalityHandler {
	return &PersonalityHandler{service: service}
}

func (h *PersonalityHandler) List(c *gin.Context) {
	rows, err := h.service.List(tenant.GetTenantID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

func (h *PersonalityHandler) Get(c *gin.Context) {
	row, err := h.service.Get(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

type createPersonalityReq struct {
	Name        string `json:"name" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Prompt      string `json:"prompt" binding:"required"`
	Enabled     *bool  `json:"enabled"`
}

func (h *PersonalityHandler) Create(c *gin.Context) {
	var req createPersonalityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	row, err := h.service.Create(tenant.GetTenantID(c), &services.CreatePersonalityInput{Name: req.Name, Title: req.Title, Description: req.Description, Prompt: req.Prompt, Enabled: req.Enabled})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": row})
}

type updatePersonalityReq struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Prompt      *string `json:"prompt"`
	Enabled     *bool   `json:"enabled"`
	ChangeNote  string  `json:"changeNote"`
}

func (h *PersonalityHandler) Update(c *gin.Context) {
	var req updatePersonalityReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	row, err := h.service.Update(tenant.GetTenantID(c), c.Param("name"), &services.UpdatePersonalityInput{Title: req.Title, Description: req.Description, Prompt: req.Prompt, Enabled: req.Enabled, ChangeNote: req.ChangeNote})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, personality.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
}

func (h *PersonalityHandler) Delete(c *gin.Context) {
	err := h.service.Delete(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, personality.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "人格已删除"})
}
