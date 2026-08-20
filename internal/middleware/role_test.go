package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequireRole 锁定 RequireRole 的多角色命中/拒绝语义：
// 用户角色命中 allowed 任一角色 → 放行（200）；否则 → 拒绝（403 + Abort）。
// 这是 member 只读权限（admin|maintainer|member 三参数集）的回归基线。
// 注：brief 最初的「CreateTestContext + IsAborted」写法在 TestMode 下不可行
// （CreateTestContext 不设置 c.Request，拒绝路径日志 c.Request.URL 会 nil panic），
// 按 brief 允许的替代方案改为完整 gin 链路断言：httptest router + 返回 200 的
// dummy handler，403/200 状态码即真实反映「Abort 与否」。
func TestRequireRole(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		roles    []string // context 中用户拥有的角色
		allowed  []string // RequireRole 允许的角色
		wantCode int
	}{
		{"member hits three-role read set", []string{"member"}, []string{"admin", "maintainer", "member"}, http.StatusOK},
		{"member misses two-role manager set", []string{"member"}, []string{"admin", "maintainer"}, http.StatusForbidden},
		{"maintainer hits manager set", []string{"maintainer"}, []string{"admin", "maintainer"}, http.StatusOK},
		{"no roles rejected", []string{}, []string{"admin", "maintainer", "member"}, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 完整链路：注入 roles 的中间件 → RequireRole → 兜底写 200 的 dummy handler
			router := gin.New()
			router.GET("/protected", func(c *gin.Context) {
				c.Set("roles", tc.roles)
				c.Next()
			}, RequireRole(tc.allowed...), func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			router.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("roles=%v allowed=%v: got status %d, want %d", tc.roles, tc.allowed, rr.Code, tc.wantCode)
			}
		})
	}
}
