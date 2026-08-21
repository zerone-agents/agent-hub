package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/auth"
	"control-panel/internal/config"
	authdom "control-panel/internal/domain/auth"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

// ---- OrgCheck / CasdoorMode（Task 4：组织预检端点 + /auth/mode multiOrg）----

func setupOrgCheckRouter(t *testing.T, seed ...string) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.TenantOAuthClient{}))
	for i, org := range seed {
		row := authdom.TenantOAuthClient{Org: org, ClientID: "cid-" + org, ClientSecretEnc: "enc"}
		if i == 0 {
			dk := "default"
			row.DefaultKey = &dk // 首行设为 default，符合 repo 语义
		}
		require.NoError(t, db.Create(&row).Error)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewOrgCheckHandler(repository.NewTenantOAuthClientRepository())
	r.GET("/auth/org-check", h.OrgCheck)
	r.GET("/auth/mode", h.CasdoorMode)
	return r
}

func TestOrgCheckMissingParam(t *testing.T) {
	r := setupOrgCheckRouter(t, "acme")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/org-check", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestOrgCheckRegistered(t *testing.T) {
	r := setupOrgCheckRouter(t, "acme")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/org-check?org=acme", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"exists":true`)
}

func TestOrgCheckNotRegistered(t *testing.T) {
	r := setupOrgCheckRouter(t, "acme")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/org-check?org=ghost", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "组织不存在或未注册")
}

func TestCasdoorModeMultiOrg(t *testing.T) {
	r := setupOrgCheckRouter(t) // 空表
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/auth/mode", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"multiOrg":false`)

	r2 := setupOrgCheckRouter(t, "acme")
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/auth/mode", nil))
	require.Equal(t, http.StatusOK, w2.Code)
	require.Contains(t, w2.Body.String(), `"multiOrg":true`)
}
