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
	if err := s.Register(context.Background(), "general", 3000); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRegister_CreatesServiceAndRoute(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "general", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-general"] == nil {
		t.Fatal("expected service agent-general to be created")
	}
	if fk.services["agent-general"].Host != testServiceHost || fk.services["agent-general"].Port != 3000 {
		t.Fatalf("unexpected service host/port: %+v", fk.services["agent-general"])
	}
	if fk.routes["agent-general-route"] == nil {
		t.Fatal("expected route agent-general-route to be created")
	}
	r := fk.routes["agent-general-route"]
	if len(r.Hosts) != 1 || r.Hosts[0] != testRouteHost {
		t.Fatalf("unexpected route hosts: %v", r.Hosts)
	}
	if len(r.Paths) != 1 || r.Paths[0] != "/general" {
		t.Fatalf("unexpected route paths: %v", r.Paths)
	}
	if !r.StripPath {
		t.Fatal("expected strip_path to be true")
	}
	if r.Service == nil || r.Service.ID != fk.services["agent-general"].ID {
		t.Fatal("expected route to reference the service")
	}
	if r.RequestBuffering == nil || *r.RequestBuffering {
		t.Fatal("expected request_buffering to be false")
	}
	if r.ResponseBuffering == nil || *r.ResponseBuffering {
		t.Fatal("expected response_buffering to be false")
	}
}

func TestRegister_IsIdempotent_SecondCallNoDup(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	if err := s.Register(context.Background(), "general", 3000); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := s.Register(context.Background(), "general", 3000); err != nil {
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
	if err := s.Register(context.Background(), "Bad Name", 3000); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fk.services) != 0 || len(fk.routes) != 0 {
		t.Fatal("expected no resources created for invalid name")
	}
}

func TestDeregister_RemovesServiceAndIsIdempotent(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", 3000)

	if err := s.Deregister(context.Background(), "general"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fk.services["agent-general"] != nil {
		t.Fatal("expected service to be removed")
	}
	if fk.routes["agent-general-route"] != nil {
		t.Fatal("expected route to be removed")
	}

	// Idempotent: deregistering again should not error
	if err := s.Deregister(context.Background(), "general"); err != nil {
		t.Fatalf("idempotent deregister failed: %v", err)
	}
}

func TestUpdateUpstream_UpdatesPort(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo(nil)
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", 3000)
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
		{Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-general"] == nil {
		t.Fatal("expected service to be created")
	}
}

func TestReconcile_FixesPortDrift(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{Name: "general", DeploymentStatus: "running", RuntimePort: 4000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", 3000)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-general"].Port != 4000 {
		t.Fatalf("expected port 4000, got %d", fk.services["agent-general"].Port)
	}
	if fk.services["agent-general"].Host != testServiceHost {
		t.Fatalf("expected service host %s, got %s", testServiceHost, fk.services["agent-general"].Host)
	}
}

func TestReconcile_FixesRouteHostDrift(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", 3000)

	// Simulate route host drift by changing it to a stale value.
	fk.routes["agent-general-route"].Hosts = []string{"stale.example.com"}

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if len(fk.routes["agent-general-route"].Hosts) != 1 || fk.routes["agent-general-route"].Hosts[0] != testRouteHost {
		t.Fatalf("expected route host %s, got %v", testRouteHost, fk.routes["agent-general-route"].Hosts)
	}
}

func TestReconcile_DeregistersUnserviceable(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{Name: "general", DeploymentStatus: "stopped", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)
	_ = s.Register(context.Background(), "general", 3000)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix, got %d", fixes)
	}
	if fk.services["agent-general"] != nil {
		t.Fatal("expected service to be removed")
	}
}

func TestReconcile_PurgesOrphan(t *testing.T) {
	fk := newFakeKong()
	repo := newMemRepo([]agent.AgentConfig{
		{Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
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
	if fk.services["agent-general"] == nil {
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
		{Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
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
		{Name: "general", DesktopEnabled: false, MobileEnabled: false, DeploymentStatus: "running", RuntimePort: 3000},
	})
	s := newKongService(fk, repo)

	fixes, err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fixes != 1 {
		t.Fatalf("expected 1 fix (register route), got %d", fixes)
	}
	if fk.services["agent-general"] == nil {
		t.Fatal("expected route to be registered for running agent with no platform flags")
	}
}

func TestReconcile_Disabled_NoOp(t *testing.T) {
	repo := newMemRepo([]agent.AgentConfig{
		{Name: "general", DeploymentStatus: "running", RuntimePort: 3000},
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
	if got := s.RouteURL("general"); got != "https://deploy.example.com/general" {
		t.Fatalf("unexpected route url: %q", got)
	}
	if got := NewKongGatewayService(nil, testServiceHost, "", nil, 60).RouteURL("general"); got != "" {
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
