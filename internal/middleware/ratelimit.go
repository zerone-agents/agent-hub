package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// IPRateLimit is a simple fixed-window per-IP in-memory rate limiter. Each IP
// gets up to `limit` requests per `window`; requests beyond that return 429.
// Counters reset when the window elapses. State is in-memory and per-process,
// suitable for single-instance deployments; counters reset on restart.
func IPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	type bucket struct {
		count     int
		windowEnd time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*bucket{}

	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()

		mu.Lock()
		b, ok := buckets[ip]
		if !ok || now.After(b.windowEnd) {
			b = &bucket{count: 0, windowEnd: now.Add(window)}
			buckets[ip] = b
		}
		b.count++
		count := b.count
		mu.Unlock()

		if count > limit {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
