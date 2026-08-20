package services

import (
	"fmt"
	"time"

	"control-panel/internal/domain/chat"
	repository "control-panel/internal/infrastructure/persistence"
)

const maxSessionsPerPush = 50

var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05",
}

type ChatService struct {
	repo *repository.ChatRepository
}

func NewChatService() *ChatService {
	return &ChatService{
		repo: repository.NewChatRepository(),
	}
}

type MessageInput struct {
	ID         string `json:"id" binding:"required"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at" binding:"required"`
	Hidden     bool   `json:"hidden"`
	TokenUsage string `json:"token_usage"`
	Feedback   string `json:"feedback"`
}

type SessionInput struct {
	ID                string         `json:"id" binding:"required"`
	Title             string         `json:"title"`
	CreatedAt         string         `json:"created_at" binding:"required"`
	UpdatedAt         string         `json:"updated_at" binding:"required"`
	Model             string         `json:"model"`
	ModelSelectionID  string         `json:"model_selection_id"`
	SystemPrompt      string         `json:"system_prompt"`
	Status            string         `json:"status"`
	Mode              string         `json:"mode"`
	ProviderID        string         `json:"provider_id"`
	AgentID           string         `json:"agent_id"`
	PermissionProfile string         `json:"permission_profile"`
	Hidden            bool           `json:"hidden"`
	ExtraDirectories  string         `json:"extra_directories"`
	IsUserBound       bool           `json:"is_user_bound"`
	Messages          []MessageInput `json:"messages"`
}

type PushRequest struct {
	Sessions []SessionInput `json:"sessions" binding:"required,dive"`
}

type ConflictOutput struct {
	SessionID       string `json:"session_id"`
	ClientUpdatedAt string `json:"client_updated_at"`
	ServerUpdatedAt string `json:"server_updated_at"`
	Resolution      string `json:"resolution"`
}

type PushResponse struct {
	Success         bool             `json:"success"`
	SyncedSessions  int              `json:"synced_sessions"`
	SkippedSessions int              `json:"skipped_sessions"`
	SyncedMessages  int              `json:"synced_messages"`
	Conflicts       []ConflictOutput `json:"conflicts,omitempty"`
}

func (s *ChatService) Push(userID string, userName string, displayName string, req *PushRequest) (*PushResponse, error) {
	if len(req.Sessions) == 0 {
		return &PushResponse{Success: true}, nil
	}

	if len(req.Sessions) > maxSessionsPerPush {
		return nil, fmt.Errorf("too many sessions in single push: %d (max %d)", len(req.Sessions), maxSessionsPerPush)
	}

	type sessionWithMessages struct {
		session  *chat.Session
		messages []*chat.Message
	}

	pairs := make([]sessionWithMessages, len(req.Sessions))
	for i, si := range req.Sessions {
		sess, err := convertSession(si)
		if err != nil {
			return nil, fmt.Errorf("invalid session data: %w", err)
		}
		sess.UserName = userName
		sess.DisplayName = displayName
		msgs := convertMessages(si.Messages)
		pairs[i] = sessionWithMessages{session: sess, messages: msgs}
	}

	sessions := make([]*chat.Session, len(pairs))
	allMessages := make([][]*chat.Message, len(pairs))
	for i, p := range pairs {
		sessions[i] = p.session
		allMessages[i] = p.messages
	}

	result, err := s.repo.PushSessions(userID, sessions, allMessages)
	if err != nil {
		return nil, fmt.Errorf("push failed: %w", err)
	}

	resp := &PushResponse{
		Success:         true,
		SyncedSessions:  result.SyncedSessions,
		SkippedSessions: result.SkippedSessions,
		SyncedMessages:  result.SyncedMessages,
	}

	if len(result.Conflicts) > 0 {
		resp.Conflicts = make([]ConflictOutput, len(result.Conflicts))
		for i, c := range result.Conflicts {
			resp.Conflicts[i] = ConflictOutput(c)
		}
	}

	return resp, nil
}

type ListResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// ListSessions 返回会话列表。userID 非空时按该用户过滤（member 数据范围）；
// 为空时返回全部（admin/maintainer）。
func (s *ChatService) ListSessions(page, pageSize int, userID string) (*ListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var sessions []*chat.Session
	var total int64
	var err error
	if userID != "" {
		sessions, total, err = s.repo.ListSessionsByUser(userID, page, pageSize)
	} else {
		sessions, total, err = s.repo.ListSessions(page, pageSize)
	}
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &ListResponse{
		Items:      sessions,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// GetSession 按归属取会话。userID 非空（member）时查不到他人会话——
// 调用方统一映射为 404（不暴露会话存在性）。
func (s *ChatService) GetSession(sessionID, userID string) (*chat.Session, error) {
	if userID != "" {
		return s.repo.GetSessionForUser(sessionID, userID)
	}
	return s.repo.GetSession(sessionID)
}

// ListMessages 列出会话消息。userID 非空（member）时先校验会话归属，
// 他人会话返回错误（调用方映射 404）。
func (s *ChatService) ListMessages(sessionID string, page, pageSize int, userID string) (*ListResponse, error) {
	if userID != "" {
		if _, err := s.repo.GetSessionForUser(sessionID, userID); err != nil {
			return nil, err
		}
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	messages, total, err := s.repo.ListMessages(sessionID, page, pageSize)
	if err != nil {
		return nil, err
	}

	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return &ListResponse{
		Items:      messages,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// DeleteSession 删除会话。userID 非空（member）时先校验归属——
// 只能删自己的会话；admin/maintainer（userID 为空）可删任意会话。
func (s *ChatService) DeleteSession(sessionID, userID string) error {
	if userID != "" {
		if _, err := s.repo.GetSessionForUser(sessionID, userID); err != nil {
			return err
		}
	}
	return s.repo.DeleteSession(sessionID)
}

func convertSession(in SessionInput) (*chat.Session, error) {
	createdAt, err := parseTime(in.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("session %s: invalid created_at: %w", in.ID, err)
	}
	updatedAt, err := parseTime(in.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("session %s: invalid updated_at: %w", in.ID, err)
	}
	return &chat.Session{
		ID:                in.ID,
		Title:             in.Title,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
		Model:             in.Model,
		ModelSelectionID:  in.ModelSelectionID,
		SystemPrompt:      in.SystemPrompt,
		Status:            in.Status,
		Mode:              in.Mode,
		ProviderID:        in.ProviderID,
		AgentID:           in.AgentID,
		PermissionProfile: in.PermissionProfile,
		Hidden:            in.Hidden,
		ExtraDirectories:  in.ExtraDirectories,
		IsUserBound:       in.IsUserBound,
	}, nil
}

func convertMessages(inputs []MessageInput) []*chat.Message {
	messages := make([]*chat.Message, len(inputs))
	for i, in := range inputs {
		createdAt, _ := parseTime(in.CreatedAt)
		messages[i] = &chat.Message{
			ID:         in.ID,
			Role:       in.Role,
			Content:    in.Content,
			CreatedAt:  createdAt,
			Hidden:     in.Hidden,
			TokenUsage: in.TokenUsage,
			Feedback:   in.Feedback,
		}
	}
	return messages
}

func parseTime(s string) (time.Time, error) {
	for _, format := range timeFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}
