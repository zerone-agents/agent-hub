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
	CreateUploadRecords(tenantID string, records []*chat.UploadRecord) error
	GetUploadRecord(tenantID, sessionID, id string) (*chat.UploadRecord, error)
	HasUploadRecordPath(tenantID, sessionID, path, containerID string) (bool, error)
	DeleteMessageByID(tenantID, sessionID, messageID string) error
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

// filePart is the canonical attachment shape persisted in Message.Content.
// Ordered BEFORE the optional text part (issue #94).
type filePart struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

// GetMessages returns messages for a session owned by userID.
func (s *AgentChatService) GetMessages(tenantID, userID, sessionID string, page, pageSize int) ([]*chat.Message, int64, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, 0, fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.ListMessages(tenantID, sessionID, page, pageSize)
}

// DeleteSession deletes a session owned by userID, including its upload
// records (attachment descriptors are session-scoped authorization anchors —
// they must not outlive the session). The records are removed inside the
// repository's DeleteSession transaction (issue #94 review R2 F3), so a
// partial failure can never leave records behind for a deleted session.
func (s *AgentChatService) DeleteSession(tenantID, userID, sessionID string) error {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.DeleteSession(tenantID, sessionID)
}

// SaveUserMessage persists a user message in the given session. content and
// attachments are wrapped into the canonical JSON array format: file parts
// first, then the optional text part (empty content adds no text part).
func (s *AgentChatService) SaveUserMessage(tenantID, userID, sessionID, content string, attachments []AttachmentDesc) (*chat.Message, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	parts := make([]interface{}, 0, len(attachments)+1)
	for _, a := range attachments {
		parts = append(parts, filePart{
			Type: "file", ID: a.ID, Name: a.Name, Mime: a.Mime, Size: a.Size, Path: a.Path,
		})
	}
	if content != "" {
		parts = append(parts, contentPart{Type: "text", Text: content})
	}
	wrapped, _ := json.Marshal(parts)

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
// the runtime base URL, the per-agent runtime API key (decrypted from
// the agents table), and the deployer-reported container id.
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
//
// containerID is the immutable deployment-generation anchor (issue #94
// review R3): Docker assigns a fresh id on every recreate (redeploy) but
// keeps it across in-place restarts — exactly mirroring the on-disk lifetime
// of `.zerone-uploads`. Callers bind upload records to it at upload time and
// re-check it on send/download, so stale-generation records fail closed with
// no time tolerance. An empty containerID (deployer did not report one)
// must be treated as "generation unknown" by authorization callers.
func (s *AgentChatService) ResolveRuntime(tenantID, agentName string) (string, string, string, error) {
	status, err := s.deployerSvc.GetStatus(tenantID, agentName)
	if err != nil {
		return "", "", "", fmt.Errorf("get deployment status: %w", err)
	}
	if status.Status != "running" {
		return "", "", "", fmt.Errorf("agent not running (status=%s)", status.Status)
	}
	if status.HostPort == 0 {
		return "", "", "", fmt.Errorf("agent running but no host port")
	}
	baseURL, err := s.resolveBaseURL(s.kongEnabledForChat(), status.RuntimeURL, status.HostPort)
	if err != nil {
		return "", "", "", err
	}
	return baseURL, status.APIKey, status.ContainerID, nil
}

// attachmentProbe caches one capability probe result.
type attachmentProbe struct {
	ok bool
	at time.Time
}

// ProbeAttachmentSupport probes the runtime /health endpoint (no auth) on an
// already-resolved base URL. It returns whether the version supports chat
// attachments (>= 2.5.0, the release that shipped runtime PR #48). Version
// gating only — deployment-generation binding is done via the deployer-
// reported container id from ResolveRuntime, not via /health (issue #94
// review R3).
func (s *AgentChatService) ProbeAttachmentSupport(ctx context.Context, baseURL string) (bool, error) {
	info, err := s.runtimeClient.Health(ctx, baseURL)
	if err != nil {
		return false, err
	}
	return compareSemver(info.Version, attachmentRuntimeMinVersion) >= 0, nil
}

// AttachmentsSupportedAt is a thin wrapper over ProbeAttachmentSupport for
// callers that only need the version verdict.
func (s *AgentChatService) AttachmentsSupportedAt(ctx context.Context, baseURL string) bool {
	supported, err := s.ProbeAttachmentSupport(ctx, baseURL)
	return err == nil && supported
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
	if baseURL, _, _, err := s.ResolveRuntime(tenantID, agentName); err == nil {
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

// SessionHasAttachment reports whether path belongs to a server-side upload
// record of the session created in the CURRENT runtime container generation
// (containerID from ResolveRuntime — issue #94 review F1/R3: records are
// written only by the hub at upload time — the unforgeable authorization
// anchor for the user-facing content proxy; message file parts are
// display-only). Runtime /v1/files/content can read the whole cwd, so this
// cross-check must only ever admit paths the runtime handed out to THIS
// session in THIS container generation. The match is an exact container-id
// equality — no time tolerance.
func (s *AgentChatService) SessionHasAttachment(tenantID, userID, sessionID, path, containerID string) (bool, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return false, fmt.Errorf("session not found: %w", err)
	}
	if containerID == "" {
		// Authorization path: fail closed when the generation is unknown
		// (deployer did not report a container id) rather than treating
		// every record as current.
		return false, nil
	}
	return s.chatRepo.HasUploadRecordPath(tenantID, sessionID, path, containerID)
}

// SaveUploadRecords persists the runtime-issued attachment descriptors after
// a successful upload, binding them to (tenant, session, user) AND the
// immutable container generation the upload was served by (containerID is
// captured from ResolveRuntime BEFORE the upload request is issued, so it is
// naturally the generation that actually processed the upload — an upload
// interrupted by a recreate leaves no record, and a recreate right after a
// successful upload marks the record stale). These records — not the
// client-supplied message descriptors — later authorize both the content
// proxy and SendMessage attachment acceptance. All descriptors of one
// upload persist atomically (issue #94 review R2 F4): the runtime has
// already issued the ids/files, so a partial persist would leave files no
// record authorizes.
func (s *AgentChatService) SaveUploadRecords(tenantID, userID, sessionID, containerID string, files []AttachmentDesc) error {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	records := make([]*chat.UploadRecord, 0, len(files))
	for _, f := range files {
		records = append(records, &chat.UploadRecord{
			ID:          f.ID,
			SessionID:   sessionID,
			UserID:      userID,
			Name:        f.Name,
			Mime:        f.Mime,
			Size:        f.Size,
			Path:        f.Path,
			ContainerID: containerID,
			CreatedAt:   time.Now().UTC(),
		})
	}
	if err := s.chatRepo.CreateUploadRecords(tenantID, records); err != nil {
		return fmt.Errorf("create upload records: %w", err)
	}
	return nil
}

// GetUploadRecord returns the server-side upload record for id, scoped to the
// session owned by userID. Callers compare the record against a client-
// supplied descriptor; a missing record means the descriptor was never issued
// by an upload in this session (forged) and must be rejected.
func (s *AgentChatService) GetUploadRecord(tenantID, userID, sessionID, id string) (*chat.UploadRecord, error) {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.GetUploadRecord(tenantID, sessionID, id)
}

// DeleteMessageByID removes one message from a session owned by userID. Used
// to roll back the optimistic user-message persist when the runtime rejects
// the run pre-flight (e.g. attachment_missing after a container rebuild) so a
// retry does not duplicate the user turn.
func (s *AgentChatService) DeleteMessageByID(tenantID, userID, sessionID, messageID string) error {
	if _, err := s.chatRepo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	return s.chatRepo.DeleteMessageByID(tenantID, sessionID, messageID)
}
