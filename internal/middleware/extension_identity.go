// H7.4 扩展身份认证中间件（P0）。
//
// 与 ExtensionAuthz（授权）配对使用：本中间件负责"你是谁"，授权中间件
// 负责"你能不能"。职责分离后，授权中间件不再读取任何请求头，只信任本
// 中间件写入上下文的已认证扩展名 —— 请求头自报身份这条绕过路径被彻底关闭。
//
// 判定规则（fail-closed）：
//   - 两个头都不带 → 视为非扩展请求（人类控制台 / Agent Runtime），放行且不写身份；
//   - 只带其中一个 → 401。残缺身份不降级为"非扩展请求"，否则任何一次
//     拼写错误或刻意漏传都能把带凭据的请求降级成无检查的请求；
//   - 两个都带但校验不通过（凭据错误 / 已停用 / 未签发）→ 401，不写身份；
//   - 校验过程本身出错（数据库不可用）→ 503，同样不写身份。
//
// 在通用 API 链路中，这枚凭据就是扩展的独立认证身份：扩展不需要、
// 也不应持有人类管理员 JWT/CLI Token。认证后由 ExtensionAPIGuard
// 执行默认拒绝的接口白名单与 grants 检查。Agent Runtime 专用 MCP 链路
// 仍可把它作为 Runtime Token 之上的额外收窄。
package middleware

import (
	"log"
	"net/http"
	"strings"

	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

const (
	// ExtensionTokenHeaderName 是扩展身份凭据请求头（Hub 签发的一次性明文）。
	ExtensionTokenHeaderName = "X-Extension-Token"
	// ExtensionIdentityContextKey 是已认证扩展名在 gin.Context 中的键。
	ExtensionIdentityContextKey = "authenticatedExtensionName"
	// ExtensionTenantHeaderName is required only for tenant-scoped extensions.
	// The credential is still verified against this tenant, so the header is a
	// lookup hint rather than an authority claim.
	ExtensionTenantHeaderName = "X-Extension-Tenant"
)

// ExtensionVerifier 是 ExtensionIdentityService.Verify 的最小接口形态
// （避免 middleware 包反向依赖具体服务实现，便于测试替换）。
type ExtensionVerifier interface {
	Verify(tenantID, extensionName, token string) (bool, error)
}

// ExtensionIdentityOf 返回本请求的已认证扩展名；非扩展请求返回空串。
// 只有 ExtensionIdentity 中间件校验通过才会写入该值。
func ExtensionIdentityOf(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(ExtensionIdentityContextKey); ok {
		if name, ok := v.(string); ok {
			return name
		}
	}
	return ""
}

// ExtensionIdentity 返回扩展身份认证中间件。
func ExtensionIdentity(verifier ExtensionVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := strings.TrimSpace(c.GetHeader(ExtensionHeaderName))
		token := strings.TrimSpace(c.GetHeader(ExtensionTokenHeaderName))
		if name == "" && token == "" {
			c.Next()
			return
		}
		if name == "" || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "扩展身份不完整：需同时提供 X-Extension-Name 与 X-Extension-Token",
			})
			c.Abort()
			return
		}
		tenantID := strings.TrimSpace(c.GetHeader(ExtensionTenantHeaderName))
		if tenantID == "" {
			tenantID = tenant.GetTenantID(c)
		}
		if tenantID == "" {
			tenantID = tenant.DefaultID
		}
		ok, err := verifier.Verify(tenantID, name, token)
		if err != nil {
			log.Printf("[h7] extension identity verify failed: tenant=%s extension=%s: %v", tenantID, name, err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"error":   "扩展身份校验暂时不可用，请稍后再试",
			})
			c.Abort()
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "扩展身份校验失败",
			})
			c.Abort()
			return
		}
		tenant.SetTenantID(c, tenantID)
		c.Set(ExtensionIdentityContextKey, name)
		c.Set("auth_method", "extension")
		c.Next()
	}
}
