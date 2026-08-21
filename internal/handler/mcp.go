package handler

import (
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type McpHandler struct {
	service *services.McpService
}

func NewMcpHandler(svc *services.McpService) *McpHandler {
	return &McpHandler{service: svc}
}

// ==================== 管理：CRUD ====================

func (h *McpHandler) List(c *gin.Context) {
	items, err := h.service.ListAll()
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, items)
}

func (h *McpHandler) Get(c *gin.Context) {
	name := c.Param("name")
	item, err := h.service.GetByName(name)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondSuccess(c, item)
}

func (h *McpHandler) Create(c *gin.Context) {
	var input services.CreateMcpInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.Create(&input)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondCreated(c, item)
}

func (h *McpHandler) Update(c *gin.Context) {
	name := c.Param("name")
	var input services.UpdateMcpInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.service.Update(name, &input)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, item)
}

func (h *McpHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.Delete(name); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "MCP 已删除")
}

// ==================== 管理：Agent ↔ MCP 绑定 ====================

type updateAgentMcpsReq struct {
	McpNames []string `json:"mcpNames"`
}

func (h *McpHandler) GetAgentMcps(c *gin.Context) {
	agentName := c.Param("name")
	names, err := h.service.GetAgentMcps(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, names)
}

func (h *McpHandler) UpdateAgentMcps(c *gin.Context) {
	agentName := c.Param("name")
	var req updateAgentMcpsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.service.UpdateAgentMcps(tenant.GetTenantID(c), agentName, req.McpNames); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "Agent MCP 关系已更新")
}

func (h *McpHandler) ProbeByConfig(c *gin.Context) {
	var input services.McpProbeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.service.ProbeByConfig(c.Request.Context(), &input)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, result)
}

func (h *McpHandler) ProbeByName(c *gin.Context) {
	name := c.Param("name")
	result, err := h.service.ProbeByName(c.Request.Context(), name)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, result)
}

// ==================== 公开：客户端拉取接口 ====================

// GetClientMcpsByAgent 客户端按 agent name 拉取完整 MCP 配置（已解密）。
// 路由：GET /api/v1/mcps?agent=<name>
func (h *McpHandler) GetClientMcpsByAgent(c *gin.Context) {
	agentName := c.Query("agent")
	if agentName == "" {
		respondError(c, http.StatusBadRequest, "缺少 agent 查询参数")
		return
	}
	items, err := h.service.GetClientMcpsByAgent(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondSuccess(c, items)
}
