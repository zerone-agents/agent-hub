// H7.4 扩展身份认证 + 授权强制检查 + 限流中间件测试。
//
// 回归 P0：扩展身份曾经只靠请求头自报，于是"带这个头才检查、去掉头即放行"
// ——既不是边界（想绕过的调用方只要不发头），也让合法扩展永远拿不到授权。
// 现在身份必须先通过凭据校验（ExtensionIdentity），授权中间件只读校验通过
// 后写入上下文的名字。本文件用真实的两级中间件链断言：
//   - 不带身份头 → 放行且不触发任何判定（既有请求零影响）；
//   - 残缺身份（只带名字或只带凭据）→ 401，不降级为"非扩展请求"；
//   - 凭据无效 → 401，且**不触发判定**（自报的名字无法影响结果）；
//   - 凭据有效但无授权 → 403；有授权 → 放行；撤销即时生效。
package middleware

import (
	"encoding/json"
	"errors"
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

func (f *fakeEnforcer) snapshot() []enforceCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]enforceCall(nil), f.calls...)
}

// fakeVerifier 模拟 ExtensionIdentityService.Verify：只有
// (tenant, name, token) 命中 credentials 才算认证通过。
type fakeVerifier struct {
	credentials map[string]bool // key = name + "|" + token
	err         error
	calls       int
	mu          sync.Mutex
}

func (v *fakeVerifier) Verify(tenantID, extensionName, token string) (bool, error) {
	v.mu.Lock()
	v.calls++
	err := v.err
	v.mu.Unlock()
	if err != nil {
		return false, err
	}
	return v.credentials[extensionName+"|"+token], nil
}

func (v *fakeVerifier) callCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.calls
}

// newAuthzTestRouter 复刻生产接线：租户注入 → 扩展身份认证 → 授权判定。
func newAuthzTestRouter(enforcer *fakeEnforcer, verifier *fakeVerifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	})
	r.GET("/api/v1/admin/runs/:id",
		ExtensionIdentity(verifier),
		ExtensionAuthz(enforcer, "run", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

// doIdentityRequest 发一个带 (扩展名, 凭据) 的请求；空串表示不带该头。
func doIdentityRequest(t *testing.T, r *gin.Engine, path, extName, extToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if extName != "" {
		req.Header.Set(ExtensionHeaderName, extName)
	}
	if extToken != "" {
		req.Header.Set(ExtensionTokenHeaderName, extToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExtensionIdentityAbsentPassesThrough(t *testing.T) {
	enforcer := &fakeEnforcer{next: false}
	verifier := &fakeVerifier{}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Empty(t, enforcer.snapshot(), "非扩展请求不应触发授权判定")
	require.Zero(t, verifier.callCount(), "非扩展请求不应触发身份校验")
}

// 回归 P0：只带扩展名（不带凭据）必须 401，不能降级成"无头请求"直接放行。
func TestExtensionIdentityNameWithoutTokenRejected(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "io.zerone.ext", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "扩展身份不完整")
	require.Empty(t, enforcer.snapshot(), "身份不完整不得进入授权判定")
}

// 只带凭据（不带名字）同样 401。
func TestExtensionIdentityTokenWithoutNameRejected(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.a.b": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "", "zx1.a.b")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Empty(t, enforcer.snapshot())
}

// 回归 P0 的核心：拿一个自报的扩展名 + 任意凭据，既不能拿到判定结果，
// 也不能凭名字横穿授权面。
func TestExtensionIdentityBogusTokenCannotInfluenceAuthz(t *testing.T) {
	enforcer := &fakeEnforcer{next: true} // 就算 Enforce 会放行，也必须先过身份
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.good.secret": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "io.zerone.ext", "zx1.bad.guess")
	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "扩展身份校验失败")
	require.Empty(t, enforcer.snapshot(), "凭据无效的请求不得进入授权判定")
	require.Equal(t, 1, verifier.callCount())
}

func TestExtensionIdentityDeniedByAuthz403(t *testing.T) {
	enforcer := &fakeEnforcer{next: false}
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.good.secret": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "io.zerone.ext", "zx1.good.secret")
	require.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "扩展未被授予该权限", body["error"])

	calls := enforcer.snapshot()
	require.Len(t, calls, 1)
	require.Equal(t, "run", calls[0].Permission)
	require.Equal(t, "read", calls[0].Action)
	require.Equal(t, "42", calls[0].Resource)
	require.Equal(t, "tenant-a", calls[0].TenantID)
	require.Equal(t, "io.zerone.ext", calls[0].ExtensionName)
}

func TestExtensionIdentityAllowedPasses(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.good.secret": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/7", "io.zerone.ext", "zx1.good.secret")
	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, enforcer.snapshot(), 1)
}

func TestExtensionAuthzRevokeReflectedImmediately(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.good.secret": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	require.Equal(t, http.StatusOK, doIdentityRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext", "zx1.good.secret").Code)
	// 模拟撤销：Enforce 实时查库，fake 翻转结果即代表撤销即时生效
	enforcer.mu.Lock()
	enforcer.next = false
	enforcer.mu.Unlock()
	require.Equal(t, http.StatusForbidden, doIdentityRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext", "zx1.good.secret").Code)
}

// 凭据本身有效，但轮换后旧 token 失效 —— 用 verifier 的凭据集合变化模拟。
func TestExtensionIdentityRotationInvalidatesOldToken(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{credentials: map[string]bool{"io.zerone.ext|zx1.v1.old": true}}
	r := newAuthzTestRouter(enforcer, verifier)

	require.Equal(t, http.StatusOK, doIdentityRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext", "zx1.v1.old").Code)

	verifier.mu.Lock()
	verifier.credentials = map[string]bool{"io.zerone.ext|zx1.v2.new": true}
	verifier.mu.Unlock()
	require.Equal(t, http.StatusUnauthorized, doIdentityRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext", "zx1.v1.old").Code)
	require.Equal(t, http.StatusOK, doIdentityRequest(t, r, "/api/v1/admin/runs/1", "io.zerone.ext", "zx1.v2.new").Code)
}

// 身份校验本身出错（数据库不可用）→ 503 且 fail-closed（不写身份、不进入判定）。
func TestExtensionIdentityVerifierErrorFailsClosed(t *testing.T) {
	enforcer := &fakeEnforcer{next: true}
	verifier := &fakeVerifier{err: errors.New("db unavailable")}
	r := newAuthzTestRouter(enforcer, verifier)

	w := doIdentityRequest(t, r, "/api/v1/admin/runs/42", "io.zerone.ext", "zx1.good.secret")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Empty(t, enforcer.snapshot())
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

	// 无扩展身份请求不受限（既有请求零影响）
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { tenant.SetTenantID(c, "tenant-a"); c.Next() })
	r2.GET("/x", ExtensionRateLimit(ExtensionRateLimitConfig{RequestsPerMinute: 1}), func(c *gin.Context) { c.Status(http.StatusOK) })
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}

// 限流维度取"已认证扩展名"：自报的请求头不再参与分桶，
// 否则换一个名字就能重置窗口。这里直接验证 header 变化不影响同一身份的分桶。
func TestExtensionRateLimitKeysByAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { tenant.SetTenantID(c, "tenant-a"); c.Next() })
	r.GET("/x", func(c *gin.Context) { c.Set(ExtensionIdentityContextKey, "io.zerone.a"); c.Next() },
		ExtensionRateLimit(ExtensionRateLimitConfig{RequestsPerMinute: 1}),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	statuses := []int{}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		// 每次都换一个自报名字：不得重置计数窗口
		req.Header.Set(ExtensionHeaderName, "io.zerone.rotating-"+string(rune('a'+i)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		statuses = append(statuses, w.Code)
	}
	require.Equal(t, []int{http.StatusOK, http.StatusTooManyRequests}, statuses,
		"轮换自报名字不得绕过限流：分桶必须基于已认证身份")
}
