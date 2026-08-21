package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/auth"
	"control-panel/internal/config"

	"github.com/gin-gonic/gin"
)

func newLoginTestRouter(t *testing.T, perOrg map[string]*auth.TenantClientCreds, def *auth.TenantClientCreds) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := auth.InitCasdoor(&config.CasdoorConfig{
		Endpoint:     "https://casdoor.example",
		ClientID:     "global-id",
		ClientSecret: "global-secret",
		Certificate:  "global-cert",
	}); err != nil {
		t.Fatal(err)
	}
	auth.SetTenantClientLookup(func(org string) (*auth.TenantClientCreds, bool) {
		if org == "" {
			if def == nil {
				return nil, false
			}
			return def, true
		}
		c, ok := perOrg[org]
		return c, ok
	})
	t.Cleanup(func() { auth.SetTenantClientLookup(nil) })

	r := gin.New()
	r.GET("/auth/login", Login)
	return r
}

func TestLoginOrgRegistered(t *testing.T) {
	r := newLoginTestRouter(t, map[string]*auth.TenantClientCreds{
		"acme": {ClientID: "acme-id", ClientSecret: "acme-secret"},
	}, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/login?org=acme", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "client_id=acme-id") {
		t.Fatalf("Location missing acme client_id: %s", loc)
	}
}

func TestLoginOrgNotRegistered(t *testing.T) {
	r := newLoginTestRouter(t, map[string]*auth.TenantClientCreds{
		"acme": {ClientID: "acme-id", ClientSecret: "acme-secret"},
	}, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/login?org=ghost", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "组织未注册或不存在") {
		t.Fatalf("body missing unified message: %s", w.Body.String())
	}
}

func TestLoginNoOrgFallsBackToGlobal(t *testing.T) {
	r := newLoginTestRouter(t, map[string]*auth.TenantClientCreds{
		"acme": {ClientID: "acme-id", ClientSecret: "acme-secret"},
	}, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "client_id=global-id") {
		t.Fatalf("Location missing global client_id: %s", loc)
	}
}
