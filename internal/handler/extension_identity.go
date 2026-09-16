// H7.4 扩展身份凭据管理 API（P0）。
//
// 挂在 /api/v1/admin 分组（由 cmd/server/main.go 接线，admin|maintainer）。
// 明文 token 只在签发响应里返回一次，之后无论谁都无法再取回 —— 丢失只能重签。
package handler

import (
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// ExtensionIdentityHandler 是扩展身份凭据的管理端 handler。
type ExtensionIdentityHandler struct {
	identity *services.ExtensionIdentityService
}

func NewExtensionIdentityHandler(identity *services.ExtensionIdentityService) *ExtensionIdentityHandler {
	return &ExtensionIdentityHandler{identity: identity}
}

// Describe GET /api/v1/admin/extensions/:id/credential
// 返回凭据的公开信息（tokenId / 签发时间 / 是否已签发），不含 secret。
func (h *ExtensionIdentityHandler) Describe(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	info, err := h.identity.DescribeCredential(tenant.GetTenantID(c), id)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	respondSuccess(c, info)
}

// Issue POST /api/v1/admin/extensions/:id/credential
// 签发或轮换扩展身份凭据。响应里的 token 明文只出现这一次；轮换后旧凭据
// 立即失效（调用方需尽快把新 token 配置到扩展侧）。
func (h *ExtensionIdentityHandler) Issue(c *gin.Context) {
	id, err := parseExtensionID(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid extension id")
		return
	}
	issue, err := h.identity.IssueCredential(tenant.GetTenantID(c), id)
	if err != nil {
		respondLifecycleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    issue,
		"message": "扩展身份凭据已签发：token 仅此一次返回，请立即保存；调用 Hub API 时需同时携带 X-Extension-Name 与 X-Extension-Token",
	})
}
