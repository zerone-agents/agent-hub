package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIPRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", IPRateLimit(3, time.Minute), func(c *gin.Context) { c.Status(200) })
	do := func() int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 3; i++ {
		if code := do(); code != 200 {
			t.Fatalf("request %d got %d", i, code)
		}
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("4th request got %d, want 429", code)
	}
}

func TestIPRateLimitSharedInstanceAcrossRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 同一个 rl 实例挂在多个端点上 = 共享同一个计数桶（main.go casdoor 分支
	// 的历史形态：/mode + /org-check + /login + /refresh 合计 10 次/分）。
	r := gin.New()
	rl := IPRateLimit(3, time.Minute)
	r.GET("/a", rl, func(c *gin.Context) { c.Status(200) })
	r.GET("/b", rl, func(c *gin.Context) { c.Status(200) })
	hit := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "1.2.3.4:5678"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	// /a 打 3 次打满 → /b 被拖累 429：这正是登录操作挤占 /auth/mode 的机制。
	for i := 0; i < 3; i++ {
		if code := hit("/a"); code != 200 {
			t.Fatalf("/a request %d = %d", i, code)
		}
	}
	if code := hit("/b"); code != http.StatusTooManyRequests {
		t.Fatalf("/b after /a saturates shared bucket = %d, want 429", code)
	}
}

func TestIPRateLimitIndependentInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 独立实例互不挤占：/mode 单独挂实例时，/login 打满不影响 /mode——
	// main.go casdoor 分支修复后的形态（rlMode 独立于 rl）。
	r := gin.New()
	r.GET("/mode", IPRateLimit(3, time.Minute), func(c *gin.Context) { c.Status(200) })
	r.GET("/login", IPRateLimit(3, time.Minute), func(c *gin.Context) { c.Status(200) })
	hit := func(path string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "1.2.3.4:5678"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 3; i++ {
		if code := hit("/login"); code != 200 {
			t.Fatalf("/login request %d = %d", i, code)
		}
	}
	if code := hit("/login"); code != http.StatusTooManyRequests {
		t.Fatalf("/login exceeded = %d, want 429", code)
	}
	// /mode 不受 /login 打满影响，仍 200。
	if code := hit("/mode"); code != http.StatusOK {
		t.Fatalf("/mode after /login saturates own bucket = %d, want 200", code)
	}
}

func TestIPRateLimitSeparateIPs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", IPRateLimit(1, time.Minute), func(c *gin.Context) { c.Status(200) })
	hit := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := hit("1.1.1.1:1"); code != 200 {
		t.Fatalf("first ip first req = %d", code)
	}
	if code := hit("2.2.2.2:2"); code != 200 {
		t.Fatalf("second ip first req = %d", code)
	}
	if code := hit("1.1.1.1:1"); code != http.StatusTooManyRequests {
		t.Fatalf("first ip second req = %d", code)
	}
}

func TestIPLimiterCapacityRejectsNewKeys(t *testing.T) {
	base := time.Now()
	now := base
	l := newIPLimiter(10, time.Minute, 8, func() time.Time { return now })
	// 单窗口内 8 个不同 IP（bucket 全部活跃）→ 第 9 个新 IP 被拒绝
	for i := 0; i < 8; i++ {
		ok, rejectedNew := l.allow("10.0.0." + string(rune('1'+i)))
		require.True(t, ok)
		require.False(t, rejectedNew)
	}
	ok, rejectedNew := l.allow("10.1.0.1")
	require.False(t, ok)
	require.True(t, rejectedNew) // 拒绝新 key，绝不淘汰活跃 bucket（spec §5.1）
}

func TestIPLimiterLimitedIPNotEvictedUnderPressure(t *testing.T) {
	base := time.Now()
	now := base
	l := newIPLimiter(1, time.Minute, 4, func() time.Time { return now })
	l.allow("9.9.9.9") // 计数 1（已达上限）
	l.allow("9.9.9.9") // 超限被阻
	for i := 0; i < 4; i++ {
		l.allow("10.0.0." + string(rune('1'+i))) // 填满容量
	}
	ok, _ := l.allow("9.9.9.9") // 已限流 IP 不被淘汰，依旧被阻止
	require.False(t, ok)
}

func TestIPLimiterSweepFreesExpired(t *testing.T) {
	base := time.Now()
	now := base
	l := newIPLimiter(1, time.Minute, 2, func() time.Time { return now })
	l.allow("1.1.1.1")
	now = base.Add(2 * time.Minute) // 窗口过期
	l.sweep(now)
	require.LessOrEqual(t, l.size(), 0) // 过期 bucket 被清理，槽位释放
}
