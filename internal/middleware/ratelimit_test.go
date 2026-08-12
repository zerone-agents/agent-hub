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
