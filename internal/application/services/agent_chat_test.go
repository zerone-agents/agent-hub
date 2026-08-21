package services

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/chat"
)

// mockChatRepo is a minimal in-memory chat repository for service-level tests.
type mockChatRepo struct {
	sessions map[string]*chat.Session
	messages map[string]*chat.Message
	listSess []*chat.Session
}

func newMockChatRepo() *mockChatRepo {
	return &mockChatRepo{
		sessions: make(map[string]*chat.Session),
		messages: make(map[string]*chat.Message),
	}
}

func (m *mockChatRepo) ListSessionsByAgentAndUser(tenantID, agentID, userID, source string, page, pageSize int) ([]*chat.Session, int64, error) {
	var out []*chat.Session
	for _, s := range m.listSess {
		if s.TenantID == tenantID && s.AgentID == agentID && s.UserID == userID {
			if source == "" || s.Source == source {
				out = append(out, s)
			}
		}
	}
	return out, int64(len(out)), nil
}

func (m *mockChatRepo) GetSessionForUser(tenantID, sessionID, userID string) (*chat.Session, error) {
	s, ok := m.sessions[sessionID]
	if !ok || s.TenantID != tenantID || s.UserID != userID {
		return nil, errNotFound
	}
	return s, nil
}

func (m *mockChatRepo) CreateSession(tenantID string, sess *chat.Session) error {
	sess.TenantID = tenantID
	m.sessions[sess.ID] = sess
	m.listSess = append(m.listSess, sess)
	return nil
}

func (m *mockChatRepo) CreateMessage(tenantID string, msg *chat.Message) error {
	msg.TenantID = tenantID
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockChatRepo) DeleteSession(tenantID, sessionID string) error {
	if s, ok := m.sessions[sessionID]; ok && s.TenantID == tenantID {
		delete(m.sessions, sessionID)
	}
	return nil
}

func (m *mockChatRepo) ListMessages(tenantID, sessionID string, page, pageSize int) ([]*chat.Message, int64, error) {
	var out []*chat.Message
	for _, msg := range m.messages {
		if msg.TenantID == tenantID && msg.SessionID == sessionID {
			out = append(out, msg)
		}
	}
	return out, int64(len(out)), nil
}

func (m *mockChatRepo) UpdateSessionRuntimeSessionID(tenantID, sessionID, runtimeSessionID string) error {
	if s, ok := m.sessions[sessionID]; ok && s.TenantID == tenantID {
		s.RuntimeSessionID = runtimeSessionID
	}
	return nil
}

func (m *mockChatRepo) UpdateSessionTitle(tenantID, sessionID, title string) error {
	if s, ok := m.sessions[sessionID]; ok && s.TenantID == tenantID {
		if s.Title == "" {
			s.Title = title
		}
	}
	return nil
}

// errNotFound mirrors gorm.ErrRecordNotFound for test simplicity.
var errNotFound = &notFoundErr{}

type notFoundErr struct{}

func (e *notFoundErr) Error() string { return "record not found" }

// mockAgentRepoForChat returns a hardcoded agent config for service tests.
type mockAgentRepoForChat struct {
	cfg *agent.AgentConfig
}

func (m *mockAgentRepoForChat) GetByName(name string) (*agent.AgentConfig, error) {
	return m.cfg, nil
}

func TestCreateSession_PopulatesFromAgent(t *testing.T) {
	repo := newMockChatRepo()
	agentRepo := &mockAgentRepoForChat{cfg: &agent.AgentConfig{
		Name:         "coder",
		SystemPrompt: "you are coder",
		ModelID:      "gpt-4o",
	}}
	svc := &AgentChatService{
		chatRepo:  repo,
		agentRepo: agentRepo,
	}

	sess, err := svc.CreateSession("tenant-a", "u1", "coder", "", "小红", "Xiao Hong")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if sess.AgentID != "coder" {
		t.Errorf("AgentID = %q, want coder", sess.AgentID)
	}
	if sess.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", sess.Model)
	}
	if sess.SystemPrompt != "you are coder" {
		t.Errorf("SystemPrompt = %q", sess.SystemPrompt)
	}
	if sess.Mode != "agent_chat" {
		t.Errorf("Mode = %q, want agent_chat", sess.Mode)
	}
	if sess.UserName != "小红" {
		t.Errorf("UserName = %q, want 小红", sess.UserName)
	}
	if sess.DisplayName != "Xiao Hong" {
		t.Errorf("DisplayName = %q, want Xiao Hong", sess.DisplayName)
	}
}

func TestSaveUserMessage_StoresContent(t *testing.T) {
	repo := newMockChatRepo()
	agentRepo := &mockAgentRepoForChat{cfg: &agent.AgentConfig{Name: "coder"}}
	svc := &AgentChatService{chatRepo: repo, agentRepo: agentRepo}

	sess, _ := svc.CreateSession("tenant-a", "u1", "coder", "", "", "")
	msg, err := svc.SaveUserMessage("tenant-a", "u1", sess.ID, "hello")
	if err != nil {
		t.Fatalf("SaveUserMessage failed: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("Role = %q, want user", msg.Role)
	}
	if msg.Content != `[{"type":"text","text":"hello"}]` {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestSaveAssistantMessage_StoresContent(t *testing.T) {
	repo := newMockChatRepo()
	agentRepo := &mockAgentRepoForChat{cfg: &agent.AgentConfig{Name: "coder"}}
	svc := &AgentChatService{chatRepo: repo, agentRepo: agentRepo}

	sess, _ := svc.CreateSession("tenant-a", "u1", "coder", "", "", "")
	content := `[{"type":"text","text":"hi back"}]`
	msg, err := svc.SaveAssistantMessage("tenant-a", "u1", sess.ID, content, "")
	if err != nil {
		t.Fatalf("SaveAssistantMessage failed: %v", err)
	}
	if msg.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", msg.Role)
	}
}

func TestSaveAssistantMessage_StoresAigcLabel(t *testing.T) {
	repo := newMockChatRepo()
	agentRepo := &mockAgentRepoForChat{cfg: &agent.AgentConfig{Name: "coder"}}
	svc := &AgentChatService{chatRepo: repo, agentRepo: agentRepo}

	sess, _ := svc.CreateSession("tenant-a", "u1", "coder", "", "", "")
	label := `{"Label":"1","ProduceID":"p-1"}`
	msg, err := svc.SaveAssistantMessage("tenant-a", "u1", sess.ID, `[{"type":"text","text":"hi"}]`, label)
	if err != nil {
		t.Fatalf("SaveAssistantMessage failed: %v", err)
	}
	if msg.Aigc != label {
		t.Errorf("Aigc = %q, want %q", msg.Aigc, label)
	}
}

func TestDeleteSession_DelegatesToRepo(t *testing.T) {
	repo := newMockChatRepo()
	agentRepo := &mockAgentRepoForChat{cfg: &agent.AgentConfig{Name: "coder"}}
	svc := &AgentChatService{chatRepo: repo, agentRepo: agentRepo}

	sess, _ := svc.CreateSession("tenant-a", "u1", "coder", "", "", "")
	if err := svc.DeleteSession("tenant-a", "u1", sess.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	if _, ok := repo.sessions[sess.ID]; ok {
		t.Errorf("session still present after delete")
	}
}
