package middleware

import "github.com/gin-gonic/gin"

// ApplyTrustedProxies 配置 gin 可信代理 CIDR（spec §5.2）。
// 默认空 = 不信任任何代理（ClientIP 取对端地址、忽略 X-Forwarded-For）。
// 非法 CIDR 返回错误：main 调用方必须 log.Fatalf（带 stack）终止启动。
func ApplyTrustedProxies(r *gin.Engine, proxies []string) error {
	if len(proxies) == 0 {
		return r.SetTrustedProxies(nil)
	}
	return r.SetTrustedProxies(proxies)
}
