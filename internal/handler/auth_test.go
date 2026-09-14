package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/auth"
	"control-panel/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCallback_TokenExchangeErrorNeutralMessage(t *testing.T) {
	// GetLoginURL / ExchangeCodeForToken 依赖全局 casdoor 配置：指向不可达
	// 本地端口，使 token 交换确定性失败（connection refused）。
	require.NoError(t, auth.InitCasdoor(&config.CasdoorConfig{
		Endpoint: "http://127.0.0.1:1", ClientID: "cid", ClientSecret: "sec",
		Organization: "orga",
	}))
	auth.SetTenantClientLookup(func(org string) (*auth.TenantClientCreds, bool) {
		return &auth.TenantClientCreds{ClientID: "cid", ClientSecret: "sec"}, true
	})
	t.Cleanup(func() { auth.SetTenantClientLookup(nil) })

	state, err := auth.GenerateState()
	require.NoError(t, err)
	verifier, err := auth.GenerateCodeVerifier()
	require.NoError(t, err)
	_, err = auth.GetLoginURL("orga", state, verifier, "") // 副作用：存 session
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/callback", Callback(nil)) // 失败路径不触达 provider，nil 安全

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/callback?code=x&state="+state, nil))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "登录回调处理失败")
	// 原始错误细节（endpoint / exchange 失败原文）不得外泄给客户端。
	require.NotContains(t, w.Body.String(), "127.0.0.1")
	require.NotContains(t, w.Body.String(), "exchange")
}

// TestBuildCallbackRedirect callback 落地 URL 构造：query 合并、hash 置尾、
// token 始终可被 URLSearchParams 提取（review 第 3 项）。
func TestBuildCallbackRedirect(t *testing.T) {
	cases := []struct{ name, redirect, want string }{
		{"默认根路径", "/", "/static/?token=t1"},
		{"聊天首页", "/agents/chat", "/static/agents/chat?token=t1"},
		{"带 query", "/agents/chat?x=1", "/static/agents/chat?token=t1&x=1"},
		{"带 query 与 hash", "/agents/chat?x=1#f", "/static/agents/chat?token=t1&x=1#f"},
		{"仅 hash", "/agents/chat#f", "/static/agents/chat?token=t1#f"},
		{"剥离 redirect 自带的认证参数", "/agents/chat?refreshToken=evil&token=evil", "/static/agents/chat?token=t1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildCallbackRedirect(tc.redirect, "t1", "")
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
	// refreshToken 存在时追加
	if got := buildCallbackRedirect("/", "t1", "r1"); got != "/static/?refreshToken=r1&token=t1" && got != "/static/?token=t1&refreshToken=r1" {
		t.Fatalf("refreshToken missing: %q", got)
	}
}

func TestRefreshToken_NeutralErrorMessage(t *testing.T) {
	// 指向不可达端口 → 确定性失败；响应不得外泄内部细节（原实现拼接 err.Error()，RED）
	require.NoError(t, auth.InitCasdoor(&config.CasdoorConfig{
		Endpoint: "http://127.0.0.1:1", ClientID: "cid", ClientSecret: "sec", Organization: "orga",
	}))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/auth/refresh", RefreshToken)

	w := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"refresh_token":"fake-header.eyJvd25lciI6Im9yZ2EifQ.fake"}`)
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/refresh", body))

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "刷新令牌无效")
	require.NotContains(t, w.Body.String(), "127.0.0.1")
	require.NotContains(t, w.Body.String(), "refresh token:")
	require.NotContains(t, w.Body.String(), "failed to refresh")
}
