package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

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

	resp, err := h.service.Push(tenant.GetTenantID(c), userID.(string), userName.(string), displayName.(string), &req)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) ListSessions(c *gin.Context) {
	page, pageSize := parsePagination(c, 1, 20)

	resp, err := h.service.ListSessions(tenant.GetTenantID(c), page, pageSize, chatScopeUserID(c))
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

	// member 访问他人会话时 GetSessionForUser 查不到 → 404（不暴露存在性）
	session, err := h.service.GetSession(tenant.GetTenantID(c), sessionID, chatScopeUserID(c))
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

	// 归属校验失败（member 访问他人会话）或会话不存在 → 404
	resp, err := h.service.ListMessages(tenant.GetTenantID(c), sessionID, page, pageSize, chatScopeUserID(c))
	if err != nil {
		respondError(c, http.StatusNotFound, "session not found")
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

	// member 删他人会话 → 404；admin/maintainer 任意删
	if err := h.service.DeleteSession(tenant.GetTenantID(c), sessionID, chatScopeUserID(c)); err != nil {
		respondError(c, http.StatusNotFound, "session not found")
		return
	}

	respondMessage(c, http.StatusOK, "session deleted")
}

// chatScopeUserID 返回调用者的数据范围：admin/maintainer 返回 ""（不过滤），
// member 返回自己的 user_id（只能看/删自己的会话）。
func chatScopeUserID(c *gin.Context) string {
	roles, _ := c.Get("roles")
	if rs, ok := roles.([]string); ok {
		for _, r := range rs {
			if r == "admin" || r == "maintainer" {
				return ""
			}
		}
	}
	uid, _ := c.Get("user_id")
	s, _ := uid.(string)
	return s
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
