package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/chat"
	"control-panel/internal/infrastructure/runtime"

	"github.com/google/uuid"
)

// chatRepositoryForAgent is the subset of ChatRepository used by AgentChatService.
type chatRepositoryForAgent interface {
	ListSessionsByAgentAndUser(tenantID, agentID, userID, source string, page, pageSize int) ([]*chat.Session, int64, error)
	GetSessionForUser(tenantID, sessionID, userID string) (*chat.Session, error)
	CreateSession(tenantID string, sess *chat.Session) error
	CreateMessage(tenantID string, msg *chat.Message) error
	DeleteSession(tenantID, sessionID string) error
	ListMessages(tenantID, sessionID string, page, pageSize int) ([]*chat.Message, int64, error)
	UpdateSessionRuntimeSessionID(tenantID, sessionID, runtimeSessionID string) error
	UpdateSessionTitle(tenantID, sessionID, title string) error
}

// agentRepoForChat is the subset of AgentRepository used by AgentChatService.
type agentRepoForChat interface {
	GetByName(tenantID, name string) (*agent.AgentConfig, error)
}

// AgentChatService handles agent chat sessions and message persistence.
// Streaming itself is delegated to the HTTP handler, which calls
// svc.RuntimeClient().StreamRun(...) and forwards bytes via flusher.Flush().
// Persistence of the user message before streaming and the assistant message
// after streaming completes is handled here.
type AgentChatService struct {
	chatRepo      chatRepositoryForAgent
	agentRepo     agentRepoForChat
	deployerSvc   *AgentDeployerService
	runtimeClient *runtime.Client
	publicHost    string
	runtimeKey    string
	upstreamHost  string // cfg.Deployer.DeployerURLHost; "" = fail closed (no-Kong upstream)

	// attachment capability probe cache（15s TTL，issue #94）
	probeMu    sync.Mutex
	probeCache map[string]attachmentProbe
}

// NewAgentChatService constructs an AgentChatService.
func NewAgentChatService(
	chatRepo chatRepositoryForAgent,
	agentRepo agentRepoForChat,
	deployerSvc *AgentDeployerService,
	rtClient *runtime.Client,
	publicHost, runtimeKey, upstreamHost string,
) *AgentChatService {
	return &AgentChatService{
		chatRepo:      chatRepo,
		agentRepo:     agentRepo,
		deployerSvc:   deployerSvc,
		runtimeClient: rtClient,
		publicHost:    publicHost,
		runtimeKey:    runtimeKey,
		upstreamHost:  upstreamHost,
	}
}

// RuntimeClient exposes the runtime client for the handler to stream from.
func (s *AgentChatService) RuntimeClient() *runtime.Client { return s.runtimeClient }

// RuntimeKey returns the runtime API key (used by the handler when calling StreamRun).
func (s *AgentChatService) RuntimeKey() string { return s.runtimeKey }

// PublicHost returns the configured public host for runtime URLs.
func (s *AgentChatService) PublicHost() string { return s.publicHost }

// Source constant identifying sessions created from the Agent Chatbox page.
const SourceAgentChatPage = "agent_chat_page"

// attachmentRuntimeMinVersion is the runtime release that shipped upload +
// rich-content Run support (PR #48 → runtime-v2.5.0; the "2.4.0" in issue
// #94 is wrong — see the correction comment on the issue).
const attachmentRuntimeMinVersion = "2.5.0"

// attachmentProbeTTL bounds capability probe caching.
const attachmentProbeTTL = 15 * time.Second

// ListSessions returns sessions for the given (agent, user) pair.
// source filters the session origin (e.g. "agent_chat_page"); pass empty to list all.
func (s *AgentChatService) ListSessions(tenantID, userID, agentName, source string, page, pageSize int) ([]*chat.Session, int64, error) {
	return s.chatRepo.ListSessionsByAgentAndUser(tenantID, agentName, userID, source, page, pageSize)
}

// CreateSession creates a new empty session for the given user + agent.
// Agent config (model, system_prompt, provider_id, permission_mode) is copied in.
// userName and displayName are taken from the JWT context so the session header
// can show a human-readable creator name instead of a truncated user id.
func (s *AgentChatService) CreateSession(tenantID, userID, agentName, title, userName, displayName string) (*chat.Session, error) {
	cfg, err := s.agentRepo.GetByName(tenantID, agentName)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	now := time.Now().UTC()
	sess := &chat.Session{
		UserID:            userID,
		ID:                uuid.NewString(),
		Title:             title,
		UserName:          userName,
		DisplayName:       displayName,
		CreatedAt:         now,
		UpdatedAt:         now,
		Model:             cfg.ModelID,
		SystemPrompt:      cfg.SystemPrompt,
		Status:            "active",
		Mode:              "agent_chat",
		ProviderID:        providerIDString(cfg.ProviderID),
		AgentID:           agentName,
		PermissionProfile: cfg.PermissionMode,
		Source:            SourceAgentChatPage,
	}
	if err := s.chatRepo.CreateSession(tenantID, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// providerIDString safely converts *uint64 to string for the Session.ProviderID field.
func providerIDString(p *uint64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

// contentPart is the canonical text content shape persisted in Message.Content.
// A struct (not a map) is used so json.Marshal emits fields in a stable order
// matching the canonical form `[{"type":"text","text":"..."}]`.
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GetMessages returns messages for a session owned by userID.
func (s *AgentChatService) GetMessages(tenantID, userID, sessionID string, page, pageSize int) ([]*chat.Message, int64, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, 0, fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.ListMessages(tenantID, sessionID, page, pageSize)
}

// DeleteSession deletes a session owned by userID.
func (s *AgentChatService) DeleteSession(tenantID, userID, sessionID string) error {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.DeleteSession(tenantID, sessionID)
}

// SaveUserMessage persists a user message in the given session.
// content is wrapped into the canonical JSON array format.
func (s *AgentChatService) SaveUserMessage(tenantID, userID, sessionID, content string) (*chat.Message, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	wrapped, _ := json.Marshal([]contentPart{
		{Type: "text", Text: content},
	})

	msg := &chat.Message{
		UserID:    userID,
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      "user",
		Content:   string(wrapped),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.chatRepo.CreateMessage(tenantID, msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return msg, nil
}

// SaveAssistantMessage persists an assistant message with pre-aggregated JSON
// content and the AIGC label extracted from the stream ("" when unlabeled).
func (s *AgentChatService) SaveAssistantMessage(tenantID, userID, sessionID, content, aigc string) (*chat.Message, error) {
	return s.saveMessage(tenantID, userID, sessionID, "assistant", content, aigc)
}

// SaveSystemMessage persists a system message (e.g. runtime error) into the session.
func (s *AgentChatService) SaveSystemMessage(tenantID, userID, sessionID, content string) (*chat.Message, error) {
	return s.saveMessage(tenantID, userID, sessionID, "system", content, "")
}

func (s *AgentChatService) saveMessage(tenantID, userID, sessionID, role, content, aigc string) (*chat.Message, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	msg := &chat.Message{
		UserID:    userID,
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		Aigc:      aigc,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.chatRepo.CreateMessage(tenantID, msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}
	return msg, nil
}

// GetSession returns a session owned by userID.
func (s *AgentChatService) GetSession(tenantID, userID, sessionID string) (*chat.Session, error) {
	return s.chatRepo.GetSessionForUser(tenantID, sessionID, userID)
}

// BindRuntimeSessionID stores the runtime SDK session id returned by the first
// run so subsequent messages in the same control-panel session can resume it.
func (s *AgentChatService) BindRuntimeSessionID(tenantID, sessionID, runtimeSessionID string) error {
	return s.chatRepo.UpdateSessionRuntimeSessionID(tenantID, sessionID, runtimeSessionID)
}

// AutoTitleSession sets the session title from the first user message if the
// session has no title yet. Long titles are truncated to 50 characters with "...".
func (s *AgentChatService) AutoTitleSession(tenantID, sessionID, firstUserContent string) error {
	if firstUserContent == "" {
		return nil
	}
	title := firstUserContent
	if len([]rune(title)) > 50 {
		title = string([]rune(title)[:50]) + "..."
	}
	return s.chatRepo.UpdateSessionTitle(tenantID, sessionID, title)
}

// noKongUpstream builds the internal upstream base URL for the no-Kong mode:
// hostname(AGENT_DEPLOYER_URL) + published port. Empty upstream host fails
// closed — DB may retain running state from before a config change.
func (s *AgentChatService) noKongUpstream(port int) (string, error) {
	if s.upstreamHost == "" {
		return "", fmt.Errorf("internal upstream host unavailable (AGENT_DEPLOYER_URL not configured)")
	}
	return "http://" + net.JoinHostPort(s.upstreamHost, strconv.Itoa(port)), nil
}

// kongEnabledForChat reports whether the Kong gateway chain is active. A nil
// deployerSvc is treated as no-Kong (lean toward the internal upstream).
func (s *AgentChatService) kongEnabledForChat() bool {
	return s.deployerSvc != nil && s.deployerSvc.kongEnabled()
}

// resolveBaseURL 按网关模式严格分支选取拨号目标（issue #77 验收 #10：Kong
// 链路零变化）：
//
//	① Kong 启用 → 信任 toDTO 已填充的网关 RuntimeURL；空 URL 的预注册边缘
//	   （Kong enabled 但路由尚未注册完成）保持本 PR 合入前的 main 行为：
//	   http://{publicHost}:{hostPort} 公网地址，绝不回落到 deployer 内网回源
//	   （构造方式改 net.JoinHostPort，IPv6 安全；仅构造方式变更，语义不动）。
//	② 无 Kong → 公开地址（hairpin 绝对 URL 或相对路径）永远不是内部拨号
//	   目标，一律走 deployer hostname 内网回源（issue #77 验收 #8）。
func (s *AgentChatService) resolveBaseURL(kongEnabled bool, runtimeURL string, hostPort int) (string, error) {
	if !kongEnabled {
		return s.noKongUpstream(hostPort)
	}
	if runtimeURL == "" {
		return "http://" + net.JoinHostPort(s.publicHost, strconv.Itoa(hostPort)), nil
	}
	return runtimeURL, nil
}

// ResolveRuntime verifies the agent is deployed and running, and returns
// the runtime base URL and the per-agent runtime API key (decrypted from
// the agents table).
//
// The base URL is Kong-aware: when Kong gateway is enabled, deployerSvc.toDTO
// has already populated RuntimeURL with the gateway route (e.g.
// "https://agents.example.com/zerone/pharmaceutical"). The runtime client appends
// /v1/agents/{name}/runs to this base; Kong's StripPath strips the
// agent-name prefix and forwards the canonical runtime API path to the
// container. When Kong is not configured, the public RuntimeURL (absolute
// hairpin URL or hub-relative path) is never an internal dial target — the
// internal upstream URL http://{DeployerURLHost}:{hostPort} is used instead
// so hub→runtime traffic stays on the deployer network.
func (s *AgentChatService) ResolveRuntime(tenantID, agentName string) (string, string, error) {
	status, err := s.deployerSvc.GetStatus(tenantID, agentName)
	if err != nil {
		return "", "", fmt.Errorf("get deployment status: %w", err)
	}
	if status.Status != "running" {
		return "", "", fmt.Errorf("agent not running (status=%s)", status.Status)
	}
	if status.HostPort == 0 {
		return "", "", fmt.Errorf("agent running but no host port")
	}
	baseURL, err := s.resolveBaseURL(s.kongEnabledForChat(), status.RuntimeURL, status.HostPort)
	if err != nil {
		return "", "", err
	}
	return baseURL, status.APIKey, nil
}

// attachmentProbe caches one capability probe result.
type attachmentProbe struct {
	ok bool
	at time.Time
}

// AttachmentsSupportedAt probes the runtime /health endpoint (no auth) on an
// already-resolved base URL and reports whether the version supports chat
// attachments (>= 2.5.0, the release that shipped runtime PR #48).
func (s *AgentChatService) AttachmentsSupportedAt(ctx context.Context, baseURL string) bool {
	info, err := s.runtimeClient.Health(ctx, baseURL)
	if err != nil {
		return false
	}
	return compareSemver(info.Version, attachmentRuntimeMinVersion) >= 0
}

// AttachmentsAvailable reports (15s TTL cache) whether the agent's runtime
// supports attachments. Probe failures return false — text chat is never
// blocked by this.
func (s *AgentChatService) AttachmentsAvailable(ctx context.Context, tenantID, agentName string) bool {
	key := tenantID + "\x00" + agentName
	s.probeMu.Lock()
	if hit, ok := s.probeCache[key]; ok && time.Since(hit.at) < attachmentProbeTTL {
		s.probeMu.Unlock()
		return hit.ok
	}
	s.probeMu.Unlock()

	ok := false
	if baseURL, _, err := s.ResolveRuntime(tenantID, agentName); err == nil {
		ok = s.AttachmentsSupportedAt(ctx, baseURL)
	}
	s.probeMu.Lock()
	if s.probeCache == nil {
		s.probeCache = make(map[string]attachmentProbe)
	}
	s.probeCache[key] = attachmentProbe{ok: ok, at: time.Now()}
	s.probeMu.Unlock()
	return ok
}
