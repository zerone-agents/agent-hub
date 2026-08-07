package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
)

type TenantHandler struct {
	service *services.TenantService
}

func NewTenantHandler() *TenantHandler {
	return &TenantHandler{
		service: services.NewTenantService(),
	}
}

type CreateTenantRequest struct {
	Name    string `json:"name" binding:"required"`
	Domain  string `json:"domain"`
	Plan    string `json:"plan"`
	OwnerID string `json:"owner_id" binding:"required"`
}

func (h *TenantHandler) Create(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if req.Plan == "" {
		req.Plan = "free"
	}

	resp, err := h.service.CreateTenant(&services.CreateTenantRequest{
		Name:    req.Name,
		Domain:  req.Domain,
		Plan:    req.Plan,
		OwnerID: req.OwnerID,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"tenant": resp.Tenant,
			"org_id": resp.OrgID,
		},
	})
}

func (h *TenantHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid tenant id",
		})
		return
	}

	tenant, err := h.service.GetTenant(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "tenant not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tenant,
	})
}

func (h *TenantHandler) List(c *gin.Context) {
	tenants, err := h.service.ListTenants()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tenants,
	})
}

func (h *TenantHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid tenant id",
		})
		return
	}

	tenant, err := h.service.GetTenant(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "tenant not found",
		})
		return
	}

	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	tenant.Name = req.Name
	tenant.Domain = req.Domain
	tenant.Plan = req.Plan

	if err := h.service.UpdateTenant(tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tenant,
	})
}

func (h *TenantHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid tenant id",
		})
		return
	}

	if err := h.service.DeleteTenant(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "tenant deleted",
	})
}
