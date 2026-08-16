package jwtutil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPendingApprovalGuard 表驱动覆盖待审批拦截中间件的四类行为：
//  1. 白名单路径（/auth/userinfo、/auth/logout、/health*）+ 空 roles + casdoor → 放行
//  2. 非白名单 + 空 roles + auth_method ∈ {casdoor, cli} → 403 + Abort
//  3. 空 roles + auth_method=builtin → 放行（builtin 无待审批概念，防御性不拦）
//  4. 非空 roles → 放行
func TestPendingApprovalGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		path        string
		roles       []string
		authMethod  string
		setKeys     bool // 是否注入 context keys（模拟 AuthMiddlewareWithCLI 已执行）
		wantStatus  int
		wantBlocked bool
	}{
		// ---- 白名单路径 + 空 roles + casdoor → 放行 ----
		{name: "白名单 auth/userinfo 放行", path: "/auth/userinfo", roles: []string{}, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusOK},
		{name: "白名单 auth/logout 放行（roles 为 nil）", path: "/auth/logout", roles: nil, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusOK},
		{name: "白名单 /health 放行", path: "/health", roles: []string{}, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusOK},
		{name: "白名单 /health/:service 放行", path: "/health/mysql", roles: []string{}, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusOK},

		// ---- 非白名单 + 空 roles + casdoor → 403 ----
		{name: "casdoor 空 roles 访问业务 API 拦截", path: "/api/v1/agents", roles: []string{}, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusForbidden, wantBlocked: true},
		{name: "casdoor 空 roles（nil）访问管理 API 拦截", path: "/api/v1/admin/users", roles: nil, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusForbidden, wantBlocked: true},

		// ---- 非白名单 + 空 roles + cli → 403（casdoor 用户的 cli token 同样拦截）----
		{name: "cli 空 roles 访问业务 API 拦截", path: "/api/v1/agents", roles: []string{}, authMethod: "cli", setKeys: true, wantStatus: http.StatusForbidden, wantBlocked: true},

		// ---- builtin → 放行（无待审批概念）----
		{name: "builtin 空 roles 放行", path: "/api/v1/agents", roles: []string{}, authMethod: "builtin", setKeys: true, wantStatus: http.StatusOK},

		// ---- 非空 roles → 放行 ----
		{name: "casdoor member 放行", path: "/api/v1/agents", roles: []string{"member"}, authMethod: "casdoor", setKeys: true, wantStatus: http.StatusOK},
		{name: "cli 有角色放行", path: "/api/v1/agents", roles: []string{"maintainer"}, authMethod: "cli", setKeys: true, wantStatus: http.StatusOK},

		// ---- 防御：auth_method 未设置（guard 被误挂到未鉴权路由）→ 放行 ----
		{name: "auth_method 未设置放行", path: "/api/v1/agents", setKeys: false, wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			// 前置 handler 注入 AuthMiddlewareWithCLI 产生的 context keys，
			// 随后是 guard，最后是业务 handler（被拦截时不应执行）。
			pre := func(c *gin.Context) {
				if tc.setKeys {
					c.Set("roles", tc.roles)
					c.Set("auth_method", tc.authMethod)
				}
			}
			business := func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": "business-ok"})
			}
			// 同时注册 GET/POST，与生产中 logout 为 POST 无关，这里只测路径匹配逻辑。
			r.GET(tc.path, pre, PendingApprovalGuard(), business)
			r.POST(tc.path, pre, PendingApprovalGuard(), business)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())

			if tc.wantBlocked {
				// 错误体形状与 handler.respondError 一致：{"success":false,"error":"..."}，
				// error 为字符串且以 PENDING_APPROVAL 开头，前端据此字符串匹配。
				var resp struct {
					Success bool   `json:"success"`
					Error   string `json:"error"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				assert.False(t, resp.Success)
				assert.True(t, strings.HasPrefix(resp.Error, "PENDING_APPROVAL"),
					"error 应以 PENDING_APPROVAL 开头供前端识别，got: %q", resp.Error)
				// 业务 handler 不应执行
				assert.NotContains(t, w.Body.String(), "business-ok")
			} else {
				assert.Contains(t, w.Body.String(), "business-ok", "业务 handler 应正常执行")
			}
		})
	}
}

// pendingFakeProvider 是 mode 可配置的 auth.Provider，用于端到端验证
// 真实 AuthMiddlewareWithCLI 注入的 context keys 能被 guard 正确消费。
type pendingFakeProvider struct {
	user *auth.AuthUser
	mode string
}

func (f *pendingFakeProvider) ValidateAccessToken(string) (*auth.AuthUser, error) { return f.user, nil }
func (f *pendingFakeProvider) RefreshToken(string) (*auth.TokenPair, error) {
	return nil, errors.New("x")
}
func (f *pendingFakeProvider) RevokeToken(string) error                      { return nil }
func (f *pendingFakeProvider) GetUserIdentity(string) (*auth.AuthUser, bool) { return f.user, true }
func (f *pendingFakeProvider) Mode() string                                  { return f.mode }

// newPendingRequestWithBearer 构造带 Authorization 头、指向指定路径的请求。
// （jwt_test.go 的 newRequestWithBearer 硬编码路径 "/"，不适用于本文件的 /api/v1 路由。）
func newPendingRequestWithBearer(path, token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// TestPendingApprovalGuardEndToEnd 用真实 AuthMiddlewareWithCLI + guard 串链验证：
// casdoor 空角色用户被 403 拦截、builtin 用户零感知放行。
func TestPendingApprovalGuardEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("casdoor 空角色用户被拦截", func(t *testing.T) {
		p := &pendingFakeProvider{
			user: &auth.AuthUser{ID: "u1", Username: "pending-user", Roles: []string{}},
			mode: "casdoor",
		}
		r := gin.New()
		r.GET("/api/v1/agents", AuthMiddlewareWithCLI(nil, p), PendingApprovalGuard(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true})
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, newPendingRequestWithBearer("/api/v1/agents", "any-jwt"))
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "PENDING_APPROVAL")
	})

	t.Run("builtin 用户零感知放行", func(t *testing.T) {
		p := &pendingFakeProvider{
			user: &auth.AuthUser{ID: "u2", Username: "builtin-user", Roles: []string{"member"}},
			mode: "builtin",
		}
		r := gin.New()
		r.GET("/api/v1/agents", AuthMiddlewareWithCLI(nil, p), PendingApprovalGuard(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"success": true})
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, newPendingRequestWithBearer("/api/v1/agents", "any-jwt"))
		require.Equal(t, http.StatusOK, w.Code)
	})
}
