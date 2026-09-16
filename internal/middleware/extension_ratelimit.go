// H7.4 扩展 API 代理的资源限制：per-(tenant, extension) 固定窗口
// 计数器限流。默认 60 req/min，超限 429。
//
// 内存上限说明：计数器为进程内 map，每个 (tenant, extension) 窗口
// 常驻一个 8 字节计数 + 时间戳条目；一万个活跃扩展约 1 MB 量级。
// 窗口过期条目采用惰性清理（下次同 key 访问时重建），map 规模与
// 活跃扩展数同阶，无泄漏风险。多实例部署时限流为 per-process 语义，
// 需要全局一致时应在网关层（如 Kong）叠加。
//
// H7.2 集成：挂在 /api/v1/extensions/:name/* 代理路由组上
// （见 cmd/server/main.go 接线说明）；key = tenant + ":" + :name。
package middleware

import (
	"net/http"
	"sync"
	"time"

	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
)

// ExtensionRateLimitConfig 是扩展代理限流配置。
type ExtensionRateLimitConfig struct {
	// RequestsPerMinute 每个 (tenant, extension) 每窗口（分钟）允许请求数。
	// <=0 时取默认值 60。
	RequestsPerMinute int
}

const defaultExtensionRPM = 60

// ExtensionRateLimit 返回按 (tenant, extension) 维度的固定窗口限流
// 中间件。扩展身份取 ExtensionIdentity 中间件写入上下文的已认证扩展名
// （不再读取请求头：自报的名字不是限流维度，否则轮换名字即可绕过）；
// 代理路由直接用 :name 路径参数即可（见 ExtensionRateLimitByParam）。
func ExtensionRateLimit(cfg ExtensionRateLimitConfig) gin.HandlerFunc {
	return ExtensionRateLimitByParam(cfg, ExtensionIdentityOf)
}

// ExtensionRateLimitByParam 允许调用方自定义扩展名提取函数：
// H7.2 代理路由传 func(c){ return c.Param("name") }。
func ExtensionRateLimitByParam(cfg ExtensionRateLimitConfig, nameFn func(*gin.Context) string) gin.HandlerFunc {
	limit := cfg.RequestsPerMinute
	if limit <= 0 {
		limit = defaultExtensionRPM
	}
	type bucket struct {
		count     int
		windowEnd time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	window := time.Minute

	return func(c *gin.Context) {
		name := nameFn(c)
		if name == "" {
			c.Next() // 非扩展请求不受影响
			return
		}
		key := tenant.GetTenantID(c) + ":" + name
		now := time.Now()

		mu.Lock()
		b, ok := buckets[key]
		if !ok || now.After(b.windowEnd) {
			b = &bucket{count: 0, windowEnd: now.Add(window)}
			buckets[key] = b
		}
		b.count++
		count := b.count
		mu.Unlock()

		if count > limit {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "扩展调用超出频率限制，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
