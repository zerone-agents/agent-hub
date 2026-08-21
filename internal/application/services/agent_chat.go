package services

import (
	"encoding/json"
	"fmt"
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
	GetByName(name string) (*agent.AgentConfig, error)
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
}

// NewAgentChatService constructs an AgentChatService.
func NewAgentChatService(
	chatRepo chatRepositoryForAgent,
	agentRepo agentRepoForChat,
	deployerSvc *AgentDeployerService,
	rtClient *runtime.Client,
	publicHost, runtimeKey string,
) *AgentChatService {
	return &AgentChatService{
		chatRepo:      chatRepo,
		agentRepo:     agentRepo,
		deployerSvc:   deployerSvc,
		runtimeClient: rtClient,
		publicHost:    publicHost,
		runtimeKey:    runtimeKey,
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
	cfg, err := s.agentRepo.GetByName(agentName)
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

// ResolveRuntime verifies the agent is deployed and running, and returns
// the runtime base URL and the per-agent runtime API key (decrypted from
// the agents table).
//
// The base URL is Kong-aware: when Kong gateway is enabled, deployerSvc.toDTO
// has already populated RuntimeURL with the gateway route (e.g.
// "https://agents.example.com/pharmaceutical"). The runtime client appends
// /v1/agents/{name}/runs to this base; Kong's StripPath strips the
// agent-name prefix and forwards the canonical runtime API path to the
// container. When Kong is not configured, RuntimeURL falls back to a
// direct http://{publicHost}:{hostPort} URL.
func (s *AgentChatService) ResolveRuntime(agentName string) (string, string, error) {
	status, err := s.deployerSvc.GetStatus(agentName)
	if err != nil {
		return "", "", fmt.Errorf("get deployment status: %w", err)
	}
	if status.Status != "running" {
		return "", "", fmt.Errorf("agent not running (status=%s)", status.Status)
	}
	if status.HostPort == 0 {
		return "", "", fmt.Errorf("agent running but no host port")
	}
	baseURL := status.RuntimeURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s:%d", s.publicHost, status.HostPort)
	}
	return baseURL, status.APIKey, nil
}
