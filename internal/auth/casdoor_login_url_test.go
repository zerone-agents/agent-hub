package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"control-panel/internal/config"
)

// setupLoginURLAuth 初始化 casdoor（含 CallbackURL）并注入租户凭证查询，
// 供 GenerateLoginURL 测试使用。与 setupRefreshAuth 同模式，t.Cleanup
// 恢复全局状态。
func setupLoginURLAuth(t *testing.T) {
	t.Helper()
	if err := InitCasdoor(&config.CasdoorConfig{
		Endpoint:     "https://casdoor.example.com",
		ClientID:     "global-id",
		ClientSecret: "global-secret",
		CallbackURL:  "https://console.zerone.life/auth/callback",
	}); err != nil {
		t.Fatal(err)
	}
	SetTenantClientLookup(func(org string) (*TenantClientCreds, bool) {
		if org == "acme" {
			return &TenantClientCreds{ClientID: "acme-id", ClientSecret: "acme-secret"}, true
		}
		return nil, false
	})
	t.Cleanup(func() { SetTenantClientLookup(nil) })
}

func TestGenerateLoginURL(t *testing.T) {
	setupLoginURLAuth(t)

	raw, err := GenerateLoginURL("acme")
	if err != nil {
		t.Fatalf("GenerateLoginURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url %q: %v", raw, err)
	}
	if u.Host != "casdoor.example.com" || u.Path != "/login/oauth/authorize" {
		t.Fatalf("unexpected authorize url: %s", raw)
	}
	q := u.Query()

	// 授权链接必须携带本组织解析出的 client_id（而非全局 id）。
	if q.Get("client_id") != "acme-id" {
		t.Fatalf("client_id = %q, want acme-id", q.Get("client_id"))
	}
	if q.Get("response_type") != "code" || q.Get("scope") != "read" {
		t.Fatalf("response_type/scope = %q/%q", q.Get("response_type"), q.Get("scope"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatalf("PKCE params missing: method=%q challenge=%q", q.Get("code_challenge_method"), q.Get("code_challenge"))
	}
	if q.Get("redirect_uri") != "https://console.zerone.life/auth/callback" {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	state := q.Get("state")
	if state == "" {
		t.Fatal("state is empty")
	}

	// session 已登记：回调可通过 state 取回 verifier/org；verifier 的 S256
	// challenge 与链接中的 code_challenge 一致（PKCE 闭环可兑换）。
	sess := GetSession(state)
	if sess == nil {
		t.Fatal("session not stored for state")
	}
	if sess.Org != "acme" || sess.CodeVerifier == "" {
		t.Fatalf("session = %+v", sess)
	}
	sum := sha256.Sum256([]byte(sess.CodeVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != q.Get("code_challenge") {
		t.Fatalf("verifier challenge mismatch: %q != %q", q.Get("code_challenge"), challenge)
	}
}

func TestGenerateLoginURLOrgNotRegistered(t *testing.T) {
	setupLoginURLAuth(t)

	_, err := GenerateLoginURL("ghost-org")
	if err == nil || !strings.Contains(err.Error(), "组织未注册") {
		t.Fatalf("err = %v, want 组织未注册", err)
	}
}

// TestGenerateLoginURLSemanticsMatchLoginFlow 确保登录页流（Login handler）
// 与 GenerateLoginURL 产出同一形态的授权链接：org/state/verifier 各自生成
// 后走同一条 GetLoginURL 组装路径。
func TestGenerateLoginURLSemanticsMatchLoginFlow(t *testing.T) {
	setupLoginURLAuth(t)

	state, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	loginURL, err := GetLoginURL("acme", state, verifier)
	if err != nil {
		t.Fatalf("GetLoginURL: %v", err)
	}
	u, err := url.Parse(loginURL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Path != "/login/oauth/authorize" {
		t.Fatalf("path = %q", u.Path)
	}
}
