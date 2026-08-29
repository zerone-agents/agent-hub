package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/infrastructure/deployer"
	"control-panel/internal/infrastructure/kong"
)

// scopedRecorder captures every deployer request so tests can assert the
// tenant-scoped deploy key reaches the wire (URL paths and payloads).
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
	rec.mu.Lock()
	defer rec.mu.Unlock()
	var names []string
	for _, body := range rec.postBodies {
		var parsed struct {
			Agent struct {
				Name string `json:"name"`
			} `json:"agent"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("unmarshal create body: %v", err)
		}
		names = append(names, parsed.Agent.Name)
	}
	return names
}

// TestDeploy_SendsScopedKeyToDeployer asserts that every deployer call made by
// Deploy uses the tenant-scoped key: the existence probe GET, and the create
// payload's agent name (which becomes the container key on the deployer side).
func TestDeploy_SendsScopedKeyToDeployer(t *testing.T) {
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
	if got := rec.createdAgentName(t); got != wantKey {
		t.Errorf("create payload agent.name = %q, want %q", got, wantKey)
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

	if got := rec.createdAgentNames(t); len(got) != 2 || got[0] != "zerone-assistant" || got[1] != "ayu-assistant" {
		t.Errorf("create payload agent names = %v, want [zerone-assistant ayu-assistant]", got)
	}
	if !rec.hasEntry("GET /api/v1/agents/zerone-assistant") || !rec.hasEntry("GET /api/v1/agents/ayu-assistant") {
		t.Errorf("existence probes should hit both scoped keys, got %v", rec.paths)
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
		kongSvc:    NewKongGatewayService(newFakeKong(), "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}
	dto := s.toDTO("zerone", "assistant", "running", "healthy", "c", 3000, nil, "")
	want := "https://deploy.example.com/zerone/assistant"
	if dto.RuntimeURL != want {
		t.Errorf("RuntimeURL = %q, want %q", dto.RuntimeURL, want)
	}
}

// v2：default 租户 + Kong 启用时返回裸路径 URL；非 default 租户与未启用
// Kong 时为空。
func TestToDTO_BareRuntimeURL(t *testing.T) {
	withKong := &AgentDeployerService{
		publicHost: "10.0.0.1",
		kongSvc:    NewKongGatewayService(newFakeKong(), "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}

	dto := withKong.toDTO("default", "assistant", "running", "healthy", "c", 3000, nil, "")
	if dto.BareRuntimeURL != "https://deploy.example.com/assistant" {
		t.Errorf("BareRuntimeURL = %q, want https://deploy.example.com/assistant", dto.BareRuntimeURL)
	}
	if dto.RuntimeURL != "https://deploy.example.com/default/assistant" {
		t.Errorf("RuntimeURL = %q, want scoped path", dto.RuntimeURL)
	}

	dto = withKong.toDTO("zerone", "assistant", "running", "healthy", "c", 3000, nil, "")
	if dto.BareRuntimeURL != "" {
		t.Errorf("BareRuntimeURL for non-default tenant = %q, want empty", dto.BareRuntimeURL)
	}

	noKong := &AgentDeployerService{publicHost: "10.0.0.1"}
	dto = noKong.toDTO("default", "assistant", "running", "healthy", "c", 3000, nil, "")
	if dto.BareRuntimeURL != "" {
		t.Errorf("BareRuntimeURL without kong = %q, want empty", dto.BareRuntimeURL)
	}
}

// TestDeploy_PreCleanRemovesLegacyBareEntities asserts Deploy's pre-clean
// Deregister removes the old bare-name Kong entities (using the scoped key for
// the new entities and the bare name as legacyBare).
func TestDeploy_PreCleanRemovesLegacyBareEntities(t *testing.T) {
	rec := &scopedRecorder{}
	srv := newScopedDeployerServer(t, rec, false, "created") // "created" keeps the async goroutine off
	defer srv.Close()

	fk := newFakeKong()
	fk.services["agent-general"] = &kong.Service{ID: "legacy-id", Name: "agent-general", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}
	fk.routes["agent-general-route"] = &kong.Route{ID: "legacy-route-id", Name: "agent-general-route"}

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(&deployTokenFixture{}, ""), deployTokenProviderSvc())
	s.kongSvc = NewKongGatewayService(fk, "agent-runtime", "deploy.example.com", newMemRepo(nil), 60)

	if _, err := s.Deploy("tenant-a", "general", false, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-general"] != nil {
		t.Error("expected legacy bare service to be removed by Deploy pre-clean")
	}
	if fk.routes["agent-general-route"] != nil {
		t.Error("expected legacy bare route to be removed by Deploy pre-clean")
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
		kongSvc:       NewKongGatewayService(fk, "agent-runtime", "", newMemRepo(nil), 60),
	}, fk
}

// TestRegisterWhenHealthy_MountsLegacyAfterPreClean covers the D-1 timing: by
// the time registerWhenHealthy runs, Deploy's pre-clean has already deleted
// the bare-name entities, so the legacy route must mount from the flag the
// deploy flow recorded earlier — not from probing Kong for the bare service.
func TestRegisterWhenHealthy_MountsLegacyAfterPreClean(t *testing.T) {
	s, fk := registerWhenHealthyFixture(t)

	s.registerWhenHealthy("tenant-a", "general", "general", 3000)

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
	lr := fk.routes["agent-tenant-a-general-route-legacy"]
	if lr == nil {
		t.Fatal("expected legacy route agent-tenant-a-general-route-legacy despite bare service being pre-cleaned")
	}
	if len(lr.Paths) != 1 || lr.Paths[0] != "/general" {
		t.Fatalf("legacy route paths = %v, want [/general]", lr.Paths)
	}
	if lr.Service == nil || lr.Service.ID != fk.services["agent-tenant-a-general"].ID {
		t.Fatal("expected legacy route to reference the scoped service")
	}
}

// TestRegisterWhenHealthy_NoLegacyRouteWhenNotRecorded asserts no legacy route
// mounts when the deploy flow did not record a pre-existing bare entity.
func TestRegisterWhenHealthy_NoLegacyRouteWhenNotRecorded(t *testing.T) {
	s, fk := registerWhenHealthyFixture(t)

	s.registerWhenHealthy("tenant-a", "general", "", 3000)

	if fk.routes["agent-tenant-a-general-route-legacy"] != nil {
		t.Fatal("expected no legacy route when no legacy entity was recorded")
	}
	if fk.routes["agent-tenant-a-general-route"] == nil {
		t.Fatal("expected scoped route to be registered")
	}
}

// TestLegacyBareFor_LegacyRouteOnly covers the redeploy scenario (I-1): after
// the first redeploy the bare-name Kong entities no longer exist (the first
// pre-clean deleted them), but the "<key>-legacy" route mounted by that
// redeploy is still there. The deploy flow must still pass legacyBare so
// registerWhenHealthy re-mounts the route and the old URL survives until it
// is removed by hand.
func TestLegacyBareFor_LegacyRouteOnly(t *testing.T) {
	fk := newFakeKong()
	fk.routes["agent-tenant-a-general-route-legacy"] = &kong.Route{
		ID:    "legacy-route-id",
		Name:  "agent-tenant-a-general-route-legacy",
		Paths: []string{"/general"},
	}
	s := &AgentDeployerService{
		kongSvc: NewKongGatewayService(fk, "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}

	if got := s.legacyBareFor(context.Background(), "tenant-a", "general"); got != "general" {
		t.Fatalf("legacyBareFor = %q, want %q (legacy route present, bare service gone)", got, "general")
	}
}

// TestLegacyBareFor_BareServiceStillThere covers the fresh-upgrade scenario:
// the pre-upgrade bare-name service exists, so the first redeploy detects it
// via the bare probe.
func TestLegacyBareFor_BareServiceStillThere(t *testing.T) {
	fk := newFakeKong()
	fk.services["agent-general"] = &kong.Service{ID: "legacy-id", Name: "agent-general", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}
	s := &AgentDeployerService{
		kongSvc: NewKongGatewayService(fk, "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}

	if got := s.legacyBareFor(context.Background(), "tenant-a", "general"); got != "general" {
		t.Fatalf("legacyBareFor = %q, want %q (bare service present)", got, "general")
	}
}

// TestLegacyBareFor_None asserts no legacy mount is requested when neither the
// bare-name service nor a "-legacy" route exists (never opted in, or the
// legacy route was already manually decommissioned).
func TestLegacyBareFor_None(t *testing.T) {
	fk := newFakeKong()
	s := &AgentDeployerService{
		kongSvc: NewKongGatewayService(fk, "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}

	if got := s.legacyBareFor(context.Background(), "tenant-a", "general"); got != "" {
		t.Fatalf("legacyBareFor = %q, want empty (no legacy entity anywhere)", got)
	}
}

// v2：default 租户仅保留 bare-service 探测（供 pre-clean 显式删除升级前
// 旧裸名实体），跳过 -legacy 路由探测（被裸路径 supersede）。
func TestLegacyBareFor_DefaultTenantKeepsBareServiceProbeOnly(t *testing.T) {
	fk := newFakeKong()
	s := &AgentDeployerService{
		kongSvc: NewKongGatewayService(fk, "agent-runtime", "deploy.example.com", newMemRepo(nil), 60),
	}
	// 仅 -legacy 路由存在：default 跳过该探测 → 空
	fk.routes["agent-default-general-route-legacy"] = &kong.Route{Name: "agent-default-general-route-legacy"}
	if got := s.legacyBareFor(context.Background(), "default", "general"); got != "" {
		t.Fatalf("legacyBareFor(default, -legacy only) = %q, want empty", got)
	}
	// bare service 存在：保留探测（供 pre-clean 删除旧实体）
	fk.services["agent-general"] = &kong.Service{Name: "agent-general"}
	if got := s.legacyBareFor(context.Background(), "default", "general"); got != "general" {
		t.Fatalf("legacyBareFor(default, bare service) = %q, want general", got)
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
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool { return false }}

	if _, err := s.WaitForHealthy(context.Background(), "tenant-a-general", 5*time.Second); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rec.hasEntry("GET /api/v1/agents/tenant-a-general/status") {
		t.Errorf("status poll should hit /api/v1/agents/tenant-a-general/status, got %v", rec.paths)
	}
}
