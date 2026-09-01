package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The cache policy keeps hashed assets immutable while forcing index.html /
// client routes to revalidate — the regression this guards: embed FS serves
// no validators, and without an explicit policy browsers heuristically cache
// the entry page across releases (observed on the v2.4.3 test deployment).
func TestStaticCacheHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(staticCacheHeaders)
	r.GET("/static", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/static/*filepath", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	cases := []struct {
		path string
		want string
	}{
		{"/static/assets/index-abc123.js", "public, max-age=31536000, immutable"},
		{"/static/assets/clipboard-Bu3j9iBs.js", "public, max-age=31536000, immutable"},
		{"/static", "no-cache"},
		{"/static/", "no-cache"},
		{"/static/index.html", "no-cache"},
		{"/static/agents", "no-cache"}, // SPA client route served by the fallback
		{"/static/favicon.ico", "no-cache"},
		{"/api/v1/ping", ""}, // non-static paths untouched
		// 相邻前缀负例：不属于 /static/* 的路径不得被误打缓存头
		{"/staticity", ""},
		{"/static-old", ""},
		{"/static2/assets/x.js", ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		r.ServeHTTP(rec, req)
		if got := rec.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("GET %s: Cache-Control = %q, want %q", tc.path, got, tc.want)
		}
	}
}
