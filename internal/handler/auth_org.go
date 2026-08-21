package handler

import (
	"net/http"
	"strings"

	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
)

// OrgCheckHandler 承载多组织 OAuth 的两个公共端点（无 JWT）：
//   - GET /auth/org-check?org=<名称>：登录跳转前的组织预检；
//   - GET /auth/mode（casdoor 模式）：响应附带 multiOrg，驱动前端组织选择入口。
//
// 直接注入 TenantOAuthClientRepository（与 OpsTenantClientHandler 同模式），
// 测试经 database.DB 换 sqlite 内存库。
type OrgCheckHandler struct {
	repo *repository.TenantOAuthClientRepository
}

// NewOrgCheckHandler constructs an OrgCheckHandler.
func NewOrgCheckHandler(repo *repository.TenantOAuthClientRepository) *OrgCheckHandler {
	return &OrgCheckHandler{repo: repo}
}

// OrgCheck 预检组织是否已注册：空 org → 400；命中 → 200 exists:true；
// 未命中 → 404 统一文案（与 /auth/login 一致，不区分未注册/不存在避免探测）。
func (h *OrgCheckHandler) OrgCheck(c *gin.Context) {
	org := strings.TrimSpace(c.Query("org"))
	if org == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "org 参数必填",
		})
		return
	}
	row, err := h.repo.Find(org)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "查询组织失败",
		})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "组织不存在或未注册",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"exists": true},
	})
}

// CasdoorMode 返回 casdoor 模式的 /auth/mode 响应；multiOrg =
// tenant_oauth_clients 有任意行，前端据此决定是否渲染组织选择入口。
func (h *OrgCheckHandler) CasdoorMode(c *gin.Context) {
	multiOrg := false
	if n, err := h.repo.Count(); err == nil && n > 0 {
		multiOrg = true
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"mode":        "casdoor",
		"initialized": true,
		"multiOrg":    multiOrg,
	}})
}
