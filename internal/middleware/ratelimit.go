package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const ipRateLimitMaxBuckets = 65536

// IPRateLimit：固定窗口 per-IP 限流（加固版，spec §5.1）：
//   - 过期 bucket 惰性清理 + 周期清扫（释放一次性 IP 槽位）
//   - 硬容量上限：满载时拒绝新 key（429），绝不淘汰仍活跃的 bucket
//     （淘汰活跃安全状态 = 给已被限流的攻击 IP 重置窗口）
func IPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	l := newIPLimiter(limit, window, ipRateLimitMaxBuckets, time.Now)
	go func() {
		t := time.NewTicker(window)
		for now := range t.C {
			l.sweep(now)
		}
	}()
	return func(c *gin.Context) {
		if ok, _ := l.allow(c.ClientIP()); !ok {
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

type ipBucket struct {
	count     int
	windowEnd time.Time
}

type ipLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*ipBucket
	limit      int
	window     time.Duration
	maxBuckets int
	now        func() time.Time
}

func newIPLimiter(limit int, window time.Duration, maxBuckets int, now func() time.Time) *ipLimiter {
	return &ipLimiter{buckets: map[string]*ipBucket{}, limit: limit, window: window, maxBuckets: maxBuckets, now: now}
}

// allow 返回 (是否放行, 是否因容量满被拒的新 key)。
func (l *ipLimiter) allow(ip string) (bool, bool) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok || now.After(b.windowEnd) {
		if !ok && len(l.buckets) >= l.maxBuckets {
			// 先清过期腾槽；仍满 → 拒绝新 key（活跃 bucket 不动）
			for k, bb := range l.buckets {
				if now.After(bb.windowEnd) {
					delete(l.buckets, k)
				}
			}
			if len(l.buckets) >= l.maxBuckets {
				return false, true
			}
		}
		b = &ipBucket{windowEnd: now.Add(l.window)}
		l.buckets[ip] = b
	}
	b.count++
	return b.count <= l.limit, false
}

// sweep 清理已过期 bucket（周期清扫）。
func (l *ipLimiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.buckets {
		if now.After(b.windowEnd) {
			delete(l.buckets, k)
		}
	}
}

func (l *ipLimiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
