package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/config"

	authdom "control-panel/internal/domain/auth"

	"github.com/stretchr/testify/require"
)

// TestClientForOrg_UsesOrgOwnCredentialsForGetUser is the regression for
// issue #236: casdoor's get-user filters isAdmin by the calling
// application's organization, so the Admin API call must use the org's
// OWN application credentials (tenant_oauth_clients), not the global
// (default-tenant) ones. Before the fix, a non-default tenant's org-admin
// got isAdmin=false → hub-admin synthesis never fired (and an existing
// local admin was revoked on next login).
func TestClientForOrg_UsesOrgOwnCredentialsForGetUser(t *testing.T) {
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"owner":"aigames","name":"huizhujiang","id":"u1","isAdmin":true}}`))
	}))
	defer srv.Close()

	require.NoError(t, InitCasdoor(&config.CasdoorConfig{
		Endpoint: srv.URL, ClientID: "global-id", ClientSecret: "global-secret",
		Organization: "zerone",
	}))
	setFakeLookup(t, map[string]*TenantClientCreds{
		"aigames": {ClientID: "cid-aigames", ClientSecret: "sec-aigames"},
	}, nil)

	c := ClientForOrg("aigames")
	require.NotNil(t, c)
	u, err := c.GetUserByUserId("u1")
	require.NoError(t, err)
	require.True(t, u.IsAdmin, "fake casdoor reports isAdmin=true; SDK must parse it")
	// The regression assertion: basic-auth user is the ORG's client id,
	// never the global (default-tenant) one.
	require.Equal(t, "cid-aigames", gotUser)
	require.Equal(t, "sec-aigames", gotPass)
}

// TestClientForOrg_CredentialRotationTakesEffect: per-org clients are
// built fresh (not cached), so rotating an org's client secret in
// tenant_oauth_clients takes effect on the very next call.
func TestClientForOrg_CredentialRotationTakesEffect(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"owner":"acme","name":"u","id":"u1"}}`))
	}))
	defer srv.Close()

	require.NoError(t, InitCasdoor(&config.CasdoorConfig{
		Endpoint: srv.URL, ClientID: "global-id", ClientSecret: "global-secret",
		Organization: "zerone",
	}))

	setFakeLookup(t, map[string]*TenantClientCreds{
		"acme": {ClientID: "cid-old", ClientSecret: "sec-old"},
	}, nil)
	_, err := ClientForOrg("acme").GetUserByUserId("u1")
	require.NoError(t, err)
	require.Equal(t, "cid-old", gotUser)

	// Simulate ops rotating the org's credentials.
	setFakeLookup(t, map[string]*TenantClientCreds{
		"acme": {ClientID: "cid-new", ClientSecret: "sec-new"},
	}, nil)
	_, err = ClientForOrg("acme").GetUserByUserId("u1")
	require.NoError(t, err)
	require.Equal(t, "cid-new", gotUser, "rotated credentials must take effect immediately")
}

// TestClientForOrg_FallbackToGlobal: unregistered orgs (or no lookup
// injected) keep the pre-#236 behavior — global credentials, cached.
func TestClientForOrg_FallbackToGlobal(t *testing.T) {
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, _, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"owner":"unknown","name":"u","id":"u1"}}`))
	}))
	defer srv.Close()

	require.NoError(t, InitCasdoor(&config.CasdoorConfig{
		Endpoint: srv.URL, ClientID: "global-id", ClientSecret: "global-secret",
		Organization: "zerone",
	}))
	setFakeLookup(t, map[string]*TenantClientCreds{
		"aigames": {ClientID: "cid-aigames", ClientSecret: "sec-aigames"},
	}, nil)

	_, err := ClientForOrg("unregistered").GetUserByUserId("u1")
	require.NoError(t, err)
	require.Equal(t, "global-id", gotUser, "unregistered org falls back to global credentials")

	// No lookup injected at all (embedders/tests that skip wiring).
	SetTenantClientLookup(nil)
	_, err = ClientForOrg("whatever").GetUserByUserId("u1")
	require.NoError(t, err)
	require.Equal(t, "global-id", gotUser)
}

// TestClientForOrg_EmptyOrgReturnsGlobalClient: org == "" keeps returning
// the global client instance (single-organization behavior unchanged).
func TestClientForOrg_EmptyOrgReturnsGlobalClient(t *testing.T) {
	initTestCasdoor(t)
	require.Same(t, client, ClientForOrg(""))
}

// TestClientForOrg_PerOrgCertUsedWhenPresent: unlike perOrgJWTClient, a
// missing CertPEM must NOT block the Admin API path (basic auth needs no
// cert) — the global certificate is used instead.
func TestClientForOrg_PerOrgCertUsedWhenPresent(t *testing.T) {
	initTestCasdoor(t) // global cert: "global-cert"
	setFakeLookup(t, map[string]*TenantClientCreds{
		"withcert": {ClientID: "a", ClientSecret: "b", CertPEM: "org-cert"},
		"nocert":   {ClientID: "c", ClientSecret: "d"},
	}, nil)

	require.Equal(t, "org-cert", ClientForOrg("withcert").Certificate)
	require.Equal(t, "global-cert", ClientForOrg("nocert").Certificate,
		"missing CertPEM falls back to the global cert (get-user needs no cert)")
}

// TestClientForOrg_GetUserSynthesisEndToEnd wires the real path that was
// broken in production: SyncMembership for a non-default-tenant org admin
// must resolve isAdmin=true from the org's own credentials.
func TestClientForOrg_GetUserSynthesisEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","data":{"owner":"aigames","name":"huizhujiang","id":"u1","isAdmin":true,"isForbidden":false}}`))
	}))
	defer srv.Close()

	require.NoError(t, InitCasdoor(&config.CasdoorConfig{
		Endpoint: srv.URL, ClientID: "global-id", ClientSecret: "global-secret",
		Organization: "zerone",
	}))
	setFakeLookup(t, map[string]*TenantClientCreds{
		"aigames": {ClientID: "cid-aigames", ClientSecret: "sec-aigames"},
	}, nil)

	fresh, err := defaultFetchUser("u1", "aigames")
	require.NoError(t, err)
	require.True(t, fresh.IsAdmin, "org admin must resolve isAdmin=true via the org's own credentials")

	d := SynthesizeMembership(fresh.IsAdmin, nil, false)
	require.Equal(t, authdom.RoleAdmin, d.Role)
	require.Equal(t, authdom.StatusActive, d.Status)
	require.Equal(t, OpCreate, d.Op)
}
