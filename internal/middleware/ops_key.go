package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireOpsKey 保护运维端点。key 为空 = 功能未启用，全部 404（端点隐身）；
// 非空则要求 X-Ops-Key 头常量时间匹配，不匹配 401。
func RequireOpsKey(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if key == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		got := c.GetHeader("X-Ops-Key")
		if subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
