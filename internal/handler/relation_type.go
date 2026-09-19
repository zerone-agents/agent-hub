package handler

import (
	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
)

type RelationTypeHandler struct{ service *services.RelationTypeService }

func NewRelationTypeHandler(s *services.RelationTypeService) *RelationTypeHandler {
	return &RelationTypeHandler{s}
}

func (h *RelationTypeHandler) List(c *gin.Context) {
	rows, err := h.service.List(tenant.GetTenantID(c))
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": rows})
}

type relationTypeReq struct {
	Name                  string   `json:"name"`
	Title                 string   `json:"title"`
	Description           string   `json:"description"`
	BaseType              string   `json:"baseType"`
	DirectionPolicy       string   `json:"directionPolicy"`
	DefaultStance         string   `json:"defaultStance"`
	DefaultAllowedActions []string `json:"defaultAllowedActions"`
	DefaultContextPolicy  string   `json:"defaultContextPolicy"`
	DefaultDeliveryPolicy string   `json:"defaultDeliveryPolicy"`
	DefaultConstraint     string   `json:"defaultConstraint"`
	LineColor             string   `json:"lineColor"`
	LineStyle             string   `json:"lineStyle"`
	Enabled               *bool    `json:"enabled"`
}

func (r *relationTypeReq) input() *services.RelationTypeInput {
	return &services.RelationTypeInput{Name: r.Name, Title: r.Title, Description: r.Description, BaseType: r.BaseType, DirectionPolicy: r.DirectionPolicy, DefaultStance: r.DefaultStance, DefaultAllowedActions: r.DefaultAllowedActions, DefaultContextPolicy: r.DefaultContextPolicy, DefaultDeliveryPolicy: r.DefaultDeliveryPolicy, DefaultConstraint: r.DefaultConstraint, LineColor: r.LineColor, LineStyle: r.LineStyle, Enabled: r.Enabled}
}
func (h *RelationTypeHandler) Create(c *gin.Context) {
	var req relationTypeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	row, err := h.service.Create(tenant.GetTenantID(c), req.input())
	if err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"success": true, "data": row})
}
func (h *RelationTypeHandler) Update(c *gin.Context) {
	var req relationTypeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": err.Error()})
		return
	}
	req.Name = c.Param("name")
	row, err := h.service.Update(tenant.GetTenantID(c), req.Name, req.input())
	if err != nil {
		status := 400
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": row})
}
func (h *RelationTypeHandler) Delete(c *gin.Context) {
	err := h.service.Delete(tenant.GetTenantID(c), c.Param("name"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = 404
		}
		c.JSON(status, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true})
}
