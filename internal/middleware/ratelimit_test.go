package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
