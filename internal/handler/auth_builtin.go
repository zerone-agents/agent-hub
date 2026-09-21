package handler

import (
	"errors"
	"net/http"
	"strconv"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/builtin"
	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"

	"github.com/gin-gonic/gin"
)

// builtinTokenProvider：*builtin.Provider 的窄接口缝——token 签发/撤销失败
// 路径可在同包测试中注入 stub。RefreshToken/RevokeAllForUser 是本 handler
// 实际调用的另两个 provider 方法，一并纳入使字段收窄后全方法可编译。
type builtinTokenProvider interface {
	IssueTokenPair(user *authdom.User) (*auth.TokenPair, error)
	RefreshToken(refreshToken string) (*auth.TokenPair, error)
	RevokeToken(refreshToken string) error
	RevokeAllForUser(userID uint64) error
}

// BuiltinAuthHandler serves /auth/* endpoints for auth.mode=builtin. It owns
// setup, login, register, refresh, logout, change-password and invite
// precheck. User-management (admin) endpoints live in AdminUserHandler.
type BuiltinAuthHandler struct {
	p       builtinTokenProvider
	users   *services.UserService
	invites *services.InviteService
	audit   *services.AuditRecorder
}

// NewBuiltinAuthHandler constructs a BuiltinAuthHandler.
func NewBuiltinAuthHandler(p *builtin.Provider, users *services.UserService, invites *services.InviteService, ar *services.AuditRecorder) *BuiltinAuthHandler {
	return &BuiltinAuthHandler{p: p, users: users, invites: invites, audit: ar}
}

// GetMode reports the auth mode and whether the system is initialized.
// Lets the frontend decide whether to show the setup screen / login form.
func (h *BuiltinAuthHandler) GetMode(c *gin.Context) {
	initialized, err := h.users.Initialized()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "setup_status_failed", "查询初始化状态失败")
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
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	if req.Password != req.ConfirmPassword {
		respondError(c, http.StatusBadRequest, "password_mismatch", "两次输入的密码不一致")
		return
	}
	user, err := h.users.CreateInitialAdmin(req.Password)
	if errors.Is(err, services.ErrAlreadyInitialized) {
		respondError(c, http.StatusConflict, "already_initialized", "系统已初始化")
		return
	}
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}
	// 埋点边界（spec §3.1）：CreateInitialAdmin 成功即记录——此后 IssueTokenPair
	// 失败时 admin 已创建，记录必须已存在。/auth/setup 是未认证端点（context 无
	// tenant_id），builtin 恒 default（spec §5.6）→ 显式携带 actor 落库，避免
	// 一次性 setup 事件退化为 stdout-only。
	uid := strconv.FormatUint(user.ID, 10)
	h.audit.Record(c, audit.SimpleEvent(
		audit.Actor{TenantID: "default", UserID: uid, UserName: user.Username},
		audit.ActionSetup, audit.TargetSystem, uid, user.Username))
	pair, err := h.p.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "issue_token_failed", "签发令牌失败")
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
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	user, err := h.users.Authenticate(req.Username, req.Password)
	if err != nil {
		// 埋点先于错误响应（spec §3.1）：Authenticate 失败 → invalid_credentials
		// （含锁定场景，uid 未知为空）
		h.audit.Login(c, "", req.Username, "default", audit.StatusFailure, audit.ReasonInvalidCredentials)
		if errors.Is(err, services.ErrLocked) {
			respondError(c, http.StatusTooManyRequests, "rate_limited", err.Error())
			return
		}
		respondError(c, http.StatusUnauthorized, "invalid_credentials", services.ErrInvalidCredentials.Error())
		return
	}
	pair, err := h.p.IssueTokenPair(user)
	if err != nil {
		// 凭校验已通过但会话未建立（spec §3.1：success = 签发完成）
		h.audit.Login(c, strconv.FormatUint(user.ID, 10), req.Username, "default", audit.StatusFailure, audit.ReasonTokenIssuanceFailed)
		respondError(c, http.StatusInternalServerError, "issue_token_failed", "签发令牌失败")
		return
	}
	h.audit.Login(c, strconv.FormatUint(user.ID, 10), req.Username, "default", audit.StatusSuccess, "")
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
		respondError(c, http.StatusBadRequest, "refresh_token_required", "refresh token is required")
		return
	}
	token := req.RefreshToken
	if token == "" {
		token = req.RefreshTokenSnake
	}
	pair, err := h.p.RefreshToken(token)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "invalid_refresh_token", "refresh token 无效或已过期")
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
	revErr := h.p.RevokeToken(req.RefreshToken)
	st := audit.StatusSuccess
	if revErr != nil {
		st = audit.StatusFailure // 凭证可能仍有效，如实记录（spec §3.1：状态以撤销结果为准）
	}
	h.audit.SimpleWithStatus(c, audit.ActionLogout, audit.TargetSystem, "", "", st)
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
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	inv, err := h.invites.Validate(req.InviteToken)
	if err != nil {
		respondError(c, http.StatusGone, "invalid_invite", services.ErrInviteInvalid.Error())
		return
	}
	user, err := h.users.Create(req.Username, req.Password, req.DisplayName, inv.Role)
	if errors.Is(err, services.ErrUsernameTaken) {
		respondError(c, http.StatusConflict, "conflict", err.Error())
		return
	}
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_parameter", err.Error())
		return
	}
	if _, err := h.invites.Consume(req.InviteToken); err != nil {
		// Race: the invite was consumed by a concurrent registration between
		// Validate and Consume. Roll back the just-created user so the username
		// is freed, then report 410.
		_ = h.users.Delete(user.ID)
		respondError(c, http.StatusGone, "invalid_invite", services.ErrInviteInvalid.Error())
		return
	}
	pair, err := h.p.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "issue_token_failed", "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}

// InvitePrecheck lets the register page validate a token before rendering the
// form. Returns {valid:true, note}; invalid tokens yield 410.
func (h *BuiltinAuthHandler) InvitePrecheck(c *gin.Context) {
	inv, err := h.invites.Validate(c.Param("token"))
	if err != nil {
		respondError(c, http.StatusGone, "invalid_invite", services.ErrInviteInvalid.Error())
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
		respondError(c, http.StatusBadRequest, "incomplete_parameter", "参数不完整")
		return
	}
	idStr := c.GetString("user_id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "invalid_user_identity", "无效的用户身份")
		return
	}
	if err := h.users.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, services.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
		}
		respondError(c, status, "login_failed", err.Error())
		return
	}
	// 密码变更已提交生效（spec §3.1）——后续撤销/查询/签发失败不影响本记录。
	h.audit.Simple(c, audit.ActionPasswordChange, audit.TargetUser, c.GetString("user_id"), c.GetString("user_name"))
	// Drop every existing refresh token (all other sessions die) and issue a
	// fresh pair for this session.
	_ = h.p.RevokeAllForUser(id)
	user, err := h.users.GetByID(id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "user_query_failed", "用户查询失败")
		return
	}
	pair, err := h.p.IssueTokenPair(user)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "issue_token_failed", "签发令牌失败")
		return
	}
	respondSuccess(c, pair)
}
