package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"control-panel/internal/config"
)

// fakeJWT 构造一个不签名的 JWT（仅用于让 tokenOwner 读到 owner）。
func fakeJWT(owner string) string {
	payload, _ := json.Marshal(map[string]string{"owner": owner})
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}

// newRefreshTestServer 启动假的 casdoor token 端点，记录收到的 client_id。
// x/oauth2 TokenSource 迁移后请求为标准 form 编码（原实现是 JSON body）。
func newRefreshTestServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var gotClientIDs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		mu.Lock()
		gotClientIDs = append(gotClientIDs, r.PostForm.Get("client_id"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"expires_in":    7200,
			"token_type":    "Bearer",
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &gotClientIDs
}

func setupRefreshAuth(t *testing.T, endpoint string, lookup func(string) (*TenantClientCreds, bool)) {
	t.Helper()
	if err := InitCasdoor(&config.CasdoorConfig{
		Endpoint:     endpoint,
		ClientID:     "global-id",
		ClientSecret: "global-secret",
		Certificate:  "global-cert",
	}); err != nil {
		t.Fatal(err)
	}
	SetTenantClientLookup(lookup)
	t.Cleanup(func() { SetTenantClientLookup(nil) })
}

func TestRefreshAccessTokenUsesOwnerOrgCreds(t *testing.T) {
	srv, gotIDs := newRefreshTestServer(t)
	var lookupOrgs []string
	setupRefreshAuth(t, srv.URL, func(org string) (*TenantClientCreds, bool) {
		if org == "acme" {
			lookupOrgs = append(lookupOrgs, org)
			return &TenantClientCreds{ClientID: "acme-id", ClientSecret: "acme-secret"}, true
		}
		return nil, false
	})

	resp, err := RefreshAccessToken(fakeJWT("acme"))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if resp.AccessToken != "new-access" {
		t.Fatalf("access token = %q", resp.AccessToken)
	}
	if len(*gotIDs) != 1 || (*gotIDs)[0] != "acme-id" {
		t.Fatalf("client_id used = %v, want [acme-id] (lookup orgs: %v)", *gotIDs, lookupOrgs)
	}
	if len(lookupOrgs) != 1 || lookupOrgs[0] != "acme" {
		t.Fatalf("lookup called with %v, want [acme]", lookupOrgs)
	}
}

func TestRefreshAccessTokenFallbackToGlobalOnParseFailure(t *testing.T) {
	srv, gotIDs := newRefreshTestServer(t)
	setupRefreshAuth(t, srv.URL, func(org string) (*TenantClientCreds, bool) {
		return nil, false
	})

	// 非 JWT 结构 → tokenOwner 解析失败 → owner="" → 回退全局凭证。
	if _, err := RefreshAccessToken("not-a-jwt"); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(*gotIDs) != 1 || (*gotIDs)[0] != "global-id" {
		t.Fatalf("client_id used = %v, want [global-id]", *gotIDs)
	}
}

func TestRefreshAccessTokenOwnerNotRegisteredFallsBack(t *testing.T) {
	srv, gotIDs := newRefreshTestServer(t)
	setupRefreshAuth(t, srv.URL, func(org string) (*TenantClientCreds, bool) {
		return nil, false // 任何 org（含 default）都未命中
	})

	// owner=ghost 未注册 → 回退全局，而不是报错（refresh 语义：尽力恢复会话）。
	if _, err := RefreshAccessToken(fakeJWT("ghost")); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(*gotIDs) != 1 || (*gotIDs)[0] != "global-id" {
		t.Fatalf("client_id used = %v, want [global-id]", *gotIDs)
	}
}
