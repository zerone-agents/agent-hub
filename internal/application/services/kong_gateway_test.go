package services

import (
	"context"
	"fmt"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/infrastructure/kong"
)

// fakeKong is an in-memory implementation of the kongClient interface for tests.
type fakeKong struct {
	services map[string]*kong.Service
	routes   map[string]*kong.Route
	counter  int
	// ignoreTagFilter simulates a Kong backend that silently ignores the
	// ?tags= query param and returns every service.
	ignoreTagFilter bool
}

func newFakeKong() *fakeKong {
	return &fakeKong{
		services: make(map[string]*kong.Service),
		routes:   make(map[string]*kong.Route),
	}
}

func (f *fakeKong) nextID() string {
	f.counter++
	return fmt.Sprintf("id-%d", f.counter)
}

func (f *fakeKong) GetService(ctx context.Context, name string) (*kong.Service, bool, error) {
	s, ok := f.services[name]
	if !ok {
		return nil, false, nil
	}
	return s, true, nil
}

func (f *fakeKong) CreateService(ctx context.Context, s *kong.Service) (*kong.Service, error) {
	created := *s
	created.ID = f.nextID()
	f.services[s.Name] = &created
	return &created, nil
}

func (f *fakeKong) UpdateService(ctx context.Context, name string, s *kong.Service) (*kong.Service, error) {
	if _, ok := f.services[name]; !ok {
		return nil, fmt.Errorf("service %s not found", name)
	}
	updated := *s
	updated.ID = f.services[name].ID
	f.services[name] = &updated
	return &updated, nil
}

func (f *fakeKong) DeleteService(ctx context.Context, name string) error {
	delete(f.services, name)
	// Cascade delete associated routes by service name prefix
	for rn, r := range f.routes {
		if r.Service != nil && r.Service.ID == "" {
			_ = rn
		}
		// In this fake the route is named "agent-<name>-route", so delete if it matches.
		if rn == name+"-route" {
			delete(f.routes, rn)
		}
	}
	return nil
}

func (f *fakeKong) GetRoute(ctx context.Context, name string) (*kong.Route, bool, error) {
	r, ok := f.routes[name]
	if !ok {
		return nil, false, nil
	}
	return r, true, nil
}

func (f *fakeKong) CreateRoute(ctx context.Context, r *kong.Route) (*kong.Route, error) {
	created := *r
	created.ID = f.nextID()
	f.routes[r.Name] = &created
	return &created, nil
}

func (f *fakeKong) UpdateRoute(ctx context.Context, name string, r *kong.Route) (*kong.Route, error) {
	if _, ok := f.routes[name]; !ok {
		return nil, fmt.Errorf("route %s not found", name)
	}
	updated := *r
	updated.ID = f.routes[name].ID
	f.routes[name] = &updated
	return &updated, nil
}

func (f *fakeKong) DeleteRoute(ctx context.Context, name string) error {
	delete(f.routes, name)
	return nil
}

func (f *fakeKong) ListServicesByTag(ctx context.Context, tag string) ([]kong.Service, error) {
	var out []kong.Service
	for _, s := range f.services {
		if f.ignoreTagFilter {
			out = append(out, *s)
			continue
		}
		for _, t := range s.Tags {
			if t == tag {
				out = append(out, *s)
				break
			}
		}
	}
	return out, nil
}

// memRepo implements the kongAgentRepo interface for tests.
type memRepo struct {
	items []agent.AgentConfig
}

func newMemRepo(items []agent.AgentConfig) *memRepo {
	return &memRepo{items: items}
}

func (m *memRepo) ListAllForReconcile() ([]agent.AgentConfig, error) {
	return m.items, nil
}

const (
	testServiceHost = "deployer.internal"
	testRouteHost   = "deploy.example.com"
)

func newKongService(fk *fakeKong, repo *memRepo) *KongGatewayService {
	return NewKongGatewayService(fk, testServiceHost, testRouteHost, repo, 60)
}

func TestRegister_Disabled_IsNoOp(t *testing.T) {
	s := NewKongGatewayService(nil, testServiceHost, testRouteHost, newMemRepo(nil), 60)
	if err := s.Register(context.Background(), "general", "/default/general", "", 3000); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRegister_CreatesServiceAndRoute(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "zerone-general", "/zerone/general", "", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-zerone-general"] == nil {
		t.Fatal("expected service agent-zerone-general to be created")
	}
	if fk.services["agent-zerone-general"].Host != testServiceHost || fk.services["agent-zerone-general"].Port != 3000 {
		t.Fatalf("unexpected service host/port: %+v", fk.services["agent-zerone-general"])
	}
	if fk.routes["agent-zerone-general-route"] == nil {
		t.Fatal("expected route agent-zerone-general-route to be created")
	}
	r := fk.routes["agent-zerone-general-route"]
	if len(r.Hosts) != 1 || r.Hosts[0] != testRouteHost {
		t.Fatalf("unexpected route hosts: %v", r.Hosts)
	}
	if len(r.Paths) != 1 || r.Paths[0] != "/zerone/general" {
		t.Fatalf("unexpected route paths: %v", r.Paths)
	}
	if !r.StripPath {
		t.Fatal("expected strip_path to be true")
	}
	if r.Service == nil || r.Service.ID != fk.services["agent-zerone-general"].ID {
		t.Fatal("expected route to reference the service")
	}
	if r.RequestBuffering == nil || *r.RequestBuffering {
		t.Fatal("expected request_buffering to be false")
	}
	if r.ResponseBuffering == nil || *r.ResponseBuffering {
		t.Fatal("expected response_buffering to be false")
	}
}

func TestRegister_SingleSegmentPath_Rejected(t *testing.T) {
	// pathRe requires at least two segments (/<org>/<name>): the pre-Task-2
	// single-segment "/"+bareName form must now be rejected so no route can
	// claim a cross-tenant bare path.
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "general", "/general", "", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fk.services) != 0 || len(fk.routes) != 0 {
		t.Fatalf("expected no resources for single-segment path, got %d services / %d routes", len(fk.services), len(fk.routes))
	}
}

func TestRegister_IsIdempotent_SecondCallNoDup(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "general", "/t/general", "", 3000); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := s.Register(context.Background(), "general", "/t/general", "", 3000); err != nil {
		t.Fatalf("second register failed: %v", err)
	}
	if len(fk.services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(fk.services))
	}
	if len(fk.routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(fk.routes))
	}
}

func TestRegister_InvalidName_Skipped(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "Bad Name", "/default/bad", "", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fk.services) != 0 || len(fk.routes) != 0 {
		t.Fatal("expected no resources created for invalid name")
	}
}

func TestRegister_InvalidPublicPath_Skipped(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "general", "/Bad Path", "", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fk.services) != 0 || len(fk.routes) != 0 {
		t.Fatal("expected no resources created for invalid public path")
	}
}

func TestRegister_LegacyRouteCreatedWhenBareServiceExists(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	// Pre-existing legacy bare-name service.
	fk.services["agent-assistant"] = &kong.Service{ID: "legacy-id", Name: "agent-assistant", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}

	if err := s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lr := fk.routes["agent-zerone-assistant-route-legacy"]
	if lr == nil {
		t.Fatal("expected legacy route agent-zerone-assistant-route-legacy to be created")
	}
	if len(lr.Paths) != 1 || lr.Paths[0] != "/assistant" {
		t.Fatalf("unexpected legacy route paths: %v", lr.Paths)
	}
	if !lr.StripPath {
		t.Fatal("expected legacy route strip_path to be true")
	}
	if lr.Service == nil || lr.Service.ID != fk.services["agent-zerone-assistant"].ID {
		t.Fatal("expected legacy route to reference the new service")
	}
	// The legacy bare service itself is left untouched (kept serving until
	// decommissioned by Deregister).
	if fk.services["agent-assistant"] == nil {
		t.Fatal("expected legacy bare service to survive register")
	}
}

func TestRegister_NoLegacyRouteWhenBareServiceAbsent(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.routes["agent-zerone-assistant-route-legacy"] != nil {
		t.Fatal("expected no legacy route when bare service does not exist")
	}
}

func TestRegister_LegacyRouteIdempotent(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	fk.services["agent-assistant"] = &kong.Service{ID: "legacy-id", Name: "agent-assistant", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}

	_ = s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000)
	_ = s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000)
	if len(fk.routes) != 2 {
		t.Fatalf("expected 2 routes (main + legacy), got %d", len(fk.routes))
	}
}

func TestLegacyExists(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	fk.services["agent-assistant"] = &kong.Service{ID: "legacy-id", Name: "agent-assistant", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}

	if !s.LegacyExists(context.Background(), "assistant") {
		t.Error("expected LegacyExists(assistant) = true")
	}
	if s.LegacyExists(context.Background(), "other") {
		t.Error("expected LegacyExists(other) = false")
	}
	if s.LegacyExists(context.Background(), "Bad Name") {
		t.Error("invalid bare name should report false")
	}
	disabled := NewKongGatewayService(nil, testServiceHost, testRouteHost, repo, 60)
	if disabled.LegacyExists(context.Background(), "assistant") {
		t.Error("disabled service should report false")
	}
}

// TestLegacyRouteExists covers the second legacy probe: after the first
// redeploy the bare-name entities are gone, but the mounted "<key>-legacy"
// route is the surviving proof that this agent opted into compatibility, so
// the deploy flow can keep re-mounting it on subsequent redeploys.
func TestLegacyRouteExists(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)

	if s.LegacyRouteExists(context.Background(), "zerone-assistant") {
		t.Error("expected LegacyRouteExists(zerone-assistant) = false before any registration")
	}

	if err := s.RegisterWithLegacy(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !s.LegacyRouteExists(context.Background(), "zerone-assistant") {
		t.Error("expected LegacyRouteExists(zerone-assistant) = true after forced mount")
	}
	if s.LegacyRouteExists(context.Background(), "zerone-other") {
		t.Error("expected LegacyRouteExists(zerone-other) = false")
	}

	disabled := NewKongGatewayService(nil, testServiceHost, testRouteHost, repo, 60)
	if disabled.LegacyRouteExists(context.Background(), "zerone-assistant") {
		t.Error("disabled service should report false")
	}
}

// TestRegisterWithLegacy_MountsDespiteMissingBareService covers the D-1
// timing: the deploy flow records legacy existence BEFORE its pre-clean
// Deregister deletes the bare entities, so the forced mount must not depend on
// the bare service still being present in Kong.
func TestRegisterWithLegacy_MountsDespiteMissingBareService(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)

	if err := s.RegisterWithLegacy(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lr := fk.routes["agent-zerone-assistant-route-legacy"]
	if lr == nil {
		t.Fatal("expected legacy route despite missing bare service")
	}
	if len(lr.Paths) != 1 || lr.Paths[0] != "/assistant" {
		t.Fatalf("legacy route paths = %v, want [/assistant]", lr.Paths)
	}
	if lr.Service == nil || lr.Service.ID != fk.services["agent-zerone-assistant"].ID {
		t.Fatal("expected legacy route to reference the scoped service")
	}
	if fk.routes["agent-zerone-assistant-route"] == nil {
		t.Fatal("expected main scoped route to be registered too")
	}

	// Idempotent: a second forced mount must not duplicate anything.
	if err := s.RegisterWithLegacy(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if len(fk.routes) != 2 {
		t.Fatalf("expected 2 routes (main + legacy), got %d", len(fk.routes))
	}
}

func TestDeregister_RemovesServiceAndIsIdempotent(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", "/t/general", "", 3000)

	if err := s.Deregister(context.Background(), "general", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-general"] != nil {
		t.Fatal("expected service to be removed")
	}
	if fk.routes["agent-general-route"] != nil {
		t.Fatal("expected route to be removed")
	}

	// Idempotent: deregistering again should not error
	if err := s.Deregister(context.Background(), "general", ""); err != nil {
		t.Fatalf("idempotent deregister failed: %v", err)
	}
}

func TestDeregister_CleansLegacyEntities(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	// Set up: legacy bare entities + new scoped entities (with legacy route).
	fk.services["agent-assistant"] = &kong.Service{ID: "legacy-id", Name: "agent-assistant", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}
	fk.routes["agent-assistant-route"] = &kong.Route{ID: "legacy-route-id", Name: "agent-assistant-route"}
	_ = s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "assistant", 3000)

	if err := s.Deregister(context.Background(), "zerone-assistant", "assistant"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-zerone-assistant"] != nil {
		t.Fatal("expected scoped service to be removed")
	}
	if fk.routes["agent-zerone-assistant-route-legacy"] != nil {
		t.Fatal("expected scoped legacy route to be removed")
	}
	if fk.services["agent-assistant"] != nil {
		t.Fatal("expected legacy bare service to be removed")
	}
	if fk.routes["agent-assistant-route"] != nil {
		t.Fatal("expected legacy bare route to be removed")
	}
}

func TestDeregister_EmptyLegacyBare_OnlyScopedEntities(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	// An unrelated bare-name entity that must NOT be touched when legacyBare is empty.
	fk.services["agent-assistant"] = &kong.Service{ID: "legacy-id", Name: "agent-assistant", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}

	_ = s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "", 3000)
	if err := s.Deregister(context.Background(), "zerone-assistant", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-zerone-assistant"] != nil {
		t.Fatal("expected scoped service to be removed")
	}
	if fk.services["agent-assistant"] == nil {
		t.Fatal("expected unrelated bare service to survive")
	}
}

func TestUpdateUpstream_UpdatesPort(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", "/t/general", "", 3000)
	if err := s.UpdateUpstream(context.Background(), "general", 4000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-general"].Port != 4000 {
		t.Fatalf("expected port 4000, got %d", fk.services["agent-general"].Port)
	}
}

func TestReconcile_RegistersMissing(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-zerone-general"] == nil {
		t.Fatal("expected service to be created")
	}
	if r := fk.routes["agent-zerone-general-route"]; r == nil || len(r.Paths) != 1 || r.Paths[0] != "/zerone/general" {
		t.Fatalf("expected route with path /zerone/general, got %+v", r)
	}
}

func TestReconcile_TwoTenantsSameName_BothRegistered(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "assistant", DeploymentStatus: "running", RuntimePort: 3000},
		{TenantID: "ayu", Name: "assistant", DeploymentStatus: "running", RuntimePort: 3001},
	})
	s := newKongService(fk, repo)

	if _, err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-zerone-assistant"] == nil || fk.services["agent-ayu-assistant"] == nil {
		t.Fatal("expected both tenant services to be created")
	}
	if fk.services["agent-zerone-assistant"].Port != 3000 || fk.services["agent-ayu-assistant"].Port != 3001 {
		t.Fatalf("unexpected ports: %d / %d", fk.services["agent-zerone-assistant"].Port, fk.services["agent-ayu-assistant"].Port)
	}
	if p := fk.routes["agent-zerone-assistant-route"].Paths; len(p) != 1 || p[0] != "/zerone/assistant" {
		t.Fatalf("unexpected zerone paths: %v", p)
	}
	if p := fk.routes["agent-ayu-assistant-route"].Paths; len(p) != 1 || p[0] != "/ayu/assistant" {
		t.Fatalf("unexpected ayu paths: %v", p)
	}
}

func TestReconcile_TwoTenantsSameName_NoCrossDelete(t *testing.T) {
	// Both entities already exist and match DB state: reconcile must keep both.
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "assistant", DeploymentStatus: "running", RuntimePort: 3000},
		{TenantID: "ayu", Name: "assistant", DeploymentStatus: "running", RuntimePort: 3001},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "zerone-assistant", "/zerone/assistant", "", 3000)
	_ = s.Register(context.Background(), "ayu-assistant", "/ayu/assistant", "", 3001)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 0 {
		t.Fatalf("expected 0 fixes, got %d", fixes)
	}
	if fk.services["agent-zerone-assistant"] == nil || fk.services["agent-ayu-assistant"] == nil {
		t.Fatal("expected both tenant services to survive reconcile")
	}
}

func TestReconcile_FixesPortDrift(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 4000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "zerone-general", "/zerone/general", "", 3000)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-zerone-general"].Port != 4000 {
		t.Fatalf("expected port 4000, got %d", fk.services["agent-zerone-general"].Port)
	}
	if fk.services["agent-zerone-general"].Host != testServiceHost {
		t.Fatalf("expected service host %s, got %s", testServiceHost, fk.services["agent-zerone-general"].Host)
	}
}

func TestReconcile_FixesRouteHostDrift(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "zerone-general", "/zerone/general", "", 3000)

	// Simulate route host drift by changing it to a stale value.
	fk.routes["agent-zerone-general-route"].Hosts = []string{"stale.example.com"}

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if len(fk.routes["agent-zerone-general-route"].Hosts) != 1 || fk.routes["agent-zerone-general-route"].Hosts[0] != testRouteHost {
		t.Fatalf("expected route host %s, got %v", testRouteHost, fk.routes["agent-zerone-general-route"].Hosts)
	}
}

func TestReconcile_FixesRoutePathDrift(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "zerone-general", "/zerone/general", "", 3000)

	// Simulate path drift.
	fk.routes["agent-zerone-general-route"].Paths = []string{"/stale/general"}

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if p := fk.routes["agent-zerone-general-route"].Paths; len(p) != 1 || p[0] != "/zerone/general" {
		t.Fatalf("expected path /zerone/general, got %v", p)
	}
}

func TestReconcile_DeregistersUnserviceable(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "stopped", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "zerone-general", "/zerone/general", "", 3000)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-zerone-general"] != nil {
		t.Fatal("expected service to be removed")
	}
}

func TestReconcile_PurgesOrphan(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	// Create an orphan service directly in fake Kong
	fk.services["agent-ghost"] = &kong.Service{Name: "agent-ghost", Host: "10.0.0.1", Port: 3000, Tags: []string{kongManagedTag}}

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 2 {
		t.Fatalf("expected 2 fixes (register general + purge orphan), got %d", fixes)
	}
	if fk.services["agent-zerone-general"] == nil {
		t.Fatal("expected general service to be created")
	}
	if fk.services["agent-ghost"] != nil {
		t.Fatal("expected orphan service to be removed")
	}
}

func TestReconcile_KeepsUntaggedServiceWhenTagFilterIgnored(t *testing.T) {
	fk := newFakeKong()
	// Simulate a Kong backend that ignores ?tags= and returns every service.
	fk.ignoreTagFilter = true
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	// A foreign service whose name collides with the agent-* prefix but which
	// is NOT managed by control-panel (no managed tag).
	fk.services["agent-legacy"] = &kong.Service{Name: "agent-legacy", Host: "10.0.0.1", Port: 9000}
	fk.routes["agent-legacy-route"] = &kong.Route{Name: "agent-legacy-route"}

	if _, err := s.Reconcile(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-legacy"] == nil {
		t.Fatal("expected untagged service to survive reconcile")
	}
	if fk.routes["agent-legacy-route"] == nil {
		t.Fatal("expected untagged route to survive reconcile")
	}
}

func TestReconcile_KeepsRouteForPlatformHiddenAgent(t *testing.T) {
	// Platform visibility flags (desktop/mobile) control client-side loading
	// only; a running container keeps its gateway route regardless.
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DesktopEnabled: false, MobileEnabled: false, DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix (register route), got %d", fixes)
	}
	if fk.services["agent-zerone-general"] == nil {
		t.Fatal("expected route to be registered for running agent with no platform flags")
	}
}

func TestReconcile_Disabled_NoOp(t *testing.T) {
	repo := newMemRepo([]agent.AgentConfig{
		{TenantID: "zerone", Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := NewKongGatewayService(nil, testServiceHost, testRouteHost, repo, 60)
	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 0 {
		t.Fatalf("expected 0 fixes, got %d", fixes)
	}
}

func TestRouteURL(t *testing.T) {
	s := NewKongGatewayService(nil, testServiceHost, testRouteHost, nil, 60)
	if got := s.RouteURL("/zerone/general"); got != "https://deploy.example.com/zerone/general" {
		t.Fatalf("unexpected route url: %q", got)
	}
	// Tolerance: missing leading slash is auto-fixed.
	if got := s.RouteURL("zerone/general"); got != "https://deploy.example.com/zerone/general" {
		t.Fatalf("unexpected tolerant route url: %q", got)
	}
	if got := NewKongGatewayService(nil, testServiceHost, "", nil, 60).RouteURL("/zerone/general"); got != "" {
		t.Fatalf("expected empty url, got %q", got)
	}
}

func TestAgentNameFromService(t *testing.T) {
	if got := agentNameFromService("agent-general"); got != "general" {
		t.Fatalf("expected general, got %q", got)
	}
	if got := agentNameFromService("other"); got != "other" {
		t.Fatalf("expected other, got %q", got)
	}
}

func TestDeployKey(t *testing.T) {
	cases := []struct {
		tenant, name, want string
	}{
		{"zerone", "assistant", "zerone-assistant"},
		{"ayu", "assistant", "ayu-assistant"},
		// Defensive normalization: uppercase and special chars folded.
		{"Zero_Corp!", "Assistant X", "zero-corp-assistant-x"},
		{"--ayu--", "assistant", "ayu-assistant"},
	}
	for _, c := range cases {
		if got := DeployKey(c.tenant, c.name); got != c.want {
			t.Fatalf("DeployKey(%q,%q) = %q, want %q", c.tenant, c.name, got, c.want)
		}
	}
}

func TestURLPath(t *testing.T) {
	cases := []struct {
		tenant, name, want string
	}{
		{"zerone", "assistant", "/zerone/assistant"},
		{"ayu", "assistant", "/ayu/assistant"},
		{"Zero_Corp!", "Assistant X", "/zero-corp/assistant-x"},
	}
	for _, c := range cases {
		if got := URLPath(c.tenant, c.name); got != c.want {
			t.Fatalf("URLPath(%q,%q) = %q, want %q", c.tenant, c.name, got, c.want)
		}
	}
}
