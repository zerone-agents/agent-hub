package handler

import (
	"errors"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/auth/builtin"

	"github.com/gin-gonic/gin"
)

// BuiltinAuthHandler serves /auth/* endpoints for auth.mode=builtin. It owns
// setup, login, register, refresh, logout, change-password and invite
// precheck. User-management (admin) endpoints live in AdminUserHandler.
type BuiltinAuthHandler struct {
	provider *builtin.Provider
	users    *services.UserService
	invites  *services.InviteService
}

// NewBuiltinAuthHandler constructs a BuiltinAuthHandler.
func NewBuiltinAuthHandler(p *builtin.Provider, users *services.UserService, invites *services.InviteService) *BuiltinAuthHandler {
	return &BuiltinAuthHandler{provider: p, users: users, invites: invites}
}

// GetMode reports the auth mode and whether the system is initialized.
// Lets the frontend decide whether to show the setup screen / login form.
func (h *BuiltinAuthHandler) GetMode(c *gin.Context) {
	initialized, err := h.users.Initialized()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "查询初始化状态失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"mode":        "builtin",
		"initialized": initialized,
	}})
}

// Setup creates the initial admin (fixed username "admin") exactly once and
// returns a token pair so the caller is immediately logged in.
func (h *BuiltinAuthHandler) Setup(c *gin.Context) {
	var req struct {
		Password        string `json:"password" binding:"required"`
		ConfirmPassword string `json:"confirmPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	if req.Password != req.ConfirmPassword {
		respondError(c, http.StatusBadRequest, "两次输入的密码不一致")
		return
	}
	user, err := h.users.CreateInitialAdmin(req.Password)
	if errors.Is(err, services.ErrAlreadyInitialized) {
		respondError(c, http.StatusConflict, "系统已初始化")
		return
	}
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	pair, err := h.provider.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}

// Login authenticates with username+password. Lockout → 429; any credential
// problem → 401 with a uniform message (no user enumeration).
func (h *BuiltinAuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	user, err := h.users.Authenticate(req.Username, req.Password)
	if errors.Is(err, services.ErrLocked) {
		respondError(c, http.StatusTooManyRequests, err.Error())
		return
	}
	if err != nil {
		respondError(c, http.StatusUnauthorized, services.ErrInvalidCredentials.Error())
		return
	}
	pair, err := h.provider.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}

// Refresh rotates a refresh token. Accepts both camelCase (refreshToken) and
// snake_case (refresh_token) JSON keys for compatibility with the existing
// frontend interceptor that posts refresh_token.
func (h *BuiltinAuthHandler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken      string `json:"refreshToken"`
		RefreshTokenSnake string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "refresh token is required")
		return
	}
	token := req.RefreshToken
	if token == "" {
		token = req.RefreshTokenSnake
	}
	pair, err := h.provider.RefreshToken(token)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "refresh token 无效或已过期")
		return
	}
	respondSuccess(c, pair)
}

// Logout revokes the refresh token carried in the body. Idempotent: an empty
// or unknown token still returns success.
func (h *BuiltinAuthHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = c.ShouldBindJSON(&req) // empty body is allowed (idempotent logout)
	_ = h.provider.RevokeToken(req.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "logged out successfully"})
}

// Register consumes a one-time invite and creates the account, then
// auto-logs the user in (returns a token pair). An invalid/used/expired invite
// yields 410; a username collision yields 409.
func (h *BuiltinAuthHandler) Register(c *gin.Context) {
	var req struct {
		InviteToken string `json:"inviteToken" binding:"required"`
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
		DisplayName string `json:"displayName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	inv, err := h.invites.Validate(req.InviteToken)
	if err != nil {
		respondError(c, http.StatusGone, services.ErrInviteInvalid.Error())
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, inv.Role)
	if errors.Is(err, services.ErrUsernameTaken) {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.invites.Consume(req.InviteToken); err != nil {
		// Race: the invite was consumed by a concurrent registration between
		// Validate and Consume. Roll back the just-created user and report 410.
		respondError(c, http.StatusGone, services.ErrInviteInvalid.Error())
		return
	}
	pair, err := h.provider.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}

// InvitePrecheck lets the register page validate a token before rendering the
// form. Returns {valid:true, note}; invalid tokens yield 410.
func (h *BuiltinAuthHandler) InvitePrecheck(c *gin.Context) {
	inv, err := h.invites.Validate(c.Param("token"))
	if err != nil {
		respondError(c, http.StatusGone, services.ErrInviteInvalid.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"valid": true,
		"note":  inv.Note,
	}})
}

// ChangePassword verifies the old password, sets the new one, revokes all
// existing sessions (delete refresh tokens), and returns a fresh token pair
// for the current session so the caller stays logged in.
func (h *BuiltinAuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"oldPassword" binding:"required"`
		NewPassword string `json:"newPassword" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "参数不完整")
		return
	}
	idStr := c.GetString("user_id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "无效的用户身份")
		return
	}
	if err := h.users.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
		}
		respondError(c, status, err.Error())
		return
	}
	// Drop every existing refresh token (all other sessions die) and issue a
	// fresh pair for this session.
	_ = h.provider.RevokeAllForUser(id)
	user, err := h.users.GetByID(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "用户查询失败")
		return
	}
	pair, err := h.provider.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}
