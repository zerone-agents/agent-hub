package handler

import (
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type ToolHandler struct {
	service *services.ToolService
}

func NewToolHandler(service *services.ToolService) *ToolHandler {
	return &ToolHandler{service: service}
}

func (h *ToolHandler) List(c *gin.Context) {
	tools, err := h.service.ListAll(tenant.GetTenantID(c))
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
	t, err := h.service.GetByName(tenant.GetTenantID(c), name)
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

// Create 是 issue #88 Task 3 的临时编译垫片：旧 JSON CreateToolInput/Create
// 服务端语义已随制品生命周期改造删除（被 CreateCustomTool 取代），multipart
// 端点由后续 handler 改造（issue #88）提供。
func (h *ToolHandler) Create(c *gin.Context) {
	c.JSON(http.StatusBadRequest, gin.H{
		"success": false,
		"error":   "Tool 创建已迁移为文件上传方式，当前接口暂不可用",
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
	t, err := h.service.Update(tenant.GetTenantID(c), name, &input)
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
	if err := h.service.Delete(tenant.GetTenantID(c), name); err != nil {
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
	if err := h.service.UpdateAgentTools(tenant.GetTenantID(c), agentName, req.ToolNames); err != nil {
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
	toolNames, err := h.service.GetAgentTools(tenant.GetTenantID(c), agentName)
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
