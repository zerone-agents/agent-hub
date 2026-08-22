package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"control-panel/internal/config"
)

func setFakeLookup(t *testing.T, perOrg map[string]*TenantClientCreds, def *TenantClientCreds) {
	t.Helper()
	SetTenantClientLookup(func(org string) (*TenantClientCreds, bool) {
		if org == "" {
			if def == nil {
				return nil, false
			}
			return def, true
		}
		c, ok := perOrg[org]
		return c, ok
	})
	t.Cleanup(func() { SetTenantClientLookup(nil) })
}

func initTestCasdoor(t *testing.T) {
	t.Helper()
	if err := InitCasdoor(&config.CasdoorConfig{
		Endpoint:     "https://casdoor.example",
		ClientID:     "global-id",
		ClientSecret: "global-secret",
		Certificate:  "global-cert",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveClientCreds(t *testing.T) {
	initTestCasdoor(t)
	acme := &TenantClientCreds{ClientID: "acme-id", ClientSecret: "acme-secret", CertPEM: "acme-cert"}
	setFakeLookup(t, map[string]*TenantClientCreds{"acme": acme}, nil)

	// org 已注册 → 返回该 org 凭证
	got, err := resolveClientCreds("acme")
	if err != nil || got != acme {
		t.Fatalf("acme: got %v, %v", got, err)
	}

	// org 未注册 → 错误（绝不回落全局）
	if _, err := resolveClientCreds("ghost"); err == nil {
		t.Fatal("ghost should error")
	}

	// org 空 且无 default → 全局 env client（兼容存量单组织部署）
	got, err = resolveClientCreds("")
	if err != nil || got.ClientID != "global-id" || got.ClientSecret != "global-secret" {
		t.Fatalf("empty org fallback: got %v, %v", got, err)
	}
}

func TestResolveClientCredsDefaultRow(t *testing.T) {
	initTestCasdoor(t)
	def := &TenantClientCreds{ClientID: "def-id", ClientSecret: "def-secret"}
	setFakeLookup(t, nil, def)

	got, err := resolveClientCreds("")
	if err != nil || got != def {
		t.Fatalf("default row: got %v, %v", got, err)
	}
}

func TestPerOrgJWTClient(t *testing.T) {
	initTestCasdoor(t)
	acme := &TenantClientCreds{ClientID: "acme-id", ClientSecret: "acme-secret", CertPEM: "acme-cert"}
	setFakeLookup(t, map[string]*TenantClientCreds{"acme": acme}, nil)

	c := perOrgJWTClient("acme")
	if c.Certificate != "acme-cert" {
		t.Fatalf("acme cert override: got %q", c.Certificate)
	}
	if c.Certificate == client.Certificate {
		t.Fatal("per-org client cert should differ from global")
	}

	// 无记录 / CertPEM 空 → 全局证书
	if got := perOrgJWTClient("unknown"); got != client {
		t.Fatal("unknown org should fall back to global client")
	}
	setFakeLookup(t, map[string]*TenantClientCreds{"nocert": {ClientID: "x", ClientSecret: "y"}}, nil)
	if got := perOrgJWTClient("nocert"); got != client {
		t.Fatal("empty CertPEM should fall back to global client")
	}
}

func TestTokenOwner(t *testing.T) {
	// header.payload.sig，payload = {"owner":"acme",...} 的 base64url
	enc := base64.RawURLEncoding.EncodeToString([]byte(`{"owner":"acme","name":"alice"}`))
	tok := "h." + enc + ".s"
	if got := tokenOwner(tok); got != "acme" {
		t.Fatalf("tokenOwner: got %q", got)
	}
	if got := tokenOwner("garbage"); got != "" {
		t.Fatalf("garbage: got %q", got)
	}
}

func TestOAuthSessionStoresOrg(t *testing.T) {
	initTestCasdoor(t)
	setFakeLookup(t, map[string]*TenantClientCreds{"acme": {ClientID: "acme-id"}}, nil)

	url, err := GetLoginURL("acme", "st1", "cv1")
	if err != nil {
		t.Fatal(err)
	}
	if want := "client_id=acme-id"; !strings.Contains(url, want) {
		t.Fatalf("login URL missing acme client_id: %s", url)
	}
	s := GetSession("st1")
	if s == nil || s.Org != "acme" {
		t.Fatalf("session org not stored: %+v", s)
	}
}
