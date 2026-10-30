package handler

// ChatSyncHandler：v2 快照同步协议四端点（issue #232 H4）。
//
// 路由挂载于认证组（JWT/CLI 用户身份 + GuestGuard）：
//
//	GET  /api/v2/chat/sync/capabilities        能力宣告
//	POST /api/v2/chat/sync/sessions/:id/claim  认领（sourceId + origin）
//	PUT  /api/v2/chat/sync/sessions/:id        应用快照 envelope
//	GET  /api/v2/chat/sync/sessions/:id        查询同步状态
//
// 身份规则：sourceId 不是身份凭证；tenant/user 只取认证上下文（body 伪造
// 的租户字段在 wire 严格解析层即被拒）。协议响应用裸 JSON（claim/GET 返回
// SyncState、PUT 返回 Reply）——精确回显优先于 house envelope；409 为协议
// 结果（conflict/deleted 附状态），400/404/413 走稳定码错误信封。

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

type ChatSyncHandler struct {
	service *services.ChatSyncService
}

func NewChatSyncHandler(service *services.ChatSyncService) *ChatSyncHandler {
	return &ChatSyncHandler{service: service}
}

// Capabilities 返回协议能力（协议/投影版本、摘要算法、大小上限、tombstone、
// serverIdentity）。
func (h *ChatSyncHandler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, h.service.Capabilities())
}

// ClaimBody 是 claim 请求体。origin：native（新建入口）| imported（一次性
// 导入既有会话——只有 imported 允许认领自己的 v1 历史）。
type ClaimBody struct {
	SourceID string `json:"sourceId" binding:"required"`
	Origin   string `json:"origin" binding:"required"`
}

func (h *ChatSyncHandler) Claim(c *gin.Context) {
	userID, ok := authUserID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, ErrCodeUnauthorized, "user not authenticated")
		return
	}
	var req ClaimBody
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}
	if req.Origin != "native" && req.Origin != "imported" {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, "origin 必须为 native|imported")
		return
	}
	if !chat.IsValidSyncUUID(req.SourceID) {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, "sourceId 须为小写规范 UUID")
		return
	}

	st, err := h.service.Claim(c.Request.Context(), tenant.GetTenantID(c), userID, c.Param("id"), req.SourceID, req.Origin)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, st)
	case errors.Is(err, chat.ErrSyncClaimConflict), errors.Is(err, chat.ErrSyncClaimDeleted):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
	default:
		log.Printf("[ChatSync] claim failed: %v", err)
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal error")
	}
}

func (h *ChatSyncHandler) Apply(c *gin.Context) {
	userID, ok := authUserID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, ErrCodeUnauthorized, "user not authenticated")
		return
	}
	sessionID := c.Param("id")

	// 大小限制：Content-Length 预检 + MaxBytesReader 兜底（谎报头部的长体）
	// → 413，不分片、不部分替换。限额为本体 maxBytes（评审修订 S2：恰好超
	// 一字节的体也必须 413，不得漏进解析层变 400）。
	const maxBytes = services.SyncMaxRequestBytes
	tooLarge := fmt.Sprintf("payload exceeds maxRequestBytes %d", maxBytes)
	if c.Request.ContentLength > maxBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": tooLarge})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": tooLarge})
		return
	}

	e, err := chat.ParseEnvelope(raw)
	if err != nil {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter, err.Error())
		return
	}
	if e.AppSessionID != sessionID {
		respondError(c, http.StatusBadRequest, ErrCodeInvalidParameter,
			fmt.Sprintf("URL session id %q 与 body appSessionId %q 不一致", sessionID, e.AppSessionID))
		return
	}

	reply, err := h.service.Apply(c.Request.Context(), tenant.GetTenantID(c), userID, e)
	if err != nil {
		if errors.Is(err, chat.ErrSyncNotClaimed) || errors.Is(err, chat.ErrSyncNotFound) {
			respondError(c, http.StatusNotFound, ErrCodeNotFound, "sync key not claimed")
			return
		}
		log.Printf("[ChatSync] apply failed: %v", err)
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal error")
		return
	}
	switch reply.Status {
	case chat.StatusConflict, chat.StatusDeleted:
		c.JSON(http.StatusConflict, reply)
	default: // applied | duplicate | superseded
		c.JSON(http.StatusOK, reply)
	}
}

func (h *ChatSyncHandler) GetState(c *gin.Context) {
	userID, ok := authUserID(c)
	if !ok {
		respondError(c, http.StatusUnauthorized, ErrCodeUnauthorized, "user not authenticated")
		return
	}
	st, err := h.service.GetState(c.Request.Context(), tenant.GetTenantID(c), userID, c.Param("id"))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, st)
	case errors.Is(err, chat.ErrSyncNotFound):
		respondError(c, http.StatusNotFound, ErrCodeNotFound, "sync state not found")
	default:
		log.Printf("[ChatSync] get state failed: %v", err)
		respondError(c, http.StatusInternalServerError, ErrCodeInternalError, "internal error")
	}
}

// authUserID 提取认证用户（中间件保证存在；缺失视为未认证）。
func authUserID(c *gin.Context) (string, bool) {
	v, exists := c.Get("user_id")
	if !exists {
		return "", false
	}
	id, ok := v.(string)
	return id, ok && id != ""
}
