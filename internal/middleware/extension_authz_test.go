// H7.4 扩展身份强制检查与限流中间件测试：
// 无 X-Extension-Name 头直接放行（既有请求零影响）；
// 有头无授权 403；有头有授权放行；撤销即时 403；限流超限 429。
package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeEnforcer struct {
	mu    sync.Mutex
	calls []enforceCall
	next  bool
}

type enforceCall struct {
	TenantID, ExtensionName, Permission, Action, Resource, IP string
}

func (f *fakeEnforcer) Enforce(tenantID, extensionName, permission, action, resource, ip string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, enforceCall{tenantID, extensionName, permission, action, resource, ip})
	return f.next
}

func newAuthzTestRouter(enforcer *fakeEnforcer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	})
	r.GET("/api/v1/admin/runs/:id",
		ExtensionAuthz(enforcer, "run", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func doRequest(t *testing.T, r *gin.Engine, path, extName string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if extName != "" {
		req.Header.Set(ExtensionHeaderName, extName)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExtensionAuthzNoHeaderPassesThrough(t *testing.T) {
	enforcer := &fakeEnforcer{next: false}
	r := newAuthzTestRouter(enforcer)
	w := doRequest(t, r, "/api/v1/admin/runs/42", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, enforcer.calls) // 未触发判定
}

func TestExtensionAuthzDenied403(t *testing.T) {
	enforcer := &fakeEnforcer{next: false}
	r := newAuthzTestRouter(enforcer)
	w := doRequest(t, r, "/api/v1/admin/runs/42", "io.zerone.ext")
	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "扩展未被授予该权限", body["error"])
	require.Len(t, enforcer.calls, 1)
	require.Equal(t, "run", enforcer.calls[0].Permission)
	require.Equal(t, "read", enforcer.calls[0].Action)
	require.Equal(t, "42", enforcer.calls[0].Resource)
	require.Equal(t, "tenant-a", enforcer.calls[0].TenantID)
	require.Equal(t, "io.zerone.ext", enforcer.calls[0].ExtensionName)
}

func TestExtensionAuthzAllowedPasses(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	r := newAuthzTestRouter(enforcer)
	w := doRequest(t, r, "/api/v1/admin/runs/7", "io.zerone.ext")
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, enforcer.calls, 1)
}

func TestExtensionAuthzRevokeReflectedImmediately(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	r := newAuthzTestRouter(enforcer)
	require.Equal(t, http.StatusOK, doRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext").Code)
	// 模拟撤销：同进程内 Enforce 实时查库，fake 翻转结果即代表撤销即时生效
	enforcer.mu.Lock()
	enforcer.next = false
	enforcer.mu.Unlock()
	require.Equal(t, http.StatusForbidden, doRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext").Code)
}

func TestExtensionRateLimit429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	})
	r.GET("/api/v1/extensions/:name/proxy",
		ExtensionRateLimitByParam(ExtensionRateLimitConfig{RequestsPerMinute: 3}, func(c *gin.Context) string { return c.Param("name") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	ok, limited := 0, 0
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.a/proxy", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		switch w.Code {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	require.Equal(t, 3, ok)
	require.Equal(t, 2, limited)

	// 其他扩展名不受牵连（per-tenant+extension 维度）
	req := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.b/proxy", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// 无扩展身份请求不受限
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { tenant.SetTenantID(c, "tenant-a"); c.Next() })
	r2.GET("/x", ExtensionRateLimit(ExtensionRateLimitConfig{RequestsPerMinute: 1}), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}
