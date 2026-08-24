package middleware

import (
	"crypto/subtle"
	"net/http"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/jwtutil"

	"github.com/gin-gonic/gin"
)

// ChatPushAuth 是 /api/v1/chat/push 专用的双通道鉴权中间件：
//
//  1. 请求带 X-Chat-Push-Key（非空）→ push-key 通道：与 CHAT_PUSH_API_KEY
//     常量时间比对。服务端未配置或比对失败均 401（显式选择该通道即明确
//     报错，不静默回落 JWT）。通过后仅标记 auth_method="chat_push_key"，
//     不注入任何用户身份（user_id/roles/tenant 缺失，归属由请求 body 的
//     user_name/org 决定）。
//  2. 不带该 header → 复用标准 JWT/CLI 鉴权（jwtutil.Authenticate）+
//     PendingApprovalGuard，语义与原先挂在 v1group 下完全一致。注意不能
//     直接调 AuthMiddlewareWithCLI——其成功路径内部 c.Next() 会先执行
//     handler 再回来补 guard，待审批用户会漏拦（有测试锁定此顺序）。
//
// 该密钥独立于 OPS_API_KEY，权限仅限本端点。
func ChatPushAuth(pushKey string, cliSvc *services.CLITokenService, p auth.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader("X-Chat-Push-Key")
		if got != "" {
			if pushKey == "" || subtle.ConstantTimeCompare([]byte(got), []byte(pushKey)) != 1 {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"error":   "invalid chat push key",
				})
				return
			}
			c.Set("auth_method", "chat_push_key")
			c.Next()
			return
		}

		// JWT/CLI 分支：Authenticate 只注入身份不推进链；guard 通过时由
		// 其内部 c.Next() 正确推进到后续 handler，任一环节 abort 则到此为止。
		jwtutil.Authenticate(c, cliSvc, p)
		if c.IsAborted() {
			return
		}
		jwtutil.PendingApprovalGuard()(c)
	}
}
