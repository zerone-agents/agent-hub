package auth

import (
	"crypto/sha256"
	"encoding/base64"
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
