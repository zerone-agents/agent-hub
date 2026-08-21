package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
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
	GetToolsByAgent(agentID uint64) ([]string, error)
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
	DeployerConfig() (*deployer.AigcConfig, error)
}

// AgentDeployerService handles agent deployment operations.
type AgentDeployerService struct {
	client        *deployer.Client
	publicHost    string
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
	healthProbe   func(ctx context.Context, publicHost string, port int) bool
	gatewayHealth *sync.Map // agent name -> *gatewayHealthEntry
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

// storeGatewayHealth records the gateway health for an agent.
func (s *AgentDeployerService) storeGatewayHealth(name string, healthy bool) {
	if s == nil || s.gatewayHealth == nil {
		return
	}
	s.gatewayHealth.Store(name, &gatewayHealthEntry{healthy: healthy, probedAt: time.Now()})
}

// gatewayHealthy returns the cached gateway health for an agent, or nil if there
// is no recent (< TTL) cached entry. It evicts stale entries on read.
func (s *AgentDeployerService) gatewayHealthy(name string) *bool {
	if s == nil || s.gatewayHealth == nil {
		return nil
	}
	raw, ok := s.gatewayHealth.Load(name)
	if !ok {
		return nil
	}
	entry, ok := raw.(*gatewayHealthEntry)
	if !ok {
		s.gatewayHealth.Delete(name)
		return nil
	}
	if time.Since(entry.probedAt) > gatewayHealthTTL {
		s.gatewayHealth.Delete(name)
		return nil
	}
	return &entry.healthy
}

// refreshGatewayHealth probes the Kong route for an agent and caches the result.
func (s *AgentDeployerService) refreshGatewayHealth(name string) {
	if s == nil || s.kongSvc == nil || !s.kongSvc.enabled() {
		return
	}
	gatewayURL := s.kongSvc.RouteURL(name)
	if gatewayURL == "" {
		return
	}
	ctx := context.Background()
	healthy := probeURL(ctx, gatewayURL+"/health", 3*time.Second)
	s.storeGatewayHealth(name, healthy)
}

// probeGatewayHealthy probes the Kong route for an agent and reports whether it
// is reachable. Unlike refreshGatewayHealth, it does not cache the result.
func (s *AgentDeployerService) probeGatewayHealthy(name string) bool {
	if s == nil || s.kongSvc == nil || !s.kongSvc.enabled() {
		return false
	}
	gatewayURL := s.kongSvc.RouteURL(name)
	if gatewayURL == "" {
		return false
	}
	ctx := context.Background()
	return probeURL(ctx, gatewayURL+"/health", 3*time.Second)
}

// defaultHealthProbe performs an active HTTP check against the runtime /health endpoint.
func defaultHealthProbe(ctx context.Context, publicHost string, port int) bool {
	url := fmt.Sprintf("http://%s:%d/health", publicHost, port)
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
func NewAgentDeployerService(client *deployer.Client, publicHost, cdnHost, encryptionKey, runtimeAPIKey string, knowledgeSvc *KnowledgeService, kongSvc *KongGatewayService, aigcSvc *AigcConfigService) *AgentDeployerService {
	s := &AgentDeployerService{
		client:        client,
		publicHost:    publicHost,
		cdnHost:       cdnHost,
		encryptionKey: encryptionKey,
		runtimeAPIKey: runtimeAPIKey,
		agentRepo:     repository.NewAgentRepository(),
		toolRepo:      repository.NewToolRepository(),
		skillRepo:     repository.NewSkillRepository(),
		providerSvc:   NewProviderService(encryptionKey),
		mcpSvc:        NewMcpService(encryptionKey),
		knowledgeSvc:  knowledgeSvc,
		kongSvc:       kongSvc,
		healthProbe:   defaultHealthProbe,
		gatewayHealth: &sync.Map{},
	}
	if aigcSvc != nil {
		s.aigcSvc = aigcSvc
	}
	return s
}

// WaitForHealthy polls the deployer and actively probes /health until the agent
// is ready or timeout is reached. It returns the current host port.
func (s *AgentDeployerService) WaitForHealthy(ctx context.Context, name string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		st, err := s.client.GetStatus(ctx, name)
		if err == nil {
			if st.Health == "healthy" {
				return st.HostPort, nil
			}
			if st.HostPort > 0 && s.healthProbe(ctx, s.publicHost, st.HostPort) {
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

	// Decide the runtime token before building the request. control-panel is
	// the sole generator and keeper of runtime tokens; the deployer only
	// injects the value it is given as ZERONE_AGENT_HTTP_API_KEY.
	token, err := s.resolveRuntimeToken(ctx, name, agentCfg, force, rotateKey)
	if err != nil {
		return nil, err
	}

	// Deregister any existing Kong route before recreating. This is idempotent
	// (no-op if the agent was never registered) and avoids serving 502s while
	// the container is being rebuilt.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, name)
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
		_ = s.client.DeleteAgent(ctx, name, false)
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

	dto := s.toDTO(name, resp.Status, "", resp.ContainerName, resp.HostPort, &deployedAt, "")
	if resp.Status == "running" && resp.HostPort > 0 {
		dto.APIKey = token
		// Register Kong route asynchronously once the runtime reports healthy.
		if s.kongSvc != nil {
			go s.registerWhenHealthy(name, resp.HostPort)
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
func (s *AgentDeployerService) resolveRuntimeToken(ctx context.Context, name string, cfg *agent.AgentConfig, force, rotateKey bool) (string, error) {
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
		if _, err := s.client.GetAgent(ctx, name); err == nil {
			return "", fmt.Errorf("agent %s 在 deployer 上已存在容器，但本地无 Runtime Token 记录（可能因加密密钥变更或数据丢失），无法恢复；请强制重新部署，将生成新的 API Key", name)
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

// registerWhenHealthy waits for the agent to become healthy and then registers
// its Kong route. After registration it probes the gateway route with retries
// so route propagation delay is accounted for. Errors are logged, not returned.
func (s *AgentDeployerService) registerWhenHealthy(name string, hostPort int) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var hp int
	var err error
	if hp, err = s.WaitForHealthy(ctx, name, 120*time.Second); err == nil {
		_ = s.kongSvc.Register(ctx, name, hp)
	} else if hostPort > 0 {
		_ = s.kongSvc.Register(ctx, name, hostPort)
	} else {
		return
	}
	if s.kongSvc == nil || !s.kongSvc.enabled() {
		return
	}
	gatewayURL := s.kongSvc.RouteURL(name)
	if gatewayURL == "" {
		return
	}
	healthURL := gatewayURL + "/health"
	for i := 0; i < 3; i++ {
		if probeURL(ctx, healthURL, 3*time.Second) {
			s.storeGatewayHealth(name, true)
			return
		}
		if i < 2 {
			time.Sleep(3 * time.Second)
		}
	}
	log.Printf("gateway health check failed for agent %s: %s", name, gatewayURL)
	s.storeGatewayHealth(name, false)
}

// GetStatus queries the deployer for the current status of an agent container.
func (s *AgentDeployerService) GetStatus(tenantID, name string) (*DeploymentDTO, error) {
	name = NormalizeAgentName(name)
	// Load agent from DB
	agentCfg, err := s.agentRepo.GetByName(tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	ctx := context.Background()
	statusResp, err := s.client.GetAgent(ctx, name)
	if err != nil {
		// If deployer says not found, return not_found status
		return s.toDTO(name, "not_found", "", "", 0, agentCfg.DeployedAt, "未部署或已被清理"), nil
	}

	// If status is running, also query health
	health := ""
	if statusResp.Status == "running" {
		if healthResp, err := s.client.GetStatus(ctx, name); err == nil {
			health = healthResp.Health
			statusResp.HostPort = healthResp.HostPort
		} else {
			log.Printf("GetStatus: health query failed for agent %s: %v", name, err)
		}
	}

	// When Kong is enabled and the container is running+healthy, also factor in gateway health.
	var gatewayMessage string
	if s.kongSvc != nil && s.kongSvc.enabled() && statusResp.Status == "running" && health == "healthy" {
		if cached := s.gatewayHealthy(name); cached != nil {
			if !*cached {
				health = "unhealthy"
				gatewayMessage = "Kong 网关路由不可达"
			}
		} else {
			// No recent cache yet. Probe synchronously so we do not report
			// healthy while the Kong route is still propagating or broken.
			// Only store successful results; failures during propagation are
			// transient and should render as 'starting', not 'unhealthy'.
			if s.probeGatewayHealthy(name) {
				s.storeGatewayHealth(name, true)
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

	dto := s.toDTO(name, statusResp.Status, health, statusResp.ContainerName, statusResp.HostPort, agentCfg.DeployedAt, gatewayMessage)
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
	if err := s.client.StopAgent(ctx, name); err != nil {
		return fmt.Errorf("stop agent failed: %w", err)
	}

	// Update DB status to stopped, clear RuntimePort but keep encrypted token so
	// it can be shown again when the container is restarted.
	if err := s.updateStatus(tenantID, agentCfg, "stopped", 0, agentCfg.DeployedAt); err != nil {
		return fmt.Errorf("update status failed: %w", err)
	}

	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, name)
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
	// Deregister any existing Kong route before restarting.
	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, name)
	}
	if err := s.client.StartAgent(ctx, name); err != nil {
		return nil, fmt.Errorf("start agent failed: %w", err)
	}

	// Query the deployer for updated status/port
	statusResp, err := s.client.GetAgent(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get status after start failed: %w", err)
	}

	if err := s.updateStatus(tenantID, agentCfg, statusResp.Status, statusResp.HostPort, agentCfg.DeployedAt); err != nil {
		return nil, fmt.Errorf("update status failed: %w", err)
	}

	dto := s.toDTO(name, statusResp.Status, "", statusResp.ContainerName, statusResp.HostPort, agentCfg.DeployedAt, "")
	if statusResp.Status == "running" && statusResp.HostPort > 0 && agentCfg.RuntimeToken != "" {
		dto.APIKey, _ = providerdomain.Decrypt(agentCfg.RuntimeToken, s.encryptionKey)
		// Register Kong route asynchronously once the runtime reports healthy.
		if s.kongSvc != nil {
			go s.registerWhenHealthy(name, statusResp.HostPort)
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
	if err := s.client.DeleteAgent(ctx, name, purge); err != nil {
		return fmt.Errorf("delete agent failed: %w", err)
	}

	if s.kongSvc != nil {
		_ = s.kongSvc.Deregister(ctx, name)
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
func (s *AgentDeployerService) loadAgentRelations(tenantID string, cfg *agent.AgentConfig) ([]string, []*skill.Skill, []agent.AgentConfig, error) {
	tools, err := s.toolRepo.GetToolsByAgent(cfg.ID)
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

	return tools, skills, subagents, nil
}

// buildCreateRequest builds the deployer.CreateAgentRequest from agent config and relations.
func (s *AgentDeployerService) buildCreateRequest(
	ctx context.Context,
	tenantID string,
	cfg *agent.AgentConfig,
	providerDTO providerdomain.Provider,
	tools []string,
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

	// Build skill sources from full skill records
	skillSources := make([]deployer.SkillSource, 0, len(skills))
	for _, sk := range skills {
		if sk.Name == "" || sk.URL == "" || sk.FileHash == "" {
			log.Printf("buildCreateRequest: skip skill %s: missing name/url/hash", sk.Name)
			continue
		}
		skillSources = append(skillSources, deployer.SkillSource{
			Name: sk.Name,
			URL:  s.buildSkillURL(sk.URL),
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
			Name:            cfg.Name,
			Description:     firstNonEmpty(cfg.Description["zh"], cfg.Description["en"], cfg.Name),
			Model:           cfg.ModelID,
			SystemPrompt:    cfg.SystemPrompt,
			MaxTurns:        intPtr(cfg.MaxTurns),
			MaxSessionTurns: cfg.MaxSessionTurns,
			PermissionMode:  cfg.PermissionMode,
			Tools:           tools,
			Skills:          skillSources,
			Subagents:       subagentDefs,
			McpServers:      mcpServerConfigs,
			Datasets:        agentDatasets,
		},
		Provider: providerConfig,
	}

	if err := s.applyAigc(req); err != nil {
		return nil, err
	}

	return req, nil
}

// applyAigc injects the GB 45438-2025 labeling config when configured.
// A nil service or missing config leaves the request untouched (omitempty).
func (s *AgentDeployerService) applyAigc(req *deployer.CreateAgentRequest) error {
	if s.aigcSvc == nil {
		return nil
	}
	cfg, err := s.aigcSvc.DeployerConfig()
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

// buildSkillURL turns a stored OSS object key (e.g. "expert/foo/abc.zip") into
// a public http(s) URL the deployer can fetch. Skills store OSS keys rather
// than full URLs because presigned URLs expire; the CDN host provides the
// public prefix. When cdnHost is empty the key is returned unchanged so the
// failure surfaces clearly at the deployer boundary instead of being masked.
func (s *AgentDeployerService) buildSkillURL(ossKey string) string {
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

// toDTO converts deployment information into a DeploymentDTO.
func (s *AgentDeployerService) toDTO(agentName, status, health, containerName string, port int, deployedAt *time.Time, message string) *DeploymentDTO {
	var deployedAtStr string
	if deployedAt != nil {
		deployedAtStr = deployedAt.UTC().Format(time.RFC3339)
	}

	url := s.runtimeURL(port)
	if s.kongSvc != nil && s.kongSvc.enabled() {
		if kongURL := s.kongSvc.RouteURL(agentName); kongURL != "" {
			url = kongURL
		}
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
// stored in vendor_presets (anthropic / openai) into the API type names
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
