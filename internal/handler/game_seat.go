package handler

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"github.com/gin-gonic/gin"
)

type GameSeatHandler struct {
	service *services.GameSeatService
	key     string
}

func NewGameSeatHandler(service *services.GameSeatService, key string) *GameSeatHandler {
	return &GameSeatHandler{service: service, key: key}
}

func (h *GameSeatHandler) Register(router *gin.Engine) {
	group := router.Group("/api/v1/game-seats", h.authorize)
	group.GET("/roster", h.roster)
	group.POST("/sessions", h.session)
	group.POST("/sessions/:sessionId/generate", h.generate)
	group.POST("/sessions/:sessionId/events", h.event)
	group.GET("/memory", h.memory)
}

func (h *GameSeatHandler) authorize(c *gin.Context) {
	provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if h.key == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(h.key)) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid game-seat service key"})
		return
	}
	c.Next()
}

func (h *GameSeatHandler) roster(c *gin.Context) {
	items, err := h.service.Roster(c.Query("gameKey"))
	if err != nil {
		c.JSON(500, gin.H{"error": "roster unavailable"})
		return
	}
	c.JSON(200, items)
}

func (h *GameSeatHandler) session(c *gin.Context) {
	var input services.GameSeatSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "invalid session request"})
		return
	}
	id, err := h.service.CreateSession(input)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"sessionId": id})
}

func (h *GameSeatHandler) generate(c *gin.Context) {
	var input services.GameSeatGenerateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "invalid generate request"})
		return
	}
	response, err := h.service.Generate(c.Request.Context(), c.Param("sessionId"), input)
	if err != nil {
		if errors.Is(err, services.ErrGameSeatBudget) {
			c.JSON(429, gin.H{"error": "game_seat_budget_exceeded"})
			return
		}
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	defer response.Body.Close()
	contentType := "application/json"
	if input.Stream {
		contentType = "text/event-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "no-cache")
	c.Status(200)
	if !input.Stream {
		_, _ = io.Copy(c.Writer, response.Body)
		return
	}
	buffer := make([]byte, 4096)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buffer[:n]); writeErr != nil {
				return
			}
			c.Writer.Flush()
		}
		if readErr != nil {
			return
		}
	}
}

func (h *GameSeatHandler) event(c *gin.Context) {
	var input services.GameSeatEventInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "invalid event request"})
		return
	}
	if err := h.service.Event(c.Param("sessionId"), input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.Status(204)
}

func (h *GameSeatHandler) memory(c *gin.Context) {
	view, err := h.service.Memory(c.Query("gameKey"), c.Query("playerId"), c.Query("characterId"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, view)
}
