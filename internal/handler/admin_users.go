package handler

import (
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"
	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
)

// AdminUserHandler serves /api/v1/admin/users and /api/v1/admin/invites for
// auth.mode=builtin, guarded by RequireAdmin.
type AdminUserHandler struct {
	users    *services.UserService
	invites  *services.InviteService
	provider *builtin.Provider
	audit    *services.AuditRecorder
}

// NewAdminUserHandler constructs an AdminUserHandler.
func NewAdminUserHandler(users *services.UserService, invites *services.InviteService, p *builtin.Provider, ar *services.AuditRecorder) *AdminUserHandler {
	return &AdminUserHandler{users: users, invites: invites, provider: p, audit: ar}
}

// userDTO is the safe projection of a user for the admin UI. PasswordHash is
// never included (the model tag is json:"-").
type userDTO struct {
	ID          uint64    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

func toUserDTO(u *authdom.User) userDTO {
	return userDTO{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName,
		Email: u.Email, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt,
	}
}

// ListUsers returns all users ordered by id. Password hashes are never exposed.
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	users, err := h.users.List()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "user_query_failed", "查询用户失败")
		return
	}
	dtos := make([]userDTO, 0, len(users))
	for _, u := range users {
		dtos = append(dtos, toUserDTO(u))
	}
	respondSuccess(c, dtos)
}

// UpdateUser changes a user's role and/or status. Disabling a user also drops
// all their refresh tokens (immediate logout on next access-token expiry / at
// once for refresh). Self-disable and last-admin-loss are rejected.
func (h *AdminUserHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_user_id", "无效的用户 ID")
		return
	}
	actorID, err := strconv.ParseUint(c.GetString("user_id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "invalid_user_identity", "无效的用户身份")
		return
	}
	var req struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	if req.Role == "" && req.Status == "" {
		respondError(c, http.StatusBadRequest, "field_required", "至少需要提供一个字段")
		return
	}
	if req.Role != "" {
		rcpt, err := h.users.UpdateRole(id, actorID, req.Role)
		if err != nil {
			respondError(c, http.StatusBadRequest, "user_role_update_failed", err.Error())
			return
		}
		// 规则 1：role 已生效即记（即使同请求随后 status 失败——部分成功不漏审）
		h.audit.RoleChanged(c, strconv.FormatUint(id, 10), h.displayUserName(id),
			string(rcpt.RoleBefore), string(rcpt.RoleAfter), audit.StatusSuccess)
	}
	if req.Status != "" {
		rcpt, err := h.users.SetStatus(id, actorID, req.Status)
		if err != nil {
			respondError(c, http.StatusBadRequest, "user_status_update_failed", err.Error())
			return
		}
		// builtin 无远端：from/to 取 receipt 的 Effective 值（== Status）
		h.audit.StatusChanged(c, strconv.FormatUint(id, 10), h.displayUserName(id),
			string(rcpt.EffectiveStatusBefore), string(rcpt.EffectiveStatusAfter))
		if req.Status == authdom.StatusDisabled {
			_ = h.provider.RevokeAllForUser(id)
		}
	}
	respondSuccess(c, nil)
}

// displayUserName 预读展示名（best-effort，读失败退回数字 id 字符串）。
// 仅用于审计 TargetName 展示：from/to 变更判定一律取 receipt（预读禁令，
// spec §3.2——竞态最坏只影响显示名）。
func (h *AdminUserHandler) displayUserName(id uint64) string {
	if u, err := h.users.GetByID(id); err == nil {
		return u.Username
	}
	return strconv.FormatUint(id, 10)
}

// ResetUserPassword sets a random password, returns the plaintext once, and
// logs the user out everywhere. 不能对自己重置（走自助改密）。
func (h *AdminUserHandler) ResetUserPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_user_id", "无效的用户 ID")
		return
	}
	actorID, _ := strconv.ParseUint(c.GetString("user_id"), 10, 64)
	plain, err := h.users.ResetPassword(id, actorID)
	if err != nil {
		respondError(c, http.StatusBadRequest, "password_reset_failed", err.Error())
		return
	}
	_ = h.provider.RevokeAllForUser(id)
	h.audit.Simple(c, audit.ActionResetPassword, audit.TargetUser,
		strconv.FormatUint(id, 10), h.displayUserName(id))
	respondSuccess(c, gin.H{"password": plain})
}

// CreateInvite makes a one-time invite. The plaintext token is returned
// exactly once (only its hash is stored); the caller must copy the invite URL
// immediately.
func (h *AdminUserHandler) CreateInvite(c *gin.Context) {
	var req struct {
		Role          string `json:"role" binding:"required"`
		Note          string `json:"note"`
		ExpiresInDays int    `json:"expiresInDays"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	actorID, _ := strconv.ParseUint(c.GetString("user_id"), 10, 64)
	res, err := h.invites.Create(req.Role, req.Note, actorID, req.ExpiresInDays)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invite_creation_failed", err.Error())
		return
	}
	// 审计使用真实值（PR #150 审查 P2）：res.ID（携带于结果，无需反查）与
	// 规范化后的实际 TTL——service 已把 <=0 默认为 7 天，原始 req.ExpiresInDays
	// 可能为 0/负数，直接记录会失真。
	h.audit.InviteCreated(c, strconv.FormatUint(res.ID, 10), req.Role, res.TtlDays)
	respondSuccess(c, res)
}

// inviteDTO is the safe projection of an invite for the admin UI. The token
// hash is never included.
type inviteDTO struct {
	ID        uint64     `json:"id"`
	Role      string     `json:"role"`
	Note      string     `json:"note"`
	Status    string     `json:"status"` // pending | used | expired
	ExpiresAt time.Time  `json:"expiresAt"`
	UsedAt    *time.Time `json:"usedAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ListInvites returns all invites, newest first. Plaintext tokens are never
// present in the response.
func (h *AdminUserHandler) ListInvites(c *gin.Context) {
	invites, err := h.invites.List()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "invite_query_failed", "查询邀请失败")
		return
	}
	dtos := make([]inviteDTO, 0, len(invites))
	for _, inv := range invites {
		status := "pending"
		if inv.UsedAt != nil {
			status = "used"
		} else if time.Now().After(inv.ExpiresAt) {
			status = "expired"
		}
		dtos = append(dtos, inviteDTO{
			ID: inv.ID, Role: inv.Role, Note: inv.Note, Status: status,
			ExpiresAt: inv.ExpiresAt, UsedAt: inv.UsedAt, CreatedAt: inv.CreatedAt,
		})
	}
	respondSuccess(c, dtos)
}

// RevokeInvite deletes an unused invite. Used invites cannot be revoked (they
// are already consumed and thus invalid for registration).
func (h *AdminUserHandler) RevokeInvite(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_invite_id", "无效的邀请 ID")
		return
	}
	if err := h.invites.Revoke(id); err != nil {
		respondError(c, http.StatusBadRequest, "invite_revoke_failed", err.Error())
		return
	}
	// 无 Detail、不存码片段（spec §3）
	h.audit.Simple(c, audit.ActionInviteRevoke, audit.TargetInvite, strconv.FormatUint(id, 10), "")
	respondSuccess(c, nil)
}
