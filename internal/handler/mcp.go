package handler

import (
	"errors"
	"log"
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type McpHandler struct {
	service *services.McpService
}

func NewMcpHandler(svc *services.McpService) *McpHandler {
	return &McpHandler{service: svc}
}

// respondMcpError 映射 MCP 领域错误（issue #95 P2 同款边界分流）：
// 领域 sentinel（ErrMcpNotFound）→ 404 中文；用户面校验错误
// （ValidationError）→ 400 完整链原文；基础设施故障 → 500 中性，
// 完整错误链只在服务端日志。
func respondMcpError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, mcp.ErrMcpNotFound):
		respondError(c, http.StatusNotFound, mcp.ErrMcpNotFound.Error())
	case errors.Is(err, agent.ErrAgentNotFound):
		respondError(c, http.StatusNotFound, agent.ErrAgentNotFound.Error())
	default:
		var ve *mcp.ValidationError
		if errors.As(err, &ve) {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("[McpHandler] internal error: %v", err)
		respondError(c, http.StatusInternalServerError, "服务器内部错误，请稍后重试")
	}
}

// ==================== 管理：CRUD ====================

func (h *McpHandler) List(c *gin.Context) {
	items, err := h.service.ListAll(tenant.GetTenantID(c))
	if err != nil {
		respondMcpError(c, err)
		return
	}
	respondSuccess(c, items)
}

func (h *McpHandler) Get(c *gin.Context) {
	name := c.Param("name")
	item, err := h.service.GetByName(tenant.GetTenantID(c), name)
	if err != nil {
		// 行为修正（原「所有错误一律 404」）：not-found 走 404 中文，
		// DB 等基础设施故障由 respondMcpError 落 500 中性（issue #95 P2）。
		respondMcpError(c, err)
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
	item, err := h.service.Create(tenant.GetTenantID(c), &input)
	if err != nil {
		respondMcpError(c, err)
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
	item, err := h.service.Update(tenant.GetTenantID(c), name, &input)
	if err != nil {
		respondMcpError(c, err)
		return
	}
	respondSuccess(c, item)
}

func (h *McpHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.service.Delete(tenant.GetTenantID(c), name); err != nil {
		var inUse *agent.McpInUseError
		if errors.As(err, &inUse) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": inUse.Error(), "data": gin.H{"agents": inUse.Agents, "foreign": inUse.Foreign}})
			return
		}
		respondMcpError(c, err)
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
		respondMcpError(c, err)
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
		respondMcpError(c, err)
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
		respondMcpError(c, err)
		return
	}
	respondSuccess(c, result)
}

func (h *McpHandler) ProbeByName(c *gin.Context) {
	name := c.Param("name")
	result, err := h.service.ProbeByName(c.Request.Context(), tenant.GetTenantID(c), name)
	if err != nil {
		respondMcpError(c, err)
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
		respondMcpError(c, err)
		return
	}
	respondSuccess(c, items)
}
