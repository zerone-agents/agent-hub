package handler

import (
	"errors"
	"net/http"

	"control-panel/internal/directory"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// UserDirectory is the user-management surface for casdoor mode.
type UserDirectory interface {
	ListUsers(tenantID string) ([]directory.ManagedUser, error)
	UpdateRole(tenantID, userID, role, actorID string) error
	SetDisabled(tenantID, userID string, disabled bool, actorID string) error
	ResetPassword(tenantID, userID, actorID string) (string, error)
}

// LoginURLBuilder 按组织生成一次性的 OAuth 授权登录链接（带 client_id、
// PKCE S256、redirect_uri）。实现注入（main.go 传 auth.GenerateLoginURL），
// handler 不直接依赖 auth 包，测试可注入 fake。
type LoginURLBuilder func(org string) (string, error)

// CasdoorUserHandler serves the admin user-management endpoints backed by a
// casdoor directory.
type CasdoorUserHandler struct {
	dir        UserDirectory
	loginURLFn LoginURLBuilder
}

// NewCasdoorUserHandler constructs the handler. loginURLFn builds the
// per-tenant OAuth authorize URL (each org resolves its own client creds).
func NewCasdoorUserHandler(dir UserDirectory, loginURLFn LoginURLBuilder) *CasdoorUserHandler {
	return &CasdoorUserHandler{dir: dir, loginURLFn: loginURLFn}
}

// ListUsers serves GET /admin/users for casdoor mode.
func (h *CasdoorUserHandler) ListUsers(c *gin.Context) {
	users, err := h.dir.ListUsers(tenant.GetTenantID(c))
	if err != nil {
		respondError(c, http.StatusBadGateway, "Casdoor 用户查询失败: "+err.Error())
		return
	}
	respondSuccess(c, users)
}

// UpdateUser serves PATCH /admin/users/:id (role and/or status).
func (h *CasdoorUserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	actorID := c.GetString("user_id")
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
	// Validate status before applying any change, so a PATCH with a valid role
	// plus an invalid status is rejected without side effects.
	if req.Status != "" && req.Status != "active" && req.Status != "disabled" {
		respondError(c, http.StatusBadRequest, "无效的 status")
		return
	}
	if req.Role != "" {
		if err := h.dir.UpdateRole(tenant.GetTenantID(c), id, req.Role, actorID); err != nil {
			respondDirectoryError(c, err)
			return
		}
	}
	if req.Status != "" {
		if err := h.dir.SetDisabled(tenant.GetTenantID(c), id, req.Status == "disabled", actorID); err != nil {
			respondDirectoryError(c, err)
			return
		}
	}
	respondSuccess(c, nil)
}

// ResetUserPassword serves POST /admin/users/:id/reset-password.
func (h *CasdoorUserHandler) ResetUserPassword(c *gin.Context) {
	plain, err := h.dir.ResetPassword(tenant.GetTenantID(c), c.Param("id"), c.GetString("user_id"))
	if err != nil {
		respondDirectoryError(c, err)
		return
	}
	respondSuccess(c, gin.H{"password": plain})
}

// LoginURL serves GET /admin/users/login-url. The URL is built per-request
// for the caller's tenant: each org resolves its own OAuth client creds in
// tenant_oauth_clients, so admins hand out an authorize link that lands new
// users on their org's login/register flow (instead of a static signup page).
func (h *CasdoorUserHandler) LoginURL(c *gin.Context) {
	loginURL, err := h.loginURLFn(tenant.GetTenantID(c))
	if err != nil {
		// 生成失败通常表示本组织未注册 OAuth client（配置缺失），详情给
		// 管理员排障；与登录入口的 404 中性文案不同，这里是管理端工具。
		respondError(c, http.StatusBadGateway, "生成登录链接失败: "+err.Error())
		return
	}
	respondSuccess(c, gin.H{"loginUrl": loginURL})
}

// respondDirectoryError maps directory sentinel errors to HTTP status codes.
func respondDirectoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, directory.ErrSelfOperation):
		respondError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, directory.ErrInvalidRole):
		respondError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, directory.ErrUserNotFound):
		respondError(c, http.StatusNotFound, err.Error())
	default:
		respondError(c, http.StatusBadGateway, "Casdoor 操作失败: "+err.Error())
	}
}
