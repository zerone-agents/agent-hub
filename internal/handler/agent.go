package handler

import (
	"fmt"
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type AgentHandler struct {
	service         *services.AgentService
	deployerService *services.AgentDeployerService
}

func NewAgentHandler(service *services.AgentService, deployerService *services.AgentDeployerService) *AgentHandler {
	return &AgentHandler{
		service:         service,
		deployerService: deployerService,
	}
}

func (h *AgentHandler) Manifest(c *gin.Context) {
	resp, err := h.service.GetManifest(tenant.GetTenantID(c), c.Query("platform"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (h *AgentHandler) List(c *gin.Context) {
	resp, err := h.service.GetDesktopAgents(tenant.GetTenantID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (h *AgentHandler) ListAdmin(c *gin.Context) {
	resp, err := h.service.GetAllAgentsAdmin(tenant.GetTenantID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (h *AgentHandler) Get(c *gin.Context) {
	name := c.Param("name")

	resp, err := h.service.GetAgent(tenant.GetTenantID(c), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

type createAgentReq struct {
	Name           string                 `json:"name" binding:"required"`
	Config         map[string]interface{} `json:"config" binding:"required"`
	DesktopEnabled *bool                  `json:"desktopEnabled"`
	MobileEnabled  *bool                  `json:"mobileEnabled"`
	IsDefault      *bool                  `json:"isDefault"`
}

func (h *AgentHandler) Create(c *gin.Context) {
	var req createAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	resp, err := h.service.CreateAgent(tenant.GetTenantID(c), &services.CreateAgentInput{
		Name:           req.Name,
		Config:         req.Config,
		DesktopEnabled: req.DesktopEnabled,
		MobileEnabled:  req.MobileEnabled,
		IsDefault:      req.IsDefault,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    resp,
	})
}

type updateAgentReq struct {
	Config         *map[string]interface{} `json:"config"`
	DesktopEnabled *bool                   `json:"desktopEnabled"`
	MobileEnabled  *bool                   `json:"mobileEnabled"`
	IsDefault      *bool                   `json:"isDefault"`
	Source         string                  `json:"source"`
}

func (h *AgentHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req updateAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	resp, err := h.service.UpdateAgent(tenant.GetTenantID(c), name, &services.UpdateAgentInput{
		Config:         req.Config,
		DesktopEnabled: req.DesktopEnabled,
		MobileEnabled:  req.MobileEnabled,
		IsDefault:      req.IsDefault,
		Source:         req.Source,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
	})
}

func (h *AgentHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	// Reject deletion when the agent still has a live deployment. GetStatus
	// syncs the real container state from the deployer, so its result is
	// authoritative. "not_found" and "archived" mean no container is running
	// (archived = container removed but data retained by the deployer).
	if deployment, err := h.deployerService.GetStatus(tenant.GetTenantID(c), name); err == nil {
		s := deployment.Status
		if s != "" && s != "not_found" && s != "archived" {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   fmt.Sprintf("该 Agent 仍有活跃部署（状态: %s），请先在部署面板中删除部署后再删除 Agent", s),
			})
			return
		}
	}

	if err := h.service.DeleteAgent(tenant.GetTenantID(c), name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent 已删除",
	})
}

type probeAgentReq struct {
	ProviderID *uint64 `json:"providerId"`
	APIKey     string  `json:"apiKey"`
	BaseURL    string  `json:"baseUrl"`
}

func (h *AgentHandler) ProbeAgent(c *gin.Context) {
	name := c.Param("name")

	var req probeAgentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	result, err := h.service.ProbeAgent(tenant.GetTenantID(c), name, req.ProviderID, req.APIKey, req.BaseURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func deployerErrorStatus(err error) int {
	msg := err.Error()
	if strings.Contains(msg, "agent has no provider") ||
		strings.Contains(msg, "agent has no model") ||
		strings.Contains(msg, "agent not found") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func (h *AgentHandler) DeployAgent(c *gin.Context) {
	name := c.Param("name")
	force := c.Query("force") == "true"
	rotateKey := c.Query("rotate_key") == "true"

	resp, err := h.deployerService.Deploy(tenant.GetTenantID(c), name, force, rotateKey)
	if err != nil {
		respondError(c, deployerErrorStatus(err), err.Error())
		return
	}
	respondSuccess(c, resp)
}

func (h *AgentHandler) GetDeployment(c *gin.Context) {
	name := c.Param("name")

	resp, err := h.deployerService.GetStatus(tenant.GetTenantID(c), name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondSuccess(c, resp)
}

func (h *AgentHandler) StopDeployment(c *gin.Context) {
	name := c.Param("name")

	if err := h.deployerService.Stop(tenant.GetTenantID(c), name); err != nil {
		respondError(c, deployerErrorStatus(err), err.Error())
		return
	}
	respondMessage(c, http.StatusOK, "已停止")
}

func (h *AgentHandler) StartDeployment(c *gin.Context) {
	name := c.Param("name")

	resp, err := h.deployerService.Start(tenant.GetTenantID(c), name)
	if err != nil {
		respondError(c, deployerErrorStatus(err), err.Error())
		return
	}
	respondSuccess(c, resp)
}

func (h *AgentHandler) DeleteDeployment(c *gin.Context) {
	name := c.Param("name")
	purge := c.Query("purge") == "true"

	var err error
	if purge {
		err = h.deployerService.Purge(tenant.GetTenantID(c), name)
	} else {
		err = h.deployerService.Delete(tenant.GetTenantID(c), name)
	}
	if err != nil {
		respondError(c, deployerErrorStatus(err), err.Error())
		return
	}
	if purge {
		respondMessage(c, http.StatusOK, "已彻底删除")
	} else {
		respondMessage(c, http.StatusOK, "已归档")
	}
}

type updateSubagentsReq struct {
	Subagents []string `json:"subagents" binding:"required"`
}

func (h *AgentHandler) UpdateSubagents(c *gin.Context) {
	name := c.Param("name")

	var req updateSubagentsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.UpdateSubagents(tenant.GetTenantID(c), name, req.Subagents); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "子 Agent 关系已更新",
	})
}

type updateAgentKnowledgeReq struct {
	DatasetIDs []string `json:"dataset_ids"`
}

func (h *AgentHandler) GetAgentKnowledge(c *gin.Context) {
	name := c.Param("name")
	datasetIDs, err := h.service.GetAgentKnowledgeDatasets(tenant.GetTenantID(c), name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": map[string]interface{}{
			"agent":       name,
			"dataset_ids": datasetIDs,
		},
	})
}

func (h *AgentHandler) UpdateAgentKnowledge(c *gin.Context) {
	name := c.Param("name")
	var req updateAgentKnowledgeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := h.service.UpdateAgentKnowledgeDatasets(tenant.GetTenantID(c), name, req.DatasetIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent 知识库绑定已更新",
	})
}
