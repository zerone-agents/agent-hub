package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	service *services.ChatService
}

func NewChatHandler(authMode string, resolveDefaultOrg func() (string, bool)) *ChatHandler {
	return &ChatHandler{
		service: services.NewChatService(authMode, resolveDefaultOrg),
	}
}

func (h *ChatHandler) Push(c *gin.Context) {
	// X-Chat-Push-Key 通道：归属由 body 的 user_name/org 决定
	// （中间件已保证此分支不可能携带 token 身份）。
	if am, _ := c.Get("auth_method"); am == "chat_push_key" {
		var req services.PushRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
			return
		}
		resp, err := h.service.PushWithSessionIdentity(&req)
		if err != nil {
			if errors.Is(err, services.ErrPushValidation) {
				respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
				return
			}
			log.Printf("[ChatPush] push-key push failed: %v", err)
			respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal error")
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, ErrCodeUnauthorized, "user not authenticated")
		return
	}

	var req services.PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}

	userName, _ := c.Get("user_name")
	displayName, _ := c.Get("display_name")

	resp, err := h.service.Push(tenant.GetTenantID(c), userID.(string), userName.(string), displayName.(string), &req)
	if err != nil {
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *ChatHandler) ListSessions(c *gin.Context) {
	page, pageSize := parsePagination(c, 1, 20)

	resp, err := h.service.ListSessions(tenant.GetTenantID(c), page, pageSize, chatScopeUserID(c))
	if err != nil {
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	respondSuccess(c, resp)
}

func (h *ChatHandler) GetSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, ErrCodeSessionIdRequired, "session id is required")
		return
	}

	// member 访问他人会话时 GetSessionForUser 查不到 → 404（不暴露存在性）
	session, err := h.service.GetSession(tenant.GetTenantID(c), sessionID, chatScopeUserID(c))
	if err != nil {
		respondError(c, http.StatusNotFound, ErrCodeSessionNotFound, "session not found")
		return
	}

	respondSuccess(c, session)
}

func (h *ChatHandler) ListMessages(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, ErrCodeSessionIdRequired, "session id is required")
		return
	}

	page, pageSize := parsePagination(c, 1, 50)

	// 归属校验失败（member 访问他人会话）或会话不存在 → 404
	resp, err := h.service.ListMessages(tenant.GetTenantID(c), sessionID, page, pageSize, chatScopeUserID(c))
	if err != nil {
		respondError(c, http.StatusNotFound, ErrCodeSessionNotFound, "session not found")
		return
	}

	respondSuccess(c, resp)
}

func (h *ChatHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		respondError(c, http.StatusBadRequest, ErrCodeSessionIdRequired, "session id is required")
		return
	}

	// member 删他人会话 → 404；admin/maintainer 任意删
	if err := h.service.DeleteSession(tenant.GetTenantID(c), sessionID, chatScopeUserID(c)); err != nil {
		respondError(c, http.StatusNotFound, ErrCodeSessionNotFound, "session not found")
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

const (
	// maxPageSize 分页条数上限，与前端下拉（10/20/50/100）及审计日志同口径：
	// 不设限时 agent 聊天路径会把 page_size=1e6 直落 SQL LIMIT，把整表拉进内存。
	maxPageSize = 100
	// maxPage page 上界：防 (page-1)*pageSize 整型溢出成负 offset（GORM 忽略负
	// offset 会静默返回首页数据）与超大 offset 慢查询——审计日志同款防线。
	maxPage = 1_000_000
)

func parsePagination(c *gin.Context, defaultPage, defaultPageSize int) (page, pageSize int) {
	page = defaultPage
	pageSize = defaultPageSize

	// 非法值仍按「未传」处理（取默认），不收紧成 400：存量客户端可能发脏值；
	// 超界值则钳到边界（不报错），保证请求总能拿到数据。
	if p, err := strconv.Atoi(c.Query("page")); err == nil && p > 0 {
		page = p
	}
	if page > maxPage {
		page = maxPage
	}
	if ps, err := strconv.Atoi(c.Query("page_size")); err == nil && ps > 0 {
		pageSize = ps
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return
}
