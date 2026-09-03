package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	repository "control-panel/internal/infrastructure/persistence"
)

// ErrDeployerNoDeploymentKey reports that the connected agent-deployer does
// not implement the v3.1.0 deploymentKey contract (deployer#18): it would
// silently key deployment resources by the bare rootAgentId and reintroduce
// cross-tenant same-name collisions. Deployment stays blocked until the
// deployer is upgraded (upgrade order in docs/configuration.md).
var ErrDeployerNoDeploymentKey = errors.New("agent-deployer does not support the deploymentKey contract (requires >= v3.1.0); upgrade agent-deployer and retry")

// DeploymentDTO represents the deployment status of an agent.
type DeploymentDTO struct {
	Status        string `json:"status"`
	Health        string `json:"health"`
	RuntimeURL    string `json:"runtimeUrl"`
	ContainerName string `json:"containerName"`
	DeployedAt    string `json:"deployedAt"`
	Message       string `json:"message"`
	HostPort      int    `json:"hostPort"`
	APIKey        string `json:"apiKey"`
}

// agentRepository defines the methods needed from the agent repository.
type agentRepository interface {
	GetByName(tenantID, name string) (*agent.AgentConfig, error)
	GetSubagents(agentID uint64) ([]string, error)
	GetKnowledgeDatasetIDsByAgent(agentID uint64) ([]string, error)
	Update(tenantID string, a *agent.AgentConfig) error
}

// toolRepository defines the methods needed from the tool repository.
type toolRepository interface {
	GetToolRecordsByAgent(agentID uint64) ([]*agent.Tool, error)
}

// skillRepository defines the methods needed from the skill repository.
type skillRepository interface {
	GetAgentSkills(agentID uint64) ([]string, error)
	GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error)
}

// providerService defines the methods needed from the provider service.
type providerService interface {
	GetByID(tenantID string, id uint64) (providerdomain.Provider, error)
	GetRawAPIKey(tenantID string, id uint64) (string, error)
}

// mcpService defines the methods needed from the MCP service.
type mcpService interface {
	GetClientMcpsByAgent(tenantID, name string) (map[string]*McpClientDTO, error)
}

// knowledgeService defines the methods needed from the knowledge service.
type knowledgeService interface {
	GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error)
}

// aigcConfigProvider defines the method needed from the AIGC config service.
type aigcConfigProvider interface {
	DeployerConfig(tenantID string) (*deployer.AigcConfig, error)
}

// AgentDeployerService handles agent deployment operations.
type AgentDeployerService struct {
	client        *deployer.Client
	publicHost    string
	upstreamHost  string // cfg.Deployer.DeployerURLHost; strict, no PublicHost fallback
	cdnHost       string
	encryptionKey string
	runtimeAPIKey string
	agentRepo     agentRepository
	toolRepo      toolRepository
	skillRepo     skillRepository
	providerSvc   providerService
	mcpSvc        mcpService
	knowledgeSvc  knowledgeService
	kongSvc       *KongGatewayService
	aigcSvc       aigcConfigProvider
	// chatPushAPIKey / chatPushPublicURL 同时非空时，部署请求注入 hub 段
	// （runtime 聊天记录回传配置）；任一为空则不注入 = 回传关闭。
	chatPushAPIKey    string
	chatPushPublicURL string
	healthProbe       func(ctx context.Context, publicHost string, port int) bool
	gatewayHealth     *sync.Map // deploy key (DeployKey) -> *gatewayHealthEntry
	// authMode selects the public-URL policy (issue #114): ModeBuiltin serves
	// bare "/<name>" and never surfaces the implicit default tenant.
	authMode AuthMode
}

// gatewayHealthEntry caches the result of a gateway health probe for an agent.
type gatewayHealthEntry struct {
	healthy  bool
	probedAt time.Time
}

const gatewayHealthTTL = 15 * time.Second

// probeURL performs an HTTP GET against the given URL with the supplied timeout
// and reports whether it returns HTTP 200.
func probeURL(ctx context.Context, url string, timeout time.Duration) bool {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// storeGatewayHealth records the gateway health for an agent (keyed by its
// tenant-scoped deploy key).
func (s *AgentDeployerService) storeGatewayHealth(key string, healthy bool) {
	if s == nil || s.gatewayHealth == nil {
		return
	}
	s.gatewayHealth.Store(key, &gatewayHealthEntry{healthy: healthy, probedAt: time.Now()})
}

// gatewayHealthy returns the cached gateway health for an agent (keyed by its
// tenant-scoped deploy key), or nil if there
// is no recent (< TTL) cached entry. It evicts stale entries on read.
func (s *AgentDeployerService) gatewayHealthy(key string) *bool {
	if s == nil || s.gatewayHealth == nil {
		return nil
	}
	raw, ok := s.gatewayHealth.Load(key)
	if !ok {
		return nil
	}
	entry, ok := raw.(*gatewayHealthEntry)
	if !ok {
		s.gatewayHealth.Delete(key)
		return nil
	}
	if time.Since(entry.probedAt) > gatewayHealthTTL {
		s.gatewayHealth.Delete(key)
		return nil
	}
	return &entry.healthy
}

// gatewayHealthCacheKey returns the cache key for gateway health entries: the
// tenant-scoped deploy key, so same-name agents in different tenants never
// share cache state.
func gatewayHealthCacheKey(tenantID, name string) string {
	return DeployKey(tenantID, name)
}

// gatewayURL returns the public gateway URL (mode-aware public path) for an
// agent, or "" when Kong is disabled.
func (s *AgentDeployerService) gatewayURL(tenantID, name string) string {
	if s == nil || s.kongSvc == nil || !s.kongSvc.enabled() {
		return ""
	}
	return s.kongSvc.RouteURL(s.publicPath(tenantID, name))
}

// publicPath is this service's mode-aware view of an agent's public path
// (issue #114): builtin "/<name>", casdoor "/<org>/<name>". Every public-URL
// surface (toDTO, gateway probes, route registration) goes through here so
// the two modes never diverge.
func (s *AgentDeployerService) publicPath(tenantID, agentName string) string {
	return PublicPath(s.authMode, tenantID, agentName)
}

// refreshGatewayHealth probes the Kong route for an agent and caches the result.
func (s *AgentDeployerService) refreshGatewayHealth(tenantID, name string) {
	gatewayURL := s.gatewayURL(tenantID, name)
	if gatewayURL == "" {
		return
	}
	ctx := context.Background()
	healthy := probeURL(ctx, gatewayURL+"/health", 3*time.Second)
	s.storeGatewayHealth(gatewayHealthCacheKey(tenantID, name), healthy)
}

// probeGatewayHealthy probes the Kong route for an agent and reports whether it
// is reachable. Unlike refreshGatewayHealth, it does not cache the result.
func (s *AgentDeployerService) probeGatewayHealthy(tenantID, name string) bool {
	gatewayURL := s.gatewayURL(tenantID, name)
	if gatewayURL == "" {
		return false
	}
	ctx := context.Background()
	return probeURL(ctx, gatewayURL+"/health", 3*time.Second)
}

// healthProbeURL 构造默认（无 Kong）健康探针 URL。JoinHostPort 保证 IPv6
// publicHost 产出合法的带方括号 URL（http://[2001:db8::1]:8080/health），
// Sprintf "%s:%d" 形式对 IPv6 是非法 URL。与 Kong 路由探测用的
// probeURL(ctx, url, timeout) 是两回事（后者接收完整 URL）。
func healthProbeURL(host string, port int) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/health"
}

// defaultHealthProbe performs an active HTTP check against the runtime /health endpoint.
func defaultHealthProbe(ctx context.Context, publicHost string, port int) bool {
	url := healthProbeURL(publicHost, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// NewAgentDeployerService creates a new AgentDeployerService.
// cdnHost is the public CDN base URL used to turn stored OSS keys into
// fetchable http(s) URLs when sending skills to the deployer.
// chatPushAPIKey / chatPushPublicURL（来自 CHAT_PUSH_API_KEY /
// CHAT_PUSH_PUBLIC_URL）同时非空时，部署请求注入 runtime 聊天记录回传配置。
func NewAgentDeployerService(client *deployer.Client, publicHost, upstreamHost, cdnHost, encryptionKey, runtimeAPIKey string, knowledgeSvc *KnowledgeService, kongSvc *KongGatewayService, aigcSvc *AigcConfigService, chatPushAPIKey, chatPushPublicURL string, authMode AuthMode) *AgentDeployerService {
	s := &AgentDeployerService{
		client:            client,
		publicHost:        publicHost,
		upstreamHost:      upstreamHost,
		cdnHost:           cdnHost,
		encryptionKey:     encryptionKey,
		runtimeAPIKey:     runtimeAPIKey,
		agentRepo:         repository.NewAgentRepository(),
		toolRepo:          repository.NewToolRepository(),
		skillRepo:         repository.NewSkillRepository(),
		providerSvc:       NewProviderService(encryptionKey),
		mcpSvc:            NewMcpService(encryptionKey),
		knowledgeSvc:      knowledgeSvc,
		kongSvc:           kongSvc,
		authMode:          authMode,
		chatPushAPIKey:    chatPushAPIKey,
		chatPushPublicURL: chatPushPublicURL,
		healthProbe:       defaultHealthProbe,
		gatewayHealth:     &sync.Map{},
	}
	if aigcSvc != nil {
		s.aigcSvc = aigcSvc
	}
	return s
}

// kongEnabled mirrors kong_gateway.go's nil-safe check.
func (s *AgentDeployerService) kongEnabled() bool {
	return s.kongSvc != nil && s.kongSvc.enabled()
}

// probeHost selects the health-probe target per mode (spec D1): Kong mode
// deliberately probes the public path — a deployment public-reachability
// signal; no-Kong mode probes the internal deployer hostname (no hairpin).
func (s *AgentDeployerService) probeHost() (string, error) {
	if s.kongEnabled() {
		return s.publicHost, nil
	}
	if s.upstreamHost == "" {
		return "", fmt.Errorf("internal upstream host unavailable (AGENT_DEPLOYER_URL not configured)")
	}
	return s.upstreamHost, nil
}

// WaitForHealthy polls the deployer and actively probes /health until the agent
// is ready or timeout is reached. It returns the current host port.
func (s *AgentDeployerService) WaitForHealthy(ctx context.Context, name string, timeout time.Duration) (int, error) {
	probeHost, err := s.probeHost()
	if err != nil {
		return 0, err // fail closed: never dial without a valid probe host
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		st, err := s.client.GetStatus(ctx, name)
		if err == nil {
			if st.Health == "healthy" {
				return st.HostPort, nil
			}
			if st.HostPort > 0 && s.healthProbe(ctx, probeHost, st.HostPort) {
				return st.HostPort, nil
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("agent %s not healthy within %s", name, timeout)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Deploy deploys an agent container. If force is true, it will recreate the container if it already exists.
// rotateKey requests a fresh runtime token; it only takes effect when the
// container is actually (re)created (force=true or first deploy), matching
// the deployer's idempotent-return semantics for existing containers.
func (s *AgentDeployerService) Deploy(tenantID, name string, force bool, rotateKey bool) (*DeploymentDTO, error) {
	name = NormalizeAgentName(name)
	// Load agent from DB
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Validate agent is deployable
	if err := s.validateDeployable(agentCfg); err != nil {
		return nil, err
	}

	// Load provider
	p, err := s.providerSvc.GetByID(tenantID, *agentCfg.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("load provider failed: %w", err)
	}

	// Build create request. The complete agent graph (root + mounted
	// subagents with their own capabilities) is resolved inside
	// buildCreateRequest via loadAgentGraph; MCP servers load per-agent.
	ctx := context.Background()

	// Lifecycle deployer calls (get/status/start/stop/delete, token
	// provisioning) address the container by the tenant-scoped deploy key
	// (<org>-<name>); only DB lookups and the runtime agent id use the bare
	// name (issue #114).
	key := DeployKey(tenantID, name)

	// Capability gate (issue #114): the v3.1 deploymentKey split must be live
	// before we send a bare rootAgentId — a v3.0.x deployer silently keys
	// containers by rootAgentId, reintroducing cross-tenant collisions. Fail
	// closed; the probe is a pre-docker 400 on both generations.
	supported, err := s.client.SupportsDeploymentKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("deployer capability check failed: %w", err)
	}
	if !supported {
		return nil, ErrDeployerNoDeploymentKey
	}

	// Decide the runtime token before building the request. control-panel is
	// the sole generator and keeper of runtime tokens; the deployer only
	// injects the value it is given as ZERONE_AGENT_HTTP_API_KEY.
	token, err := s.resolveRuntimeToken(ctx, key, agentCfg, force, rotateKey)
	if err != nil {
		return nil, err
	}

	// Build the create request first: graph construction and capability
	// validation (loadAgentGraph) are pure DB reads with no deployer side
	// effects, so a validation failure below returns without touching Kong —
	// a misconfigured child must not take a healthy agent's route offline.
	req, err := s.buildCreateRequest(ctx, tenantID, agentCfg, p)
	if err != nil {
		return nil, fmt.Errorf("build create request failed: %w", err)
	}

	req.RuntimeToken = token
	s.resolveMcpHeaders(req, token)

	// Call deployer. Nothing is deregistered from Kong before this point: a
	// pre-rejection (4xx protocol validation, 503 runtime floor) must leave a
	// currently healthy deployment fully intact — container, DB status and
	// gateway route alike.
	resp, err := s.client.CreateAgent(ctx, req, force)
	if err != nil {
		// Mid-flight failures (5xx, network) may have left a half-created
		// container behind: archive it, drop the now backend-less Kong route
		// (a lingering route would only serve 503s) and mark the deployment
		// errored. Pre-rejections (4xx protocol validation, 503 runtime
		// floor) happen before the deployer touches Docker, so any existing
		// container is still healthy and must stay exactly as is.
		if !deployerPreRejected(err) {
			_ = s.client.DeleteAgent(ctx, key, false)
			if s.kongSvc != nil {
				_ = s.kongSvc.Deregister(ctx, key)
			}
			_ = s.updateStatus(tenantID, agentCfg, "error", 0, nil)
		}
		return nil, fmt.Errorf("deploy agent failed: %w", err)
	}

	// The create succeeded, so the previous container is gone (or, on an
	// idempotent return, the existing container stays put): drop the old Kong
	// route so it cannot keep pointing at the old port. registerWhenHealthy
	// below re-registers against the new container once healthy; for the
	// idempotent case this deregister→re-register window is far shorter than
	// deregistering before the create call would be.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, key)
	}

	// Update DB with deployment info and persist the runtime token encrypted.
	// The persisted value is the token we sent: the deployer no longer
	// generates tokens, and its response merely echoes the request value.
	deployedAt := time.Now()
	encryptedToken, err := providerdomain.Encrypt(token, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt runtime token failed: %w", err)
	}
	agentCfg.RuntimeToken = encryptedToken
	if err := s.updateStatus(tenantID, agentCfg, resp.Status, resp.HostPort, &deployedAt); err != nil {
		return nil, fmt.Errorf("update deployment status failed: %w", err)
	}

	dto := s.toDTO(tenantID, name, resp.Status, "", resp.ContainerName, resp.HostPort, &deployedAt, "")
	if resp.Status == "running" && resp.HostPort > 0 {
		dto.APIKey = token
		// Register Kong route asynchronously once the runtime reports healthy.
		if s.kongSvc != nil {
			go s.registerWhenHealthy(tenantID, name, resp.HostPort)
		}
	}
	return dto, nil
}

// resolveRuntimeToken decides which runtime token a (re)deployment should use.
// control-panel generates and keeps the tokens itself; the deployer never
// reveals or regenerates them.
//
//   - force + rotateKey: always mint a fresh token (old one dies with the
//     rebuilt container).
//   - stored token exists and no rotation: reuse it, so redeploys keep
//     existing clients working.
//   - no usable stored token: mint a fresh one — except when the deployer
//     already has a container for this agent and we are not rebuilding it.
//     That container holds an unrecoverable token, so we refuse and ask the
//     caller to force-redeploy, letting both sides converge on a fresh token.
func (s *AgentDeployerService) resolveRuntimeToken(ctx context.Context, key string, cfg *agent.AgentConfig, force, rotateKey bool) (string, error) {
	stored := ""
	if cfg.RuntimeToken != "" {
		// A decrypt failure (e.g. rotated ENCRYPTION_KEY) is treated as
		// "no usable token", mirroring GetStatus which ignores the error.
		stored, _ = providerdomain.Decrypt(cfg.RuntimeToken, s.encryptionKey)
	}

	if stored != "" && !(force && rotateKey) {
		return stored, nil
	}

	if !force && stored == "" {
		if _, err := s.client.GetAgent(ctx, key); err == nil {
			return "", fmt.Errorf("agent %s 在 deployer 上已存在容器，但本地无 Runtime Token 记录（可能因加密密钥变更或数据丢失），无法恢复；请强制重新部署，将生成新的 API Key", key)
		}
	}

	return generateRuntimeToken()
}

// agentRuntimeTokenPlaceholder is the placeholder allowed in MCP server
// headers (notably the built-in knowledge MCP's Authorization header). The
// deployer no longer expands it when writing agents.yaml, so control-panel
// substitutes the real token before sending the create request.
const agentRuntimeTokenPlaceholder = "$agent_runtime_token"

// agentIdentityHeader mirrors the handler-side constant of the same name
// (internal/handler knowledge_mcp.go): the deployment-trusted per-agent
// identity carried on the built-in knowledge MCP's connection headers.
// Both packages hold their own copy — changes must stay in sync.
const agentIdentityHeader = "X-Agent-Id"

// resolveMcpHeaders replaces the $agent_runtime_token placeholder in MCP
// server header values with the actual runtime token being deployed, across
// every agent node of the graph — mounted agents carry their own MCP servers
// and their headers need the same substitution as the root's. Header maps
// are rebuilt rather than mutated in place so the MCP DTOs returned by the
// MCP service are never modified.
func (s *AgentDeployerService) resolveMcpHeaders(req *deployer.CreateAgentRequest, token string) {
	for i := range req.Agents {
		for name, mcp := range req.Agents[i].McpServers {
			if len(mcp.Headers) == 0 {
				continue
			}
			headers := make(map[string]string, len(mcp.Headers))
			for k, v := range mcp.Headers {
				headers[k] = strings.ReplaceAll(v, agentRuntimeTokenPlaceholder, token)
			}
			mcp.Headers = headers
			req.Agents[i].McpServers[name] = mcp
		}
	}
}

// knowledgeMcpHeaders returns the connection headers for one MCP server,
// injecting the deployment-trusted per-agent identity on the built-in
// knowledge MCP (issue #111 review P1-1). The hub-side authorizer resolves
// the identity tenant-scoped and grants only that node's own dataset
// bindings, so every graph node — root included — carries its DB bare name
// (never the root's deploy key). The runtime can only call knowledge_search
// with dataset_ids arguments and cannot forge connection headers, which is
// what makes this header deployment-trusted. The key is owned exclusively
// by the deployment: a same-named key configured on the MCP server is
// overridden. The map is rebuilt rather than mutated so the MCP service
// DTO stays untouched, mirroring resolveMcpHeaders (which in turn preserves
// this key — its value carries no placeholder).
func knowledgeMcpHeaders(mcpName string, src map[string]string, agentName string) map[string]string {
	if mcpName != "knowledge" {
		return src
	}
	headers := make(map[string]string, len(src)+1)
	for k, v := range src {
		headers[k] = v
	}
	headers[agentIdentityHeader] = agentName
	return headers
}

// generateRuntimeToken returns a cryptographically random 32-char hex token,
// matching the format the deployer used to mint so existing clients see no
// format change.
func generateRuntimeToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate runtime token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// registerWhenHealthy waits for the agent to become healthy and then registers
// its Kong route under the tenant-scoped key/path. After registration it probes
// the gateway route with retries so route propagation delay is accounted for.
// Errors are logged, not returned.
func (s *AgentDeployerService) registerWhenHealthy(tenantID, name string, hostPort int) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	key := DeployKey(tenantID, name)
	publicPath := s.publicPath(tenantID, name)
	register := func(port int) {
		_ = s.kongSvc.Register(ctx, key, publicPath, port)
	}
	var hp int
	var err error
	if hp, err = s.WaitForHealthy(ctx, key, 120*time.Second); err == nil {
		register(hp)
	} else if hostPort > 0 {
		register(hostPort)
	} else {
		return
	}
	if s.kongSvc == nil || !s.kongSvc.enabled() {
		return
	}
	gatewayURL := s.kongSvc.RouteURL(publicPath)
	if gatewayURL == "" {
		return
	}
	healthURL := gatewayURL + "/health"
	for i := 0; i < 3; i++ {
		if probeURL(ctx, healthURL, 3*time.Second) {
			s.storeGatewayHealth(key, true)
			return
		}
		if i < 2 {
			time.Sleep(3 * time.Second)
		}
	}
	log.Printf("gateway health check failed for agent %s: %s", key, gatewayURL)
	s.storeGatewayHealth(key, false)
}

// GetStatus queries the deployer for the current status of an agent container.
func (s *AgentDeployerService) GetStatus(tenantID, name string) (*DeploymentDTO, error) {
	name = NormalizeAgentName(name)
	// Load agent from DB
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Deployer calls use the tenant-scoped deploy key.
	key := DeployKey(tenantID, name)
	ctx := context.Background()
	statusResp, err := s.client.GetAgent(ctx, key)
	if err != nil {
		// If deployer says not found, return not_found status
		return s.toDTO(tenantID, name, "not_found", "", "", 0, agentCfg.DeployedAt, "未部署或已被清理"), nil
	}

	// If status is running, also query health
	health := ""
	if statusResp.Status == "running" {
		if healthResp, err := s.client.GetStatus(ctx, key); err == nil {
			health = healthResp.Health
			statusResp.HostPort = healthResp.HostPort
		} else {
			log.Printf("GetStatus: health query failed for agent %s: %v", key, err)
		}
	}

	// When Kong is enabled and the container is running+healthy, also factor in gateway health.
	var gatewayMessage string
	if s.kongSvc != nil && s.kongSvc.enabled() && statusResp.Status == "running" && health == "healthy" {
		if cached := s.gatewayHealthy(key); cached != nil {
			if !*cached {
				health = "unhealthy"
				gatewayMessage = "Kong 网关路由不可达"
			}
		} else {
			// No recent cache yet. Probe synchronously so we do not report
			// healthy while the Kong route is still propagating or broken.
			// Only store successful results; failures during propagation are
			// transient and should render as 'starting', not 'unhealthy'.
			if s.probeGatewayHealthy(tenantID, name) {
				s.storeGatewayHealth(key, true)
			} else {
				health = "starting"
				gatewayMessage = "Kong 网关路由检测中"
			}
		}
	}

	// Sync deployment status and runtime_port from the deployer so any stale or
	// unexpected cached status (e.g. a manually/archived value in the DB) is
	// overwritten with the source of truth. Only stable Docker states are
	// persisted: transient ones (created, restarting, paused, removing,
	// unknown) would otherwise stick in the DB until the next GetStatus call
	// and make the Kong reconciler tear down the route of a healthy agent.
	if isStableDeploymentStatus(statusResp.Status) &&
		(statusResp.Status != agentCfg.DeploymentStatus || statusResp.HostPort != agentCfg.RuntimePort) {
		agentCfg.DeploymentStatus = statusResp.Status
		agentCfg.RuntimePort = statusResp.HostPort
		if err := s.agentRepo.Update(tenantID, agentCfg); err != nil {
			log.Printf("GetStatus: failed to update agent %s: %v", name, err)
		}
	}

	dto := s.toDTO(tenantID, name, statusResp.Status, health, statusResp.ContainerName, statusResp.HostPort, agentCfg.DeployedAt, gatewayMessage)
	if statusResp.Status == "running" && statusResp.HostPort > 0 {
		apiKey := ""
		if agentCfg.RuntimeToken != "" {
			apiKey, _ = providerdomain.Decrypt(agentCfg.RuntimeToken, s.encryptionKey)
		}
		dto.APIKey = apiKey
	}
	return dto, nil
}

// Stop stops an agent container.
func (s *AgentDeployerService) Stop(tenantID, name string) error {
	name = NormalizeAgentName(name)
	// Load agent from DB
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	ctx := context.Background()
	if err := s.client.StopAgent(ctx, DeployKey(tenantID, name)); err != nil {
		return fmt.Errorf("stop agent failed: %w", err)
	}

	// Update DB status to stopped, clear RuntimePort but keep encrypted token so
	// it can be shown again when the container is restarted.
	if err := s.updateStatus(tenantID, agentCfg, "stopped", 0, agentCfg.DeployedAt); err != nil {
		return fmt.Errorf("update status failed: %w", err)
	}

	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, DeployKey(tenantID, name))
	}

	return nil
}

// Start starts a stopped agent container and refreshes its port/status.
func (s *AgentDeployerService) Start(tenantID, name string) (*DeploymentDTO, error) {
	name = NormalizeAgentName(name)
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	ctx := context.Background()
	key := DeployKey(tenantID, name)
	// Deregister any existing Kong route before restarting.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, key)
	}
	if err := s.client.StartAgent(ctx, key); err != nil {
		return nil, fmt.Errorf("start agent failed: %w", err)
	}

	// Query the deployer for updated status/port
	statusResp, err := s.client.GetAgent(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get status after start failed: %w", err)
	}

	if err := s.updateStatus(tenantID, agentCfg, statusResp.Status, statusResp.HostPort, agentCfg.DeployedAt); err != nil {
		return nil, fmt.Errorf("update status failed: %w", err)
	}

	dto := s.toDTO(tenantID, name, statusResp.Status, "", statusResp.ContainerName, statusResp.HostPort, agentCfg.DeployedAt, "")
	if statusResp.Status == "running" && statusResp.HostPort > 0 && agentCfg.RuntimeToken != "" {
		dto.APIKey, _ = providerdomain.Decrypt(agentCfg.RuntimeToken, s.encryptionKey)
		// Register Kong route asynchronously once the runtime reports healthy.
		if s.kongSvc != nil {
			go s.registerWhenHealthy(tenantID, name, statusResp.HostPort)
		}
	}
	return dto, nil
}

// Delete archives an agent container (container removed, data retained).
// The deployer marks the agent as status=archived afterwards.
func (s *AgentDeployerService) Delete(tenantID, name string) error {
	return s.deleteWithPurge(tenantID, name, false)
}

// Purge permanently deletes an agent container and its data.
func (s *AgentDeployerService) Purge(tenantID, name string) error {
	return s.deleteWithPurge(tenantID, name, true)
}

func (s *AgentDeployerService) deleteWithPurge(tenantID, name string, purge bool) error {
	name = NormalizeAgentName(name)
	// Load agent from DB
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	ctx := context.Background()
	if err := s.client.DeleteAgent(ctx, DeployKey(tenantID, name), purge); err != nil {
		return fmt.Errorf("delete agent failed: %w", err)
	}

	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, DeployKey(tenantID, name))
	}

	// Update DB status. When archived we keep the record with status=archived so
	// the user can redeploy and recover the retained data. When purged we wipe
	// the record entirely since both container and data are gone.
	if purge {
		agentCfg.RuntimeToken = ""
		if err := s.updateStatus(tenantID, agentCfg, "", 0, nil); err != nil {
			return fmt.Errorf("update status failed: %w", err)
		}
	} else {
		agentCfg.RuntimePort = 0
		// Keep runtime token encrypted; it is still valid after redeploy and is
		// decrypted and returned again by GetStatus when the container is back.
		if err := s.updateStatus(tenantID, agentCfg, "archived", 0, agentCfg.DeployedAt); err != nil {
			return fmt.Errorf("update status failed: %w", err)
		}
	}

	return nil
}

// ── Helpers ──────────────────────────────────────────────────────

// validateDeployable checks if the agent has the required fields for deployment.
func (s *AgentDeployerService) validateDeployable(cfg *agent.AgentConfig) error {
	if cfg.ProviderID == nil || *cfg.ProviderID == 0 {
		return fmt.Errorf("agent has no provider configured")
	}
	if cfg.ModelID == "" {
		return fmt.Errorf("agent has no model configured")
	}
	return nil
}

// definitionOpts carries graph-position context into buildAgentDefinition.
// The root node is the only node allowed to carry the runtime-global fields
// (Model, MaxSessionQueries, PermissionMode) — the deployer v3 rejects them
// on children. Root and child nodes all carry bare runtime names (issue
// #114); the tenant-scoped identity lives in the dedicated deploymentKey.
type definitionOpts struct {
	isRoot bool
}

// buildAgentDefinition assembles one agent's complete AgentDefinition: its
// own tools allow-list (extended with SDK-qualified MCP tool names) and
// agent-local deny list, custom tool artifacts, skills, MCP servers,
// knowledge datasets and subagent id references. Mounted agents never
// inherit or fall back to the root's capabilities — an empty relation stays
// empty. It also returns the agent's subagent names so loadAgentGraph can
// traverse the closure without a second query.
func (s *AgentDeployerService) buildAgentDefinition(ctx context.Context, tenantID string, cfg *agent.AgentConfig, opts definitionOpts) (*deployer.AgentDefinition, []string, error) {
	name := cfg.Name
	if opts.isRoot {
		// Root carries the bare runtime agent id (issue #114), normalized so
		// it satisfies the deployer's sanitized-name contract and matches
		// RootAgentID exactly. Children keep raw DB names — subagent id
		// references and definitions both use cfg.Name.
		name = NormalizeAgentName(cfg.Name)
	}
	def := &deployer.AgentDefinition{
		Name:         name,
		Description:  firstNonEmpty(cfg.Description["zh"], cfg.Description["en"], cfg.Name),
		SystemPrompt: cfg.SystemPrompt,
		MaxTurns:     intPtr(cfg.MaxTurns),
	}
	if opts.isRoot {
		def.Model = cfg.ModelID
		def.MaxSessionQueries = cfg.MaxSessionQueries
		def.PermissionMode = cfg.PermissionMode
	}
	// Agent-local deny list (issue #111): root and children are isomorphic —
	// each node carries exactly its own user-configured disallowedTools on top
	// of its allow-list; nil/empty omits the key via DTO omitempty.
	def.DisallowedTools = cfg.DisallowedTools

	// Custom tool artifacts (issue #88): Tools stays the complete allow-list;
	// CustomTools only carries source=custom && ready rows, sorted by name so
	// the request and generated YAML are reproducible.
	toolRecords, err := s.toolRepo.GetToolRecordsByAgent(cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load tools failed: %w", err)
	}
	// Load the agent's own MCP servers (already decrypted). MCP mounts are
	// per-agent: a mounted agent's servers come from its own relations and
	// are never inherited from the root.
	mcpServers, err := s.mcpSvc.GetClientMcpsByAgent(tenantID, cfg.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("load mcp servers failed: %w", err)
	}
	toolNames := make([]string, 0, len(toolRecords))
	customToolSources := make([]deployer.ToolSource, 0)
	missingCustom := make([]string, 0)
	for _, t := range toolRecords {
		toolNames = append(toolNames, t.Name)
		if t.Source != agent.ToolSourceCustom {
			continue
		}
		if t.ArtifactStatus() != agent.ToolArtifactReady {
			missingCustom = append(missingCustom, t.Name)
			continue
		}
		customToolSources = append(customToolSources, deployer.ToolSource{
			Name:     t.Name,
			URL:      s.buildArtifactURL(t.FileURL),
			Hash:     t.FileHash,
			FileName: t.FileName,
		})
	}
	toolNames = appendMcpToolNames(toolNames, mcpServers)
	if len(missingCustom) > 0 {
		return nil, nil, fmt.Errorf("自定义工具缺少制品文件，无法部署：%s。请先在工具页补传文件或解除挂载", strings.Join(missingCustom, "、"))
	}
	if len(customToolSources) > 0 && s.cdnHost == "" {
		return nil, nil, fmt.Errorf("未配置 OSS_CDN_HOST，无法为自定义工具构造下载地址（共 %d 个）。请配置 CDN 后重新部署", len(customToolSources))
	}
	sort.Strings(toolNames)
	sort.Slice(customToolSources, func(i, j int) bool { return customToolSources[i].Name < customToolSources[j].Name })
	def.Tools = toolNames
	def.CustomTools = customToolSources

	// Build skill sources from full skill records. A node that declares any
	// skill must also set settingSources=["user"] (deployer v3 contract:
	// skills are installed per-agent and scanned at user level).
	skills, err := s.skillRepo.GetAgentSkillsFull(cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load skills failed: %w", err)
	}
	skillSources := make([]deployer.SkillSource, 0, len(skills))
	for i, sk := range skills {
		// Review F4 (issue #111): incomplete artifact metadata must fail the
		// deploy instead of silently shipping a partial capability closure —
		// same fail-fast contract as custom tools above. A nameless dirty row
		// is identified by its 1-based position.
		if sk.Name == "" || sk.URL == "" || sk.FileHash == "" {
			label := sk.Name
			if label == "" {
				label = fmt.Sprintf("第 %d 个技能", i+1)
			}
			return nil, nil, fmt.Errorf("Agent %q 挂载的技能 %q 缺少制品元数据（name/url/hash 不完整），无法部署。请先在技能页补传文件或解除挂载", cfg.Name, label)
		}
		skillSources = append(skillSources, deployer.SkillSource{
			Name: sk.Name,
			URL:  s.buildArtifactURL(sk.URL),
			Hash: sk.FileHash,
		})
	}
	def.Skills = skillSources
	if len(skillSources) > 0 {
		def.SettingSources = []string{"user"}
	}

	// Build MCP server configs (headers are already decrypted by the MCP service).
	mcpServerConfigs := make(map[string]deployer.McpServerConfig, len(mcpServers))
	for name, mcp := range mcpServers {
		if name == "knowledge" && strings.TrimSpace(mcp.URL) == "" {
			return nil, nil, fmt.Errorf("内置 knowledge MCP 未配置可达地址，请设置 KNOWLEDGE_MCP_URL（完整路径需包含 /api/v1/knowledge/mcp），重启 Hub 后重新部署 Agent")
		}
		mcpServerConfigs[name] = deployer.McpServerConfig{
			Type:    mcp.Type,
			URL:     mcp.URL,
			Headers: knowledgeMcpHeaders(name, mcp.Headers, cfg.Name),
		}
	}
	def.McpServers = mcpServerConfigs

	// Load bound knowledge datasets and resolve their metadata.
	datasetIDs, err := s.agentRepo.GetKnowledgeDatasetIDsByAgent(cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load agent knowledge datasets failed: %w", err)
	}
	agentDatasets := make(map[string]string, len(datasetIDs))
	for _, id := range datasetIDs {
		ds, err := s.knowledgeSvc.GetDataset(ctx, id)
		if err != nil {
			// Review F4 (issue #111): unavailable dataset metadata must fail
			// the deploy instead of silently shipping a partial closure.
			return nil, nil, fmt.Errorf("Agent %q 绑定的知识库 %s 不存在或元数据不可用，无法部署。请解除绑定后重试", cfg.Name, id)
		}
		desc := strings.TrimSpace(stringOrEmpty((*ds)["description"]))
		if desc == "" {
			return nil, nil, fmt.Errorf("dataset %s 缺少 description，无法下发给 Agent 运行时，请完善知识库描述后重新部署", id)
		}
		agentDatasets[id] = desc
	}
	def.Datasets = agentDatasets

	// Subagent id references (deployer v3: pure ids to sibling graph entries).
	subagentNames, err := s.agentRepo.GetSubagents(cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load subagents failed: %w", err)
	}
	def.Subagents = subagentNames

	return def, subagentNames, nil
}

// loadAgentGraph resolves the deploy closure of root: root plus its directly
// mounted subagents, each as a complete AgentDefinition. Depth is fixed at
// one delegation level by the runtime; any deeper relation, cycle, or
// dangling reference fails explicitly instead of being silently skipped.
func (s *AgentDeployerService) loadAgentGraph(ctx context.Context, tenantID string, rootCfg *agent.AgentConfig) ([]deployer.AgentDefinition, error) {
	// The root's deployer graph identity is its bare agent id (issue #114);
	// a subagent with the same name would collide as a duplicate agents[]
	// entry, so guard it hub-side with a clear message.
	rootBareID := NormalizeAgentName(rootCfg.Name)

	rootDef, rootSubNames, err := s.buildAgentDefinition(ctx, tenantID, rootCfg, definitionOpts{isRoot: true})
	if err != nil {
		return nil, fmt.Errorf("构造根 Agent 定义失败: %w", err)
	}
	defs := []deployer.AgentDefinition{*rootDef}

	for _, subName := range rootSubNames {
		if subName == rootCfg.Name {
			return nil, fmt.Errorf("Agent %q 不能挂载自己作为子 Agent", rootCfg.Name)
		}
		if subName == rootBareID {
			return nil, fmt.Errorf("子 Agent %q 与根 Agent 的运行时标识 %q 冲突，请重命名", subName, rootBareID)
		}
		if !validAgentNamePattern.MatchString(subName) { // 存量非 canonical 防御
			return nil, fmt.Errorf("子 Agent %q 标识不合法（仅小写字母、数字、连字符），无法部署", subName)
		}
		sub, err := s.agentRepo.GetByName(tenantID, subName)
		if err != nil {
			return nil, fmt.Errorf("挂载的子 Agent %q 不存在，请先解除挂载或创建该 Agent", subName)
		}
		subDef, subSubNames, err := s.buildAgentDefinition(ctx, tenantID, sub, definitionOpts{})
		if err != nil {
			return nil, fmt.Errorf("构造子 Agent %q 定义失败: %w", subName, err)
		}
		for _, grand := range subSubNames {
			if grand == rootCfg.Name {
				return nil, fmt.Errorf("检测到子 Agent 挂载环：%q 与 %q 互相挂载", subName, rootCfg.Name)
			}
			return nil, fmt.Errorf("子 Agent %q 自身还挂载了 %q：运行时仅支持一层委托，请先解除嵌套挂载", subName, grand)
		}
		defs = append(defs, *subDef)
	}
	return defs, nil
}

// buildCreateRequest builds the deployer v3 CreateAgentRequest: the complete
// agent graph resolved by loadAgentGraph plus the runtime-global provider
// config (and AIGC / chat-pushback sections). Graph violations — missing,
// cyclic or too-deep mounts, per-agent capability errors — fail the deploy
// explicitly inside loadAgentGraph.
func (s *AgentDeployerService) buildCreateRequest(
	ctx context.Context,
	tenantID string,
	cfg *agent.AgentConfig,
	providerDTO providerdomain.Provider,
) (*deployer.CreateAgentRequest, error) {
	agents, err := s.loadAgentGraph(ctx, tenantID, cfg)
	if err != nil {
		return nil, err
	}
	// Build provider config
	baseURL := providerDTO.BaseURL()
	apiKey, _ := s.providerSvc.GetRawAPIKey(tenantID, providerDTO.ID())
	if cfg.FieldOverrides != "" {
		overrides, err := decryptFieldOverrides(cfg.FieldOverrides, *cfg.ProviderID, s.encryptionKey)
		if err == nil {
			if v, ok := overrides["base_url"]; ok && v != "" {
				baseURL = v
			}
			if v, ok := overrides["api_key"]; ok && v != "" {
				apiKey = v
			}
		} else {
			log.Printf("buildCreateRequest: decrypt field overrides failed for agent %s: %v", cfg.Name, err)
		}
	}

	providerConfig := deployer.ProviderConfig{
		Protocol:     mapProtocolToRuntime(providerDTO.Protocol()),
		BaseURL:      baseURL,
		LockedAPIKey: apiKey,
	}

	req := &deployer.CreateAgentRequest{
		// v3.1 split (issue #114): rootAgentId carries only the runtime agent
		// graph identity (bare name); the tenant-scoped DeployKey moved to
		// the dedicated deploymentKey field, which keys containers,
		// directories and all lifecycle addressing on the deployer side.
		RootAgentID:   NormalizeAgentName(cfg.Name),
		DeploymentKey: DeployKey(tenantID, cfg.Name),
		Agents:        agents,
		Provider:      providerConfig,
	}

	if err := s.applyAigc(req, tenantID); err != nil {
		return nil, err
	}

	s.applyHub(req, tenantID)

	return req, nil
}

// appendMcpToolNames adds the SDK-qualified names of probed MCP tools to an
// agent's allow-list. agent-sdk registers MCP tools as
// `mcp__<server-name>__<tool-name>` and then applies allowedTools by exact
// name; omitting these entries makes a healthy MCP connection invisible to
// the model whenever the agent has any ordinary tool allow-list.
func appendMcpToolNames(toolNames []string, mcpServers map[string]*McpClientDTO) []string {
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		seen[name] = struct{}{}
	}
	for serverName, server := range mcpServers {
		if server == nil {
			continue
		}
		for _, tool := range server.Tools {
			toolName := strings.TrimSpace(tool.Name)
			if toolName == "" {
				continue
			}
			qualified := "mcp__" + serverName + "__" + toolName
			if _, ok := seen[qualified]; ok {
				continue
			}
			toolNames = append(toolNames, qualified)
			seen[qualified] = struct{}{}
		}
	}
	return toolNames
}

// applyHub injects the runtime chat-record pushback config (agents.yaml hub
// section via the deployer) when both the push key and this hub's public URL
// are configured. Either one missing leaves the section omitted (= pushback
// disabled, backwards compatible).
//
// tenantID 是该 agent 的部署租户（builtin 恒 "default"，casdoor 为部署时的
// org），作为可信 org 下发（issue #78）：runtime 以此为回传会话盖章租户，
// 不再采信调用方伪造的 X-Org 头。
func (s *AgentDeployerService) applyHub(req *deployer.CreateAgentRequest, tenantID string) {
	if s.chatPushAPIKey == "" || s.chatPushPublicURL == "" {
		return
	}
	req.Hub = &deployer.HubConfig{
		Enabled:     true,
		BaseURL:     s.chatPushPublicURL,
		ChatPushKey: s.chatPushAPIKey,
		Org:         tenantID,
	}
}

// applyAigc injects the GB 45438-2025 labeling config when configured.
// A nil service or missing config leaves the request untouched (omitempty).
func (s *AgentDeployerService) applyAigc(req *deployer.CreateAgentRequest, tenantID string) error {
	if s.aigcSvc == nil {
		return nil
	}
	cfg, err := s.aigcSvc.DeployerConfig(tenantID)
	if err != nil {
		return fmt.Errorf("load aigc config: %w", err)
	}
	if cfg != nil {
		req.Aigc = cfg
	}
	return nil
}

// updateStatus updates the agent's deployment status in the database.
func (s *AgentDeployerService) updateStatus(tenantID string, cfg *agent.AgentConfig, status string, port int, deployedAt *time.Time) error {
	cfg.DeploymentStatus = status
	cfg.RuntimePort = port
	cfg.DeployedAt = deployedAt
	return s.agentRepo.Update(tenantID, cfg)
}

// runtimeURL returns the public runtime URL for an agent given its host port.
func (s *AgentDeployerService) runtimeURL(port int) string {
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("http://%s:%d", s.publicHost, port)
}

// buildArtifactURL turns a stored OSS object key into a public http(s) URL the
// deployer can fetch (CDN host + key; the deployer never follows redirects).
// When cdnHost is empty the key is returned unchanged so the failure surfaces
// clearly at the deployer boundary — buildCreateRequest fail-fasts earlier for
// custom tools, skills keep the legacy behavior.
func (s *AgentDeployerService) buildArtifactURL(ossKey string) string {
	if s.cdnHost == "" || ossKey == "" {
		return ossKey
	}
	return strings.TrimRight(s.cdnHost, "/") + "/" + strings.TrimLeft(ossKey, "/")
}

// deployerPreRejected reports whether the deployer rejected the request
// before creating anything (4xx protocol validation or 503 runtime
// floor). In that case any existing container is untouched by the
// deployer and must not be archived on the hub side.
func deployerPreRejected(err error) bool {
	var httpErr *deployer.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	return httpErr.StatusCode == http.StatusServiceUnavailable ||
		(httpErr.StatusCode >= 400 && httpErr.StatusCode < 500)
}

// isStableDeploymentStatus reports whether a Docker-reported container status
// is a stable state worth persisting to the DB. Transitional states (created,
// restarting, paused, removing) and unknown values are excluded.
func isStableDeploymentStatus(status string) bool {
	switch status {
	case "running", "exited", "stopped", "dead":
		return true
	}
	return false
}

// toDTO converts deployment information into a DeploymentDTO. The RuntimeURL
// reflects the tenant-scoped gateway path (/<org>/<name>) when Kong is enabled,
// or the hub-relative proxy path (/runtime/<org>/<name>) in no-Kong mode.
func (s *AgentDeployerService) toDTO(tenantID, agentName, status, health, containerName string, port int, deployedAt *time.Time, message string) *DeploymentDTO {
	var deployedAtStr string
	if deployedAt != nil {
		deployedAtStr = deployedAt.UTC().Format(time.RFC3339)
	}

	url := s.runtimeURL(port)
	publicPath := s.publicPath(tenantID, agentName)
	if s.kongSvc != nil && s.kongSvc.enabled() {
		if kongURL := s.kongSvc.RouteURL(publicPath); kongURL != "" {
			url = kongURL
		}
	} else if status == "running" && port > 0 {
		// No-Kong public address is the hub-relative proxy path (issue #77);
		// frontend resolves it against the current origin. The path itself is
		// mode-aware (issue #114): builtin "/runtime/<name>", casdoor
		// "/runtime/<org>/<name>".
		url = "/runtime" + publicPath
	}

	return &DeploymentDTO{
		Status:        status,
		Health:        health,
		RuntimeURL:    url,
		ContainerName: containerName,
		DeployedAt:    deployedAtStr,
		Message:       message,
		HostPort:      port,
	}
}

// firstNonEmpty returns the first non-empty string from the given arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// stringOrEmpty coerces a value to a string, returning an empty string when
// the value is nil or not a string.
func stringOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// mapProtocolToRuntime translates the internal short protocol identifiers
// stored in provider_summaries (anthropic / openai) into the API type names
// expected by open-agent-runtime (anthropic-messages / openai-completions).
// agent-deployer should accept these full names; if it rejects with a
// "protocol must be one of: anthropic, openai" error, that's a deployer-side
// validation bug to fix there, not here.
func mapProtocolToRuntime(protocol string) string {
	switch protocol {
	case "anthropic":
		return "anthropic-messages"
	case "openai":
		return "openai-completions"
	default:
		return protocol
	}
}

// intPtr returns a pointer to the given int value.
func intPtr(i int) *int {
	return &i
}
