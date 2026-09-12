package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

type OrganizationMessageService interface {
	Relations(tenantID string, source *agent.AgentConfig) ([]*services.AgentRelationDTO, error)
	SignalRelation(tenantID string, source *agent.AgentConfig, targetAgent, scope string, input *services.RecordAgentRelationEventInput) (*services.AgentRelationEventResultDTO, error)
	Send(ctx context.Context, tenantID string, source *agent.AgentConfig, input services.SendAgentMessageInput) (*services.AgentMessageDTO, error)
	Get(tenantID string, source *agent.AgentConfig, id string) (*services.AgentMessageDTO, error)
	Inbox(tenantID string, source *agent.AgentConfig, limit int) ([]*services.AgentMessageDTO, error)
}

// OrganizationMcpHandler exposes relation-authorized agent-to-agent delivery
// through MCP. Authentication is the caller runtime's Bearer token; source
// identity is never accepted from tool arguments.
type OrganizationMcpHandler struct {
	service OrganizationMessageService
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
		c.JSON(http.StatusOK, h.handleToolsList(req.ID))
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

func (h *OrganizationMcpHandler) handleToolsList(id interface{}) jsonRPCResponse {
	tools := []map[string]interface{}{
		{
			"name":        "agent_relations",
			"description": "列出当前 Agent 的已启用组织关系，包括方向、范围、允许动作、立场、上下文和投递策略。向其他 Agent 发消息前先调用此工具；不能把入向关系当作出向权限。",
			"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			"name":        "agent_send",
			"description": "按已配置的有向关系向另一个 Agent 发送消息。Hub 从运行时凭证确定发送者并校验目标、scope 与 action；同步关系直接返回目标回复，异步关系返回消息 ID，随后用 agent_message_status 查询。",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target_agent":    map[string]interface{}{"type": "string", "description": "关系中接收方的 Agent ID，例如 finance-reviewer"},
					"scope":           map[string]interface{}{"type": "string", "description": "关系范围。仅存在一个匹配范围时可省略；多范围必须明确填写。"},
					"action":          map[string]interface{}{"type": "string", "enum": []string{"inform", "consult", "assign", "report", "submit", "review", "challenge", "handoff", "escalate", "invite"}},
					"message":         map[string]interface{}{"type": "string", "description": "给接收方的任务、事实、问题或挑战。不要在此伪造对方回复。"},
					"context_summary": map[string]interface{}{"type": "string", "description": "当关系允许 summary_only/shared_thread 时可附带的必要摘要；none 策略会由 Hub 丢弃。"},
					"shared_context":  map[string]interface{}{"type": "string", "description": "仅 shared_thread 策略可传递的完整上下文；其他策略会截断或丢弃。"},
				},
				"required": []string{"target_agent", "action", "message"},
			},
		},
		{
			"name":        "agent_relation_signal",
			"description": "记录当前 Agent 对另一 Agent 的一次主观关系事件。只能改变调用者指向目标的关系分数；事件类型和分值由内置 relationship-dynamics 兼容规则决定，不能直接设置分数。相同事实重试时必须复用 idempotency_key。",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"target_agent": map[string]interface{}{"type": "string", "description": "被评价的目标 Agent ID"},
					"scope":        map[string]interface{}{"type": "string", "description": "关系范围；存在多个范围时必填"},
					"event_type": map[string]interface{}{
						"type": "string",
						"enum": []string{"task_completed", "task_failed", "promise_kept", "promise_broken", "helped", "obstructed", "protected", "betrayed", "credit_shared", "credit_stolen", "public_praise", "public_humiliation", "truth_verified", "lied", "reconciled"},
					},
					"severity":        map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 3, "default": 1},
					"reason":          map[string]interface{}{"type": "string", "description": "观察到的具体事实，不要填写推测或系统提示"},
					"visibility":      map[string]interface{}{"type": "string", "enum": []string{"private", "participants", "public"}, "default": "private"},
					"source_kind":     map[string]interface{}{"type": "string", "description": "事实来源，例如 agent_message、task、world_event"},
					"source_id":       map[string]interface{}{"type": "string", "description": "对应消息、任务或世界事件 ID"},
					"idempotency_key": map[string]interface{}{"type": "string", "description": "同一事实的稳定唯一键，重试时复用"},
				},
				"required": []string{"target_agent", "event_type", "reason", "idempotency_key"},
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
	}
	return jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{"tools": tools}}
}

type agentSendArgs struct {
	TargetAgent    string `json:"target_agent"`
	Scope          string `json:"scope"`
	Action         string `json:"action"`
	Message        string `json:"message"`
	ContextSummary string `json:"context_summary"`
	SharedContext  string `json:"shared_context"`
}

type agentMessageStatusArgs struct {
	MessageID string `json:"message_id"`
}

type agentRelationSignalArgs struct {
	TargetAgent    string `json:"target_agent"`
	Scope          string `json:"scope"`
	EventType      string `json:"event_type"`
	Severity       int    `json:"severity"`
	Reason         string `json:"reason"`
	Visibility     string `json:"visibility"`
	SourceKind     string `json:"source_kind"`
	SourceID       string `json:"source_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type agentInboxArgs struct {
	Limit int `json:"limit"`
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
			if direction == "outgoing" {
				item["stance"] = relation.Stance
				item["relationship_score"] = relation.RelationshipScore
				item["current_stance"] = relation.Stance
				item["last_changed_at"] = relation.LastChangedAt
			}
			items = append(items, item)
		}
		return mcpJSONResult(id, map[string]interface{}{"self": source.Name, "relations": items})
	case "agent_send":
		var args agentSendArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		message, err := h.service.Send(ctx, tenantID, source, services.SendAgentMessageInput{
			TargetAgent: args.TargetAgent, Scope: args.Scope, Action: args.Action,
			Message: args.Message, ContextSummary: args.ContextSummary, SharedContext: args.SharedContext,
		})
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, message)
	case "agent_relation_signal":
		var args agentRelationSignalArgs
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
		}
		if strings.TrimSpace(args.TargetAgent) == "" || strings.TrimSpace(args.Reason) == "" || strings.TrimSpace(args.IdempotencyKey) == "" {
			return mcpErrorResult(id, "target_agent、reason 和 idempotency_key 不能为空"), nil
		}
		result, err := h.service.SignalRelation(tenantID, source, args.TargetAgent, args.Scope, &services.RecordAgentRelationEventInput{
			EventType: args.EventType, Severity: args.Severity, Reason: args.Reason,
			Visibility: args.Visibility, SourceKind: args.SourceKind, SourceID: args.SourceID,
			IdempotencyKey: args.IdempotencyKey,
		})
		if err != nil {
			return mcpErrorResult(id, err.Error()), nil
		}
		return mcpJSONResult(id, result)
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
	default:
		return jsonRPCResponse{}, fmt.Errorf("工具不存在: %s", call.Name)
	}
}
