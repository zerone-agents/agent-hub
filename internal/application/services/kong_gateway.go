package services

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/infrastructure/kong"
)

// kongClient is the subset of Kong admin operations needed by the gateway
// service. The concrete *kong.Client in internal/infrastructure/kong satisfies
// this interface.
type kongClient interface {
	GetService(ctx context.Context, name string) (*kong.Service, bool, error)
	CreateService(ctx context.Context, s *kong.Service) (*kong.Service, error)
	UpdateService(ctx context.Context, name string, s *kong.Service) (*kong.Service, error)
	DeleteService(ctx context.Context, name string) error
	GetRoute(ctx context.Context, name string) (*kong.Route, bool, error)
	CreateRoute(ctx context.Context, r *kong.Route) (*kong.Route, error)
	UpdateRoute(ctx context.Context, name string, r *kong.Route) (*kong.Route, error)
	DeleteRoute(ctx context.Context, name string) error
	ListServicesByTag(ctx context.Context, tag string) ([]kong.Service, error)
	ListRoutesByTag(ctx context.Context, tag string) ([]kong.Route, error)
}

const kongManagedTag = "managed-by:control-panel"

// Kong timeout budgets for agent runtime upstreams, in milliseconds.
//
// ReadTimeout/WriteTimeout must comfortably exceed the longest legitimate
// silent window on the SSE stream. During long tool calls (e.g. WriteTool on
// a large file) the runtime emits nothing for minutes; Kong's default 60s
// would silently kill the stream mid-turn. The control-panel heartbeat (15s
// in agent_chat.go) keeps the downstream browser connection alive, but Kong
// applies its read_timeout on the upstream leg independently, so we raise it
// to 1 hour. connect_timeout stays at Kong's default since establishing TCP
// to the runtime container is fast and we want failover to be snappy.
const (
	kongConnectTimeoutMs = 60_000
	kongReadTimeoutMs    = 3_600_000
	kongWriteTimeoutMs   = 3_600_000
)

var agentNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// pathRe validates a public route path: at least two segments of the form
// "/<org>/<name>" (extra segments allowed), each segment lowercase-alnum-led
// and may contain hyphens — matching both agentNameRe and the defensive
// orgSlug folding, which can produce hyphenated orgs for legacy tenant rows
// (registration-side validation restricts new orgs further). Since Task 2
// every caller passes URLPath(tenantID, name); single-segment bare-name paths
// are rejected so no route can claim a cross-tenant bare path.
var pathRe = regexp.MustCompile(`^/[a-z][a-z0-9-]*(?:/[a-z][a-z0-9-]*)+$`)

// orgSlugRe folds any run of non-alphanumeric characters into a single "-".
var orgSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// orgSlug defensively normalizes a tenant/org identifier for use in entity
// names and URL segments: lowercase, non-alphanumeric runs folded to "-",
// leading/trailing "-" trimmed. Registration-side validation already restricts
// orgs to ^[a-z][a-z0-9]{0,62}$ (Task 3); this is a safety net for legacy rows.
func orgSlug(tenantID string) string {
	s := strings.ToLower(tenantID)
	s = orgSlugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// DeployKey returns the tenant-scoped deployment key for an agent:
// "<org>-<NormalizeAgentName(name)>". The key names Kong service/route
// entities (via svcName/routeName) and deployer containers, decoupling them
// from the DB bare name so same-name agents across tenants no longer collide.
func DeployKey(tenantID, agentName string) string {
	return orgSlug(tenantID) + "-" + NormalizeAgentName(agentName)
}

// URLPath returns the public URL path segment for an agent:
// "/<org>/<NormalizeAgentName(name)>".
func URLPath(tenantID, agentName string) string {
	return "/" + orgSlug(tenantID) + "/" + NormalizeAgentName(agentName)
}

// kongAgentRepo is the minimal repository surface needed by KongGatewayService.
// *repository.AgentRepository implements this interface.
type kongAgentRepo interface {
	ListAllForReconcile() ([]agent.AgentConfig, error)
}

// KongGatewayService registers and maintains Kong Services and Routes for each
// agent. The service is optional: when constructed with a nil kongClient, every
// method is a no-op so the gateway integration can be disabled by omitting
// KONG_ADMIN_URL.
type KongGatewayService struct {
	client       kongClient
	serviceHost  string // Kong Service upstream host (internal runtime address)
	routeHost    string // Kong Route host / public URL host
	repo         kongAgentRepo
	reconcileSec int
	logger       *log.Logger
}

// NewKongGatewayService creates a new gateway service. reconcileSec defaults to
// 300 seconds when non-positive.
func NewKongGatewayService(client kongClient, serviceHost, routeHost string, repo kongAgentRepo, reconcileSec int) *KongGatewayService {
	if reconcileSec <= 0 {
		reconcileSec = 300
	}
	// A typed-nil concrete client (e.g. *kong.Client(nil)) assigned to the
	// kongClient interface is non-nil, which would make enabled() true and
	// cause a nil-pointer panic in Reconcile. Normalize it to a true nil
	// interface so an "unconfigured" client is treated as a no-op.
	if client != nil {
		rv := reflect.ValueOf(client)
		if rv.Kind() == reflect.Pointer && rv.IsNil() {
			client = nil
		}
	}
	return &KongGatewayService{
		client:       client,
		serviceHost:  serviceHost,
		routeHost:    routeHost,
		repo:         repo,
		reconcileSec: reconcileSec,
		logger:       log.Default(),
	}
}

func (s *KongGatewayService) enabled() bool { return s != nil && s.client != nil }

func svcName(agentName string) string   { return "agent-" + agentName }
func routeName(agentName string) string { return "agent-" + agentName + "-route" }
func tagsFor(agentName string) []string { return []string{kongManagedTag, "agent:" + agentName} }

// serviceFor builds a Kong Service payload for an agent runtime, with timeout
// budgets tuned for long SSE streams (see kong*TimeoutMs constants above).
func serviceFor(name, host string, port int, tags []string) *kong.Service {
	return &kong.Service{
		Name:           name,
		Protocol:       "http",
		Host:           host,
		Port:           port,
		ConnectTimeout: kongConnectTimeoutMs,
		ReadTimeout:    kongReadTimeoutMs,
		WriteTimeout:   kongWriteTimeoutMs,
		Tags:           tags,
	}
}

// routeFor builds a Kong Route for an agent runtime, disabling request and
// response buffering so SSE streams flow through end-to-end without Kong
// materializing the full body. paths is the public path set (usually the
// tenant-scoped URLPath).
func routeFor(key, routeHost string, paths []string, svc *kong.ServiceRef, tags []string) *kong.Route {
	falseRef := false
	return &kong.Route{
		Name:              routeName(key),
		Hosts:             []string{routeHost},
		Paths:             paths,
		StripPath:         true,
		RequestBuffering:  &falseRef,
		ResponseBuffering: &falseRef,
		Service:           svc,
		Tags:              tags,
	}
}

// RouteURL returns the public URL for a route path segment, or empty if not
// configured. publicPath is a full path segment (e.g. "/zerone/assistant");
// a missing leading "/" is tolerated and auto-fixed.
func (s *KongGatewayService) RouteURL(publicPath string) string {
	if s == nil || s.routeHost == "" {
		return ""
	}
	if !strings.HasPrefix(publicPath, "/") {
		publicPath = "/" + publicPath
	}
	return fmt.Sprintf("https://%s%s", s.routeHost, publicPath)
}

// Register ensures a Kong Service and Route exist for the deployment key,
// pointing to the given upstream port, with the route matching publicPath.
// The operation is idempotent (upsert). Errors are logged but not returned so
// that gateway failures do not block agent deployment.
func (s *KongGatewayService) Register(ctx context.Context, key, publicPath string, hostPort int) error {
	if !s.enabled() {
		return nil
	}
	if !agentNameRe.MatchString(key) {
		s.logger.Printf("kong: skip register, invalid deploy key %q", key)
		return nil
	}
	if !strings.HasPrefix(publicPath, "/") {
		publicPath = "/" + publicPath
	}
	if !pathRe.MatchString(publicPath) {
		s.logger.Printf("kong: skip register %s, invalid public path %q", key, publicPath)
		return nil
	}

	sn, rn := svcName(key), routeName(key)
	tags := tagsFor(key)
	want := serviceFor(sn, s.serviceHost, hostPort, tags)

	existing, found, err := s.client.GetService(ctx, sn)
	if err != nil {
		s.logger.Printf("kong: get service %s failed: %v", sn, err)
		return nil
	}
	var svcID string
	if !found {
		created, err := s.client.CreateService(ctx, want)
		if err != nil {
			s.logger.Printf("kong: create service %s failed: %v", sn, err)
			return nil
		}
		svcID = created.ID
	} else {
		if _, err := s.client.UpdateService(ctx, sn, want); err != nil {
			s.logger.Printf("kong: update service %s failed: %v", sn, err)
		}
		svcID = existing.ID
	}

	paths := []string{publicPath}
	wantRoute := routeFor(key, s.routeHost, paths, &kong.ServiceRef{ID: svcID}, tags)
	if _, rFound, err := s.client.GetRoute(ctx, rn); err != nil {
		s.logger.Printf("kong: get route %s failed: %v", rn, err)
	} else if !rFound {
		if _, err := s.client.CreateRoute(ctx, wantRoute); err != nil {
			s.logger.Printf("kong: create route %s failed: %v", rn, err)
		}
	} else {
		if _, err := s.client.UpdateRoute(ctx, rn, wantRoute); err != nil {
			s.logger.Printf("kong: update route %s failed: %v", rn, err)
		}
	}

	return nil
}

// UpdateUpstream updates the upstream host and port for an existing deployment
// key. This is used when the agent's runtime host or port changes.
func (s *KongGatewayService) UpdateUpstream(ctx context.Context, key string, hostPort int) error {
	if !s.enabled() {
		return nil
	}
	if !agentNameRe.MatchString(key) {
		return nil
	}
	want := serviceFor(svcName(key), s.serviceHost, hostPort, tagsFor(key))
	if _, err := s.client.UpdateService(ctx, svcName(key), want); err != nil {
		s.logger.Printf("kong: update upstream for %s failed: %v", key, err)
	}
	return nil
}

// Deregister idempotently removes the Kong Route and Service for a deployment
// key. The route is deleted first to avoid foreign-key violations on Kong
// backends that do not cascade service deletes to attached routes.
func (s *KongGatewayService) Deregister(ctx context.Context, key string) error {
	if !s.enabled() {
		return nil
	}
	if !agentNameRe.MatchString(key) {
		return nil
	}
	sn, rn := svcName(key), routeName(key)
	if err := s.client.DeleteRoute(ctx, rn); err != nil {
		s.logger.Printf("kong: delete route %s failed: %v", rn, err)
	}
	if err := s.client.DeleteService(ctx, sn); err != nil {
		s.logger.Printf("kong: deregister %s failed: %v", key, err)
	}
	return nil
}

// isServiceable reports whether the agent should have a gateway route. The
// gateway follows the container lifecycle only; client-side visibility flags
// (desktop/mobile) deliberately play no role here.
func isServiceable(a agent.AgentConfig) bool {
	return a.DeploymentStatus == "running"
}

func agentNameFromService(svcName string) string {
	const prefix = "agent-"
	if len(svcName) > len(prefix) && svcName[:len(prefix)] == prefix {
		return svcName[len(prefix):]
	}
	return svcName
}

// Reconcile aligns Kong state with DB state. Agents are keyed by their
// tenant-scoped deploy key (DeployKey(TenantID, Name)); Kong-side entities are
// matched back via the "agent:<key>" service tags (parsing entity names back
// into (tenant, name) is unreliable, so the DB-side full key set is the source
// of truth for the diff). It returns the number of fixes applied.
func (s *KongGatewayService) Reconcile(ctx context.Context) (int, error) {
	if !s.enabled() {
		return 0, nil
	}
	agents, err := s.repo.ListAllForReconcile()
	if err != nil {
		return 0, fmt.Errorf("list agents: %w", err)
	}

	type agentRef struct {
		key        string
		publicPath string
		cfg        agent.AgentConfig
	}
	byKey := make(map[string]agentRef, len(agents))
	fixes := 0
	for i := range agents {
		a := agents[i]
		key := DeployKey(a.TenantID, a.Name)
		publicPath := URLPath(a.TenantID, a.Name)
		byKey[key] = agentRef{key: key, publicPath: publicPath, cfg: a}
		sn := svcName(key)
		svc, found, err := s.client.GetService(ctx, sn)
		if err != nil {
			s.logger.Printf("kong reconcile: get service %s failed: %v", sn, err)
			continue
		}
		if isServiceable(a) {
			if !found {
				if err := s.Register(ctx, key, publicPath, a.RuntimePort); err == nil {
					fixes++
				}
			} else {
				if svc.Host != s.serviceHost || svc.Port != a.RuntimePort {
					_ = s.UpdateUpstream(ctx, key, a.RuntimePort)
					fixes++
				}
				rn := routeName(key)
				route, rFound, err := s.client.GetRoute(ctx, rn)
				if err != nil {
					s.logger.Printf("kong reconcile: get route %s failed: %v", rn, err)
				} else if !rFound {
					if err := s.Register(ctx, key, publicPath, a.RuntimePort); err == nil {
						fixes++
					}
				} else if len(route.Hosts) != 1 || route.Hosts[0] != s.routeHost ||
					!slices.Equal(route.Paths, []string{publicPath}) {
					wantRoute := routeFor(key, s.routeHost, []string{publicPath}, route.Service, tagsFor(key))
					if _, err := s.client.UpdateRoute(ctx, rn, wantRoute); err != nil {
						s.logger.Printf("kong reconcile: update route %s failed: %v", rn, err)
					} else {
						fixes++
					}
				}
			}
		} else {
			if found {
				_ = s.Deregister(ctx, key)
				fixes++
			}
		}
	}

	existing, err := s.client.ListServicesByTag(ctx, kongManagedTag)
	if err != nil {
		return fixes, fmt.Errorf("list kong services: %w", err)
	}
	for _, svc := range existing {
		// Defense in depth: never touch a service that does not carry our
		// managed tag, even if the Kong backend ignored the ?tags= filter
		// and returned everything.
		if !slices.Contains(svc.Tags, kongManagedTag) {
			continue
		}
		// The managed key is "agent:<key>" (tagsFor). Fall back to parsing
		// the service name for Kong backends that drop tags.
		key := ""
		for _, tag := range svc.Tags {
			if rest, ok := strings.CutPrefix(tag, "agent:"); ok {
				key = rest
				break
			}
		}
		if key == "" {
			key = agentNameFromService(svc.Name)
		}
		ref, ok := byKey[key]
		if !ok || !isServiceable(ref.cfg) {
			_ = s.Deregister(ctx, key)
			fixes++
		}
	}
	return fixes, nil
}

// StartReconciler starts a background goroutine that periodically runs Reconcile.
func (s *KongGatewayService) StartReconciler(ctx context.Context) {
	if !s.enabled() {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(s.reconcileSec) * time.Second)
		defer ticker.Stop()
		s.runReconcileOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runReconcileOnce(ctx)
			}
		}
	}()
}

func (s *KongGatewayService) runReconcileOnce(ctx context.Context) {
	fixes, err := s.Reconcile(ctx)
	if err != nil {
		s.logger.Printf("kong reconcile error: %v", err)
	} else if fixes > 0 {
		s.logger.Printf("kong reconcile fixed %d drift(s)", fixes)
	}
}
