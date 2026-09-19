package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/collaboration"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

type OrganizationMessageService interface {
	Relations(tenantID string, source *agent.AgentConfig) ([]*services.AgentRelationDTO, error)
	Send(ctx context.Context, tenantID string, source *agent.AgentConfig, input services.SendAgentMessageInput) (*services.AgentMessageDTO, error)
	Get(tenantID string, source *agent.AgentConfig, id string) (*services.AgentMessageDTO, error)
	Inbox(tenantID string, source *agent.AgentConfig, limit int) ([]*services.AgentMessageDTO, error)
	MessageChainForAgent(tenantID string, source *agent.AgentConfig, conversationID, rootMessageID string, limit int) (*services.AgentMessageChainDTO, error)
}

type GroupOrganizationMessageService interface {
	GroupSend(tenantID string, source *agent.AgentConfig, input services.GroupMessageInput) (*services.AgentMessageDispatchDTO, error)
	GroupMessageStatus(tenantID string, source *agent.AgentConfig, id string) (*services.AgentMessageDispatchDTO, error)
	StartSession(tenantID string, source *agent.AgentConfig, sessionID string) (*collaboration.Session, error)
	EndSession(tenantID string, source *agent.AgentConfig, sessionID, summary string) (*collaboration.Session, error)
}

// OrganizationMcpHandler exposes relation-authorized agent-to-agent delivery
// through MCP. Authentication is the caller runtime's Bearer token; source
// identity is never accepted from tool arguments.
type OrganizationMcpHandler struct {
	service         OrganizationMessageService
	workflowService *services.WorkflowService
	decisionService *services.DecisionService

	// H6 persona capability packs (WS6 wiring). nil pack → its tools answer
	// "能力尚未启用"; personaRunResolver nil → "仅运行会话可用".
	emotionService     *services.EmotionService
	beliefService      *services.BeliefService
	memoryService      *services.MemoryService
	relationDynService *services.RelationDynamicsService
	personaRunResolver func(tenantID string, agentID uint64) (string, error)
	// H7 P1 persona gate: nil keeps tools always available（测试基座可不接）；
	// 非 nil 时每个人物工具在入口按扩展生命周期运行时查询，被管理员
	// 停用的能力返回明确的"能力 <pack> 已被停用"错误。
	personaGate *services.PersonaCapabilityGate
}

// SetPersonaServices injects the four H6 persona capability packs. Any nil
// pack disables its tools with the standard "能力尚未启用" response.
func (h *OrganizationMcpHandler) SetPersonaServices(emotion *services.EmotionService, belief *services.BeliefService, memory *services.MemoryService, reldyn *services.RelationDynamicsService) {
	h.emotionService, h.beliefService, h.memoryService, h.relationDynService = emotion, belief, memory, reldyn
}

// SetPersonaRunResolver binds run-session resolution (runtime token → active
// run) so persona tools never need a run_id argument.
func (h *OrganizationMcpHandler) SetPersonaRunResolver(fn func(tenantID string, agentID uint64) (string, error)) {
	h.personaRunResolver = fn
}

// SetPersonaGate 接入 H7 能力门控（main.go 接线时调用）：内置人物能力
// 扩展被管理员停用时，对应工具返回"能力 <pack> 已被停用"。
func (h *OrganizationMcpHandler) SetPersonaGate(g *services.PersonaCapabilityGate) {
	h.personaGate = g
}

// personaPackEnabled 报告某人物能力包当前是否生效；未接 gate 时保持
// 工具恒可用（测试基座与旧接线兼容），但打一条一次性告警以避免
// "漏挂 SetPersonaGate"变成无人知晓的静默 fail-open。
func (h *OrganizationMcpHandler) personaPackEnabled(tenantID, pack string) bool {
	if h.personaGate == nil {
		services.WarnPersonaGateMissing("OrganizationMcpHandler")
		return true
	}
	return h.personaGate.Enabled(tenantID, pack)
}

// personaDisabledError 是人物工具被扩展生命周期停用时的统一错误。
func personaDisabledError(id interface{}, pack string) jsonRPCResponse {
	return mcpErrorResult(id, fmt.Sprintf("能力 %s 已被停用", pack))
}

func (h *OrganizationMcpHandler) SetWorkflowService(service *services.WorkflowService) {
	h.workflowService = service
}
func (h *OrganizationMcpHandler) SetDecisionService(service *services.DecisionService) {
	h.decisionService = service
}

func NewOrganizationMcpHandler(service OrganizationMessageService) *OrganizationMcpHandler {
	return &OrganizationMcpHandler{service: service}
}

func (h *OrganizationMcpHandler) HandleMessage(c *gin.Context) {
	var req jsonRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", Error: &jsonRPCError{Code: -32700, Message: "Parse error"}})
		return
	}

	switch req.Method {
	case "initialize":
		c.JSON(http.StatusOK, h.handleInitialize(req.ID))
	case "notifications/initialized":
		c.Status(http.StatusNoContent)
	case "tools/list":
		c.JSON(http.StatusOK, h.handleToolsList(c, req.ID))
	case "tools/call":
		response, err := h.handleToolsCall(c.Request.Context(), c, req.ID, req.Params)
		if err != nil {
			c.JSON(http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonRPCError{Code: -32603, Message: err.Error()}})
			return
		}
		c.JSON(http.StatusOK, response)
	default:
		c.JSON(http.StatusOK, jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &jsonRPCError{Code: -32601, Message: "Method not found"}})
	}
}

func (h *OrganizationMcpHandler) handleInitialize(id interface{}) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{"listChanged": true}},
			"serverInfo":      map[string]interface{}{"name": "organization-mcp", "version": "1.0.0"},
		},
	}
}

// personaToolPacks 把人物能力工具名映射到其所属能力包。
// tools/list 与 tools/call 共用同一份映射：宣称可用的工具集必须与
// 实际能调通的工具集一致，否则 Agent 会反复撞"能力已被停用"。
var personaToolPacks = map[string]string{
	"emotion_status": services.PersonaPackEmotion,
	"belief_list":    services.PersonaPackBelief,
	"belief_claim":   services.PersonaPackBelief,
	"memory_record":  services.PersonaPackMemory,
	"memory_recall":  services.PersonaPackMemory,
	"relation_view":  services.PersonaPackRelationshipDynamics,
}

// handleToolsList 返回当前租户下真正可用的工具清单：
// 对应内置扩展被停用的人物工具不再出现在列表里（此前的实现无条件宣称
// 全部可用，Agent 按列表调用只会拿到"能力 X 已被停用"，反复浪费一轮）。
func (h *OrganizationMcpHandler) handleToolsList(c *gin.Context, id interface{}) jsonRPCResponse {
	tools := []map[string]interface{}{
		{
			"name":        "agent_relations",
			"description": "列出当前 Agent 的已启用组织关系，包括方向、范围、允许动作、立场、上下文和投递策略。向其他 Agent 发消息前先调用此工具；不能把入向关系当作出向权限。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			"name":        "agent_send",
			"description": "按已配置的有向关系向另一个 Agent 发送消息。续跳只需提供 parent_message_id，Hub 会校验当前 Agent 是父消息接收方，并继承 conversation、root、hop、deadline、budget 和 trace；Agent 不能自行重置链路。",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target_agent":           map[string]interface{}{"type": "string", "description": "关系中接收方的 Agent ID，例如 finance-reviewer"},
					"scope":                  map[string]interface{}{"type": "string", "description": "关系范围。仅存在一个匹配范围时可省略；多范围必须明确填写。"},
					"action":                 map[string]interface{}{"type": "string", "enum": []string{"inform", "consult", "assign", "report", "submit", "review", "challenge", "handoff", "escalate", "invite"}},
					"message":                map[string]interface{}{"type": "string", "description": "给接收方的任务、事实、问题或挑战。不要在此伪造对方回复。"},
					"context_summary":        map[string]interface{}{"type": "string", "description": "当关系允许 summary_only/shared_thread 时可附带的必要摘要；none 策略会由 Hub 丢弃。"},
					"shared_context":         map[string]interface{}{"type": "string", "description": "仅 shared_thread 策略可传递的完整上下文；其他策略会截断或丢弃。"},
					"run_id":                 map[string]interface{}{"type": "string", "description": "所属 Run；后续转发必须继承。"},
					"parent_message_id":      map[string]interface{}{"type": "string", "description": "当前这一跳的直接上游消息 ID。"},
					"deadline":               map[string]interface{}{"type": "string", "format": "date-time", "description": "RFC3339 链路截止时间。"},
					"idempotency_key":        map[string]interface{}{"type": "string", "description": "本跳稳定唯一键；重试时必须复用。"},
					"route_deviation_reason": map[string]interface{}{"type": "string", "description": "Run 使用 adaptive 路由且本跳不在计划内时必填，用于审计偏离原因。"},
				},
				"required": []string{"target_agent", "action", "message"},
			},
		},
		{
			"name":        "agent_message_status",
			"description": "查询一条组织消息的状态和目标回复。只有消息的发送方或接收方可以读取。",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"message_id": map[string]interface{}{"type": "string"}},
				"required":   []string{"message_id"},
			},
		},
		{
			"name":        "agent_inbox",
			"description": "列出当前 Agent 最近的入向和出向组织消息及处理状态，用于复盘异步协作。",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"limit": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}},
			},
		},
		{
			"name":        "agent_message_chain",
			"description": "按 conversation_id 或 root_message_id 查询当前 Agent 实际参与的协作链路。只返回当前 Agent 作为发送方或接收方的消息；verified 表示 Hub 是否留有可核验执行证据。",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"conversation_id": map[string]interface{}{"type": "string"},
					"root_message_id": map[string]interface{}{"type": "string"},
					"limit":           map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "default": 100},
				},
			},
		},
		groupMessageTool("group_send", "向群组成员异步群发消息；每个接收方独立执行并留痕，不同步等待全部回复。", false),
		groupMessageTool("channel_publish", "向频道订阅者异步发布消息；支持全员、按角色、仅 leader 和 @提及订阅。", true),
		{
			"name": "group_message_status", "description": "查询群发聚合状态和当前 Agent 有权查看的逐收件人投递记录。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"dispatch_id": map[string]interface{}{"type": "string"}}, "required": []string{"dispatch_id"}},
		},
		{
			"name": "session_start", "description": "开始一个已创建的轻量会话房间。仅主持人或群组 leader 可执行。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"session_id": map[string]interface{}{"type": "string"}}, "required": []string{"session_id"}},
		},
		{
			"name": "session_end", "description": "结束活动中的轻量会话房间并保存总结。仅主持人或群组 leader 可执行。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"session_id": map[string]interface{}{"type": "string"}, "summary": map[string]interface{}{"type": "string"}}, "required": []string{"session_id", "summary"}},
		},
		{
			"name": "workflow_start", "description": "启动一个已发布的通用工作流版本。相同 idempotency_key 重试会返回同一执行实例。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"version_id": map[string]interface{}{"type": "string"}, "run_id": map[string]interface{}{"type": "string"}, "input": map[string]interface{}{"type": "object"}, "idempotency_key": map[string]interface{}{"type": "string"}}, "required": []string{"version_id", "idempotency_key"}},
		},
		{
			"name": "approval_vote", "description": "对分配给当前 Agent 的审批做出批准、拒绝或附条件批准决定。审批开始时的审批人快照不会被后续组织变化改写。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"approval_id": map[string]interface{}{"type": "string"}, "decision": map[string]interface{}{"type": "string", "enum": []string{"approve", "reject", "conditional_approve"}}, "reason": map[string]interface{}{"type": "string"}, "conditions": map[string]interface{}{"type": "object"}, "idempotency_key": map[string]interface{}{"type": "string"}}, "required": []string{"approval_id", "decision", "idempotency_key"}},
		},
		{
			"name": "decision_vote", "description": "当前 Agent 对群体决策投赞成、反对或弃权票。投票身份来自运行时令牌，且必须属于决策开始时冻结的选民快照。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"decision_id": map[string]interface{}{"type": "string"}, "choice": map[string]interface{}{"type": "string", "enum": []string{"approve", "reject", "abstain"}}, "reason": map[string]interface{}{"type": "string"}}, "required": []string{"decision_id", "choice"}},
		},
		{"name": "workflow_step_complete", "description": "回执当前 Agent 被分配的普通工作流步骤，并推进后续步骤。", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"step_run_id": map[string]interface{}{"type": "string"}, "output": map[string]interface{}{"type": "object"}, "idempotency_key": map[string]interface{}{"type": "string"}}, "required": []string{"step_run_id", "idempotency_key"}}},
		{"name": "workflow_step_fail", "description": "报告当前 Agent 被分配的工作流步骤失败，触发重试、补偿或确定性失败收敛。", "inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"step_run_id": map[string]interface{}{"type": "string"}, "error": map[string]interface{}{"type": "string"}, "idempotency_key": map[string]interface{}{"type": "string"}}, "required": []string{"step_run_id", "error", "idempotency_key"}}},
		{
			"name":        "emotion_status",
			"description": "读取自己当前的心情状态与确定性叙述。身份来自运行时令牌；仅运行会话可用。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			"name":        "belief_list",
			"description": "查看自己在当前 Run 中的看法列表，可按 factRef 过滤。只能读到送达过自己的事实。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"fact_ref": map[string]interface{}{"type": "string"}}},
		},
		{
			"name":        "belief_claim",
			"description": "提交一条“我声称…”的看法声明，生成 claim 类事实引用进入事件链，不直接改写他人看法。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"fact_ref": map[string]interface{}{"type": "string"}, "statement": map[string]interface{}{"type": "string"}}, "required": []string{"fact_ref", "statement"}},
		},
		{
			"name":        "memory_record",
			"description": "用自己的话记录一条主观记忆（关联事实、解释、可选重要度 0-100）。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"fact_ref": map[string]interface{}{"type": "string"}, "interpretation": map[string]interface{}{"type": "string"}, "importance": map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 100}}, "required": []string{"fact_ref", "interpretation"}},
		},
		{
			"name":        "memory_recall",
			"description": "按检索评分取回自己的 top-N 记忆（默认 5 条），可选 query 与 limit。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}, "limit": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50}}},
		},
		{
			"name":        "relation_view",
			"description": "查看自己对目标 Agent 的定向态度（分数、立场、叙述）。只能看自己的态度。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"target_agent_id": map[string]interface{}{"type": "integer"}}, "required": []string{"target_agent_id"}},
		},
	}
	tenantID := tenant.GetTenantID(c)
	visible := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		if pack, ok := personaToolPacks[name]; ok && !h.personaPackEnabled(tenantID, pack) {
			continue
		}
		visible = append(visible, tool)
	}
	return jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{"tools": visible}}
}

func groupMessageTool(name, description string, channel bool) map[string]interface{} {
	properties := map[string]interface{}{
		"group_id": map[string]interface{}{"type": "string"}, "action": map[string]interface{}{"type": "string"}, "message": map[string]interface{}{"type": "string"},
		"audience": map[string]interface{}{"type": "string", "enum": []string{"all", "role", "leaders", "round_robin"}, "default": "all", "description": "round_robin 每次只选一个符合订阅与 audience_role 的接收方，并持久化轮换"}, "audience_role": map[string]interface{}{"type": "string"},
		"aggregation": map[string]interface{}{"type": "string", "enum": []string{"all_replies", "first_success", "leader_summary"}, "default": "all_replies", "description": "leader_summary 采用本次收件人中 leader 的回复，不会隐式触发第二次 Agent 总结"},
		"run_id":      map[string]interface{}{"type": "string"}, "session_id": map[string]interface{}{"type": "string"}, "idempotency_key": map[string]interface{}{"type": "string"},
	}
	required := []string{"group_id", "message"}
	if channel {
		properties["channel_id"] = map[string]interface{}{"type": "string"}
		properties["mentioned_agents"] = map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}}
		required = []string{"channel_id", "message"}
	}
	return map[string]interface{}{"name": name, "description": description, "inputSchema": map[string]interface{}{"type": "object", "properties": properties, "required": required}}
}

type agentSendArgs struct {
	TargetAgent          string `json:"target_agent"`
	Scope                string `json:"scope"`
	Action               string `json:"action"`
	Message              string `json:"message"`
	ContextSummary       string `json:"context_summary"`
	SharedContext        string `json:"shared_context"`
	RunID                string `json:"run_id"`
	ParentMessageID      string `json:"parent_message_id"`
	Deadline             string `json:"deadline"`
	IdempotencyKey       string `json:"idempotency_key"`
	RouteDeviationReason string `json:"route_deviation_reason"`
}

type agentMessageStatusArgs struct {
	MessageID string `json:"message_id"`
}

type agentInboxArgs struct {
	Limit int `json:"limit"`
}

type agentMessageChainArgs struct {
	ConversationID string `json:"conversation_id"`
	RootMessageID  string `json:"root_message_id"`
	Limit          int    `json:"limit"`
}

type groupMessageArgs struct {
	GroupID         string   `json:"group_id"`
	ChannelID       string   `json:"channel_id"`
	SessionID       string   `json:"session_id"`
	Action          string   `json:"action"`
	Message         string   `json:"message"`
	Audience        string   `json:"audience"`
	AudienceRole    string   `json:"audience_role"`
	MentionedAgents []string `json:"mentioned_agents"`
	Aggregation     string   `json:"aggregation"`
	RunID           string   `json:"run_id"`
	IdempotencyKey  string   `json:"idempotency_key"`
}
type groupMessageStatusArgs struct {
	DispatchID string `json:"dispatch_id"`
}
type sessionStartArgs struct {
	SessionID string `json:"session_id"`
}
type sessionEndArgs struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
}
type workflowStartArgs struct {
	VersionID      string         `json:"version_id"`
	RunID          string         `json:"run_id"`
	Input          map[string]any `json:"input"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type approvalVoteArgs struct {
	ApprovalID     string         `json:"approval_id"`
	Decision       string         `json:"decision"`
	Reason         string         `json:"reason"`
	Conditions     map[string]any `json:"conditions"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type workflowStepArgs struct {
	StepRunID      string         `json:"step_run_id"`
	Output         map[string]any `json:"output"`
	Error          string         `json:"error"`
	IdempotencyKey string         `json:"idempotency_key"`
}
type decisionVoteArgs struct {
	DecisionID string `json:"decision_id"`
	Choice     string `json:"choice"`
	Reason     string `json:"reason"`
}

type beliefListArgs struct {
	FactRef string `json:"fact_ref"`
}
type beliefClaimArgs struct {
	FactRef   string `json:"fact_ref"`
	Statement string `json:"statement"`
}
type memoryRecordArgs struct {
	FactRef        string `json:"fact_ref"`
	Interpretation string `json:"interpretation"`
	Importance     *int   `json:"importance"`
}
type memoryRecallArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}
type relationViewArgs struct {
	TargetAgentID uint64 `json:"target_agent_id"`
}

// personaDisabled mirrors the existing disabled-capability responses.
const personaDisabled = "能力尚未启用"

// resolvePersonaRun binds the caller runtime identity to its active run
// session. Identity never comes from tool arguments.
func (h *OrganizationMcpHandler) resolvePersonaRun(tenantID string, source *agent.AgentConfig, id interface{}) (string, jsonRPCResponse, bool) {
	if h.personaRunResolver == nil {
		return "", mcpErrorResult(id, "仅运行会话可用"), false
	}
	runID, err := h.personaRunResolver(tenantID, source.ID)
	if err != nil || strings.TrimSpace(runID) == "" {
		return "", mcpErrorResult(id, "仅运行会话可用"), false
	}
	return runID, jsonRPCResponse{}, true
}

func personaDeterministicKey(tool string, agentID uint64, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("h6:mcp:%s:%d:%s", tool, agentID, hex.EncodeToString(sum[:])[:16])
}

func (h *OrganizationMcpHandler) handleToolsCall(ctx context.Context, c *gin.Context, id interface{}, raw json.RawMessage) (jsonRPCResponse, error) {
	var call toolCallParams
	if err := json.Unmarshal(raw, &call); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	source, ok := middleware.AgentFromContext(c)
	if !ok || source == nil {
		return mcpErrorResult(id, "运行时身份无效"), nil
	}
	tenantID := tenant.GetTenantID(c)

	switch call.Name {
	case "agent_relations":
		relations, err := h.service.Relations(tenantID, source)
		if err != nil {
			return mcpErrorResult(id, "组织关系读取失败"), nil
		}
		items := make([]map[string]interface{}, 0, len(relations))
		for _, relation := range relations {
			direction := "incoming"
			if relation.SourceAgentID == source.ID {
				direction = "outgoing"
			}
			item := map[string]interface{}{
				"direction": direction, "scope": relation.Scope,
				"source_agent": relation.SourceAgentName, "target_agent": relation.TargetAgentName,
				"relation_type":   relation.RelationType,
				"allowed_actions": relation.AllowedActions, "context_policy": relation.ContextPolicy,
				"delivery_policy": relation.DeliveryPolicy, "constraint": relation.Constraint,
			}
			// An agent can inspect its own opinion of others, not another
			// agent's private opinion of it. Incoming edges expose only the
			// formal communication contract.
			items = append(items, item)
		}
		return mcpJSONResult(id, map[string]interface{}{"self": source.Name, "relations": items})
	case "agent_send":
		var args agentSendArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		var deadline *time.Time
		if strings.TrimSpace(args.Deadline) != "" {
			parsed, parseErr := time.Parse(time.RFC3339, args.Deadline)
			if parseErr != nil {
				return mcpErrorResult(id, "deadline 必须为 RFC3339 时间"), nil
			}
			deadline = &parsed
		}
		message, err := h.service.Send(ctx, tenantID, source, services.SendAgentMessageInput{
			TargetAgent: args.TargetAgent, Scope: args.Scope, Action: args.Action,
			Message: args.Message, ContextSummary: args.ContextSummary, SharedContext: args.SharedContext,
			// Only first-hop Run/deadline and continuation identity are accepted
			// from MCP. The service derives all causal counters and budgets from
			// ParentMessageID, preventing an Agent from resetting hop or budget.
			RunID: args.RunID, ParentMessageID: args.ParentMessageID, DeadlineAt: deadline,
			IdempotencyKey:       args.IdempotencyKey,
			RouteDeviationReason: args.RouteDeviationReason,
		})
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, message)
	case "agent_message_status":
		var args agentMessageStatusArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		if strings.TrimSpace(args.MessageID) == "" {
			return mcpErrorResult(id, "message_id 不能为空"), nil
		}
		message, err := h.service.Get(tenantID, source, args.MessageID)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, message)
	case "agent_inbox":
		var args agentInboxArgs
		if len(call.Arguments) > 0 {
			if err := json.Unmarshal(call.Arguments, &args); err != nil {
				return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
			}
		}
		messages, err := h.service.Inbox(tenantID, source, args.Limit)
		if err != nil {
			return mcpErrorResult(id, "组织消息箱读取失败"), nil
		}
		return mcpJSONResult(id, map[string]interface{}{"messages": messages})
	case "agent_message_chain":
		var args agentMessageChainArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		chain, err := h.service.MessageChainForAgent(tenantID, source, args.ConversationID, args.RootMessageID, args.Limit)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, chain)
	case "group_send", "channel_publish":
		groupService, ok := h.service.(GroupOrganizationMessageService)
		if !ok {
			return mcpErrorResult(id, "群组协作能力尚未启用"), nil
		}
		var args groupMessageArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		if call.Name == "group_send" {
			args.ChannelID = ""
		}
		dispatch, err := groupService.GroupSend(tenantID, source, services.GroupMessageInput{GroupID: args.GroupID, ChannelID: args.ChannelID, SessionID: args.SessionID, Action: args.Action, Message: args.Message, Audience: args.Audience, AudienceRole: args.AudienceRole, MentionedAgents: args.MentionedAgents, Aggregation: args.Aggregation, RunID: args.RunID, IdempotencyKey: args.IdempotencyKey})
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, dispatch)
	case "group_message_status":
		groupService, ok := h.service.(GroupOrganizationMessageService)
		if !ok {
			return mcpErrorResult(id, "群组协作能力尚未启用"), nil
		}
		var args groupMessageStatusArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		dispatch, err := groupService.GroupMessageStatus(tenantID, source, args.DispatchID)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, dispatch)
	case "session_start":
		groupService, ok := h.service.(GroupOrganizationMessageService)
		if !ok {
			return mcpErrorResult(id, "会话房间能力尚未启用"), nil
		}
		var args sessionStartArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		room, err := groupService.StartSession(tenantID, source, args.SessionID)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, room)
	case "session_end":
		groupService, ok := h.service.(GroupOrganizationMessageService)
		if !ok {
			return mcpErrorResult(id, "会话房间能力尚未启用"), nil
		}
		var args sessionEndArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		room, err := groupService.EndSession(tenantID, source, args.SessionID, args.Summary)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, room)
	case "workflow_start":
		if h.workflowService == nil {
			return mcpErrorResult(id, "工作流能力尚未启用"), nil
		}
		var args workflowStartArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		execution, err := h.workflowService.StartExecution(tenantID, args.VersionID, args.RunID, args.Input, args.IdempotencyKey, source.Name)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, execution)
	case "approval_vote":
		if h.workflowService == nil {
			return mcpErrorResult(id, "审批能力尚未启用"), nil
		}
		var args approvalVoteArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		execution, err := h.workflowService.DecideApproval(tenantID, args.ApprovalID, source.Name, args.Decision, args.Reason, args.Conditions, args.IdempotencyKey)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, execution)
	case "decision_vote":
		if h.decisionService == nil {
			return mcpErrorResult(id, "群体决策能力尚未启用"), nil
		}
		var args decisionVoteArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		vote, err := h.decisionService.CastAgentVote(tenantID, source, args.DecisionID, args.Choice, args.Reason)
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, vote)
	case "workflow_step_complete", "workflow_step_fail":
		if h.workflowService == nil {
			return mcpErrorResult(id, "工作流能力尚未启用"), nil
		}
		var args workflowStepArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, err
		}
		var execution any
		var err error
		if call.Name == "workflow_step_complete" {
			execution, err = h.workflowService.CompleteStep(tenantID, args.StepRunID, args.Output, args.IdempotencyKey, source.Name)
		} else {
			execution, err = h.workflowService.FailStep(tenantID, args.StepRunID, args.Error, args.IdempotencyKey, source.Name)
		}
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, execution)
	case "emotion_status":
		if !h.personaPackEnabled(tenantID, services.PersonaPackEmotion) {
			return personaDisabledError(id, services.PersonaPackEmotion), nil
		}
		if h.emotionService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		state, err := h.emotionService.Status(tenantID, runID, source.ID)
		if err != nil {
			if errors.Is(err, rundomain.ErrNoActiveRun) {
				return mcpErrorResult(id, "仅运行会话可用"), nil
			}
			return mcpErrorResult(id, "心情状态读取失败"), nil
		}
		return mcpJSONResult(id, state)
	case "belief_list":
		if !h.personaPackEnabled(tenantID, services.PersonaPackBelief) {
			return personaDisabledError(id, services.PersonaPackBelief), nil
		}
		if h.beliefService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		var args beliefListArgs
		if len(call.Arguments) > 0 {
			if err := json.Unmarshal(call.Arguments, &args); err != nil {
				return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
			}
		}
		beliefs, err := h.beliefService.List(tenantID, runID, source.ID, strings.TrimSpace(args.FactRef))
		if err != nil {
			return mcpErrorResult(id, "看法列表读取失败"), nil
		}
		return mcpJSONResult(id, map[string]interface{}{"beliefs": beliefs})
	case "belief_claim":
		if !h.personaPackEnabled(tenantID, services.PersonaPackBelief) {
			return personaDisabledError(id, services.PersonaPackBelief), nil
		}
		if h.beliefService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		var args beliefClaimArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		factRef, err := h.beliefService.Claim(tenantID, runID, source.ID, strings.TrimSpace(args.FactRef), args.Statement, time.Now().UTC(), personaDeterministicKey("belief_claim", source.ID, args.FactRef, args.Statement))
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, map[string]interface{}{"factRef": factRef})
	case "memory_record":
		if !h.personaPackEnabled(tenantID, services.PersonaPackMemory) {
			return personaDisabledError(id, services.PersonaPackMemory), nil
		}
		if h.memoryService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		var args memoryRecordArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		importance := 50
		if args.Importance != nil {
			importance = *args.Importance
		}
		entry, err := h.memoryService.Record(tenantID, runID, source.ID, args.FactRef, args.Interpretation, importance, time.Now().UTC(), personaDeterministicKey("memory_record", source.ID, args.FactRef, args.Interpretation))
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, entry)
	case "memory_recall":
		if !h.personaPackEnabled(tenantID, services.PersonaPackMemory) {
			return personaDisabledError(id, services.PersonaPackMemory), nil
		}
		if h.memoryService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		var args memoryRecallArgs
		if len(call.Arguments) > 0 {
			if err := json.Unmarshal(call.Arguments, &args); err != nil {
				return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
			}
		}
		memories, err := h.memoryService.Recall(tenantID, runID, source.ID, strings.TrimSpace(args.Query), args.Limit, time.Now().UTC())
		if err != nil {
			return mcpErrorResult(id, "记忆检索失败"), nil
		}
		return mcpJSONResult(id, map[string]interface{}{"memories": memories})
	case "relation_view":
		if !h.personaPackEnabled(tenantID, services.PersonaPackRelationshipDynamics) {
			return personaDisabledError(id, services.PersonaPackRelationshipDynamics), nil
		}
		if h.relationDynService == nil {
			return mcpErrorResult(id, personaDisabled), nil
		}
		runID, failure, ok := h.resolvePersonaRun(tenantID, source, id)
		if !ok {
			return failure, nil
		}
		var args relationViewArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		if args.TargetAgentID == 0 {
			return mcpErrorResult(id, "target_agent_id 不能为空"), nil
		}
		attitude, err := h.relationDynService.View(tenantID, runID, source.ID, args.TargetAgentID)
		if err != nil {
			return mcpErrorResult(id, "关系态度读取失败"), nil
		}
		return mcpJSONResult(id, attitude)
	default:
		return jsonRPCResponse{}, fmt.Errorf("工具不存在: %s", call.Name)
	}
}
