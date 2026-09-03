package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/infrastructure/deployer"
)

// scopedRecorder captures every deployer request so tests can assert both
// deployer identities reach the wire correctly: tenant-scoped deploy keys in
// URL paths (lifecycle), and the bare/scoped split in create payloads
// (rootAgentId + deploymentKey, issue #114).
type scopedRecorder struct {
	mu         sync.Mutex
	paths      []string
	postBodies [][]byte
	getStatus  int // HTTP status for GET /api/v1/agents/<key>
}

func (rec *scopedRecorder) record(method, path string) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.paths = append(rec.paths, method+" "+path)
}

func (rec *scopedRecorder) hasEntry(entry string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, p := range rec.paths {
		if p == entry {
			return true
		}
	}
	return false
}

func (rec *scopedRecorder) hasPrefix(prefix string) bool {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, p := range rec.paths {
		if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// newScopedDeployerServer starts a mock deployer that records all requests.
// createStatus is the container status reported by the create response
// ("running" triggers the async gateway registration goroutine, anything else
// does not).
func newScopedDeployerServer(t *testing.T, rec *scopedRecorder, getFound bool, createStatus string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status") {
			// Real-time health endpoint.
			w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"running","health":"healthy","hostPort":3000}}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path != "/api/v1/agents" {
			if !getFound {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"success":false,"error":"agent not found"}`))
				return
			}
			w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"running","hostPort":3000}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents" {
			body, _ := io.ReadAll(r.Body)
			// Capability probe branch (issue #114): probe requests carry no
			// deploymentKey — mimic deployer v3.1.0's sentinel rejection so
			// the probe passes, and keep probes out of the create captures.
			var probe struct {
				DeploymentKey string `json:"deploymentKey"`
			}
			_ = json.Unmarshal(body, &probe)
			if probe.DeploymentKey == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"success":false,"error":"deploymentKey is required"}`))
				return
			}
			rec.mu.Lock()
			rec.postBodies = append(rec.postBodies, body)
			rec.mu.Unlock()
			w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"` + createStatus + `","hostPort":3000,"runtimeToken":"echoed"}}`))
			return
		}
		w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"running","health":"healthy","hostPort":3000}}`))
	}))
}

func (rec *scopedRecorder) createdAgentName(t *testing.T) string {
	t.Helper()
	names := rec.createdAgentNames(t)
	if len(names) == 0 {
		t.Fatal("expected deployer create (POST) to be called")
	}
	return names[0]
}

func (rec *scopedRecorder) createdAgentNames(t *testing.T) []string {
	t.Helper()
	payloads := rec.createdPayloads(t)
	names := make([]string, 0, len(payloads))
	for _, p := range payloads {
		names = append(names, p.RootAgentID)
	}
	return names
}

// scopedCreatePayload captures the two identities of a create request
// (issue #114): the bare rootAgentId (runtime agent id) and the
// tenant-scoped deploymentKey (deployer resource id).
type scopedCreatePayload struct {
	RootAgentID   string `json:"rootAgentId"`
	DeploymentKey string `json:"deploymentKey"`
}

func (rec *scopedRecorder) createdPayloads(t *testing.T) []scopedCreatePayload {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := make([]scopedCreatePayload, 0, len(rec.postBodies))
	for _, body := range rec.postBodies {
		var parsed scopedCreatePayload
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("unmarshal create body: %v", err)
		}
		out = append(out, parsed)
	}
	return out
}

// TestDeploy_PayloadSplitsBareRootAndScopedKey asserts the create payload
// carries the bare agent id as rootAgentId and the tenant-scoped key as the
// dedicated deploymentKey (issue #114), while the pre-create existence probe
// still addresses the scoped key (lifecycle stays resource-scoped).
func TestDeploy_PayloadSplitsBareRootAndScopedKey(t *testing.T) {
	rec := &scopedRecorder{}
	srv := newScopedDeployerServer(t, rec, false, "running")
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(&deployTokenFixture{}, ""), deployTokenProviderSvc())
	if _, err := s.Deploy("tenant-a", "general", false, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const wantKey = "tenant-a-general"
	if !rec.hasEntry("GET /api/v1/agents/" + wantKey) {
		t.Errorf("existence probe should hit /api/v1/agents/%s, got %v", wantKey, rec.paths)
	}
	payloads := rec.createdPayloads(t)
	if len(payloads) != 1 {
		t.Fatalf("expected exactly one create POST, got %d", len(payloads))
	}
	if payloads[0].RootAgentID != "general" {
		t.Errorf("create payload rootAgentId = %q, want bare %q", payloads[0].RootAgentID, "general")
	}
	if payloads[0].DeploymentKey != wantKey {
		t.Errorf("create payload deploymentKey = %q, want %q", payloads[0].DeploymentKey, wantKey)
	}
	if rec.hasEntry("GET /api/v1/agents/general") {
		t.Errorf("bare-name probe must not be used anymore, got %v", rec.paths)
	}
}

// TestDeploy_DualTenantSameName_DoNotInterfere deploys the same agent name in
// two tenants and asserts each deploy addresses only its own scoped key: the
// existence probe and create payload for zerone's "assistant" never touch
// ayu's container and vice versa.
func TestDeploy_DualTenantSameName_DoNotInterfere(t *testing.T) {
	rec := &scopedRecorder{}
	srv := newScopedDeployerServer(t, rec, false, "created")
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, dualTenantRepo(), deployTokenProviderSvc())
	if _, err := s.Deploy("zerone", "assistant", false, false); err != nil {
		t.Fatalf("deploy zerone/assistant: %v", err)
	}
	if _, err := s.Deploy("ayu", "assistant", false, false); err != nil {
		t.Fatalf("deploy ayu/assistant: %v", err)
	}

	if got := rec.createdAgentNames(t); len(got) != 2 || got[0] != "assistant" || got[1] != "assistant" {
		t.Errorf("create payload rootAgentIds = %v, want both bare %q", got, "assistant")
	}
	// Deployment keys stay tenant-scoped: same bare runtime identity, fully
	// isolated deployer resources (the core invariant of the v3.1 split).
	payloads := rec.createdPayloads(t)
	if payloads[0].DeploymentKey != "zerone-assistant" || payloads[1].DeploymentKey != "ayu-assistant" {
		t.Errorf("create payload deploymentKeys = [%s %s], want [zerone-assistant ayu-assistant]", payloads[0].DeploymentKey, payloads[1].DeploymentKey)
	}
	if !rec.hasEntry("GET /api/v1/agents/zerone-assistant") || !rec.hasEntry("GET /api/v1/agents/ayu-assistant") {
		t.Errorf("existence probes should hit both scoped keys, got %v", rec.paths)
	}
}

// TestDeploy_BlocksLegacyDeployerWithoutDeploymentKey asserts Deploy fails
// closed against a pre-v3.1 deployer (no deploymentKey concept): the probe's
// 400 fingerprint is the legacy "agents must contain..." error, exactly one
// POST happens (the probe itself — no create is attempted), and the error is
// the actionable sentinel.
func TestDeploy_BlocksLegacyDeployerWithoutDeploymentKey(t *testing.T) {
	var posts int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/agents" {
			mu.Lock()
			posts++
			mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"success":false,"error":"agents must contain at least the root agent definition"}`))
			return
		}
		w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"running","hostPort":3000}}`))
	}))
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(&deployTokenFixture{}, ""), deployTokenProviderSvc())
	_, err := s.Deploy("tenant-a", "general", false, false)
	if !errors.Is(err, ErrDeployerNoDeploymentKey) {
		t.Fatalf("Deploy error = %v, want ErrDeployerNoDeploymentKey", err)
	}
	if posts != 1 {
		t.Fatalf("expected exactly one POST (the capability probe), got %d", posts)
	}
}

// TestDeploy_ProbeTransportFailureFailsClosed asserts an unreachable deployer
// fails the deploy with the capability-check error (mapped 502 by the
// handler) instead of attempting a create.
func TestDeploy_ProbeTransportFailureFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens anymore

	s := newTestAgentDeployerService(t, url, deployTokenAgentRepo(&deployTokenFixture{}, ""), deployTokenProviderSvc())
	_, err := s.Deploy("tenant-a", "general", false, false)
	if err == nil || !strings.Contains(err.Error(), "deployer capability check failed") {
		t.Fatalf("Deploy error = %v, want capability check failure", err)
	}
}

// dualTenantRepo returns an agent repo that echoes the requested name (unlike
// deployTokenAgentRepo which hardcodes "general"), so same-name deploys across
// tenants exercise the real key-derivation path.
func dualTenantRepo() *mockAgentRepo {
	providerID := uint64(1)
	return &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID: 1, Name: name, ProviderID: &providerID, ModelID: "m",
			}, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error { return nil },
	}
}

// TestGetStatus_QueriesScopedKey asserts GetAgent/GetStatus health queries use
// the tenant-scoped key.
func TestGetStatus_QueriesScopedKey(t *testing.T) {
	rec := &scopedRecorder{}
	srv := newScopedDeployerServer(t, rec, true, "running")
	defer srv.Close()

	var persisted string
	s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())
	dto, err := s.GetStatus("tenant-a", "general")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	const wantKey = "tenant-a-general"
	if !rec.hasEntry("GET /api/v1/agents/" + wantKey) {
		t.Errorf("agent query should hit /api/v1/agents/%s, got %v", wantKey, rec.paths)
	}
	if !rec.hasEntry("GET /api/v1/agents/" + wantKey + "/status") {
		t.Errorf("health query should hit /api/v1/agents/%s/status, got %v", wantKey, rec.paths)
	}
	if rec.hasPrefix("GET /api/v1/agents/general") {
		t.Errorf("bare-name query must not be used anymore, got %v", rec.paths)
	}
	if dto.Status != "running" || dto.Health != "healthy" {
		t.Errorf("dto = %+v, want running/healthy", dto)
	}
}

// TestLifecycleCalls_UseScopedKey asserts Stop/Start/Delete address the
// deployer with the tenant-scoped key.
func TestLifecycleCalls_UseScopedKey(t *testing.T) {
	const wantKey = "tenant-a-general"

	t.Run("stop", func(t *testing.T) {
		rec := &scopedRecorder{}
		srv := newScopedDeployerServer(t, rec, true, "running")
		defer srv.Close()
		var persisted string
		s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())
		if err := s.Stop("tenant-a", "general"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rec.hasEntry("POST /api/v1/agents/" + wantKey + "/stop") {
			t.Errorf("stop should hit /api/v1/agents/%s/stop, got %v", wantKey, rec.paths)
		}
	})

	t.Run("start", func(t *testing.T) {
		rec := &scopedRecorder{}
		srv := newScopedDeployerServer(t, rec, true, "running")
		defer srv.Close()
		var persisted string
		// Empty RuntimeToken keeps the async gateway-registration goroutine off.
		s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())
		if _, err := s.Start("tenant-a", "general"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rec.hasEntry("POST /api/v1/agents/" + wantKey + "/restart") {
			t.Errorf("start should hit /api/v1/agents/%s/restart, got %v", wantKey, rec.paths)
		}
		if !rec.hasEntry("GET /api/v1/agents/" + wantKey) {
			t.Errorf("post-start status should hit /api/v1/agents/%s, got %v", wantKey, rec.paths)
		}
	})

	t.Run("delete", func(t *testing.T) {
		rec := &scopedRecorder{}
		srv := newScopedDeployerServer(t, rec, true, "running")
		defer srv.Close()
		var persisted string
		s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())
		if err := s.Delete("tenant-a", "general"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rec.hasEntry("DELETE /api/v1/agents/" + wantKey) {
			t.Errorf("delete should hit DELETE /api/v1/agents/%s, got %v", wantKey, rec.paths)
		}
	})
}

// TestGatewayHealthCache_ScopedByTenant asserts same-name agents in different
// tenants get independent gateway-health cache entries.
func TestGatewayHealthCache_ScopedByTenant(t *testing.T) {
	s := &AgentDeployerService{gatewayHealth: &sync.Map{}}
	zeroneKey := DeployKey("zerone", "assistant")
	ayuKey := DeployKey("ayu", "assistant")
	if zeroneKey == ayuKey {
		t.Fatalf("keys should differ: %q", zeroneKey)
	}
	s.storeGatewayHealth(zeroneKey, true)
	s.storeGatewayHealth(ayuKey, false)

	if got := s.gatewayHealthy(zeroneKey); got == nil || !*got {
		t.Errorf("gatewayHealthy(%q) = %v, want true", zeroneKey, got)
	}
	if got := s.gatewayHealthy(ayuKey); got == nil || *got {
		t.Errorf("gatewayHealthy(%q) = %v, want false", ayuKey, got)
	}
}

// TestToDTO_RuntimeURLUsesScopedPath asserts the public RuntimeURL reflects the
// tenant-scoped /<org>/<name> path when Kong is enabled.
func TestToDTO_RuntimeURLUsesScopedPath(t *testing.T) {
	s := &AgentDeployerService{
		publicHost: "10.0.0.1",
		kongSvc:    NewKongGatewayService(newFakeKong(), "agent-runtime", "deploy.example.com", newMemRepo(nil), 60, false),
	}
	dto := s.toDTO("zerone", "assistant", "running", "healthy", "c", 3000, nil, "")
	want := "https://deploy.example.com/zerone/assistant"
	if dto.RuntimeURL != want {
		t.Errorf("RuntimeURL = %q, want %q", dto.RuntimeURL, want)
	}
}

func TestToDTO_NoKongRunningReturnsRelativeRuntimeURL(t *testing.T) {
	s := &AgentDeployerService{publicHost: "203.0.113.10"} // kongSvc == nil → no Kong
	dto := s.toDTO("default", "test", "running", "healthy", "c-default-test", 32100, nil, "")
	if dto.RuntimeURL != "/runtime/default/test" {
		t.Fatalf("RuntimeURL = %q, want /runtime/default/test", dto.RuntimeURL)
	}
}

func TestToDTO_BuiltinNoKongReturnsBareRuntimeURL(t *testing.T) {
	// builtin mode omits the implicit default tenant from public URLs
	// (issue #114): /runtime/<name>, not /runtime/default/<name>.
	s := &AgentDeployerService{publicHost: "203.0.113.10", builtinAuth: true}
	dto := s.toDTO("default", "test", "running", "healthy", "c-default-test", 32100, nil, "")
	if dto.RuntimeURL != "/runtime/test" {
		t.Fatalf("RuntimeURL = %q, want /runtime/test", dto.RuntimeURL)
	}
}

func TestToDTO_NotRunning_NoProxyURL(t *testing.T) {
	s := &AgentDeployerService{publicHost: "203.0.113.10"}
	dto := s.toDTO("default", "test", "stopped", "unhealthy", "c", 0, nil, "")
	if dto.RuntimeURL == "/runtime/default/test" {
		t.Fatal("non-running deployment must not get a proxy URL")
	}
}

// registerWhenHealthyFixture builds a service whose deployer reports a healthy
// container, plus a Kong service with an empty routeHost (skips the post-
// register probe loop so the test stays fast).
func registerWhenHealthyFixture(t *testing.T) (*AgentDeployerService, *fakeKong) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"agentName":"scoped","containerName":"c","containerId":"id","status":"running","health":"healthy","hostPort":3000}}`))
	}))
	t.Cleanup(srv.Close)
	fk := newFakeKong()
	return &AgentDeployerService{
		client:        deployer.NewClient(srv.URL, "test-key"),
		publicHost:    "10.0.0.1",
		healthProbe:   func(ctx context.Context, host string, port int) bool { return false },
		gatewayHealth: &sync.Map{},
		kongSvc:       NewKongGatewayService(fk, "agent-runtime", "", newMemRepo(nil), 60, false),
	}, fk
}

// TestRegisterWhenHealthy_RegistersScopedRoute asserts registerWhenHealthy
// registers exactly one route with the tenant-scoped path.
func TestRegisterWhenHealthy_RegistersScopedRoute(t *testing.T) {
	s, fk := registerWhenHealthyFixture(t)

	s.registerWhenHealthy("tenant-a", "general", 3000)

	if fk.services["agent-tenant-a-general"] == nil {
		t.Fatal("expected scoped service agent-tenant-a-general")
	}
	r := fk.routes["agent-tenant-a-general-route"]
	if r == nil {
		t.Fatal("expected scoped route agent-tenant-a-general-route")
	}
	if len(r.Paths) != 1 || r.Paths[0] != "/tenant-a/general" {
		t.Fatalf("scoped route paths = %v, want [/tenant-a/general]", r.Paths)
	}
	if len(fk.routes) != 1 {
		t.Fatalf("expected exactly 1 route, got %d", len(fk.routes))
	}
}

// TestWaitForHealthy_UsesScopedKey is a smoke check that WaitForHealthy (used
// by registerWhenHealthy with the scoped key) addresses the deployer with the
// scoped key.
func TestWaitForHealthy_UsesScopedKey(t *testing.T) {
	rec := &scopedRecorder{}
	srv := newScopedDeployerServer(t, rec, true, "running")
	defer srv.Close()

	client := deployer.NewClient(srv.URL, "test-key")
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", upstreamHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool { return false }}

	if _, err := s.WaitForHealthy(context.Background(), "tenant-a-general", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.hasEntry("GET /api/v1/agents/tenant-a-general/status") {
		t.Errorf("status poll should hit /api/v1/agents/tenant-a-general/status, got %v", rec.paths)
	}
}
