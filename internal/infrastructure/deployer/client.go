package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AgentDefinition is one node of the deployment agent graph (deployer v3).
// Subagents are pure id references to other entries in the same graph;
// mounted agents never inherit or fall back to parent capabilities — an
// empty field stays empty. Runtime-global fields (Model, MaxSessionQueries,
// PermissionMode) are root-only: the deployer rejects them on children.
type AgentDefinition struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Model        string `json:"model,omitempty"`
	SystemPrompt string `json:"systemPrompt"`
	MaxTurns     *int   `json:"maxTurns,omitempty"`
	// MaxSessionQueries is the session-level query cap (SDK 3.1.0 rename of
	// maxSessionTurns; runtime >= v2.6.0). Root-only.
	MaxSessionQueries *int     `json:"maxSessionQueries,omitempty"`
	PermissionMode    string   `json:"permissionMode,omitempty"`
	Tools             []string `json:"tools,omitempty"`
	// DisallowedTools is the agent-local deny list (issue #111): a
	// user-configured tool-name blacklist (built-in names like "Bash" or
	// MCP-qualified names, no referential integrity) applied on top of the
	// Tools allow-list. Distinct from read-only restrictions such as Explore,
	// which stay dynamic in the runtime/SDK. Root and children are
	// isomorphic; empty omits the key.
	DisallowedTools []string      `json:"disallowedTools,omitempty"`
	CustomTools     []ToolSource  `json:"customTools,omitempty"`
	Skills          []SkillSource `json:"skills,omitempty"`
	// SettingSources is set to ["user"] when (and only when) this agent
	// declares skills — the deployer v3 validation requires it, its storage
	// layer defaults root to ["project"] otherwise.
	SettingSources []string                   `json:"settingSources,omitempty"`
	Subagents      []string                   `json:"subagents,omitempty"`
	McpServers     map[string]McpServerConfig `json:"mcpServers,omitempty"`
	Datasets       map[string]string          `json:"datasets,omitempty"`
}

// McpServerConfig defines the configuration for a single MCP server.
// Only "sse" and "http" transports are supported; stdio has been removed.
type McpServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SkillSource defines a skill source for agent-deployer.
// All three fields are required by the deployer validation.
type SkillSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

// ToolSource defines a single custom Tool file for agent-deployer (issue #88).
// Mirrors the deployer's model.ToolSource: name must match ^[A-Za-z0-9._-]{1,64}$;
// url must be absolute http(s) — the deployer never follows redirects; hash is
// 64-hex sha256; fileName's extension must be .ts/.mts/.js/.mjs (case-sensitive)
// and its directory components are never used by the deployer.
type ToolSource struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Hash     string `json:"hash"`
	FileName string `json:"fileName"`
}

// ProviderConfig defines the LLM provider configuration.
type ProviderConfig struct {
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"baseUrl"`
	LockedAPIKey string `json:"lockedApiKey"`
}

// AigcConfig carries the GB 45438-2025 content-labeling configuration. When
// nil the deployer/runtime does not label outputs (backwards compatible).
type AigcConfig struct {
	Enabled         bool              `json:"enabled"`
	ContentProducer string            `json:"contentProducer"`
	SigningKey      string            `json:"signingKey,omitempty"`
	ExplicitHint    *bool             `json:"explicitHint,omitempty"`
	ModelCodes      map[string]string `json:"modelCodes,omitempty"`
}

// HubConfig carries the runtime's chat-record pushback configuration: the
// runtime (agent-runtime ≥ v2.1.1 channel support) pushes completed sessions
// to this hub via POST {baseUrl}/api/v1/chat/push with X-Chat-Push-Key.
// Schema mirrors the deployer's HubConfig validation (absolute http(s) baseUrl
// + non-blank chatPushKey when enabled) and the runtime's agents.yaml `hub`
// section. When nil the section is omitted = pushback disabled.
//
// Org is the agent's **trusted deployment tenant** (issue #78): builtin mode
// is always "default"; casdoor mode is the tenant the agent was deployed
// under. The runtime stamps pushback sessions with this org — the X-Org
// header has been removed from agent-runtime (≥ 2.2.0, agent-runtime#28), so
// pushback tenant affinity cannot be forged. Requires agent-deployer ≥
// v2.2.0 (HubConfig.org support) and agent-runtime ≥ 2.2.0 on the deploy
// path; older components silently drop the field (no error), but then
// pushback falls back to legacy semantics (default-tenant resolution), which
// can misattribute tenants in multi-tenant casdoor deployments — upgrade the
// chain and redeploy existing agents (see docs/configuration.md).
type HubConfig struct {
	Enabled     bool   `json:"enabled"`
	BaseURL     string `json:"baseUrl"`
	ChatPushKey string `json:"chatPushKey,omitempty"`
	Org         string `json:"org,omitempty"`
}

// CreateAgentRequest is the deployer v3.1 deployment payload: a complete agent
// graph plus runtime-global provider config. RootAgentID names the runtime
// agent graph entry (bare agent id, issue #114) and must match one
// Agents[].Name entry; the tenant-scoped DeploymentKey keys containers,
// directories and all lifecycle addressing on the deployer side — the two
// identities are deliberately independent (deployer#18).
type CreateAgentRequest struct {
	RootAgentID   string            `json:"rootAgentId"`
	DeploymentKey string            `json:"deploymentKey"`
	Agents        []AgentDefinition `json:"agents"`
	Provider      ProviderConfig    `json:"provider"`
	RuntimeToken  string            `json:"runtime_token"`
	Force         bool              `json:"force,omitempty"`
	Aigc          *AigcConfig       `json:"aigc,omitempty"`
	Hub           *HubConfig        `json:"hub,omitempty"`
}

// createAgentBody wraps CreateAgentRequest with a separate force field to avoid
// mutating the caller's request.
type createAgentBody struct {
	*CreateAgentRequest
	Force bool `json:"force"`
}

// AgentResponse is the response for agent creation and detail queries.
type AgentResponse struct {
	AgentName     string `json:"agentName"`
	InstanceID    string `json:"instanceId"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Status        string `json:"status"`
	HostPort      int    `json:"hostPort"`
	CreatedAt     string `json:"createdAt,omitempty"`
	YAMLPath      string `json:"yamlPath,omitempty"`
	SessionDir    string `json:"sessionDir,omitempty"`
	SkillsDir     string `json:"skillsDir,omitempty"`
	// ContainerSkillsDir / ToolsDir are v3 additions the hub does not consume
	// yet; defined so the DTO cannot drift from the deployer contract.
	ContainerSkillsDir string `json:"containerSkillsDir,omitempty"`
	ToolsDir           string `json:"toolsDir,omitempty"`
	RuntimeToken       string `json:"runtimeToken"`
}

// AgentStatusResponse is the response for agent status queries.
type AgentStatusResponse struct {
	AgentName     string `json:"agentName"`
	ContainerName string `json:"containerName"`
	ContainerID   string `json:"containerId"`
	Status        string `json:"status"`
	Health        string `json:"health"`
	HostPort      int    `json:"hostPort"`
	Image         string `json:"image"`
}

// successEnvelope is the common response envelope from the deployer API.
type successEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

// Client is an HTTP client for the agent-deployer service.
type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewClient creates a new deployer client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	return c.client.Do(req)
}

// HTTPError carries a non-2xx deployer response so handlers can map status
// codes without string matching. Message comes from the deployer's
// {"success":false,"error":...} envelope (neutral, log-safe copy).
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("deployer returned HTTP %d: %s", e.StatusCode, e.Message)
}

// decodeSuccess reads and closes resp.Body.
// It checks the HTTP status code before decoding JSON. If the status is not 2xx,
// it reads the body (up to a limit) and returns an *HTTPError carrying the
// status code plus the envelope error message (or the raw body as fallback).
func decodeSuccess(resp *http.Response, out interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(body))
		var env struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &env) == nil && env.Error != "" {
			msg = env.Error
		}
		return &HTTPError{StatusCode: resp.StatusCode, Message: msg}
	}

	var env successEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !env.Success {
		return fmt.Errorf("deployer error: %s", env.Error)
	}

	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}

	return nil
}

// deploymentKeyProbeRoot is a sanitized rootAgentId so both deployer
// generations pass the name-pattern guard and reject the probe at a later
// field validation — before any docker side effect can happen.
const deploymentKeyProbeRoot = "hub-capability-probe"

// SupportsDeploymentKey reports whether the deployer implements the v3.1.0
// deploymentKey split (deployer#18, hub#114). Probe: POST /api/v1/agents with
// a valid rootAgentId but no deploymentKey and no agents.
//   - v3.1.0+: 400 "deploymentKey is required" (validation order: rootAgentID
//     → deploymentKey → agents) → supported.
//   - v3.0.x: 400 "agents must contain at least the root agent definition"
//     (no deploymentKey concept) → unsupported.
//
// Both rejections happen pre-docker, so the probe never creates containers.
// The sentinel string is a deployer test-pinned contract (e3b0b3a); any other
// 400 fails closed as unsupported, and transport failures surface as errors.
func (c *Client) SupportsDeploymentKey(ctx context.Context) (bool, error) {
	payload, err := json.Marshal(map[string]any{
		"rootAgentId": deploymentKeyProbeRoot,
		"agents":      []any{},
	})
	if err != nil {
		return false, err
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	probeErr := decodeSuccess(resp, &struct{}{})
	if probeErr == nil {
		return false, fmt.Errorf("deployer accepted an invalid capability probe; cannot verify deploymentKey support")
	}
	var httpErr *HTTPError
	if errors.As(probeErr, &httpErr) && httpErr.StatusCode == http.StatusBadRequest {
		return strings.Contains(httpErr.Message, "deploymentKey is required"), nil
	}
	return false, fmt.Errorf("probe deployer capability: %w", probeErr)
}

// CreateAgent creates a new agent container.
func (c *Client) CreateAgent(ctx context.Context, req *CreateAgentRequest, force bool) (*AgentResponse, error) {
	body, err := json.Marshal(createAgentBody{CreateAgentRequest: req, Force: force})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	var result AgentResponse
	if err := decodeSuccess(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAgent retrieves agent details by name.
func (c *Client) GetAgent(ctx context.Context, name string) (*AgentResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, fmt.Errorf("get agent: %w", err)
	}

	var result AgentResponse
	if err := decodeSuccess(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetStatus retrieves the real-time status and health of an agent container.
func (c *Client) GetStatus(ctx context.Context, name string) (*AgentStatusResponse, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/agents/"+url.PathEscape(name)+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("get status: %w", err)
	}

	var result AgentStatusResponse
	if err := decodeSuccess(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StopAgent stops an agent container.
func (c *Client) StopAgent(ctx context.Context, name string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(name)+"/stop", nil)
	if err != nil {
		return fmt.Errorf("stop agent: %w", err)
	}
	return decodeSuccess(resp, nil)
}

// StartAgent starts a stopped agent container via docker restart.
// agent-deployer exposes /restart which works for both running and
// stopped containers; for stopped ones it acts as a start.
func (c *Client) StartAgent(ctx context.Context, name string) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/agents/"+url.PathEscape(name)+"/restart", nil)
	if err != nil {
		return fmt.Errorf("start agent: %w", err)
	}
	return decodeSuccess(resp, nil)
}

// DeleteAgent deletes an agent container.
// When purge is false the deployer archives the agent (container removed, data
// retained) and returns status=archived on subsequent GETs.
// When purge is true it removes both the container and its data.
func (c *Client) DeleteAgent(ctx context.Context, name string, purge bool) error {
	path := "/api/v1/agents/" + url.PathEscape(name)
	if purge {
		path += "?purge=true"
	}

	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return decodeSuccess(resp, nil)
}
