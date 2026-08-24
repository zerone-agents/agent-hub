package services

import (
	"errors"
	"fmt"
	"sort"
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
	// authMode 决定 push-key 通道的租户语义：builtin（单租户）忽略 org 字段，
	// 恒 "default"；casdoor 下 org 缺省时经 resolveDefaultOrg 解析为
	// tenant_oauth_clients 的 default 行组织。
	authMode string
	// resolveDefaultOrg 解析 casdoor 默认租户组织（ops 登记的 default 行）；
	// 返回 (org, true)，查不到/出错返回 ("", false)。builtin 模式可为 nil。
	resolveDefaultOrg func() (string, bool)
}

func NewChatService(authMode string, resolveDefaultOrg func() (string, bool)) *ChatService {
	return &ChatService{
		repo:              repository.NewChatRepository(),
		authMode:          authMode,
		resolveDefaultOrg: resolveDefaultOrg,
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
	ID                string `json:"id" binding:"required"`
	Title             string `json:"title"`
	CreatedAt         string `json:"created_at" binding:"required"`
	UpdatedAt         string `json:"updated_at" binding:"required"`
	Model             string `json:"model"`
	ModelSelectionID  string `json:"model_selection_id"`
	SystemPrompt      string `json:"system_prompt"`
	Status            string `json:"status"`
	Mode              string `json:"mode"`
	ProviderID        string `json:"provider_id"`
	AgentID           string `json:"agent_id"`
	PermissionProfile string `json:"permission_profile"`
	Hidden            bool   `json:"hidden"`
	ExtraDirectories  string `json:"extra_directories"`
	IsUserBound       bool   `json:"is_user_bound"`
	// UserName / Org 仅在 chat_push_key 鉴权通道（X-Chat-Push-Key）下生效：
	// UserName / Org 仅在 chat_push_key 鉴权通道（X-Chat-Push-Key）下生效：
	// user_name 必填且直接作为 user_id/display_name。org 语义按部署模式分：
	// builtin（单租户）忽略该字段，恒落 "default"；casdoor 缺省时解析为
	// tenant_oauth_clients 的 default 行组织（未登记则 400）。
	// JWT/CLI 通道下服务端忽略这两个字段（归属强制取 token 身份）。
	UserName string         `json:"user_name"`
	Org      string         `json:"org"`
	Messages []MessageInput `json:"messages"`
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

func (s *ChatService) Push(tenantID, userID string, userName string, displayName string, req *PushRequest) (*PushResponse, error) {
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

	result, err := s.repo.PushSessions(tenantID, userID, sessions, allMessages)
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

// ErrPushValidation 标记 push 请求体校验错误（handler 映射 400）。
var ErrPushValidation = errors.New("push validation failed")

// PushWithSessionIdentity 处理 X-Chat-Push-Key 通道的 push：归属完全由 body
// 决定——user_name 直接作为 user_id/user_name/display_name；租户按部署模式
// 解析（见 resolveTenant：builtin 忽略 org 恒 "default"，casdoor 缺省 org
// 解析为 tenant_oauth_clients default 行组织）。按 (tenant, user) 二元组
// 分组逐组写库（repo.PushSessions 会把同一 userID 盖到该次调用的全部
// session 上，故必须按组调用）；任一组失败整体失败（与现有单租户失败语义
// 一致）。casdoor 显式 org 不校验租户存在性。
func (s *ChatService) PushWithSessionIdentity(req *PushRequest) (*PushResponse, error) {
	if len(req.Sessions) == 0 {
		return &PushResponse{Success: true}, nil
	}
	if len(req.Sessions) > maxSessionsPerPush {
		return nil, fmt.Errorf("too many sessions in single push: %d (max %d)", len(req.Sessions), maxSessionsPerPush)
	}

	type groupKey struct{ tenant, user string }
	type entry struct {
		session  *chat.Session
		messages []*chat.Message
	}
	groups := make(map[groupKey][]entry)

	for i, si := range req.Sessions {
		if si.UserName == "" {
			return nil, fmt.Errorf("%w: session[%d]: user_name is required (chat push key mode)", ErrPushValidation, i)
		}
		sess, err := convertSession(si)
		if err != nil {
			return nil, fmt.Errorf("%w: session[%d]: %v", ErrPushValidation, i, err)
		}
		sess.UserName = si.UserName
		sess.DisplayName = si.UserName
		tenant, err := s.resolveTenant(si.Org, i)
		if err != nil {
			return nil, err
		}
		groups[groupKey{tenant: tenant, user: si.UserName}] = append(
			groups[groupKey{tenant: tenant, user: si.UserName}],
			entry{session: sess, messages: convertMessages(si.Messages)},
		)
	}

	// 排序保证写库与聚合顺序确定（测试可断言）。
	keys := make([]groupKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].tenant != keys[j].tenant {
			return keys[i].tenant < keys[j].tenant
		}
		return keys[i].user < keys[j].user
	})

	resp := &PushResponse{Success: true}
	for _, k := range keys {
		sessions := make([]*chat.Session, len(groups[k]))
		msgs := make([][]*chat.Message, len(groups[k]))
		for i, e := range groups[k] {
			sessions[i] = e.session
			msgs[i] = e.messages
		}
		result, err := s.repo.PushSessions(k.tenant, k.user, sessions, msgs)
		if err != nil {
			return nil, fmt.Errorf("push failed (tenant=%s user=%s): %w", k.tenant, k.user, err)
		}
		resp.SyncedSessions += result.SyncedSessions
		resp.SkippedSessions += result.SkippedSessions
		resp.SyncedMessages += result.SyncedMessages
		for _, c := range result.Conflicts {
			resp.Conflicts = append(resp.Conflicts, ConflictOutput(c))
		}
	}
	return resp, nil
}

// resolveTenant 解析 push-key 通道的 session 租户归属：
//   - builtin（单租户，不支持多租户）：忽略 org 字段，恒 "default"
//   - casdoor + 显式 org：直接用所传值（不校验租户存在性）
//   - casdoor + 缺省 org：解析为 tenant_oauth_clients 的 default 行组织；
//     未登记 default 行（或 resolver 未注入）→ ErrPushValidation（400）——
//     配置缺失是硬错误，不静默写无人可见的幽灵租户
func (s *ChatService) resolveTenant(org string, sessionIdx int) (string, error) {
	if s.authMode == "builtin" {
		return "default", nil
	}
	if org != "" {
		return org, nil
	}
	if s.resolveDefaultOrg != nil {
		if defOrg, ok := s.resolveDefaultOrg(); ok && defOrg != "" {
			return defOrg, nil
		}
	}
	return "", fmt.Errorf("%w: session[%d]: org is required (no default tenant client registered)", ErrPushValidation, sessionIdx)
}

type ListResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// ListSessions 返回会话列表。userID 非空时按该用户过滤（member 数据范围）；
// 为空时返回全部（admin/maintainer）。两种模式都限定在 tenantID 内。
func (s *ChatService) ListSessions(tenantID string, page, pageSize int, userID string) (*ListResponse, error) {
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
		sessions, total, err = s.repo.ListSessionsByUser(tenantID, userID, page, pageSize)
	} else {
		sessions, total, err = s.repo.ListSessions(tenantID, page, pageSize)
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
// 调用方统一映射为 404（不暴露会话存在性）。两种模式都限定在 tenantID 内。
func (s *ChatService) GetSession(tenantID, sessionID, userID string) (*chat.Session, error) {
	if userID != "" {
		return s.repo.GetSessionForUser(tenantID, sessionID, userID)
	}
	return s.repo.GetSession(tenantID, sessionID)
}

// ListMessages 列出会话消息。userID 非空（member）时先校验会话归属，
// 他人会话返回错误（调用方映射 404）。
func (s *ChatService) ListMessages(tenantID, sessionID string, page, pageSize int, userID string) (*ListResponse, error) {
	if userID != "" {
		if _, err := s.repo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
			return nil, err
		}
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	messages, total, err := s.repo.ListMessages(tenantID, sessionID, page, pageSize)
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
func (s *ChatService) DeleteSession(tenantID, sessionID, userID string) error {
	if userID != "" {
		if _, err := s.repo.GetSessionForUser(tenantID, sessionID, userID); err != nil {
			return err
		}
	}
	return s.repo.DeleteSession(tenantID, sessionID)
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
