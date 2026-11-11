package handler

// memory_mcp.go implements the built-in memory MCP endpoint (spec
// docs/superpowers/specs/2026-11-11-memory-mcp-design.md §3/§5): four
// agent-facing tools over the same JSON-RPC envelope as the knowledge MCP.
// Identity is server-pinned — the HMAC-signed memory capability (issued at
// deploy time) binds tenant, agent name, scope mode, tool set, and token
// fingerprint; tool arguments never carry identity. All errors are neutral
// MCP results with stable codes; audit rows record the scope hash and
// memory metadata only — never content or quotes.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/audit"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

const (
	// memoryCapabilityHeader 与 knowledgeCapabilityHeader 同值同语义
	//（两侧各自持有副本，改动须保持同步——照 knowledge 惯例）。
	memoryCapabilityHeader = "X-Agent-Capability"

	// 保留标识（spec §3），勿改值。
	memoryAppReserved    = "hub-agent"
	memoryLifecycleConst = "agent-memory-v1"
	memoryScopeModeAgent = "agent"
)

// MCP 工具错误码（照 knowledge 码风格：稳定契约只增不改；capability 一切
// 拒绝共用 memory_not_authorized，不区分失败步骤——不给探测 oracle）。
const (
	mcpErrMemoryNotAuthorized = "memory_not_authorized"
	mcpErrMemoryInvalidArgs   = "memory_invalid_arguments"
	mcpErrMemoryConflict      = "memory_conflict"
	mcpErrMemoryPoolFull      = "memory_pool_full"
	mcpErrMemoryForbidden     = "memory_forbidden"
	mcpErrMemoryNotFound      = "memory_not_found"
	mcpErrMemoryUnavailable   = "memory_unavailable"
)

const memoryMcpDeniedMessage = "无权访问记忆工具"

// AgentMemoryStore narrows the ledger service surface the MCP tools need.
type AgentMemoryStore interface {
	EnsureAgentLifecycle(ctx context.Context, sc memory.Scope) error
	RememberAgentFact(ctx context.Context, sc memory.Scope, in memory.AgentMemoryInput) (memory.AgentMemoryRecord, error)
	RecallAgentMemories(ctx context.Context, sc memory.Scope, query string, limit int) ([]memory.AgentMemoryRecord, error)
	ListAgentMemories(ctx context.Context, sc memory.Scope, category, memoryID string, limit int) ([]memory.AgentMemoryRecord, error)
	UpdateAgentFact(ctx context.Context, sc memory.Scope, id string, expectedVersion uint64, in memory.AgentMemoryInput) (memory.AgentMemoryRecord, error)
}

// AgentMemoryResolver resolves the per-tenant store (Task 5 registry).
type AgentMemoryResolver interface {
	AgentMemoryStore(tenantID string) (AgentMemoryStore, error)
}

type MemoryMcpHandler struct {
	resolver AgentMemoryResolver
	caps     *services.MemoryCapabilityService
	audit    *services.AuditRecorder
}

func NewMemoryMcpHandler(resolver AgentMemoryResolver, caps *services.MemoryCapabilityService, audit *services.AuditRecorder) *MemoryMcpHandler {
	return &MemoryMcpHandler{resolver: resolver, caps: caps, audit: audit}
}

// RegisterMemoryMcpRoutes mounts the endpoint; auth is the AgentRuntimeAuth
// middleware (same wiring as the knowledge MCP).
func RegisterMemoryMcpRoutes(rg *gin.RouterGroup, auth gin.HandlerFunc, h *MemoryMcpHandler) {
	rg.POST("/memory/mcp", auth, h.HandleMessage)
}

// HandleMessage dispatches JSON-RPC requests (mirror of the knowledge MCP).
func (h *MemoryMcpHandler) HandleMessage(c *gin.Context) {
	var req jsonRPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			Error:   &jsonRPCError{Code: -32700, Message: "Parse error"},
		})
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
		result, err := h.handleToolsCall(c.Request.Context(), c, req.ID, req.Params)
		if err != nil {
			c.JSON(http.StatusOK, jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32603, Message: err.Error()},
			})
			return
		}
		c.JSON(http.StatusOK, result)
	default:
		c.JSON(http.StatusOK, jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32601, Message: "Method not found"},
		})
	}
}

func (h *MemoryMcpHandler) handleInitialize(id interface{}) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{"listChanged": true}},
			"serverInfo":      map[string]interface{}{"name": "memory-mcp", "version": "1.0.0"},
		},
	}
}

func memoryToolSchema(name, description string, properties map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return map[string]interface{}{"name": name, "description": description, "inputSchema": schema}
}

func (h *MemoryMcpHandler) handleToolsList(id interface{}) jsonRPCResponse {
	str := func(d string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": d}
	}
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{"tools": []map[string]interface{}{
			memoryToolSchema("memory_remember",
				"Store one durable memory about the conversation partner, distilled from what was actually said in this conversation. Quote must be a verbatim excerpt from the current dialogue as evidence. Newly written memories become searchable after a short delay.",
				map[string]interface{}{
					"subject":      str("记忆主体（如「用户」或具体对象），≤128 字符"),
					"category":     str("业务自定义类别（由你的系统提示词约定，如「情绪」「偏好」「事实」），≤64 字符"),
					"content":      str("记忆正文（陈述句），≤2000 码点"),
					"quote":        str("来源原文摘录（必须逐字来自当前对话），≤600 码点"),
					"sourceTurnId": str("可选：本轮对话的轮次 ID"),
				},
				[]string{"content", "quote"}),
			memoryToolSchema("memory_recall",
				"Search previously stored memories relevant to a query. Use it when context about the conversation partner would improve the reply.",
				map[string]interface{}{
					"query": str("检索查询（保留核心实体与意图）"),
					"limit": map[string]interface{}{"type": "integer", "description": "返回条数 1-20，默认 5"},
				},
				[]string{"query"}),
			memoryToolSchema("memory_list",
				"List your own previously written memories (newest first) — for deduplication, conflict discovery, or fetching the version needed by memory_update. Pass memoryId to fetch one record directly.",
				map[string]interface{}{
					"category": str("可选：按类别过滤"),
					"memoryId": str("可选：按 id 直取单条（绕过 50 条窗口）"),
				},
				nil),
			memoryToolSchema("memory_update",
				"Correct one memory you previously wrote. The fresh quote (verbatim from the current dialogue) is mandatory evidence for the correction.",
				map[string]interface{}{
					"memoryId":        str("目标记忆 id（经 memory_list 获取）"),
					"expectedVersion": map[string]interface{}{"type": "integer", "description": "CAS 版本（memory_list 返回的 version）"},
					"content":         str("修正后正文"),
					"quote":           str("修正佐证（必须逐字来自当前对话）"),
					"category":        str("可选：同步修正类别"),
				},
				[]string{"memoryId", "expectedVersion", "content", "quote"}),
		}},
	}
}

func (h *MemoryMcpHandler) handleToolsCall(ctx context.Context, c *gin.Context, id interface{}, params json.RawMessage) (jsonRPCResponse, error) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	switch p.Name {
	case "memory_remember", "memory_recall", "memory_list", "memory_update":
		return h.handleMemoryTool(ctx, c, id, p)
	default:
		return jsonRPCResponse{}, fmt.Errorf("工具不存在: %s", p.Name)
	}
}

// resolveMemoryScope runs the fail-closed identity chain: agent context →
// tenant → capability presentation (absent/blank/duplicate → deny) →
// signature + agent/token bindings → tenant/scopeMode/tool-set assertions →
// per-tenant store. Rejection reasons stay in server logs; the client gets
// one neutral message (no probing oracle).
func (h *MemoryMcpHandler) resolveMemoryScope(c *gin.Context, id interface{}, tool string) (memory.Scope, AgentMemoryStore, *jsonRPCResponse, error) {
	agentCfg, ok := middleware.AgentFromContext(c)
	if !ok {
		return memory.Scope{}, nil, nil, fmt.Errorf("上下文中未找到 Agent 身份")
	}
	tenantID := tenant.GetTenantID(c)
	if tenantID == "" {
		// 防御性拒绝（AgentRuntimeAuth 命中后必写 tenant），避免空串
		// 租户静默解析出错误 store。
		return memory.Scope{}, nil, nil, fmt.Errorf("记忆 MCP 请求缺少租户上下文")
	}
	deny := func() *jsonRPCResponse {
		resp := mcpCodedErrorResult(id, mcpErrMemoryNotAuthorized, memoryMcpDeniedMessage)
		return &resp
	}
	caps := c.Request.Header.Values(memoryCapabilityHeader)
	if len(caps) > 1 {
		log.Printf("memory-mcp: duplicate %s headers rejected (tenant=%s agent=%s)", memoryCapabilityHeader, tenantID, agentCfg.Name)
		return memory.Scope{}, nil, deny(), nil
	}
	raw := ""
	if len(caps) == 1 {
		raw = strings.TrimSpace(caps[0])
	}
	if raw == "" {
		// 缺失与空值一律拒绝：Phase 1 capability 由部署期武装，无回退通道。
		log.Printf("memory-mcp: capability absent or blank (tenant=%s agent=%s)", tenantID, agentCfg.Name)
		return memory.Scope{}, nil, deny(), nil
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	payload, err := h.caps.VerifyWithToken(raw, agentCfg.Name, bearer)
	if err != nil {
		log.Printf("memory-mcp: capability rejected (tenant=%s agent=%s): %v", tenantID, agentCfg.Name, err)
		return memory.Scope{}, nil, deny(), nil
	}
	if payload.TenantID != tenantID || payload.ScopeMode != memoryScopeModeAgent {
		log.Printf("memory-mcp: capability assertion mismatch (tenant=%s agent=%s)", tenantID, agentCfg.Name)
		return memory.Scope{}, nil, deny(), nil
	}
	if !containsString(payload.Tools, tool) {
		log.Printf("memory-mcp: tool not in capability set (tenant=%s agent=%s tool=%s)", tenantID, agentCfg.Name, tool)
		return memory.Scope{}, nil, deny(), nil
	}
	store, err := h.resolver.AgentMemoryStore(tenantID)
	if err != nil {
		log.Printf("memory-mcp: resolve store failed (tenant=%s): %v", tenantID, err)
		return memory.Scope{}, nil, nil, fmt.Errorf("记忆服务暂不可用")
	}
	return memory.Scope{
		TenantID:      tenantID,
		ApplicationID: memoryAppReserved,
		UserID:        "agent:" + agentCfg.Name,
		LifecycleID:   memoryLifecycleConst,
	}, store, nil, nil
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// 身份字段（userId/tenantId/applicationId）不出现在任何参数结构里：
// 出现于 arguments 也会因无对应字段而被静默忽略（spec §5 验收反例 2）。
type memoryRememberArgs struct {
	Subject      string `json:"subject"`
	Category     string `json:"category"`
	Content      string `json:"content"`
	Quote        string `json:"quote"`
	SourceTurnID string `json:"sourceTurnId"`
}

type memoryRecallArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type memoryListArgs struct {
	Category string `json:"category"`
	MemoryID string `json:"memoryId"`
}

type memoryUpdateArgs struct {
	MemoryID        string `json:"memoryId"`
	ExpectedVersion uint64 `json:"expectedVersion"`
	Content         string `json:"content"`
	Quote           string `json:"quote"`
	Category        string `json:"category"`
}

// memoryStoreErrorResult maps ledger errors to actionable neutral messages
// (spec §7): pool-full is diagnosable so the agent stops retrying; CAS
// conflicts point at memory_list; everything else degrades generically.
func memoryStoreErrorResult(id interface{}, err error) jsonRPCResponse {
	switch {
	case errors.Is(err, memory.ErrPoolFull):
		return mcpCodedErrorResult(id, mcpErrMemoryPoolFull, "记忆容量已满，请停止重试，可经管理台清理腾位")
	case errors.Is(err, memory.ErrForbidden):
		return mcpCodedErrorResult(id, mcpErrMemoryForbidden, "无权修改该记忆")
	case errors.Is(err, memory.ErrConflict):
		return mcpCodedErrorResult(id, mcpErrMemoryConflict, "版本冲突，请重新 memory_list 获取最新版本后重试")
	case errors.Is(err, memory.ErrInvalid):
		return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "记忆参数不合法：正文与摘录必填且不超限，修正必须携带佐证")
	case errors.Is(err, memory.ErrNotFound):
		return mcpCodedErrorResult(id, mcpErrMemoryNotFound, "记忆不存在")
	default:
		return mcpCodedErrorResult(id, mcpErrMemoryUnavailable, "记忆服务暂不可用")
	}
}

func (h *MemoryMcpHandler) handleMemoryTool(ctx context.Context, c *gin.Context, id interface{}, p toolCallParams) (jsonRPCResponse, error) {
	scope, store, deny, err := h.resolveMemoryScope(c, id, p.Name)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	if err := store.EnsureAgentLifecycle(ctx, scope); err != nil {
		log.Printf("memory-mcp: ensure lifecycle failed (tenant=%s agent-scope=%s): %v", scope.TenantID, scope.UserID, err)
		return memoryStoreErrorResult(id, err), nil
	}

	switch p.Name {
	case "memory_remember":
		var args memoryRememberArgs
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "记忆参数不合法"), nil
		}
		record, err := store.RememberAgentFact(ctx, scope, memory.AgentMemoryInput{
			Subject: args.Subject, Category: args.Category, Content: args.Content, Quote: args.Quote, TurnID: args.SourceTurnID,
		})
		if err != nil {
			return memoryStoreErrorResult(id, err), nil
		}
		h.recordAudit(c, scope, "memory.mcp.remember", record.ID, record.Version, 0)
		return mcpJSONResult(id, map[string]interface{}{"memoryId": record.ID, "version": record.Version})

	case "memory_recall":
		var args memoryRecallArgs
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "记忆参数不合法"), nil
		}
		if strings.TrimSpace(args.Query) == "" {
			return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "query 不能为空"), nil
		}
		records, err := store.RecallAgentMemories(ctx, scope, args.Query, args.Limit)
		if err != nil {
			return memoryStoreErrorResult(id, err), nil
		}
		h.recordAudit(c, scope, "memory.mcp.recall", "", 0, len(records))
		return mcpJSONResult(id, map[string]interface{}{"memories": records})

	case "memory_list":
		var args memoryListArgs
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "记忆参数不合法"), nil
		}
		records, err := store.ListAgentMemories(ctx, scope, args.Category, args.MemoryID, 50)
		if err != nil {
			return memoryStoreErrorResult(id, err), nil
		}
		h.recordAudit(c, scope, "memory.mcp.list", "", 0, len(records))
		return mcpJSONResult(id, map[string]interface{}{"memories": records})

	case "memory_update":
		var args memoryUpdateArgs
		if err := json.Unmarshal(p.Arguments, &args); err != nil {
			return mcpCodedErrorResult(id, mcpErrMemoryInvalidArgs, "记忆参数不合法"), nil
		}
		record, err := store.UpdateAgentFact(ctx, scope, args.MemoryID, args.ExpectedVersion, memory.AgentMemoryInput{
			Content: args.Content, Quote: args.Quote, Category: args.Category,
		})
		if err != nil {
			return memoryStoreErrorResult(id, err), nil
		}
		h.recordAudit(c, scope, "memory.mcp.update", record.ID, record.Version, 0)
		return mcpJSONResult(id, map[string]interface{}{"memoryId": record.ID, "version": record.Version})
	}
	return jsonRPCResponse{}, fmt.Errorf("工具不存在: %s", p.Name)
}

// recordAudit writes one audit row per tool call: scope hash + memory
// metadata only — content and quotes never appear (spec §7, NotContains
// 钉住于测试)。
func (h *MemoryMcpHandler) recordAudit(c *gin.Context, scope memory.Scope, action, memoryID string, version uint64, count int) {
	if h.audit == nil {
		return
	}
	scopeJSON, _ := json.Marshal(scope)
	digest := sha256.Sum256(scopeJSON)
	entry := audit.SimpleEvent(
		audit.Actor{TenantID: scope.TenantID, UserID: scope.UserID},
		audit.Action(action),
		audit.TargetType("memory"),
		hex.EncodeToString(digest[:]),
		"",
	)
	detail := map[string]any{"tool": action}
	if memoryID != "" {
		detail["memoryId"] = memoryID
		detail["version"] = version
	}
	if count > 0 {
		detail["count"] = count
	}
	entry.Detail = detail
	if c.Writer.Status() >= 400 {
		entry.Status = audit.StatusFailure
	}
	h.audit.Record(c, entry)
}
