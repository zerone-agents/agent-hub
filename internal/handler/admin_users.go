package handler

import (
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"
	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
)

// AdminUserHandler serves /api/v1/admin/users and /api/v1/admin/invites for
// auth.mode=builtin, guarded by RequireAdmin.
type AdminUserHandler struct {
	users    *services.UserService
	invites  *services.InviteService
	provider *builtin.Provider
}

// NewAdminUserHandler constructs an AdminUserHandler.
func NewAdminUserHandler(users *services.UserService, invites *services.InviteService, p *builtin.Provider) *AdminUserHandler {
	return &AdminUserHandler{users: users, invites: invites, provider: p}
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
		respondError(c, http.StatusInternalServerError, "查询用户失败")
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
		respondError(c, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	actorID, err := strconv.ParseUint(c.GetString("user_id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "无效的用户身份")
		return
	}
	var req struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	if req.Role == "" && req.Status == "" {
		respondError(c, http.StatusBadRequest, "至少需要提供一个字段")
		return
	}
	if req.Role != "" {
		if err := h.users.UpdateRole(id, actorID, req.Role); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Status != "" {
		if err := h.users.SetStatus(id, actorID, req.Status); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
		if req.Status == authdom.StatusDisabled {
			_ = h.provider.RevokeAllForUser(id)
		}
	}
	respondSuccess(c, nil)
}

// ResetUserPassword sets a random password, returns the plaintext once, and
// logs the user out everywhere.
func (h *AdminUserHandler) ResetUserPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的用户 ID")
		return
	}
	plain, err := h.users.ResetPassword(id)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.provider.RevokeAllForUser(id)
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
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	actorID, _ := strconv.ParseUint(c.GetString("user_id"), 10, 64)
	res, err := h.invites.Create(req.Role, req.Note, actorID, req.ExpiresInDays)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
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
		respondError(c, http.StatusInternalServerError, "查询邀请失败")
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
		respondError(c, http.StatusBadRequest, "无效的邀请 ID")
		return
	}
	if err := h.invites.Revoke(id); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	respondSuccess(c, nil)
}
