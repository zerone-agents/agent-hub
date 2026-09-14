package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	eventdomain "control-panel/internal/domain/event"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/usage"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxAgentMessageRunes        = 16000
	maxAgentSummaryContextRunes = 4000
	maxAgentSharedContextRunes  = 12000
	maxAgentRunResponseBytes    = 16 << 20
	defaultSyncWaitTimeout      = 45 * time.Second
	defaultExecutionTimeout     = 5 * time.Minute
)

// AgentMessageRunner is deliberately small so relation policy can be tested
// without starting an HTTP runtime. AgentChatService implements it through
// RunOneShot.
type AgentMessageRunner interface {
	RunOneShot(ctx context.Context, tenantID, agentName, message string) (string, error)
}

type AgentMessageService struct {
	relationRepo     *repository.AgentRelationRepository
	messageRepo      *repository.AgentMessageRepository
	agentRepo        *repository.AgentRepository
	relationSvc      *AgentRelationService
	runner           AgentMessageRunner
	events           *EventService
	syncWaitTimeout  time.Duration
	executionTimeout time.Duration
	collaboration    *CollaborationService
	targetQueues     sync.Map // tenantID + NUL + agent id -> *agentMessageTargetQueue
	roundRobinMu     sync.Mutex
	// H6 persona hooks (WS6): nil packs leave Core behavior unchanged.
	personaBelief *BeliefService
	personaRelDyn *RelationDynamicsService
	// H7 P1 persona gate: nil leaves hooks always on（测试基座可不接）；
	// 非 nil 时投递/关系钩子生效前按扩展生命周期运行时查询，停用立即生效。
	personaGate *PersonaCapabilityGate
	// H7.5 usage hook: nil leaves behavior unchanged; Record never blocks.
	usage *UsageService
}

// SetUsageService 注入 H7.5 用量采集（advisory）。
func (s *AgentMessageService) SetUsageService(u *UsageService) { s.usage = u }

// SetPersonaHooks injects the H6 belief and relation-dynamics packs into the
// message dispatch path. Both hooks are advisory: failures are logged and
// never break message delivery or authorization.
func (s *AgentMessageService) SetPersonaHooks(belief *BeliefService, reldyn *RelationDynamicsService) {
	s.personaBelief, s.personaRelDyn = belief, reldyn
}

// SetPersonaCapabilityGate 接入 H7 能力门控（main.go 接线时调用）：
// 对应内置扩展被管理员停用后，投递/关系钩子不再生效。
func (s *AgentMessageService) SetPersonaCapabilityGate(g *PersonaCapabilityGate) { s.personaGate = g }

// personaPackEnabled 报告某人物能力包当前是否生效；未接 gate 时保持
// 原有"恒生效"行为（测试基座与旧接线兼容）。
func (s *AgentMessageService) personaPackEnabled(tenantID, pack string) bool {
	if s.personaGate == nil {
		return true
	}
	return s.personaGate.Enabled(tenantID, pack)
}

// recordBeliefDelivery is the H6 delivery hook: a durably delivered message
// becomes a fact the target agent holds a belief about. Idempotency key is
// deterministic per (message, agent) so replays are no-ops.
func (s *AgentMessageService) recordBeliefDelivery(tenantID string, message *agentrelation.AgentMessage) {
	if s.personaBelief == nil || !s.personaPackEnabled(tenantID, PersonaPackBelief) || message == nil || message.RunID == "" {
		return
	}
	err := s.personaBelief.RecordDelivery(tenantID, message.RunID, message.TargetAgentID, message.ID, message.CreatedAt, fmt.Sprintf("belief-delivery:%s:%d", message.ID, message.TargetAgentID))
	if err != nil {
		log.Printf("[h6] belief delivery record failed: message=%s agent=%d: %v", message.ID, message.TargetAgentID, err)
	}
}

type agentMessageTargetQueue struct {
	token chan struct{}
}

func NewAgentMessageService(runner AgentMessageRunner) *AgentMessageService {
	relationRepo := repository.NewAgentRelationRepository()
	agentRepo := repository.NewAgentRepository()
	return &AgentMessageService{
		relationRepo:     relationRepo,
		messageRepo:      repository.NewAgentMessageRepository(),
		agentRepo:        agentRepo,
		relationSvc:      &AgentRelationService{repo: relationRepo, agentRepo: agentRepo},
		runner:           runner,
		events:           NewEventService(relationRepo.DB()),
		syncWaitTimeout:  defaultSyncWaitTimeout,
		executionTimeout: defaultExecutionTimeout,
		collaboration:    NewCollaborationService(relationRepo.DB()),
	}
}

func newAgentMessageService(
	relationRepo *repository.AgentRelationRepository,
	messageRepo *repository.AgentMessageRepository,
	agentRepo *repository.AgentRepository,
	runner AgentMessageRunner,
) *AgentMessageService {
	return &AgentMessageService{
		relationRepo:     relationRepo,
		messageRepo:      messageRepo,
		agentRepo:        agentRepo,
		relationSvc:      &AgentRelationService{repo: relationRepo, agentRepo: agentRepo},
		runner:           runner,
		events:           NewEventService(messageRepo.DB()),
		syncWaitTimeout:  defaultSyncWaitTimeout,
		executionTimeout: defaultExecutionTimeout,
		collaboration:    NewCollaborationService(messageRepo.DB()),
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
	RouteDeviationReason    string
	SyncStack               []uint64
}

type AgentMessageDTO struct {
	ID               string   `json:"id"`
	RelationID       uint64   `json:"relationId"`
	DispatchID       string   `json:"dispatchId,omitempty"`
	GroupID          string   `json:"groupId,omitempty"`
	ChannelID        string   `json:"channelId,omitempty"`
	SessionID        string   `json:"sessionId,omitempty"`
	SourceAgentID    uint64   `json:"sourceAgentId"`
	TargetAgentID    uint64   `json:"targetAgentId"`
	Scope            string   `json:"scope"`
	SourceAgent      string   `json:"sourceAgent"`
	TargetAgent      string   `json:"targetAgent"`
	Action           string   `json:"action"`
	DeliveryPolicy   string   `json:"deliveryPolicy"`
	ContextPolicy    string   `json:"contextPolicy"`
	Status           string   `json:"status"`
	Content          string   `json:"content,omitempty"`
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
	RouteMode        string   `json:"routeMode,omitempty"`
	RoutePlanned     bool     `json:"routePlanned"`
	RouteDeviation   string   `json:"routeDeviation,omitempty"`
	Verified         bool     `json:"verified"`
	Verification     string   `json:"verification"`
}

type AgentMessageChainDTO struct {
	ConversationID string             `json:"conversationId"`
	RootMessageID  string             `json:"rootMessageId"`
	Messages       []*AgentMessageDTO `json:"messages"`
	MessageCount   int                `json:"messageCount"`
	Verified       bool               `json:"verified"`
}

type GroupMessageInput struct {
	GroupID, ChannelID, SessionID           string
	Action, Message, Audience, AudienceRole string
	MentionedAgents                         []string
	Aggregation, RunID, IdempotencyKey      string
}

type AgentMessageDispatchDTO struct {
	ID             string             `json:"id"`
	GroupID        string             `json:"groupId,omitempty"`
	ChannelID      string             `json:"channelId,omitempty"`
	SessionID      string             `json:"sessionId,omitempty"`
	Status         string             `json:"status"`
	Audience       string             `json:"audience"`
	AudienceRole   string             `json:"audienceRole,omitempty"`
	Aggregation    string             `json:"aggregation"`
	RecipientCount int                `json:"recipientCount"`
	CompletedCount int                `json:"completedCount"`
	FailedCount    int                `json:"failedCount"`
	Result         string             `json:"result,omitempty"`
	Deliveries     []*AgentMessageDTO `json:"deliveries"`
}

// GroupSend creates one durable dispatch and one independently executable
// delivery per recipient. It never waits for model responses; callers poll
// GroupMessageStatus, which prevents broadcast wait cycles and request storms.
func (s *AgentMessageService) GroupSend(tenantID string, source *agent.AgentConfig, input GroupMessageInput) (*AgentMessageDispatchDTO, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
	}
	input.GroupID, input.ChannelID, input.SessionID = strings.TrimSpace(input.GroupID), strings.TrimSpace(input.ChannelID), strings.TrimSpace(input.SessionID)
	input.Message, input.Action = strings.TrimSpace(input.Message), strings.TrimSpace(input.Action)
	if input.Message == "" {
		return nil, agentrelation.ErrMessageRequired
	}
	if utf8.RuneCountInString(input.Message) > maxAgentMessageRunes {
		return nil, agentrelation.ErrMessageTooLong
	}
	if input.Action == "" {
		input.Action = "inform"
	}
	if _, ok := agentrelation.Actions[input.Action]; !ok {
		return nil, agentrelation.ErrInvalidAction
	}
	if input.Audience == "" {
		input.Audience = "all"
	}
	if input.Aggregation == "" {
		input.Aggregation = agentrelation.AggregationAllReplies
	}
	if !validDispatchAudience(input.Audience, input.AudienceRole) {
		return nil, fmt.Errorf("audience 必须为 all、role、leaders 或 round_robin；role 模式必须提供 audience_role")
	}
	if !validDispatchAggregation(input.Aggregation) {
		return nil, fmt.Errorf("aggregation 必须为 all_replies、first_success 或 leader_summary")
	}
	if input.ChannelID != "" {
		channel, err := s.collaboration.Channel(tenantID, input.ChannelID)
		if err != nil {
			return nil, err
		}
		if input.GroupID != "" && input.GroupID != channel.GroupID {
			return nil, fmt.Errorf("channel 不属于指定 group")
		}
		input.GroupID = channel.GroupID
	}
	if input.GroupID == "" {
		return nil, fmt.Errorf("group_id 不能为空")
	}
	if _, err := s.collaboration.GetGroupMember(tenantID, input.GroupID, source.ID); err != nil {
		return nil, fmt.Errorf("发送方不是群组成员")
	}
	if input.SessionID != "" {
		room, err := s.collaboration.Session(tenantID, input.SessionID)
		if err != nil {
			return nil, err
		}
		if room.Status != collaboration.SessionActive || room.ChannelID != input.ChannelID {
			return nil, fmt.Errorf("会话房间未开始或不属于指定频道")
		}
		participant := false
		for _, item := range room.Participants {
			if item.AgentID == source.ID {
				participant = true
				break
			}
		}
		if !participant {
			return nil, fmt.Errorf("发送方不是会话房间参与者")
		}
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		key = uuid.NewString()
	}
	if existing, err := s.messageRepo.FindDispatchByIdempotencyKey(tenantID, key, source.ID); err == nil {
		return s.GroupMessageStatus(tenantID, source, existing.ID)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	recipients, err := s.resolveGroupRecipients(tenantID, source.ID, input)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("没有符合受众和订阅条件的接收方")
	}
	if input.Audience == "round_robin" {
		selected, selectErr := s.selectRoundRobinRecipient(tenantID, input, recipients)
		if selectErr != nil {
			return nil, selectErr
		}
		recipients = []agent.AgentConfig{selected}
	}
	if strings.TrimSpace(input.RunID) != "" {
		for _, target := range recipients {
			runValid, participantsValid, validateErr := s.messageRepo.ValidateRunParticipants(tenantID, input.RunID, source.ID, target.ID)
			if validateErr != nil {
				return nil, validateErr
			}
			if !runValid {
				return nil, agentrelation.ErrRouteNotFound
			}
			if !participantsValid {
				return nil, fmt.Errorf("群消息接收方 %s 不是 Run 参与者: %w", target.Name, agentrelation.ErrRunParticipantDenied)
			}
		}
	}
	dispatch := &agentrelation.AgentMessageDispatch{ID: uuid.NewString(), SourceAgentID: source.ID, SourceAgent: source.Name, GroupID: input.GroupID, ChannelID: input.ChannelID, SessionID: input.SessionID, Audience: input.Audience, AudienceRole: input.AudienceRole, Action: input.Action, Content: input.Message, Aggregation: input.Aggregation, Status: agentrelation.DispatchStatusQueued, RecipientCount: len(recipients), IdempotencyKey: key, RunID: strings.TrimSpace(input.RunID), CreatedAt: time.Now().UTC()}
	if err := s.messageRepo.CreateDispatch(tenantID, dispatch); err != nil {
		if existing, findErr := s.messageRepo.FindDispatchByIdempotencyKey(tenantID, key, source.ID); findErr == nil {
			return s.GroupMessageStatus(tenantID, source, existing.ID)
		}
		return nil, err
	}
	type deliveryPlan struct {
		target   agent.AgentConfig
		message  *agentrelation.AgentMessage
		envelope string
	}
	plans := make([]deliveryPlan, 0, len(recipients))
	deliveries := make([]*AgentMessageDTO, 0, len(recipients))
	for _, target := range recipients {
		message := &agentrelation.AgentMessage{ID: uuid.NewString(), DispatchID: dispatch.ID, GroupID: input.GroupID, ChannelID: input.ChannelID, SessionID: input.SessionID, RunID: dispatch.RunID, ConversationID: dispatch.ID, RootMessageID: dispatch.ID, Hop: 1, MaxHops: 8, EventBudget: 64, EventCount: 1, TokenBudget: 65536, TokensUsed: estimateMessageTokens(input.Message), VisitedAgentIDs: []uint64{source.ID, target.ID}, IdempotencyKey: key + ":" + fmt.Sprint(target.ID), Scope: "group:" + input.GroupID, SourceAgentID: source.ID, SourceAgent: source.Name, TargetAgentID: target.ID, TargetAgent: target.Name, Action: input.Action, DeliveryPolicy: "async", ContextPolicy: "none", Content: input.Message, Status: agentrelation.MessageStatusQueued, CreatedAt: time.Now().UTC()}
		persisted, created, createErr := s.createMessageWithEvents(tenantID, message)
		if createErr != nil {
			return nil, fmt.Errorf("创建群消息收件人投递失败: %w", createErr)
		}
		if !created {
			return nil, fmt.Errorf("群消息收件人投递幂等键冲突")
		}
		s.recordBeliefDelivery(tenantID, persisted)
		relation := &agentrelation.AgentRelation{Scope: message.Scope, RelationType: "group_member", DeliveryPolicy: "async", ContextPolicy: "none"}
		envelope := buildAgentMessageEnvelope(source, &target, relation, persisted)
		deliveries = append(deliveries, agentMessageToDTO(persisted))
		plans = append(plans, deliveryPlan{target: target, message: persisted, envelope: envelope})
	}
	for _, plan := range plans {
		go func(target agent.AgentConfig, msg *agentrelation.AgentMessage, env string) {
			ctx, cancel := context.WithTimeout(context.Background(), s.executionTimeout)
			defer cancel()
			s.execute(ctx, tenantID, target.ID, msg, env)
		}(plan.target, plan.message, plan.envelope)
	}
	return &AgentMessageDispatchDTO{ID: dispatch.ID, GroupID: dispatch.GroupID, ChannelID: dispatch.ChannelID, SessionID: dispatch.SessionID, Status: dispatch.Status, Audience: dispatch.Audience, AudienceRole: dispatch.AudienceRole, Aggregation: dispatch.Aggregation, RecipientCount: dispatch.RecipientCount, Deliveries: deliveries}, nil
}

func validDispatchAudience(audience, role string) bool {
	return audience == "all" || audience == "leaders" || audience == "round_robin" || (audience == "role" && strings.TrimSpace(role) != "")
}
func validDispatchAggregation(v string) bool {
	return v == agentrelation.AggregationAllReplies || v == agentrelation.AggregationFirstSuccess || v == agentrelation.AggregationLeader
}

func (s *AgentMessageService) resolveGroupRecipients(tenantID string, sourceID uint64, input GroupMessageInput) ([]agent.AgentConfig, error) {
	members, err := s.collaboration.Members(tenantID, input.GroupID)
	if err != nil {
		return nil, err
	}
	allowed := make(map[uint64]bool, len(members))
	roles := make(map[uint64]string, len(members))
	for _, member := range members {
		if member.AgentID != sourceID {
			allowed[member.AgentID] = true
			roles[member.AgentID] = member.Role
		}
	}
	if input.ChannelID != "" {
		recipients, err := s.collaboration.ListChannelRecipients(tenantID, input.ChannelID)
		if err != nil {
			return nil, err
		}
		mentioned := map[string]bool{}
		for _, name := range input.MentionedAgents {
			mentioned[NormalizeAgentName(name)] = true
		}
		allowed = map[uint64]bool{}
		for _, recipient := range recipients {
			if recipient.Member.AgentID == sourceID {
				continue
			}
			if recipient.SubscriptionMode == collaboration.SubscriptionMentions {
				a, findErr := s.agentRepo.GetByID(tenantID, recipient.Member.AgentID)
				if findErr != nil || !mentioned[a.Name] {
					continue
				}
			}
			allowed[recipient.Member.AgentID] = true
		}
	}
	if input.SessionID != "" {
		room, err := s.collaboration.Session(tenantID, input.SessionID)
		if err != nil {
			return nil, err
		}
		participants := make(map[uint64]bool, len(room.Participants))
		for _, participant := range room.Participants {
			participants[participant.AgentID] = true
		}
		for id := range allowed {
			if !participants[id] {
				delete(allowed, id)
			}
		}
	}
	result := make([]agent.AgentConfig, 0, len(allowed))
	for id := range allowed {
		role := roles[id]
		if input.Audience == "leaders" && role != collaboration.RoleLeader {
			continue
		}
		if input.Audience == "role" && role != input.AudienceRole {
			continue
		}
		if input.Audience == "round_robin" && strings.TrimSpace(input.AudienceRole) != "" && role != input.AudienceRole {
			continue
		}
		target, err := s.agentRepo.GetByID(tenantID, id)
		if err != nil {
			continue
		}
		result = append(result, *target)
	}
	return result, nil
}

func (s *AgentMessageService) selectRoundRobinRecipient(tenantID string, input GroupMessageInput, candidates []agent.AgentConfig) (agent.AgentConfig, error) {
	if len(candidates) == 0 {
		return agent.AgentConfig{}, fmt.Errorf("没有可轮询的接收方")
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	scope := "group:" + input.GroupID
	if input.ChannelID != "" {
		scope = "channel:" + input.ChannelID
	}
	if strings.TrimSpace(input.AudienceRole) != "" {
		scope += ":role:" + strings.TrimSpace(input.AudienceRole)
	}
	s.roundRobinMu.Lock()
	defer s.roundRobinMu.Unlock()
	selected := candidates[0]
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		selected = candidates[0]
		err = s.collaboration.db.Transaction(func(tx *gorm.DB) error {
			var cursor agentrelation.AgentMessageDispatchCursor
			result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id=? AND scope_key=?", tenantID, scope).First(&cursor)
			if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			if result.Error == nil {
				for _, candidate := range candidates {
					if candidate.ID > cursor.LastAgentID {
						selected = candidate
						break
					}
				}
				cursor.LastAgentID = selected.ID
				cursor.UpdatedAt = time.Now().UTC()
				return tx.Save(&cursor).Error
			}
			cursor = agentrelation.AgentMessageDispatchCursor{TenantID: tenantID, ScopeKey: scope, LastAgentID: selected.ID, UpdatedAt: time.Now().UTC()}
			return tx.Create(&cursor).Error
		})
		if err == nil {
			return selected, nil
		}
		// Another Hub replica may have inserted the unique cursor between our
		// miss and create. Retrying converts that race into a locked update.
		var count int64
		if countErr := s.collaboration.db.Model(&agentrelation.AgentMessageDispatchCursor{}).Where("tenant_id=? AND scope_key=?", tenantID, scope).Count(&count).Error; countErr != nil || count == 0 {
			break
		}
	}
	return selected, err
}

func (s *AgentMessageService) GroupMessageStatus(tenantID string, source *agent.AgentConfig, id string) (*AgentMessageDispatchDTO, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
	}
	if _, err := s.messageRepo.RefreshDispatch(tenantID, id); err != nil {
		return nil, err
	}
	dispatch, messages, err := s.messageRepo.GetDispatchForAgent(tenantID, id, source.ID)
	if err != nil {
		return nil, err
	}
	if source.ID == dispatch.SourceAgentID {
		result := aggregateDispatchResult(dispatch.Aggregation, messages, func(agentID uint64) bool {
			member, memberErr := s.collaboration.GetGroupMember(tenantID, dispatch.GroupID, agentID)
			return memberErr == nil && member.Role == collaboration.RoleLeader
		})
		if result != dispatch.Result {
			_ = s.messageRepo.UpdateDispatchResult(tenantID, dispatch.ID, result)
			dispatch.Result = result
		}
	} else {
		dispatch.Result = ""
	}
	dto := &AgentMessageDispatchDTO{ID: dispatch.ID, GroupID: dispatch.GroupID, ChannelID: dispatch.ChannelID, SessionID: dispatch.SessionID, Status: dispatch.Status, Audience: dispatch.Audience, AudienceRole: dispatch.AudienceRole, Aggregation: dispatch.Aggregation, RecipientCount: dispatch.RecipientCount, CompletedCount: dispatch.CompletedCount, FailedCount: dispatch.FailedCount, Result: dispatch.Result, Deliveries: make([]*AgentMessageDTO, 0, len(messages))}
	for _, message := range messages {
		dto.Deliveries = append(dto.Deliveries, agentMessageToDTO(message))
	}
	return dto, nil
}

func aggregateDispatchResult(mode string, messages []*agentrelation.AgentMessage, isLeader func(uint64) bool) string {
	type reply struct {
		Agent string `json:"agent"`
		Reply string `json:"reply"`
	}
	replies := make([]reply, 0, len(messages))
	for _, message := range messages {
		if message.Status != agentrelation.MessageStatusCompleted || strings.TrimSpace(message.Reply) == "" {
			continue
		}
		if mode == agentrelation.AggregationLeader && !isLeader(message.TargetAgentID) {
			continue
		}
		if mode == agentrelation.AggregationFirstSuccess || mode == agentrelation.AggregationLeader {
			return message.Reply
		}
		replies = append(replies, reply{Agent: message.TargetAgent, Reply: message.Reply})
	}
	if len(replies) == 0 {
		return ""
	}
	raw, err := json.Marshal(replies)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *AgentMessageService) StartSession(tenantID string, source *agent.AgentConfig, sessionID string) (*collaboration.Session, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
	}
	room, err := s.collaboration.Session(tenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	channel, err := s.collaboration.Channel(tenantID, room.ChannelID)
	if err != nil {
		return nil, err
	}
	member, err := s.collaboration.GetGroupMember(tenantID, channel.GroupID, source.ID)
	if err != nil {
		return nil, fmt.Errorf("当前 Agent 不是会话房间成员")
	}
	if room.HostAgentID != 0 && room.HostAgentID != source.ID && member.Role != collaboration.RoleLeader {
		return nil, fmt.Errorf("只有主持人或群组 leader 可以开始会话")
	}
	return s.collaboration.StartSession(tenantID, room.ID)
}

func (s *AgentMessageService) EndSession(tenantID string, source *agent.AgentConfig, sessionID, summary string) (*collaboration.Session, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
	}
	room, err := s.collaboration.Session(tenantID, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	channel, err := s.collaboration.Channel(tenantID, room.ChannelID)
	if err != nil {
		return nil, err
	}
	member, err := s.collaboration.GetGroupMember(tenantID, channel.GroupID, source.ID)
	if err != nil {
		return nil, fmt.Errorf("当前 Agent 不是会话房间成员")
	}
	if room.HostAgentID != 0 && room.HostAgentID != source.ID && member.Role != collaboration.RoleLeader {
		return nil, fmt.Errorf("只有主持人或群组 leader 可以结束会话")
	}
	return s.collaboration.CompleteSession(tenantID, room.ID, summary)
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
	routeMode, routePlanned, routeDeviation := "", false, strings.TrimSpace(input.RouteDeviationReason)
	persistAuthorizationGuard := func(reason string, authErr error) (*AgentMessageDTO, error) {
		guard := &agentrelation.AgentMessage{ID: messageID, RunID: runID, ConversationID: chainID, RootMessageID: rootID, ParentMessageID: strings.TrimSpace(input.ParentMessageID), Hop: hop, MaxHops: maxHops, DeadlineAt: &deadline, EventBudget: eventBudget, EventCount: eventCount, TokenBudget: tokenBudget, TokensUsed: tokensUsed, VisitedAgentIDs: visited, IdempotencyKey: idempotencyKey, GuardReason: reason, RouteMode: routeMode, RoutePlanned: routePlanned, RouteDeviation: routeDeviation, Scope: input.Scope, SourceAgentID: source.ID, SourceAgent: source.Name, TargetAgentID: target.ID, TargetAgent: target.Name, Action: input.Action, DeliveryPolicy: "async", ContextPolicy: "none", Content: input.Message, Status: agentrelation.MessageStatusGuarded, CreatedAt: time.Now().UTC()}
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
	if runID != "" {
		routeMode, routePlanned, err = s.messageRepo.EvaluateRunRoute(tenantID, runID, hop, source.ID, target.ID, input.Action)
		if err != nil {
			return nil, err
		}
		if routeMode == rundomain.RouteModeStrict && !routePlanned {
			return persistAuthorizationGuard("run_route_denied", agentrelation.ErrRunRouteDenied)
		}
		if routeMode == rundomain.RouteModeAdaptive && !routePlanned && routeDeviation == "" {
			return persistAuthorizationGuard("route_deviation_reason_required", agentrelation.ErrRouteReasonRequired)
		}
	}
	// H6 WS6 relation-dynamics influence hook: the attitude the TARGET holds
	// toward the source may require the target's confirmation for assign-like
	// actions; inform/report are unaffected. This never rewrites the relation
	// AllowedActions authorization above — it only records and surfaces.
	relationGate := ""
	if s.personaRelDyn != nil && runID != "" && s.personaPackEnabled(tenantID, PersonaPackRelationshipDynamics) {
		allowed, needConfirm, gateErr := s.personaRelDyn.Gate(tenantID, runID, target.ID, source.ID, input.Action)
		switch {
		case gateErr != nil:
			log.Printf("[h6] relation gate evaluation failed: run=%s pair=%d->%d: %v", runID, source.ID, target.ID, gateErr)
		case !allowed:
			return persistAuthorizationGuard("relation_gate_denied", fmt.Errorf("对方当前态度拒绝 %s 请求", input.Action))
		case needConfirm:
			relationGate = "relation_gate_need_confirm"
		}
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
	message := &agentrelation.AgentMessage{
		ID:         messageID,
		RelationID: relation.ID,
		RunID:      runID, ConversationID: chainID, RootMessageID: rootID, ParentMessageID: strings.TrimSpace(input.ParentMessageID),
		Hop: hop, MaxHops: maxHops, DeadlineAt: &deadline, EventBudget: eventBudget, EventCount: eventCount,
		TokenBudget: tokenBudget, TokensUsed: tokensUsed, VisitedAgentIDs: visited, IdempotencyKey: idempotencyKey, GuardReason: guardReason,
		RouteMode: routeMode, RoutePlanned: routePlanned, RouteDeviation: routeDeviation,
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
	// H6: the message is durably delivered from here on — record the belief
	// delivery fact for the target, and persist the gate verdict when the
	// relation-dynamics hook demanded the target's confirmation.
	s.recordBeliefDelivery(tenantID, message)
	if relationGate != "" {
		message.GuardReason = relationGate
		if saveErr := s.saveStatusWithEvent(tenantID, message); saveErr != nil {
			log.Printf("[h6] relation gate audit persist failed: message=%s: %v", message.ID, saveErr)
		}
	}

	envelope := buildAgentMessageEnvelope(source, target, relation, message)
	if relation.DeliveryPolicy == "async" {
		dto := agentMessageToDTO(message)
		go func() {
			asyncCtx, cancel := context.WithTimeout(context.Background(), s.executionTimeout)
			defer cancel()
			s.execute(asyncCtx, tenantID, target.ID, message, envelope)
		}()
		return dto, nil
	}

	// A synchronous connection controls how long the caller waits, not the
	// lifetime of the target execution. Browser/MCP cancellation must not turn
	// accepted work into a false failure. The durable message can be polled with
	// the same idempotency key or agent_message_status after this wait expires.
	done := make(chan struct{})
	go func() {
		executionCtx, cancel := context.WithTimeout(context.Background(), s.executionTimeout)
		defer cancel()
		s.execute(executionCtx, tenantID, target.ID, message, envelope)
		close(done)
	}()
	wait := s.syncWaitTimeout
	if wait <= 0 {
		wait = defaultSyncWaitTimeout
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
		return agentMessageToDTO(message), nil
	case <-timer.C:
		current, getErr := s.messageRepo.GetForAgent(tenantID, message.ID, source.ID)
		if getErr == nil {
			return agentMessageToDTO(current), nil
		}
		return agentMessageToDTO(message), nil
	}
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
		Data:           map[string]any{"messageId": message.ID, "parentMessageId": message.ParentMessageID, "sourceAgent": message.SourceAgent, "targetAgent": message.TargetAgent, "action": message.Action, "hop": message.Hop, "status": state, "guardReason": message.GuardReason, "tokensUsed": message.TokensUsed, "routeMode": message.RouteMode, "routePlanned": message.RoutePlanned, "routeDeviation": message.RouteDeviation},
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

func (s *AgentMessageService) ListChannel(tenantID, channelID string, limit int) ([]*AgentMessageDTO, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(channelID) == "" {
		return nil, fmt.Errorf("tenant and channel are required")
	}
	if _, err := s.collaboration.Channel(tenantID, channelID); err != nil {
		return nil, err
	}
	rows, err := s.messageRepo.ListForChannel(tenantID, channelID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]*AgentMessageDTO, 0, len(rows))
	for _, row := range rows {
		result = append(result, agentMessageToDTO(row))
	}
	return result, nil
}

// MessageChainForAgent exposes only hops in which the runtime identity is an
// endpoint. Knowing a conversation/root id never grants access to other hops.
func (s *AgentMessageService) MessageChainForAgent(tenantID string, source *agent.AgentConfig, conversationID, rootMessageID string, limit int) (*AgentMessageChainDTO, error) {
	if source == nil || source.TenantID != tenantID {
		return nil, agentrelation.ErrAgentNotFound
	}
	return s.messageChain(tenantID, source.ID, conversationID, rootMessageID, limit)
}

// MessageChainAdmin returns the complete tenant-local chain for audit users.
func (s *AgentMessageService) MessageChainAdmin(tenantID, conversationID, rootMessageID string, limit int) (*AgentMessageChainDTO, error) {
	return s.messageChain(tenantID, 0, conversationID, rootMessageID, limit)
}

func (s *AgentMessageService) messageChain(tenantID string, agentID uint64, conversationID, rootMessageID string, limit int) (*AgentMessageChainDTO, error) {
	conversationID, rootMessageID = strings.TrimSpace(conversationID), strings.TrimSpace(rootMessageID)
	if (conversationID == "") == (rootMessageID == "") {
		return nil, fmt.Errorf("必须且只能提供 conversation_id 或 root_message_id")
	}
	var rows []*agentrelation.AgentMessage
	var err error
	if agentID == 0 {
		rows, err = s.messageRepo.ListChain(tenantID, conversationID, rootMessageID, limit)
	} else {
		rows, err = s.messageRepo.ListChainForAgent(tenantID, conversationID, rootMessageID, agentID, limit)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, agentrelation.ErrMessageNotFound
	}
	result := &AgentMessageChainDTO{ConversationID: rows[0].ConversationID, RootMessageID: rows[0].RootMessageID, Messages: make([]*AgentMessageDTO, 0, len(rows)), MessageCount: len(rows), Verified: true}
	for _, row := range rows {
		dto := agentMessageToDTO(row)
		result.Messages = append(result.Messages, dto)
		result.Verified = result.Verified && dto.Verified
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
	// H7.5 埋点：message + model_call（tokens 为估算值；advisory）。
	if s.usage != nil {
		latencyMs := completed.Sub(started).Milliseconds()
		tokens := message.TokensUsed
		var msgErr string
		if err != nil {
			msgErr = "agent_run_failed"
		}
		var targetID uint64
		if message.TargetAgentID != 0 {
			targetID = message.TargetAgentID
		}
		s.usage.Record(UsageRecordInput{
			Kind: usage.KindMessage, TenantID: tenantID, AgentID: &targetID,
			RunID: message.ConversationID, LatencyMs: &latencyMs, Error: msgErr,
		})
		s.usage.Record(UsageRecordInput{
			Kind: usage.KindModelCall, TenantID: tenantID, AgentID: &targetID,
			RunID: message.ConversationID,
			TokensIn: &tokens, LatencyMs: &latencyMs, Error: msgErr,
		})
	}
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

func buildAgentMessageEnvelope(source, target *agent.AgentConfig, relation *agentrelation.AgentRelation, message *agentrelation.AgentMessage) string {
	var b strings.Builder
	connection := relation.ConnectionContract()
	fmt.Fprintf(&b, "[Agent Hub 组织消息]\n消息ID：%s\n你是接收方 %s；发送方是 %s。\n", message.ID, target.Name, source.Name)
	fmt.Fprintf(&b, "连接范围：%s\n连接类型：%s\n动作：%s\n上下文策略：%s\n", connection.Scope, connection.RelationType, message.Action, connection.ContextPolicy)
	deadline := "none"
	if message.DeadlineAt != nil {
		deadline = message.DeadlineAt.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(&b, "链路：conversation=%s root=%s parent=%s hop=%d/%d events=%d/%d tokens=%d/%d deadline=%s\n",
		message.ConversationID, message.RootMessageID, message.ParentMessageID, message.Hop, message.MaxHops,
		message.EventCount, message.EventBudget, message.TokensUsed, message.TokenBudget, deadline)
	fmt.Fprintf(&b, "已访问 Agent IDs：%v\n", message.VisitedAgentIDs)
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
		DispatchID:       message.DispatchID,
		GroupID:          message.GroupID,
		ChannelID:        message.ChannelID,
		SessionID:        message.SessionID,
		SourceAgentID:    message.SourceAgentID,
		TargetAgentID:    message.TargetAgentID,
		Scope:            message.Scope,
		SourceAgent:      message.SourceAgent,
		TargetAgent:      message.TargetAgent,
		Action:           message.Action,
		DeliveryPolicy:   message.DeliveryPolicy,
		ContextPolicy:    message.ContextPolicy,
		Status:           message.Status,
		Content:          message.Content,
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
		RouteMode:        message.RouteMode,
		RoutePlanned:     message.RoutePlanned,
		RouteDeviation:   message.RouteDeviation,
		EventBudget:      message.EventBudget, EventCount: message.EventCount,
		TokenBudget: message.TokenBudget, TokensUsed: message.TokensUsed, IdempotencyKey: message.IdempotencyKey,
	}
	dto.Verified, dto.Verification = messageVerification(message)
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

func messageVerification(message *agentrelation.AgentMessage) (bool, string) {
	switch message.Status {
	case agentrelation.MessageStatusCompleted:
		if message.CompletedAt != nil && strings.TrimSpace(message.Reply) != "" {
			return true, "completed_with_reply"
		}
		return false, "completion_evidence_incomplete"
	case agentrelation.MessageStatusGuarded:
		if strings.TrimSpace(message.GuardReason) != "" {
			return true, "guard_decision_recorded"
		}
		return false, "guard_evidence_incomplete"
	case agentrelation.MessageStatusFailed:
		if message.CompletedAt != nil && strings.TrimSpace(message.Error) != "" {
			return true, "failure_recorded"
		}
		return false, "failure_evidence_incomplete"
	case agentrelation.MessageStatusQueued, agentrelation.MessageStatusRunning:
		return true, "delivery_recorded"
	default:
		return false, "unknown_status"
	}
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
	case "run_route_denied":
		return "该次跳转不在本次任务的严格路径中"
	case "route_deviation_reason_required":
		return "自适应路由偏离计划时必须记录原因"
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
