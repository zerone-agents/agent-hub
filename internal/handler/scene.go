package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/scene"

	"github.com/gin-gonic/gin"
)

type SceneHandler struct {
	service *services.SceneService
}

func NewSceneHandler(service *services.SceneService) *SceneHandler {
	return &SceneHandler{service: service}
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

	scenes, err := h.service.List(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    scenes,
	})
}

func (h *SceneHandler) Get(c *gin.Context) {
	name := c.Param("name")

	sc, err := h.service.GetScene(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sc,
	})
}

func (h *SceneHandler) ListAdmin(c *gin.Context) {
	scenes, err := h.service.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
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

	sc, err := h.service.CreateScene(&services.CreateSceneInput{
		Name:     req.Name,
		AgentID:  req.AgentID,
		Title:    req.Title,
		TitleEn:  req.TitleEn,
		Prompt:   req.Prompt,
		PromptEn: req.PromptEn,
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

	sc, err := h.service.UpdateScene(name, &services.UpdateSceneInput{
		AgentID:  req.AgentID,
		Title:    req.Title,
		TitleEn:  req.TitleEn,
		Prompt:   req.Prompt,
		PromptEn: req.PromptEn,
		Enabled:  req.Enabled,
	})
	if err != nil {
		if err == scene.ErrSceneNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		if err == scene.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    sc,
	})
}

func (h *SceneHandler) Delete(c *gin.Context) {
	name := c.Param("name")

	if err := h.service.DeleteScene(name); err != nil {
		if err == scene.ErrSceneNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "场景已删除",
	})
}
