package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxAgentMessageRunes        = 16000
	maxAgentSummaryContextRunes = 4000
	maxAgentSharedContextRunes  = 12000
	maxAgentRunResponseBytes    = 16 << 20
)

// AgentMessageRunner is deliberately small so relation policy can be tested
// without starting an HTTP runtime. AgentChatService implements it through
// RunOneShot.
type AgentMessageRunner interface {
	RunOneShot(ctx context.Context, tenantID, agentName, message string) (string, error)
}

type AgentMessageService struct {
	relationRepo *repository.AgentRelationRepository
	messageRepo  *repository.AgentMessageRepository
	agentRepo    *repository.AgentRepository
	relationSvc  *AgentRelationService
	runner       AgentMessageRunner
	active       sync.Map // tenantID + NUL + agent id; prevents recursive delivery loops
	targetQueues sync.Map // tenantID + NUL + agent id -> *agentMessageTargetQueue
}

type agentMessageTargetQueue struct {
	token chan struct{}
}

func NewAgentMessageService(runner AgentMessageRunner) *AgentMessageService {
	relationRepo := repository.NewAgentRelationRepository()
	agentRepo := repository.NewAgentRepository()
	return &AgentMessageService{
		relationRepo: relationRepo,
		messageRepo:  repository.NewAgentMessageRepository(),
		agentRepo:    agentRepo,
		relationSvc:  &AgentRelationService{repo: relationRepo, agentRepo: agentRepo},
		runner:       runner,
	}
}

func newAgentMessageService(
	relationRepo *repository.AgentRelationRepository,
	messageRepo *repository.AgentMessageRepository,
	agentRepo *repository.AgentRepository,
	runner AgentMessageRunner,
) *AgentMessageService {
	return &AgentMessageService{
		relationRepo: relationRepo,
		messageRepo:  messageRepo,
		agentRepo:    agentRepo,
		relationSvc:  &AgentRelationService{repo: relationRepo, agentRepo: agentRepo},
		runner:       runner,
	}
}

type SendAgentMessageInput struct {
	TargetAgent    string
	Scope          string
	Action         string
	Message        string
	ContextSummary string
	SharedContext  string
}

type AgentMessageDTO struct {
	ID             string  `json:"id"`
	RelationID     uint64  `json:"relationId"`
	Scope          string  `json:"scope"`
	SourceAgent    string  `json:"sourceAgent"`
	TargetAgent    string  `json:"targetAgent"`
	Action         string  `json:"action"`
	DeliveryPolicy string  `json:"deliveryPolicy"`
	ContextPolicy  string  `json:"contextPolicy"`
	Status         string  `json:"status"`
	Reply          string  `json:"reply,omitempty"`
	Error          string  `json:"error,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	StartedAt      *string `json:"startedAt,omitempty"`
	CompletedAt    *string `json:"completedAt,omitempty"`
}

func (s *AgentMessageService) Relations(tenantID string, source *agent.AgentConfig) ([]*AgentRelationDTO, error) {
	if source == nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	relations, err := s.relationRepo.ListEnabledForAgent(tenantID, source.ID)
	if err != nil {
		return nil, fmt.Errorf("读取组织关系失败: %w", err)
	}
	result := make([]*AgentRelationDTO, 0, len(relations))
	for _, relation := range relations {
		result = append(result, relationToDTO(relation))
	}
	return result, nil
}

func (s *AgentMessageService) SignalRelation(tenantID string, source *agent.AgentConfig, targetAgent, scope string, input *RecordAgentRelationEventInput) (*AgentRelationEventResultDTO, error) {
	if source == nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	return s.relationSvc.RecordEventForAgent(tenantID, source.ID, source.Name, targetAgent, scope, input)
}

func (s *AgentMessageService) Send(ctx context.Context, tenantID string, source *agent.AgentConfig, input SendAgentMessageInput) (*AgentMessageDTO, error) {
	if source == nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	if _, busy := s.active.Load(agentMessageActiveKey(tenantID, source.ID)); busy {
		return nil, agentrelation.ErrNestedDispatch
	}
	input.TargetAgent = NormalizeAgentName(input.TargetAgent)
	input.Scope = strings.TrimSpace(input.Scope)
	input.Action = strings.TrimSpace(input.Action)
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		return nil, agentrelation.ErrMessageRequired
	}
	if utf8.RuneCountInString(input.Message) > maxAgentMessageRunes {
		return nil, agentrelation.ErrMessageTooLong
	}
	if _, ok := agentrelation.Actions[input.Action]; !ok {
		return nil, agentrelation.ErrInvalidAction
	}
	target, err := s.agentRepo.GetByName(tenantID, input.TargetAgent)
	if err != nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	relation, err := s.resolveRelation(tenantID, source.ID, target.ID, input.Scope, input.Action)
	if err != nil {
		return nil, err
	}
	// The target must act from its own directional view of the sender. The
	// authorizing A -> B edge is not evidence of how B feels about A.
	recipientView, err := s.relationRepo.FindEnabledEdge(tenantID, relation.Scope, target.ID, source.ID)
	if err != nil {
		return nil, fmt.Errorf("读取接收方关系状态失败: %w", err)
	}

	sharedContext := relationContext(relation.ContextPolicy, input.ContextSummary, input.SharedContext)
	message := &agentrelation.AgentMessage{
		ID:             uuid.NewString(),
		RelationID:     relation.ID,
		Scope:          relation.Scope,
		SourceAgentID:  source.ID,
		SourceAgent:    source.Name,
		TargetAgentID:  target.ID,
		TargetAgent:    target.Name,
		Action:         input.Action,
		DeliveryPolicy: relation.DeliveryPolicy,
		ContextPolicy:  relation.ContextPolicy,
		Content:        input.Message,
		SharedContext:  sharedContext,
		Status:         agentrelation.MessageStatusQueued,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.messageRepo.Create(tenantID, message); err != nil {
		return nil, fmt.Errorf("记录组织消息失败: %w", err)
	}

	envelope := buildAgentMessageEnvelope(source, target, relation, recipientView, input.Action, input.Message, sharedContext)
	if relation.DeliveryPolicy == "async" {
		dto := agentMessageToDTO(message)
		go func() {
			asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			s.execute(asyncCtx, tenantID, target.ID, message, envelope)
		}()
		return dto, nil
	}

	s.execute(ctx, tenantID, target.ID, message, envelope)
	return agentMessageToDTO(message), nil
}

func (s *AgentMessageService) Get(tenantID string, source *agent.AgentConfig, id string) (*AgentMessageDTO, error) {
	if source == nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	message, err := s.messageRepo.GetForAgent(tenantID, strings.TrimSpace(id), source.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, agentrelation.ErrMessageNotFound
		}
		return nil, fmt.Errorf("读取组织消息失败: %w", err)
	}
	return agentMessageToDTO(message), nil
}

func (s *AgentMessageService) Inbox(tenantID string, source *agent.AgentConfig, limit int) ([]*AgentMessageDTO, error) {
	if source == nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	messages, err := s.messageRepo.ListForAgent(tenantID, source.ID, limit)
	if err != nil {
		return nil, fmt.Errorf("读取组织消息箱失败: %w", err)
	}
	result := make([]*AgentMessageDTO, 0, len(messages))
	for _, message := range messages {
		result = append(result, agentMessageToDTO(message))
	}
	return result, nil
}

func (s *AgentMessageService) resolveRelation(tenantID string, sourceID, targetID uint64, scope, action string) (*agentrelation.AgentRelation, error) {
	if scope != "" {
		relation, err := s.relationRepo.GetEnabledEdge(tenantID, scope, sourceID, targetID)
		if err != nil {
			return nil, agentrelation.ErrRouteNotFound
		}
		if !containsRelationAction(relation.AllowedActions, action) {
			return nil, agentrelation.ErrActionNotAllowed
		}
		return relation, nil
	}

	relations, err := s.relationRepo.ListEnabledForAgent(tenantID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("读取组织关系失败: %w", err)
	}
	var matched *agentrelation.AgentRelation
	for _, relation := range relations {
		if relation.SourceAgentID != sourceID || relation.TargetAgentID != targetID || !containsRelationAction(relation.AllowedActions, action) {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("%w：存在多个范围，请明确 scope", agentrelation.ErrRouteNotFound)
		}
		matched = relation
	}
	if matched == nil {
		return nil, agentrelation.ErrRouteNotFound
	}
	return matched, nil
}

func (s *AgentMessageService) execute(ctx context.Context, tenantID string, targetAgentID uint64, message *agentrelation.AgentMessage, envelope string) {
	key := agentMessageActiveKey(tenantID, targetAgentID)
	queueValue, _ := s.targetQueues.LoadOrStore(key, &agentMessageTargetQueue{token: make(chan struct{}, 1)})
	queue := queueValue.(*agentMessageTargetQueue)
	select {
	case queue.token <- struct{}{}:
		defer func() { <-queue.token }()
	case <-ctx.Done():
		message.Status = agentrelation.MessageStatusFailed
		message.Error = "目标 Agent 排队等待超时"
		now := time.Now().UTC()
		message.CompletedAt = &now
		_ = s.messageRepo.Save(tenantID, message)
		return
	}

	s.active.Store(key, struct{}{})
	defer s.active.Delete(key)

	started := time.Now().UTC()
	message.Status = agentrelation.MessageStatusRunning
	message.StartedAt = &started
	_ = s.messageRepo.Save(tenantID, message)

	reply, err := s.runner.RunOneShot(ctx, tenantID, message.TargetAgent, envelope)
	completed := time.Now().UTC()
	message.CompletedAt = &completed
	if err != nil {
		message.Status = agentrelation.MessageStatusFailed
		message.Error = "目标 Agent 暂时无法完成消息"
	} else {
		message.Status = agentrelation.MessageStatusCompleted
		message.Reply = strings.TrimSpace(reply)
	}
	_ = s.messageRepo.Save(tenantID, message)
}

func containsRelationAction(actions []string, action string) bool {
	for _, candidate := range actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func relationContext(policy, summary, shared string) string {
	switch policy {
	case "none":
		return ""
	case "shared_thread":
		if strings.TrimSpace(shared) == "" {
			shared = summary
		}
		return truncateRunes(strings.TrimSpace(shared), maxAgentSharedContextRunes)
	default:
		return truncateRunes(strings.TrimSpace(summary), maxAgentSummaryContextRunes)
	}
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func buildAgentMessageEnvelope(source, target *agent.AgentConfig, relation, recipientView *agentrelation.AgentRelation, action, message, sharedContext string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[SPEEDING 组织消息]\n消息ID由 Hub 管理。你是接收方 %s；发送方是 %s。\n", target.Name, source.Name)
	fmt.Fprintf(&b, "关系范围：%s\n结构关系：%s\n动作：%s\n上下文策略：%s\n", relation.Scope, relation.RelationType, action, relation.ContextPolicy)
	if recipientView == nil {
		b.WriteString("你对发送方的当前关系：neutral（0，尚无反向关系状态）\n")
	} else {
		fmt.Fprintf(&b, "你对发送方的当前关系：%s（%d）\n", recipientView.Stance, recipientView.RelationshipScore)
	}
	if strings.TrimSpace(relation.Constraint) != "" {
		fmt.Fprintf(&b, "这条关系的强制约束：%s\n", relation.Constraint)
	}
	if sharedContext != "" {
		fmt.Fprintf(&b, "允许共享的上下文：\n%s\n", sharedContext)
	}
	fmt.Fprintf(&b, "发送方消息：\n%s\n\n", message)
	b.WriteString("请保持你自己的身份、利益和红线，直接回复发送方。不得假装知道未共享的上下文；本轮不要继续调用 agent_send 转发给第三人。")
	return b.String()
}

func agentMessageActiveKey(tenantID string, agentID uint64) string {
	return fmt.Sprintf("%s\x00%d", tenantID, agentID)
}

func agentMessageToDTO(message *agentrelation.AgentMessage) *AgentMessageDTO {
	dto := &AgentMessageDTO{
		ID:             message.ID,
		RelationID:     message.RelationID,
		Scope:          message.Scope,
		SourceAgent:    message.SourceAgent,
		TargetAgent:    message.TargetAgent,
		Action:         message.Action,
		DeliveryPolicy: message.DeliveryPolicy,
		ContextPolicy:  message.ContextPolicy,
		Status:         message.Status,
		Reply:          message.Reply,
		Error:          message.Error,
		CreatedAt:      message.CreatedAt.UTC().Format(time.RFC3339),
	}
	if message.StartedAt != nil {
		value := message.StartedAt.UTC().Format(time.RFC3339)
		dto.StartedAt = &value
	}
	if message.CompletedAt != nil {
		value := message.CompletedAt.UTC().Format(time.RFC3339)
		dto.CompletedAt = &value
	}
	return dto
}

// RunOneShot invokes a deployed target agent and aggregates its final text.
// It intentionally does not create a user-facing chat session: organization
// deliveries have their own durable audit log in agent_messages.
func (s *AgentChatService) RunOneShot(ctx context.Context, tenantID, agentName, message string) (string, error) {
	baseURL, apiKey, _, err := s.ResolveRuntime(tenantID, agentName)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"message": message})
	rc, err := s.runtimeClient.StreamRun(ctx, baseURL, NormalizeAgentName(agentName), apiKey, body, "")
	if err != nil {
		return "", err
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxAgentRunResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxAgentRunResponseBytes {
		return "", fmt.Errorf("目标 Agent 回复超过限制")
	}
	return extractOneShotReply(string(raw))
}

func extractOneShotReply(sse string) (string, error) {
	var eventName string
	var replies []string
	for _, rawLine := range strings.Split(sse, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch eventName {
		case "assistant":
			var data struct {
				Message struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal([]byte(payload), &data) != nil {
				continue
			}
			for _, block := range data.Message.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					replies = append(replies, block.Text)
				}
			}
		case "result":
			var data struct {
				Subtype   string   `json:"subtype"`
				ErrorType string   `json:"error_type"`
				Errors    []string `json:"errors"`
			}
			if json.Unmarshal([]byte(payload), &data) == nil && data.Subtype == "error" {
				if len(data.Errors) > 0 {
					return "", errors.New(strings.Join(data.Errors, "; "))
				}
				return "", errors.New(data.ErrorType)
			}
		}
	}
	if len(replies) == 0 {
		return "", fmt.Errorf("目标 Agent 未返回文本回复")
	}
	return strings.Join(replies, "\n"), nil
}
