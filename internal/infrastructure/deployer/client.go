package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// AgentDefinition defines the agent configuration.
type AgentDefinition struct {
	Name string `json:"name"`
	// Description is a short human/agent-readable summary of what the agent
	// does. Required by the deployer: agent-runtime 2.0 rejects configs
	// without it, and it is what the parent agent's Task tool shows when
	// mounting subagents.
	Description     string                     `json:"description"`
	Model           string                     `json:"model"`
	SystemPrompt    string                     `json:"systemPrompt"`
	MaxTurns        *int                       `json:"maxTurns,omitempty"`
	MaxSessionTurns *int                       `json:"maxSessionTurns,omitempty"`
	PermissionMode  string                     `json:"permissionMode,omitempty"`
	Tools           []string                   `json:"tools,omitempty"`
	Skills          []SkillSource              `json:"skills,omitempty"`
	Subagents       []SubagentDefinition       `json:"subagents,omitempty"`
	McpServers      map[string]McpServerConfig `json:"mcpServers,omitempty"`
	// Datasets maps dataset-id to description, consumed by the runtime for
	// building the agent system prompt and knowledge tool context.
	Datasets map[string]string `json:"datasets,omitempty"`

	// SettingSources controls runtime filesystem skill scanning. The runtime
	// SDK only loads skills via this field — `skills` above is just an
	// allow-list filter.
	//
	// control-panel does NOT populate this field: the deployer decides on
	// its own when to scan the user skill directory based on the skills
	// list, and control-panel sending ["user"] used to race with that
	// logic. The field is kept in the schema so external clients can still
	// set it explicitly when they need to override the deployer's default.
	SettingSources []string `json:"settingSources,omitempty"`

	// ExtraUserSkillDirs / ExtraProjectSkillDirs let advanced users extend
	// the runtime's search path beyond the defaults. Not currently wired
	// through the UI; exposed here so future work only needs the DB + form
	// layer, not the deployer schema.
	ExtraUserSkillDirs    []string `json:"extraUserSkillDirs,omitempty"`
	ExtraProjectSkillDirs []string `json:"extraProjectSkillDirs,omitempty"`
}

// McpServerConfig defines the configuration for a single MCP server.
// Only "sse" and "http" transports are supported; stdio has been removed.
type McpServerConfig struct {
	Type    string            `json:"type"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SubagentDefinition defines a subagent configuration.
type SubagentDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Tools       []string `json:"tools,omitempty"`
	MaxTurns    *int     `json:"maxTurns,omitempty"`
}

// SkillSource defines a skill source for agent-deployer.
// All three fields are required by the deployer validation.
type SkillSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Hash string `json:"hash"`
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
// under. The runtime stamps pushback sessions with this org instead of any
// caller-supplied X-Org header, so pushback tenant affinity cannot be forged.
// Forward-compatible: older deployer/runtime silently ignore the field
// (Go json / zod strip unknown keys).
type HubConfig struct {
	Enabled     bool   `json:"enabled"`
	BaseURL     string `json:"baseUrl"`
	ChatPushKey string `json:"chatPushKey,omitempty"`
	Org         string `json:"org,omitempty"`
}

// CreateAgentRequest is the request body for creating an agent.
type CreateAgentRequest struct {
	Agent    AgentDefinition `json:"agent"`
	Provider ProviderConfig  `json:"provider"`
	// RuntimeToken is provisioned by the caller (control-panel) and injected
	// into the container as ZERONE_AGENT_HTTP_API_KEY. The deployer no longer
	// generates, stores, or rotates tokens — it only consumes this value.
	RuntimeToken string      `json:"runtime_token"`
	Force        bool        `json:"force,omitempty"`
	Aigc         *AigcConfig `json:"aigc,omitempty"`
	Hub          *HubConfig  `json:"hub,omitempty"`
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
	RuntimeToken  string `json:"runtimeToken"`
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

// decodeSuccess reads and closes resp.Body.
// It checks the HTTP status code before decoding JSON. If the status is not 2xx,
// it reads the body (up to a limit) and returns an error including the status
// and body content.
func decodeSuccess(resp *http.Response, out interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("deployer returned HTTP %d: %s", resp.StatusCode, string(body))
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
