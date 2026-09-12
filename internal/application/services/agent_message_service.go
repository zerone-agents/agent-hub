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
	eventdomain "control-panel/internal/domain/event"
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
	events       *EventService
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
		events:       NewEventService(relationRepo.DB()),
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
		events:       NewEventService(messageRepo.DB()),
	}
}

type SendAgentMessageInput struct {
	TargetAgent             string
	Scope                   string
	Action                  string
	Message                 string
	ContextSummary          string
	SharedContext           string
	RunID                   string
	ConversationID          string
	RootMessageID           string
	ParentMessageID         string
	Hop, MaxHops            int
	DeadlineAt              *time.Time
	EventBudget, EventCount int64
	TokenBudget, TokensUsed int64
	VisitedAgentIDs         []uint64
	IdempotencyKey          string
	SyncStack               []uint64
}

type AgentMessageDTO struct {
	ID               string   `json:"id"`
	RelationID       uint64   `json:"relationId"`
	Scope            string   `json:"scope"`
	SourceAgent      string   `json:"sourceAgent"`
	TargetAgent      string   `json:"targetAgent"`
	Action           string   `json:"action"`
	DeliveryPolicy   string   `json:"deliveryPolicy"`
	ContextPolicy    string   `json:"contextPolicy"`
	Status           string   `json:"status"`
	Reply            string   `json:"reply,omitempty"`
	Error            string   `json:"error,omitempty"`
	CreatedAt        string   `json:"createdAt"`
	StartedAt        *string  `json:"startedAt,omitempty"`
	CompletedAt      *string  `json:"completedAt,omitempty"`
	RunID            string   `json:"runId,omitempty"`
	ConversationID   string   `json:"conversationId"`
	RootMessageID    string   `json:"rootMessageId"`
	ParentMessageID  string   `json:"parentMessageId,omitempty"`
	Hop              int      `json:"hop"`
	MaxHops          int      `json:"maxHops"`
	VisitedAgentIDs  []uint64 `json:"visitedAgentIds"`
	GuardReason      string   `json:"guardReason,omitempty"`
	GuardDescription string   `json:"guardDescription,omitempty"`
	DeadlineAt       *string  `json:"deadlineAt,omitempty"`
	EventBudget      int64    `json:"eventBudget"`
	EventCount       int64    `json:"eventCount"`
	TokenBudget      int64    `json:"tokenBudget"`
	TokensUsed       int64    `json:"tokensUsed"`
	IdempotencyKey   string   `json:"idempotencyKey,omitempty"`
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
	if source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
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
	target, err := s.agentRepo.GetByName(tenantID, input.TargetAgent)
	if err != nil {
		return nil, agentrelation.ErrAgentNotFound
	}
	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	if existing, findErr := s.messageRepo.FindByIdempotencyKey(tenantID, idempotencyKey, source.ID); findErr == nil {
		return agentMessageToDTO(existing), nil
	} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		return nil, findErr
	}
	messageID := uuid.NewString()
	chainID, rootID, runID := uuid.NewString(), messageID, strings.TrimSpace(input.RunID)
	hop, maxHops := 1, 8
	eventBudget, eventCount := int64(64), int64(1)
	tokenBudget, tokensUsed := int64(65536), int64(0)
	deadline := time.Now().UTC().Add(30 * time.Minute)
	if input.DeadlineAt != nil && input.DeadlineAt.Before(deadline) {
		deadline = input.DeadlineAt.UTC()
	}
	visited := []uint64{source.ID, target.ID}
	if strings.TrimSpace(input.ParentMessageID) != "" {
		parent, parentErr := s.messageRepo.GetForAgent(tenantID, strings.TrimSpace(input.ParentMessageID), source.ID)
		if parentErr != nil || parent.TargetAgentID != source.ID {
			return nil, agentrelation.ErrMessageNotFound
		}
		chainID, rootID, runID = parent.ConversationID, parent.RootMessageID, parent.RunID
		hop, maxHops = parent.Hop+1, parent.MaxHops
		eventBudget, eventCount = parent.EventBudget, parent.EventCount+1
		tokenBudget, tokensUsed = parent.TokenBudget, parent.TokensUsed
		if parent.DeadlineAt != nil {
			deadline = *parent.DeadlineAt
		}
		visited = append(append([]uint64(nil), parent.VisitedAgentIDs...), target.ID)
	}
	// A deterministic conservative estimate keeps budget enforcement stable
	// across model providers. Actual output is added when delivery completes.
	tokensUsed += estimateMessageTokens(input.Message)
	persistAuthorizationGuard := func(reason string, authErr error) (*AgentMessageDTO, error) {
		guard := &agentrelation.AgentMessage{ID: messageID, RunID: runID, ConversationID: chainID, RootMessageID: rootID, ParentMessageID: strings.TrimSpace(input.ParentMessageID), Hop: hop, MaxHops: maxHops, DeadlineAt: &deadline, EventBudget: eventBudget, EventCount: eventCount, TokenBudget: tokenBudget, TokensUsed: tokensUsed, VisitedAgentIDs: visited, IdempotencyKey: idempotencyKey, GuardReason: reason, Scope: input.Scope, SourceAgentID: source.ID, SourceAgent: source.Name, TargetAgentID: target.ID, TargetAgent: target.Name, Action: input.Action, DeliveryPolicy: "async", ContextPolicy: "none", Content: input.Message, Status: agentrelation.MessageStatusGuarded, CreatedAt: time.Now().UTC()}
		persisted, _, persistErr := s.createMessageWithEvents(tenantID, guard)
		if persistErr != nil {
			return nil, fmt.Errorf("记录链路守卫失败: %w", persistErr)
		}
		return agentMessageToDTO(persisted), authErr
	}
	if runID != "" {
		runValid, participantsValid, err := s.messageRepo.ValidateRunParticipants(tenantID, runID, source.ID, target.ID)
		if err != nil {
			return nil, err
		}
		if !runValid {
			return nil, agentrelation.ErrRouteNotFound
		}
		if !participantsValid {
			return persistAuthorizationGuard("run_participant_denied", agentrelation.ErrRunParticipantDenied)
		}
	}
	if _, ok := agentrelation.Actions[input.Action]; !ok {
		return persistAuthorizationGuard("action_not_allowed", agentrelation.ErrInvalidAction)
	}
	relation, err := s.resolveRelation(tenantID, source.ID, target.ID, input.Scope, input.Action)
	if err != nil {
		reason := "route_not_found"
		if errors.Is(err, agentrelation.ErrActionNotAllowed) {
			reason = "action_not_allowed"
		}
		return persistAuthorizationGuard(reason, err)
	}
	sharedContext := relationContext(relation.ContextPolicy, input.ContextSummary, input.SharedContext)
	tokensUsed += estimateMessageTokens(sharedContext)
	guardReason := chainGuardReason(time.Now().UTC(), hop, maxHops, &deadline, eventCount, eventBudget, tokensUsed, tokenBudget)
	if relation.DeliveryPolicy == "sync" {
		waiting, waitErr := s.messageRepo.HasRunningSourceInChain(tenantID, rootID, target.ID)
		if waitErr != nil {
			return nil, waitErr
		}
		if waiting {
			guardReason = "sync_wait_cycle"
		}
	}
	// The target must act from its own directional view of the sender. The
	// authorizing A -> B edge is not evidence of how B feels about A.
	recipientView, err := s.relationRepo.FindEnabledEdge(tenantID, relation.Scope, target.ID, source.ID)
	if err != nil {
		return nil, fmt.Errorf("读取接收方关系状态失败: %w", err)
	}

	message := &agentrelation.AgentMessage{
		ID:         messageID,
		RelationID: relation.ID,
		RunID:      runID, ConversationID: chainID, RootMessageID: rootID, ParentMessageID: strings.TrimSpace(input.ParentMessageID),
		Hop: hop, MaxHops: maxHops, DeadlineAt: &deadline, EventBudget: eventBudget, EventCount: eventCount,
		TokenBudget: tokenBudget, TokensUsed: tokensUsed, VisitedAgentIDs: visited, IdempotencyKey: idempotencyKey, GuardReason: guardReason,
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
	if guardReason != "" {
		message.Status = agentrelation.MessageStatusGuarded
	}
	persisted, created, err := s.createMessageWithEvents(tenantID, message)
	if err != nil {
		return nil, fmt.Errorf("记录组织消息失败: %w", err)
	}
	if !created {
		return agentMessageToDTO(persisted), nil
	}
	if guardReason != "" {
		if guardReason == "sync_wait_cycle" {
			return agentMessageToDTO(message), agentrelation.ErrSyncDeadlock
		}
		return agentMessageToDTO(message), agentrelation.ErrChainGuarded
	}

	envelope := buildAgentMessageEnvelope(source, target, relation, recipientView, message)
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

func (s *AgentMessageService) createMessageWithEvents(tenantID string, message *agentrelation.AgentMessage) (*agentrelation.AgentMessage, bool, error) {
	var persisted *agentrelation.AgentMessage
	created := false
	err := s.events.db.Transaction(func(tx *gorm.DB) error {
		var err error
		persisted, created, err = s.messageRepo.CreateIdempotentTx(tx, tenantID, message)
		if err != nil || !created {
			return err
		}
		if message.RunID == "" {
			return nil
		}
		accepted, err := s.publishMessageEventTx(tx, tenantID, message, "accepted", message.ParentMessageID)
		if err != nil {
			return err
		}
		_, err = s.publishMessageEventTx(tx, tenantID, message, message.Status, accepted.ID)
		return err
	})
	return persisted, created, err
}

func (s *AgentMessageService) publishMessageEventTx(tx *gorm.DB, tenantID string, message *agentrelation.AgentMessage, state, cause string) (*eventdomain.Envelope, error) {
	return s.events.PublishTx(tx, tenantID, PublishEventInput{
		RunID: message.RunID, Type: "agenthub.message." + state + ".v1", Source: "agenthub:organization",
		Scope: eventdomain.Ref{Type: "run", ID: message.RunID}, Subject: eventdomain.Ref{Type: "agent", ID: fmt.Sprint(message.TargetAgentID)}, Actor: eventdomain.Ref{Type: "agent", ID: fmt.Sprint(message.SourceAgentID)},
		Visibility: "participants", CorrelationID: message.ConversationID, CausationID: cause, RootEventID: message.RootMessageID,
		IdempotencyKey: "agent-message:" + message.ID + ":" + state,
		Data:           map[string]any{"messageId": message.ID, "parentMessageId": message.ParentMessageID, "sourceAgent": message.SourceAgent, "targetAgent": message.TargetAgent, "action": message.Action, "hop": message.Hop, "status": state, "guardReason": message.GuardReason, "tokensUsed": message.TokensUsed},
	})
}

func (s *AgentMessageService) saveStatusWithEvent(tenantID string, message *agentrelation.AgentMessage) error {
	return s.events.db.Transaction(func(tx *gorm.DB) error {
		if err := s.messageRepo.SaveTx(tx, tenantID, message); err != nil {
			return err
		}
		if message.RunID == "" {
			return nil
		}
		_, err := s.publishMessageEventTx(tx, tenantID, message, message.Status, message.ID)
		return err
	})
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

func (s *AgentMessageService) ListRun(tenantID, runID string, limit int) ([]*AgentMessageDTO, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("tenant and run are required")
	}
	messages, err := s.messageRepo.ListForRun(tenantID, strings.TrimSpace(runID), limit)
	if err != nil {
		return nil, fmt.Errorf("读取运行消息链失败: %w", err)
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
		_ = s.saveStatusWithEvent(tenantID, message)
		return
	}

	started := time.Now().UTC()
	message.Status = agentrelation.MessageStatusRunning
	message.StartedAt = &started
	_ = s.saveStatusWithEvent(tenantID, message)

	reply, err := s.runner.RunOneShot(ctx, tenantID, message.TargetAgent, envelope)
	completed := time.Now().UTC()
	message.CompletedAt = &completed
	if err != nil {
		message.Status = agentrelation.MessageStatusFailed
		message.Error = "目标 Agent 暂时无法完成消息"
	} else {
		message.Status = agentrelation.MessageStatusCompleted
		message.Reply = strings.TrimSpace(reply)
		message.TokensUsed += estimateMessageTokens(message.Reply)
		if message.TokensUsed > message.TokenBudget {
			message.Status = agentrelation.MessageStatusGuarded
			message.GuardReason = "token_budget_exceeded"
		}
	}
	_ = s.saveStatusWithEvent(tenantID, message)
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

func chainGuardReason(now time.Time, hop, maxHops int, deadline *time.Time, events, eventBudget, tokens, tokenBudget int64) string {
	switch {
	case deadline != nil && !now.Before(*deadline):
		return "deadline_exceeded"
	case hop > maxHops:
		return "max_hops_exceeded"
	case events > eventBudget:
		return "event_budget_exceeded"
	case tokens > tokenBudget:
		return "token_budget_exceeded"
	default:
		return ""
	}
}

func estimateMessageTokens(value string) int64 {
	var ascii, nonASCII int64
	for _, r := range value {
		if r <= 127 {
			ascii++
		} else {
			nonASCII++
		}
	}
	return nonASCII + (ascii+3)/4
}

func buildAgentMessageEnvelope(source, target *agent.AgentConfig, relation, recipientView *agentrelation.AgentRelation, message *agentrelation.AgentMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Agent Hub 组织消息]\n消息ID：%s\n你是接收方 %s；发送方是 %s。\n", message.ID, target.Name, source.Name)
	fmt.Fprintf(&b, "关系范围：%s\n结构关系：%s\n动作：%s\n上下文策略：%s\n", relation.Scope, relation.RelationType, message.Action, relation.ContextPolicy)
	deadline := "none"
	if message.DeadlineAt != nil {
		deadline = message.DeadlineAt.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(&b, "链路：conversation=%s root=%s parent=%s hop=%d/%d events=%d/%d tokens=%d/%d deadline=%s\n",
		message.ConversationID, message.RootMessageID, message.ParentMessageID, message.Hop, message.MaxHops,
		message.EventCount, message.EventBudget, message.TokensUsed, message.TokenBudget, deadline)
	fmt.Fprintf(&b, "已访问 Agent IDs：%v\n", message.VisitedAgentIDs)
	if recipientView == nil {
		b.WriteString("你对发送方的当前关系：neutral（0，尚无反向关系状态）\n")
	} else {
		fmt.Fprintf(&b, "你对发送方的当前关系：%s（%d）\n", recipientView.Stance, recipientView.RelationshipScore)
	}
	if strings.TrimSpace(relation.Constraint) != "" {
		fmt.Fprintf(&b, "这条关系的强制约束：%s\n", relation.Constraint)
	}
	if message.SharedContext != "" {
		fmt.Fprintf(&b, "允许共享的上下文：\n%s\n", message.SharedContext)
	}
	fmt.Fprintf(&b, "发送方消息：\n%s\n\n", message.Content)
	b.WriteString("请保持你自己的身份、利益和红线，不得假装知道未共享的上下文。你可以按已授权关系异步转发、交接或回报；续跳调用 agent_send 时必须将本消息 ID 作为 parent_message_id，并提供稳定的 idempotency_key，链路字段由 Hub 继承。不要发起会回到当前同步调用栈的同步消息。")
	return b.String()
}

func agentMessageActiveKey(tenantID string, agentID uint64) string {
	return fmt.Sprintf("%s\x00%d", tenantID, agentID)
}

func agentMessageToDTO(message *agentrelation.AgentMessage) *AgentMessageDTO {
	dto := &AgentMessageDTO{
		ID:               message.ID,
		RelationID:       message.RelationID,
		Scope:            message.Scope,
		SourceAgent:      message.SourceAgent,
		TargetAgent:      message.TargetAgent,
		Action:           message.Action,
		DeliveryPolicy:   message.DeliveryPolicy,
		ContextPolicy:    message.ContextPolicy,
		Status:           message.Status,
		Reply:            message.Reply,
		Error:            message.Error,
		CreatedAt:        message.CreatedAt.UTC().Format(time.RFC3339),
		RunID:            message.RunID,
		ConversationID:   message.ConversationID,
		RootMessageID:    message.RootMessageID,
		ParentMessageID:  message.ParentMessageID,
		Hop:              message.Hop,
		MaxHops:          message.MaxHops,
		VisitedAgentIDs:  append([]uint64(nil), message.VisitedAgentIDs...),
		GuardReason:      message.GuardReason,
		GuardDescription: guardDescription(message.GuardReason),
		EventBudget:      message.EventBudget, EventCount: message.EventCount,
		TokenBudget: message.TokenBudget, TokensUsed: message.TokensUsed, IdempotencyKey: message.IdempotencyKey,
	}
	if message.DeadlineAt != nil {
		value := message.DeadlineAt.UTC().Format(time.RFC3339)
		dto.DeadlineAt = &value
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

func guardDescription(reason string) string {
	switch reason {
	case "deadline_exceeded":
		return "该协作链已超过截止时间"
	case "max_hops_exceeded":
		return "转交层级已达上限"
	case "event_budget_exceeded":
		return "本次协作的消息数已达上限"
	case "token_budget_exceeded":
		return "本次协作的 Token 用量已达上限"
	case "sync_wait_cycle":
		return "同步回路会互相等待，请改为异步回报"
	case "route_not_found":
		return "当前 Agent 没有向目标发送该消息的组织关系"
	case "action_not_allowed":
		return "该组织关系不允许这类动作"
	case "run_participant_denied":
		return "目标 Agent 未加入本次运行"
	default:
		return ""
	}
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
