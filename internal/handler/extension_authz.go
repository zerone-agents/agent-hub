// H7.4 扩展权限管理 API：授权列表 / 撤销 / 批准 / 调用审计分页。
// 全部端点挂在 /api/v1/admin 分组（由 cmd/server/main.go 接线），
// 按租户隔离查询，中文错误。
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

// ExtensionAuthzHandler 是扩展授权与审计的管理端 handler。
type ExtensionAuthzHandler struct {
	authz *services.ExtensionAuthzService
}

func NewExtensionAuthzHandler(authz *services.ExtensionAuthzService) *ExtensionAuthzHandler {
	return &ExtensionAuthzHandler{authz: authz}
}

// ListGrants GET /api/v1/admin/extensions/:id/grants
func (h *ExtensionAuthzHandler) ListGrants(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	grants, err := h.authz.ListGrants(tenant.GetTenantID(c), id)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, gin.H{"items": grants})
}

// RevokeGrant DELETE /api/v1/admin/extensions/:id/grants/:grantId
// 撤销后该扩展立刻失去对应权限（Enforce 实时反映）。
func (h *ExtensionAuthzHandler) RevokeGrant(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	grantID, err := strconv.ParseUint(strings.TrimSpace(c.Param("grantId")), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid grant id")
		return
	}
	if err := h.authz.RevokeGrant(tenant.GetTenantID(c), id, grantID); err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "授权已撤销")
}

// ApproveGrant POST /api/v1/admin/extensions/:id/grants/:grantId/approve
// 批准 manifest mode=approval 声明产生的 pending 授权。
func (h *ExtensionAuthzHandler) ApproveGrant(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	grantID, err := strconv.ParseUint(strings.TrimSpace(c.Param("grantId")), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid grant id")
		return
	}
	if err := h.authz.ApproveGrant(tenant.GetTenantID(c), id, grantID, actorID(c)); err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondMessage(c, http.StatusOK, "授权已批准")
}

// ListAudit GET /api/v1/admin/extensions/:id/audit?page=&pageSize=&allowed=
func (h *ExtensionAuthzHandler) ListAudit(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	filter := services.AuditFilter{Page: page, PageSize: pageSize}
	if raw := strings.TrimSpace(c.Query("allowed")); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			respondError(c, http.StatusBadRequest, "allowed 必须是 true/false")
			return
		}
		filter.Allowed = &b
	}
	result, err := h.authz.ListAudit(tenant.GetTenantID(c), id, filter)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, result)
}
