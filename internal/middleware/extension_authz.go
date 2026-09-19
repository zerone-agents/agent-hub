// H7.4 扩展授权强制检查中间件。
//
// 身份来源：本中间件**不读取任何请求头**。扩展名只从
// ExtensionIdentity 中间件写入上下文的"已认证扩展名"取值（见
// extension_identity.go）。这样"自己报一个扩展名就拿到判定"的路径不复存在：
// 想影响判定结果，必须先通过凭据校验。
//
// 判定规则：按 (tenant, extension, permission, action, resource) 调 Enforce，
// 未授权返回 403「扩展未被授予该权限」。
//
// 无已认证扩展身份 → 直接放行：这些路由同时服务于人类控制台与 Agent
// Runtime，它们的访问控制由 JWT / Runtime Token 负责；扩展 grants 是叠加在
// 已认证调用方之上的**额外收窄**，不是控制台的主认证手段。
package middleware

import (
	"net/http"
	"strings"

	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// ExtensionHeaderName 是扩展身份请求头（由 ExtensionIdentity 中间件校验）。
const ExtensionHeaderName = "X-Extension-Name"

// ExtensionEnforcer 是 ExtensionAuthzService.Enforce 的最小接口形态
// （避免 middleware 包反向依赖具体服务实现，便于测试替换）。
type ExtensionEnforcer interface {
	Enforce(tenantID, extensionName, permission, action, resource, ip string) bool
}

// ExtensionAuthz 返回授权检查中间件：permission/action 描述被保护端点
// 对应的权限类与操作；resourceFn 可选，从请求提取具体资源（如路径中的
// run id / agent name / state namespace），不传则资源匹配留空。
//
// 无已认证扩展身份：直接放行（人类/Agent 请求零影响）。
// 有已认证身份但无授权：403 且写审计（Enforce 内部完成）。
func ExtensionAuthz(enforcer ExtensionEnforcer, permission, action string, resourceFn func(*gin.Context) string) gin.HandlerFunc {
	permission = strings.TrimSpace(permission)
	action = strings.TrimSpace(action)
	return func(c *gin.Context) {
		extName := ExtensionIdentityOf(c)
		if extName == "" {
			c.Next()
			return
		}
		resource := ""
		if resourceFn != nil {
			resource = strings.TrimSpace(resourceFn(c))
		}
		if !enforcer.Enforce(tenant.GetTenantID(c), extName, permission, action, resource, c.ClientIP()) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"error":   "扩展未被授予该权限",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
