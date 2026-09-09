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
					"target_agent":    map[string]interface{}{"type": "string", "description": "关系中接收方的 Agent ID，例如 speeding-mo-yuncen"},
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
			items = append(items, map[string]interface{}{
				"direction": direction, "scope": relation.Scope,
				"source_agent": relation.SourceAgentName, "target_agent": relation.TargetAgentName,
				"relation_type": relation.RelationType, "stance": relation.Stance,
				"allowed_actions": relation.AllowedActions, "context_policy": relation.ContextPolicy,
				"delivery_policy": relation.DeliveryPolicy, "constraint": relation.Constraint,
			})
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
