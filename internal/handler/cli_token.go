package handler

import (
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/audit"

	"github.com/gin-gonic/gin"
)

// CLITokenHandler exposes the CLI token lifecycle: issue / list / revoke.
// All endpoints operate on c.MustGet("user_id") set by the auth middleware.
type CLITokenHandler struct {
	svc   *services.CLITokenService
	audit *services.AuditRecorder
}

func NewCLITokenHandler(svc *services.CLITokenService, ar *services.AuditRecorder) *CLITokenHandler {
	return &CLITokenHandler{svc: svc, audit: ar}
}

type issueTokenReq struct {
	Name    string `json:"name" binding:"required"`
	TTLDays int    `json:"ttlDays"`
}

// Issue creates a new CLI token. The plaintext token is returned exactly once.
func (h *CLITokenHandler) Issue(c *gin.Context) {
	userID := c.MustGet("user_id").(string)
	var req issueTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "name_required", "缺少必填字段 name")
		return
	}
	result, err := h.svc.Issue(userID, req.Name, req.TTLDays)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}
	// Target 记 token 名而非值：明文 token 只在响应体出现一次（spec §3）。
	h.audit.Simple(c, audit.ActionCliTokenIssue, audit.TargetToken, "", req.Name)
	respondCreated(c, result)
}

// List returns all CLI tokens belonging to the current user.
func (h *CLITokenHandler) List(c *gin.Context) {
	userID := c.MustGet("user_id").(string)
	tokens, err := h.svc.List(userID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	respondSuccess(c, gin.H{"items": tokens})
}

// Revoke deletes a CLI token by id. The id must belong to the current user.
func (h *CLITokenHandler) Revoke(c *gin.Context) {
	userID := c.MustGet("user_id").(string)
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := h.svc.Revoke(id, userID); err != nil {
		respondError(c, http.StatusNotFound, "token_not_found", "token not found")
		return
	}
	h.audit.Simple(c, audit.ActionCliTokenRevoke, audit.TargetToken, idStr, "")
	respondMessage(c, http.StatusOK, "token revoked")
}
