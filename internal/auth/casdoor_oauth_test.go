package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"control-panel/internal/config"

	"github.com/stretchr/testify/require"
)

func setupOAuthTest(t *testing.T, endpoint string) {
	t.Helper()
	require.NoError(t, InitCasdoor(&config.CasdoorConfig{
		Endpoint: endpoint, ClientID: "cid-global", ClientSecret: "sec-global",
		Organization: "orga", CallbackURL: "http://hub.example/auth/callback",
	}))
	SetTenantClientLookup(func(org string) (*TenantClientCreds, bool) {
		return &TenantClientCreds{ClientID: "cid-org", ClientSecret: "sec-org"}, true
	})
	t.Cleanup(func() { SetTenantClientLookup(nil) })
}

func TestGetLoginURL_Params(t *testing.T) {
	setupOAuthTest(t, "http://casdoor.example")

	state, err := GenerateState()
	require.NoError(t, err)
	verifier, err := GenerateCodeVerifier()
	require.NoError(t, err)
	raw, err := GetLoginURL("orga", state, verifier)
	require.NoError(t, err)

	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "/login/oauth/authorize", u.Path)
	q := u.Query()
	require.Equal(t, "cid-org", q.Get("client_id")) // per-org 凭证
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "http://hub.example/auth/callback", q.Get("redirect_uri"))
	require.Equal(t, "read", q.Get("scope"))
	require.Equal(t, state, q.Get("state"))
	// code_challenge 与既有 S256 算法等价：sha256 + base64 RawURL（无填充）
	sum := sha256.Sum256([]byte(verifier))
	require.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), q.Get("code_challenge"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))

	// 副作用：session 已存（迁移不得丢失）
	require.NotNil(t, GetSession(state))
}

func TestExchangeCodeForToken_FormParams(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/login/oauth/access_token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"at-1","refresh_token":"rt-1","token_type":"Bearer","expires_in":3600}`)
	}))
	t.Cleanup(srv.Close)
	setupOAuthTest(t, srv.URL)

	tok, err := ExchangeCodeForToken("orga", "code-1", "verifier-1")
	require.NoError(t, err)
	require.Equal(t, "at-1", tok.AccessToken)
	require.Equal(t, "rt-1", tok.RefreshToken)
	require.Equal(t, "Bearer", tok.TokenType)
	require.InDelta(t, 3600, tok.ExpiresIn, 5) // 从 Expiry 折算，容许执行耗时

	// x/oauth2 标准 form 编码（原实现是 JSON body——本组断言构成 RED）
	require.Equal(t, "authorization_code", gotForm.Get("grant_type"))
	require.Equal(t, "code-1", gotForm.Get("code"))
	require.Equal(t, "cid-org", gotForm.Get("client_id"))
	require.Equal(t, "sec-org", gotForm.Get("client_secret"))
	require.Equal(t, "verifier-1", gotForm.Get("code_verifier"))
	require.Empty(t, gotForm.Get("redirect_uri")) // SDK 先例：兑换不发
}

func TestExchangeCodeForToken_CasdoorErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"invalid_grant","error_description":"code expired"}`)
	}))
	t.Cleanup(srv.Close)
	setupOAuthTest(t, srv.URL)

	_, err := ExchangeCodeForToken("orga", "bad-code", "v")
	require.Error(t, err)
	// 结构化错误：Casdoor 的 error 与 error_description 都要可见（原实现只透出 error 字段）
	require.Contains(t, err.Error(), "invalid_grant")
	require.Contains(t, err.Error(), "code expired")
}

func TestExchangeCodeForToken_ErrorPrefixFakeSuccess(t *testing.T) {
	// Casdoor 已知怪癖：200 + access_token 字面 "error: xxx" 的伪成功
	// （SDK auth.go:104 有同款守卫，原实现缺失——本测试构成 RED）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"error: the code is invalid"}`)
	}))
	t.Cleanup(srv.Close)
	setupOAuthTest(t, srv.URL)

	tok, err := ExchangeCodeForToken("orga", "c", "v")
	require.Error(t, err)
	require.Nil(t, tok)
	require.Contains(t, err.Error(), "the code is invalid")
}
