package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	service *services.ChatService
}

func NewChatHandler() *ChatHandler {
	return &ChatHandler{
		service: services.NewChatService(),
	}
}

func (h *ChatHandler) Push(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "user not authenticated")
		return
	}

	var req services.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	userName, _ := c.Get("user_name")
	displayName, _ := c.Get("display_name")

	resp, err := h.service.Push(userID.(string), userName.(string), displayName.(string), &req)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) ListSessions(c *gin.Context) {
	page, pageSize := parsePagination(c, 1, 20)

	resp, err := h.service.ListSessions(page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, resp)
}

func (h *ChatHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "session id is required")
		return
	}

	session, err := h.service.GetSession(sessionID)
	if err != nil {
		respondError(c, http.StatusNotFound, "session not found")
		return
	}

	respondSuccess(c, session)
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "session id is required")
		return
	}

	page, pageSize := parsePagination(c, 1, 50)

	resp, err := h.service.ListMessages(sessionID, page, pageSize)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondSuccess(c, resp)
}

func (h *ChatHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, "session id is required")
		return
	}

	if err := h.service.DeleteSession(sessionID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondMessage(c, http.StatusOK, "session deleted")
}

func parsePagination(c *gin.Context, defaultPage, defaultPageSize int) (page, pageSize int) {
	page = defaultPage
	pageSize = defaultPageSize

	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 {
		pageSize = ps
	}
	return
}
