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

// CasdoorUserHandler serves the admin user-management endpoints backed by a
// casdoor directory.
type CasdoorUserHandler struct {
	dir       UserDirectory
	signupURL string
}

// NewCasdoorUserHandler constructs the handler. signupURL is the casdoor
// application signup URL handed out to admins for inviting users.
func NewCasdoorUserHandler(dir UserDirectory, signupURL string) *CasdoorUserHandler {
	return &CasdoorUserHandler{dir: dir, signupURL: signupURL}
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
	if req.Role != "" {
		if err := h.dir.UpdateRole(tenant.GetTenantID(c), id, req.Role, actorID); err != nil {
			respondDirectoryError(c, err)
			return
		}
	}
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			respondError(c, http.StatusBadRequest, "无效的 status")
			return
		}
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

// SignupURL serves GET /admin/users/signup-url.
func (h *CasdoorUserHandler) SignupURL(c *gin.Context) {
	respondSuccess(c, gin.H{"signupUrl": h.signupURL})
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
