package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/scene"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type SceneHandler struct {
	service *services.SceneService
}

func NewSceneHandler(service *services.SceneService) *SceneHandler {
	return &SceneHandler{service: service}
}

// respondSceneError 映射 Scene 领域错误（issue #95 P2 同款边界分流）：
// ErrSceneNotFound → 404；ErrSceneExists / ErrAgentNotFound（关联校验
// 冲突，用户可行动）→ 400 原文；ValidationError → 400 完整链；
// 基础设施故障 → 500 中性 + 服务端日志。
func respondSceneError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, scene.ErrSceneNotFound):
		respondError(c, http.StatusNotFound, scene.ErrSceneNotFound.Error())
	case errors.Is(err, scene.ErrSceneExists), errors.Is(err, scene.ErrAgentNotFound):
		respondError(c, http.StatusBadRequest, err.Error())
	default:
		var ve *scene.ValidationError
		if errors.As(err, &ve) {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("[SceneHandler] internal error: %v", err)
		respondError(c, http.StatusInternalServerError, "服务器内部错误，请稍后重试")
	}
}

func (h *SceneHandler) List(c *gin.Context) {
	agentIDStr := c.Query("agentId")
	var agentID uint64
	if agentIDStr != "" {
		var err error
		agentID, err = strconv.ParseUint(agentIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "agentId 格式无效",
			})
			return
		}
	}

	scenes, err := h.service.List(tenant.GetTenantID(c), agentID)
	if err != nil {
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scenes,
	})
}

func (h *SceneHandler) Get(c *gin.Context) {
	name := c.Param("name")

	sc, err := h.service.GetScene(tenant.GetTenantID(c), name)
	if err != nil {
		// 行为修正（原「所有错误一律 404」）：not-found 走 404 中文，
		// 其余错误由 respondSceneError 分流（issue #95 P2）。
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sc,
	})
}

func (h *SceneHandler) ListAdmin(c *gin.Context) {
	scenes, err := h.service.ListAll(tenant.GetTenantID(c))
	if err != nil {
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scenes,
	})
}

type createSceneReq struct {
	Name     string `json:"name" binding:"required"`
	AgentID  uint64 `json:"agentId" binding:"required"`
	Title    string `json:"title" binding:"required"`
	TitleEn  string `json:"titleEn"`
	Prompt   string `json:"prompt" binding:"required"`
	PromptEn string `json:"promptEn"`
}

func (h *SceneHandler) Create(c *gin.Context) {
	var req createSceneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	sc, err := h.service.CreateScene(tenant.GetTenantID(c), &services.CreateSceneInput{
		Name:     req.Name,
		AgentID:  req.AgentID,
		Title:    req.Title,
		TitleEn:  req.TitleEn,
		Prompt:   req.Prompt,
		PromptEn: req.PromptEn,
	})
	if err != nil {
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    sc,
	})
}

type updateSceneReq struct {
	AgentID  *uint64 `json:"agentId"`
	Title    string  `json:"title"`
	TitleEn  string  `json:"titleEn"`
	Prompt   string  `json:"prompt"`
	PromptEn string  `json:"promptEn"`
	Enabled  *bool   `json:"enabled"`
}

func (h *SceneHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req updateSceneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	sc, err := h.service.UpdateScene(tenant.GetTenantID(c), name, &services.UpdateSceneInput{
		AgentID:  req.AgentID,
		Title:    req.Title,
		TitleEn:  req.TitleEn,
		Prompt:   req.Prompt,
		PromptEn: req.PromptEn,
		Enabled:  req.Enabled,
	})
	if err != nil {
		// ErrSceneNotFound → 404；ErrAgentNotFound / ErrSceneExists /
		// ValidationError → 400 原文；基础设施故障 → 500 中性。
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sc,
	})
}

func (h *SceneHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	if err := h.service.DeleteScene(tenant.GetTenantID(c), name); err != nil {
		respondSceneError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "场景已删除",
	})
}
