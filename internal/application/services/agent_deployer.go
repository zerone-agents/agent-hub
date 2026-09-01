package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// DeploymentDTO represents the deployment status of an agent.
type DeploymentDTO struct {
	Status     string `json:"status"`
	Health     string `json:"health"`
	RuntimeURL string `json:"runtimeUrl"`
	// BareRuntimeURL 是 default 租户的裸路径 URL（"/<agent>"），仅 Kong
	// 启用且 orgSlug(tenantID)=="default" 时非空；其他情况省略。
	BareRuntimeURL string `json:"bareRuntimeUrl,omitempty"`
	ContainerName  string `json:"containerName"`
	DeployedAt     string `json:"deployedAt"`
	Message        string `json:"message"`
	HostPort       int    `json:"hostPort"`
	APIKey         string `json:"apiKey"`
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

// gatewayURL returns the public gateway URL (with tenant-scoped /<org>/<name>
// path) for an agent, or "" when Kong is disabled.
func (s *AgentDeployerService) gatewayURL(tenantID, name string) string {
	if s == nil || s.kongSvc == nil || !s.kongSvc.enabled() {
		return ""
	}
	return s.kongSvc.RouteURL(URLPath(tenantID, name))
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
func NewAgentDeployerService(client *deployer.Client, publicHost, upstreamHost, cdnHost, encryptionKey, runtimeAPIKey string, knowledgeSvc *KnowledgeService, kongSvc *KongGatewayService, aigcSvc *AigcConfigService, chatPushAPIKey, chatPushPublicURL string) *AgentDeployerService {
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

	// Load relations
	tools, skills, subagents, err := s.loadAgentRelations(tenantID, agentCfg)
	if err != nil {
		return nil, err
	}

	// Load MCP servers (already decrypted)
	mcpServers, err := s.mcpSvc.GetClientMcpsByAgent(tenantID, agentCfg.Name)
	if err != nil {
		return nil, fmt.Errorf("load mcp servers failed: %w", err)
	}

	// Build create request
	ctx := context.Background()

	// All deployer calls address the container by the tenant-scoped deploy key
	// (<org>-<name>); only DB lookups use the bare name.
	key := DeployKey(tenantID, name)

	// Decide the runtime token before building the request. control-panel is
	// the sole generator and keeper of runtime tokens; the deployer only
	// injects the value it is given as ZERONE_AGENT_HTTP_API_KEY.
	token, err := s.resolveRuntimeToken(ctx, key, agentCfg, force, rotateKey)
	if err != nil {
		return nil, err
	}

	// D-1 legacy timing: record whether a legacy compatibility entity exists
	// BEFORE the pre-clean Deregister below deletes it, so the later Register
	// (in registerWhenHealthy) can still mount the "/<bare>" compatibility
	// route.
	legacyBare := s.legacyBareFor(ctx, tenantID, name)

	// Deregister any existing Kong route before recreating (scoped entities
	// plus the old bare-name entities of this agent). This is idempotent
	// (no-op if the agent was never registered) and avoids serving 502s while
	// the container is being rebuilt.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, key, name)
	}
	req, err := s.buildCreateRequest(ctx, tenantID, agentCfg, p, tools, skills, subagents, mcpServers)
	if err != nil {
		return nil, fmt.Errorf("build create request failed: %w", err)
	}
	req.RuntimeToken = token
	s.resolveMcpHeaders(req, token)

	// Call deployer
	resp, err := s.client.CreateAgent(ctx, req, force)
	if err != nil {
		// Clean up failed container
		_ = s.client.DeleteAgent(ctx, key, false)
		_ = s.updateStatus(tenantID, agentCfg, "error", 0, nil)
		return nil, fmt.Errorf("deploy agent failed: %w", err)
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
			go s.registerWhenHealthy(tenantID, name, legacyBare, resp.HostPort)
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

// resolveMcpHeaders replaces the $agent_runtime_token placeholder in MCP
// server header values with the actual runtime token being deployed.
// Header maps are rebuilt rather than mutated in place so the MCP DTOs
// returned by the MCP service are never modified.
func (s *AgentDeployerService) resolveMcpHeaders(req *deployer.CreateAgentRequest, token string) {
	for name, mcp := range req.Agent.McpServers {
		if len(mcp.Headers) == 0 {
			continue
		}
		headers := make(map[string]string, len(mcp.Headers))
		for k, v := range mcp.Headers {
			headers[k] = strings.ReplaceAll(v, agentRuntimeTokenPlaceholder, token)
		}
		mcp.Headers = headers
		req.Agent.McpServers[name] = mcp
	}
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

// legacyBareFor reports the bare agent name whose legacy compatibility route
// should survive this deploy/start cycle, or "" when none applies. Two probes
// count, both run BEFORE the pre-clean Deregister that deletes the entities:
//
//  1. the pre-upgrade bare-name Kong service still exists (fresh upgrade); or
//  2. a "<scoped-key>-legacy" route is already mounted — after the first
//     redeploy the bare service is gone, but that route proves the agent
//     opted into compatibility, so it is re-mounted every redeploy until it
//     is removed by hand (runbook step 3).
func (s *AgentDeployerService) legacyBareFor(ctx context.Context, tenantID, name string) string {
	if s.kongSvc == nil {
		return ""
	}
	// default 租户：裸路径恒挂主 route（双路径），-legacy 探测被取代；但
	// 保留 bare-service 探测，让部署 pre-clean 的 Deregister 显式删除
	// 升级前的旧裸名实体。挂载侧由 RegisterWithLegacy 退化语义兜底。
	if orgSlug(tenantID) == defaultTenantSlug {
		if s.kongSvc.LegacyExists(ctx, name) {
			return name
		}
		return ""
	}
	if s.kongSvc.LegacyExists(ctx, name) {
		return name
	}
	if s.kongSvc.LegacyRouteExists(ctx, DeployKey(tenantID, name)) {
		return name
	}
	return ""
}

// registerWhenHealthy waits for the agent to become healthy and then registers
// its Kong route under the tenant-scoped key/path. legacyBare is non-empty only
// when the deploy flow recorded a pre-existing bare-name Kong service before
// its pre-clean Deregister (D-1); in that case the "/<bare>" compatibility
// route is force-mounted on the scoped service. After registration it probes
// the gateway route with retries so route propagation delay is accounted for.
// Errors are logged, not returned.
func (s *AgentDeployerService) registerWhenHealthy(tenantID, name, legacyBare string, hostPort int) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	key := DeployKey(tenantID, name)
	publicPath := URLPath(tenantID, name)
	register := func(port int) {
		if legacyBare != "" {
			// The bare entities were already deleted by the pre-clean
			// Deregister, so Register's internal probe would never fire;
			// force-mount from the recorded flag.
			_ = s.kongSvc.RegisterWithLegacy(ctx, key, publicPath, legacyBare, port)
			return
		}
		_ = s.kongSvc.Register(ctx, key, publicPath, "", port)
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
		_ = s.kongSvc.Deregister(ctx, DeployKey(tenantID, name), name)
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
	// D-1: record any legacy compatibility entity before the pre-clean
	// Deregister deletes it, so the restart can re-mount the legacy route.
	legacyBare := s.legacyBareFor(ctx, tenantID, name)
	// Deregister any existing Kong route before restarting.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, key, name)
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
			go s.registerWhenHealthy(tenantID, name, legacyBare, statusResp.HostPort)
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
		_ = s.kongSvc.Deregister(ctx, DeployKey(tenantID, name), name)
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

// loadAgentRelations loads tools, skills, and subagents for an agent.
func (s *AgentDeployerService) loadAgentRelations(tenantID string, cfg *agent.AgentConfig) ([]*agent.Tool, []*skill.Skill, []agent.AgentConfig, error) {
	toolRecords, err := s.toolRepo.GetToolRecordsByAgent(cfg.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load tools failed: %w", err)
	}

	skills, err := s.skillRepo.GetAgentSkillsFull(cfg.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load skills failed: %w", err)
	}

	subagentNames, err := s.agentRepo.GetSubagents(cfg.ID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load subagents failed: %w", err)
	}

	// Load full subagent configs
	subagents := make([]agent.AgentConfig, 0, len(subagentNames))
	for _, subName := range subagentNames {
		sub, err := s.agentRepo.GetByName(tenantID, subName)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("load subagent %s failed: %w", subName, err)
		}
		subagents = append(subagents, *sub)
	}

	return toolRecords, skills, subagents, nil
}

// buildCreateRequest builds the deployer.CreateAgentRequest from agent config and relations.
func (s *AgentDeployerService) buildCreateRequest(
	ctx context.Context,
	tenantID string,
	cfg *agent.AgentConfig,
	providerDTO providerdomain.Provider,
	toolRecords []*agent.Tool,
	skills []*skill.Skill,
	subagents []agent.AgentConfig,
	mcpServers map[string]*McpClientDTO,
) (*deployer.CreateAgentRequest, error) {
	// Build subagent definitions
	subagentDefs := make([]deployer.SubagentDefinition, 0, len(subagents))
	for _, sub := range subagents {
		desc := firstNonEmpty(sub.Description["zh"], sub.Description["en"], sub.Name)
		subagentDefs = append(subagentDefs, deployer.SubagentDefinition{
			Name:        sub.Name,
			Description: desc,
			Prompt:      sub.SystemPrompt,
			MaxTurns:    intPtr(sub.MaxTurns),
		})
	}

	// Custom tool artifacts (issue #88): Tools stays the complete allow-list;
	// CustomTools only carries source=custom && ready rows, sorted by name so
	// the request and generated YAML are reproducible.
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
		return nil, fmt.Errorf("自定义工具缺少制品文件，无法部署：%s。请先在工具页补传文件或解除挂载", strings.Join(missingCustom, "、"))
	}
	if len(customToolSources) > 0 && s.cdnHost == "" {
		return nil, fmt.Errorf("未配置 OSS_CDN_HOST，无法为自定义工具构造下载地址（共 %d 个）。请配置 CDN 后重新部署", len(customToolSources))
	}
	sort.Strings(toolNames)
	sort.Slice(customToolSources, func(i, j int) bool { return customToolSources[i].Name < customToolSources[j].Name })

	// Build skill sources from full skill records
	skillSources := make([]deployer.SkillSource, 0, len(skills))
	for _, sk := range skills {
		if sk.Name == "" || sk.URL == "" || sk.FileHash == "" {
			log.Printf("buildCreateRequest: skip skill %s: missing name/url/hash", sk.Name)
			continue
		}
		skillSources = append(skillSources, deployer.SkillSource{
			Name: sk.Name,
			URL:  s.buildArtifactURL(sk.URL),
			Hash: sk.FileHash,
		})
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

	// Build MCP server configs (headers are already decrypted by the MCP service).
	mcpServerConfigs := make(map[string]deployer.McpServerConfig, len(mcpServers))
	for name, mcp := range mcpServers {
		if name == "knowledge" && strings.TrimSpace(mcp.URL) == "" {
			return nil, fmt.Errorf("内置 knowledge MCP 未配置可达地址，请设置 KNOWLEDGE_MCP_URL（完整路径需包含 /api/v1/knowledge/mcp），重启 Hub 后重新部署 Agent")
		}
		mcpServerConfigs[name] = deployer.McpServerConfig{
			Type:    mcp.Type,
			URL:     mcp.URL,
			Headers: mcp.Headers,
		}
	}

	// Load bound knowledge datasets and resolve their metadata.
	datasetIDs, err := s.agentRepo.GetKnowledgeDatasetIDsByAgent(cfg.ID)
	if err != nil {
		return nil, fmt.Errorf("load agent knowledge datasets failed: %w", err)
	}
	agentDatasets := make(map[string]string, len(datasetIDs))
	for _, id := range datasetIDs {
		ds, err := s.knowledgeSvc.GetDataset(ctx, id)
		if err != nil {
			log.Printf("skip dataset %s metadata for agent %s: %v", id, cfg.Name, err)
			continue
		}
		desc := strings.TrimSpace(stringOrEmpty((*ds)["description"]))
		if desc == "" {
			return nil, fmt.Errorf("dataset %s 缺少 description，无法下发给 Agent 运行时，请完善知识库描述后重新部署", id)
		}
		agentDatasets[id] = desc
	}

	// NOTE: settingSources is intentionally left unset. The deployer owns the
	// decision of when to scan the user skill directory based on the skills
	// list above; control-panel populating ["user"] here used to race with
	// that and produced confusing "skill registered twice / not at all"
	// symptoms. The field stays in the schema (AgentDefinition.SettingSources)
	// so external clients can still override when needed.
	req := &deployer.CreateAgentRequest{
		Agent: deployer.AgentDefinition{
			// The deployer container key is the tenant-scoped deploy key
			// (<org>-<name>) so same-name agents across tenants never collide;
			// subagent definitions above keep bare names (runtime-internal
			// logic names).
			Name:            DeployKey(tenantID, cfg.Name),
			Description:     firstNonEmpty(cfg.Description["zh"], cfg.Description["en"], cfg.Name),
			Model:           cfg.ModelID,
			SystemPrompt:    cfg.SystemPrompt,
			MaxTurns:        intPtr(cfg.MaxTurns),
			MaxSessionTurns: cfg.MaxSessionTurns,
			PermissionMode:  cfg.PermissionMode,
			Tools:           toolNames,
			CustomTools:     customToolSources,
			Skills:          skillSources,
			Subagents:       subagentDefs,
			McpServers:      mcpServerConfigs,
			Datasets:        agentDatasets,
		},
		Provider: providerConfig,
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
	var bareURL string
	if s.kongSvc != nil && s.kongSvc.enabled() {
		if kongURL := s.kongSvc.RouteURL(URLPath(tenantID, agentName)); kongURL != "" {
			url = kongURL
		}
		if bare := BarePath(tenantID, agentName); bare != "" {
			bareURL = s.kongSvc.RouteURL(bare)
		}
	} else if status == "running" && port > 0 {
		// No-Kong public address is the hub-relative proxy path (issue #77);
		// frontend resolves it against the current origin.
		// Org identity assumption: URLPath uses orgSlug(tenantID), which
		// equals the raw tenant_id for builtin and conforming casdoor orgs
		// (slug is identity); legacy non-conforming tenant IDs 404 through
		// the proxy by design (issue #77 acceptance #2).
		url = "/runtime" + URLPath(tenantID, agentName)
	}

	return &DeploymentDTO{
		Status:         status,
		Health:         health,
		RuntimeURL:     url,
		BareRuntimeURL: bareURL,
		ContainerName:  containerName,
		DeployedAt:     deployedAtStr,
		Message:        message,
		HostPort:       port,
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
