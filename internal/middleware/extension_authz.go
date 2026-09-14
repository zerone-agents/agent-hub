// H7.4 扩展身份强制检查中间件。
//
// 工作原理：HTTP 代理（H7.2 / H7.6 SDK）在替扩展发起调用时注入
// X-Extension-Name 请求头标识扩展身份。本中间件只在请求携带该头时
// 生效：按 (tenant, extension, permission, action, resource) 调 Enforce
// 判定，未授权返回 403「扩展未被授予该权限」；不带该头的既有请求
// 直接放行，行为零变化。
package middleware

import (
	"net/http"
	"strings"

	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

// ExtensionHeaderName 是代理注入的扩展身份请求头。
const ExtensionHeaderName = "X-Extension-Name"

// ExtensionEnforcer 是 ExtensionAuthzService.Enforce 的最小接口形态
// （避免 middleware 包反向依赖具体服务实现，便于测试替换）。
type ExtensionEnforcer interface {
	Enforce(tenantID, extensionName, permission, action, resource, ip string) bool
}

// ExtensionAuthz 返回强制检查中间件：permission/action 描述被保护端点
// 对应的权限类与操作；resourceFn 可选，从请求提取具体资源（如路径中的
// agent name / state namespace），不传则资源匹配留空（只按权限类+动作判定）。
//
// 无 X-Extension-Name 头：直接放行（既有非扩展请求零影响）。
// 有头但无授权：403 且写审计（Enforce 内部完成）。
func ExtensionAuthz(enforcer ExtensionEnforcer, permission, action string, resourceFn func(*gin.Context) string) gin.HandlerFunc {
	permission = strings.TrimSpace(permission)
	action = strings.TrimSpace(action)
	return func(c *gin.Context) {
		extName := strings.TrimSpace(c.GetHeader(ExtensionHeaderName))
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
