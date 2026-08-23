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
	_, err = auth.GetLoginURL("orga", state, verifier) // 副作用：存 session
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
	require.NotContains(t, w.Body.String(), "127.0.0.1")
	require.NotContains(t, w.Body.String(), "refresh token:")
	require.NotContains(t, w.Body.String(), "failed to refresh")
}
