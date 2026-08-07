package handler

import (
	"net/http"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
)

type ToolHandler struct {
	service *services.ToolService
}

func NewToolHandler(service *services.ToolService) *ToolHandler {
	return &ToolHandler{service: service}
}

func (h *ToolHandler) List(c *gin.Context) {
	tools, err := h.service.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    tools,
	})
}

func (h *ToolHandler) Get(c *gin.Context) {
	name := c.Param("name")
	t, err := h.service.GetByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    t,
	})
}

func (h *ToolHandler) Create(c *gin.Context) {
	var input services.CreateToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	t, err := h.service.Create(&input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    t,
	})
}

func (h *ToolHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var input services.UpdateToolInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	t, err := h.service.Update(name, &input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    t,
	})
}

func (h *ToolHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.Delete(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Tool 已删除",
	})
}

type updateAgentToolsReq struct {
	ToolNames []string `json:"toolNames" binding:"required"`
}

func (h *ToolHandler) UpdateAgentTools(c *gin.Context) {
	agentName := c.Param("name")
	var req updateAgentToolsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	if err := h.service.UpdateAgentTools(agentName, req.ToolNames); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent Tool 关系已更新",
	})
}

func (h *ToolHandler) GetAgentTools(c *gin.Context) {
	agentName := c.Param("name")
	toolNames, err := h.service.GetAgentTools(agentName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    toolNames,
	})
}
