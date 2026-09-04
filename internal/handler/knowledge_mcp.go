package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

// KnowledgeMcpService abstracts the knowledge operations needed by the MCP handler.
type KnowledgeMcpService interface {
	Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error)
	GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error)
	ListDocuments(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error)
	ListChunks(ctx context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error)
}

// AgentMcpService abstracts the agent operations needed by the MCP handler.
type AgentMcpService interface {
	// GetAgentKnowledgeDatasetsForRequest 返回「请求身份」可访问的 dataset
	// IDs：capabilityHeader 是部署时 hub 签发注入 MCP 连接头的
	// X-Agent-Capability（缺失时回退 token agent 自身），必须验签通过并
	// 绑定 token agent 本身或其直接挂载的 subagent；bearerToken 原文用于
	// token 指纹失效域校验。授权集只是该身份自己的绑定——不是部署闭包
	// 并集（child 拿不到 parent/sibling 的，root 也拿不到 child 的）。
	// 验证失败返回包装 ErrKnowledgeCapabilityDenied 的 error（handler 转
	// 中性 deny）。
	GetAgentKnowledgeDatasetsForRequest(tenantID, tokenAgentName, capabilityHeader, bearerToken string) (datasets []string, requestingAgent string, err error)
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type knowledgeSearchArgs struct {
	Query string `json:"query"`
	// Question 是已废弃的旧参数名，仅作兼容回退：缓存了旧 tools/list
	// schema 的已部署 runtime 容器升级 hub 后仍会发 question，直到
	// 重新部署/重启才会拿到只广播 query 的新 schema。
	Question               string   `json:"question"`
	DatasetIDs             []string `json:"dataset_ids"`
	TopK                   *int     `json:"top_k"`
	SimilarityThreshold    *float64 `json:"similarity_threshold"`
	VectorSimilarityWeight *float64 `json:"vector_similarity_weight"`
	Highlight              *bool    `json:"highlight"`
}

const (
	mcpDefaultPageSize = 20
	mcpMaxPageSize     = 50
)

type listDocumentsArgs struct {
	DatasetID string `json:"dataset_id"`
	Page      *int   `json:"page"`
	PageSize  *int   `json:"page_size"`
}

type listChunksArgs struct {
	DatasetID  string `json:"dataset_id"`
	DocumentID string `json:"document_id"`
	Page       *int   `json:"page"`
	PageSize   *int   `json:"page_size"`
}

// normalizePaging 应用默认值并把 page_size 钳制到上限（钳制不报错）。
func normalizePaging(page, pageSize *int) (int, int) {
	p, ps := 1, mcpDefaultPageSize
	if page != nil && *page > 0 {
		p = *page
	}
	if pageSize != nil && *pageSize > 0 {
		ps = *pageSize
	}
	if ps > mcpMaxPageSize {
		ps = mcpMaxPageSize
	}
	return p, ps
}

// KnowledgeMcpHandler implements the JSON-RPC MCP protocol for knowledge retrieval.
type KnowledgeMcpHandler struct {
	knowledgeService KnowledgeMcpService
	agentService     AgentMcpService
}

// NewKnowledgeMcpHandler creates a new KnowledgeMcpHandler.
func NewKnowledgeMcpHandler(knowledgeService KnowledgeMcpService, agentService AgentMcpService) *KnowledgeMcpHandler {
	return &KnowledgeMcpHandler{
		knowledgeService: knowledgeService,
		agentService:     agentService,
	}
}

// HandleMessage dispatches JSON-RPC requests to the appropriate handler.
func (h *KnowledgeMcpHandler) HandleMessage(c *gin.Context) {
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

func (h *KnowledgeMcpHandler) handleInitialize(id interface{}) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": true,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "knowledge-mcp",
				"version": "1.0.0",
			},
		},
	}
}

func (h *KnowledgeMcpHandler) handleToolsList(id interface{}) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "knowledge_search",
					"description": "When the user's question involves internal documents, product knowledge, private materials, or needs a fact-based answer, you MUST call this tool to retrieve relevant context. Only answer based on the returned text snippets; do not rely on training data or make up information. Do not call this tool if the question does not require a knowledge base.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "A search-optimized query for the knowledge base. Keep core entities, keywords, and intent; remove conversational filler words to retrieve more relevant snippets.",
							},
							"dataset_ids": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "string"},
								"description": "Optional. Restrict retrieval to specific knowledge bases. Valid dataset IDs can be found in the <datasets> block of the system prompt. If omitted, all knowledge bases bound to this agent are used automatically.",
							},
							"top_k": map[string]interface{}{
								"type":        "integer",
								"description": "Maximum number of relevant text snippets to return. Default is 8. Adjust only when you need to control the result count.",
								"default":     8,
								"minimum":     1,
								"maximum":     100,
							},
							"similarity_threshold": map[string]interface{}{
								"type":        "number",
								"description": "Minimum similarity threshold; results below this value are filtered. Default is 0.2. A higher threshold yields stricter results.",
								"default":     0.2,
								"minimum":     0,
								"maximum":     1,
							},
							"vector_similarity_weight": map[string]interface{}{
								"type":        "number",
								"description": "Weight of vector similarity in the hybrid ranking (0-1). Default is 0.3; the remaining weight is allocated to keyword matching.",
								"default":     0.3,
								"minimum":     0,
								"maximum":     1,
							},
							"highlight": map[string]interface{}{
								"type":        "boolean",
								"description": "Whether to highlight keywords in the results. Default is false.",
								"default":     false,
							},
						},
						"required": []string{"query"},
					},
				},
				{
					"name":        "knowledge_datasets",
					"description": "List the knowledge bases bound to this agent, with live metadata (document_count, chunk_count). Call this first to decide which dataset to browse, then use knowledge_documents / knowledge_chunks.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
				{
					"name":        "knowledge_documents",
					"description": "List documents in a knowledge base, paginated like a table of contents. Returns metadata only (id, name, chunk_count, progress, run, create_time). Use knowledge_chunks to read a document's content.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"dataset_id": map[string]interface{}{
								"type":        "string",
								"description": "Target knowledge base ID. Must be one of the datasets bound to this agent (see knowledge_datasets).",
							},
							"page": map[string]interface{}{
								"type":        "integer",
								"description": "Page number, starting from 1.",
								"default":     1,
								"minimum":     1,
							},
							"page_size": map[string]interface{}{
								"type":        "integer",
								"description": "Documents per page. Default 20, maximum 50 (values above 50 are clamped).",
								"default":     20,
								"minimum":     1,
							},
						},
						"required": []string{"dataset_id"},
					},
				},
				{
					"name":        "knowledge_chunks",
					"description": "Read a document's chunks page by page, like reading a book chapter by chapter. Control your pace with page and page_size (1-50 chunks per call). This is the only tool that returns full chunk content.",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"dataset_id": map[string]interface{}{
								"type":        "string",
								"description": "Knowledge base ID the document belongs to. Must be bound to this agent.",
							},
							"document_id": map[string]interface{}{
								"type":        "string",
								"description": "Target document ID, from knowledge_documents.",
							},
							"page": map[string]interface{}{
								"type":        "integer",
								"description": "Page number, starting from 1.",
								"default":     1,
								"minimum":     1,
							},
							"page_size": map[string]interface{}{
								"type":        "integer",
								"description": "Chunks to read per call. Default 20, maximum 50 (values above 50 are clamped). Read fewer for careful study, more for a quick scan.",
								"default":     20,
								"minimum":     1,
							},
						},
						"required": []string{"dataset_id", "document_id"},
					},
				},
			},
		},
	}
}

func (h *KnowledgeMcpHandler) handleToolsCall(ctx context.Context, c *gin.Context, id interface{}, params json.RawMessage) (jsonRPCResponse, error) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}

	switch p.Name {
	case "knowledge_search":
		return h.handleKnowledgeSearch(ctx, c, id, p.Arguments)
	case "knowledge_datasets":
		return h.handleKnowledgeDatasets(ctx, c, id)
	case "knowledge_documents":
		return h.handleKnowledgeDocuments(ctx, c, id, p.Arguments)
	case "knowledge_chunks":
		return h.handleKnowledgeChunks(ctx, c, id, p.Arguments)
	default:
		return jsonRPCResponse{}, fmt.Errorf("工具不存在: %s", p.Name)
	}
}

func (h *KnowledgeMcpHandler) handleKnowledgeSearch(ctx context.Context, c *gin.Context, id interface{}, params json.RawMessage) (jsonRPCResponse, error) {
	var args knowledgeSearchArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		args.Query = strings.TrimSpace(args.Question)
	}
	if args.Query == "" {
		return jsonRPCResponse{}, fmt.Errorf("query 不能为空")
	}

	// Apply defaults that match the inputSchema advertised in tools/list.
	if args.TopK == nil {
		args.TopK = new(int)
		*args.TopK = 8
	}
	if args.SimilarityThreshold == nil {
		args.SimilarityThreshold = new(float64)
		*args.SimilarityThreshold = 0.2
	}
	if args.VectorSimilarityWeight == nil {
		args.VectorSimilarityWeight = new(float64)
		*args.VectorSimilarityWeight = 0.3
	}
	if args.Highlight == nil {
		args.Highlight = new(bool)
		*args.Highlight = false
	}

	allowedDatasetIDs, deny, err := h.resolveAgentContext(c, id)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	if len(allowedDatasetIDs) == 0 {
		return mcpCodedErrorResult(id, mcpErrNoDatasetBound, "当前 Agent 未绑定任何知识库数据集"), nil
	}

	datasetIDs := args.DatasetIDs
	if len(datasetIDs) == 0 {
		datasetIDs = allowedDatasetIDs
	}
	if !isStringSubset(datasetIDs, allowedDatasetIDs) {
		return mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage), nil
	}

	req := knowledge.RetrievalRequest{
		// 下游 multirag /api/v1/retrieval 的契约 key 仍是 question，仅 MCP 入参改名 query。
		"question":                 args.Query,
		"dataset_ids":              datasetIDs,
		"top_k":                    *args.TopK,
		"similarity_threshold":     *args.SimilarityThreshold,
		"vector_similarity_weight": *args.VectorSimilarityWeight,
		"highlight":                *args.Highlight,
	}

	result, err := h.knowledgeService.Retrieval(ctx, req)
	if err != nil {
		// 上游细节（multirag 响应体/内网拓扑）只进服务端日志，客户端拿中性文案。
		log.Printf("knowledge-mcp: retrieval failed (datasets=%v): %v", datasetIDs, err)
		return mcpCodedErrorResult(id, mcpErrRetrievalFailed, "知识库检索失败，请稍后重试"), nil
	}

	text := formatRetrievalResult(result)
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": text},
			},
			"isError": false,
		},
	}, nil
}

func (h *KnowledgeMcpHandler) handleKnowledgeDatasets(ctx context.Context, c *gin.Context, id interface{}) (jsonRPCResponse, error) {
	allowed, deny, err := h.resolveAgentContext(c, id)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	datasets := make([]map[string]any, 0, len(allowed))
	for _, dsID := range allowed {
		ds, err := h.knowledgeService.GetDataset(ctx, dsID)
		if err != nil || ds == nil {
			// 单库元数据读取失败（或上游违反契约返回 nil）不阻断整体，降级为仅 id。
			if err != nil {
				log.Printf("knowledge-mcp: get dataset %s metadata failed: %v", dsID, err)
			}
			datasets = append(datasets, map[string]any{"id": dsID})
			continue
		}
		// NormalizeDataset 出口的计数键为 canonical doc_num/chunk_num，需映射回对外键名。
		m := map[string]any(*ds)
		item := pickFields(m, "id", "name", "description")
		if v, ok := m["doc_num"]; ok {
			item["document_count"] = v
		}
		if v, ok := m["chunk_num"]; ok {
			item["chunk_count"] = v
		}
		datasets = append(datasets, item)
	}
	return mcpJSONResult(id, map[string]any{"datasets": datasets})
}

func (h *KnowledgeMcpHandler) handleKnowledgeDocuments(ctx context.Context, c *gin.Context, id interface{}, raw json.RawMessage) (jsonRPCResponse, error) {
	var args listDocumentsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	datasetID, deny, err := h.requireDatasetAccess(c, id, args.DatasetID)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	page, pageSize := normalizePaging(args.Page, args.PageSize)
	result, err := h.knowledgeService.ListDocuments(ctx, datasetID, knowledge.DocumentListRequest{Page: page, PageSize: pageSize})
	if err != nil {
		log.Printf("knowledge-mcp: list documents failed (dataset=%s page=%d page_size=%d): %v", datasetID, page, pageSize, err)
		return mcpErrorResult(id, "知识库文档列表获取失败，请稍后重试"), nil
	}
	docs := make([]map[string]any, 0, len(result.Documents))
	for _, d := range result.Documents {
		// NormalizeDocument 出口的计数键为 canonical chunk_num，映射回对外 chunk_count。
		item := pickFields(map[string]any(d), "id", "name", "progress", "run", "create_time")
		if v, ok := map[string]any(d)["chunk_num"]; ok {
			item["chunk_count"] = v
		}
		docs = append(docs, item)
	}
	return mcpJSONResult(id, map[string]any{
		"total": result.Total, "page": page, "page_size": pageSize, "documents": docs,
	})
}

func (h *KnowledgeMcpHandler) handleKnowledgeChunks(ctx context.Context, c *gin.Context, id interface{}, raw json.RawMessage) (jsonRPCResponse, error) {
	var args listChunksArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	datasetID, deny, err := h.requireDatasetAccess(c, id, args.DatasetID)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	args.DocumentID = strings.TrimSpace(args.DocumentID)
	if args.DocumentID == "" {
		return jsonRPCResponse{}, fmt.Errorf("document_id 不能为空")
	}
	page, pageSize := normalizePaging(args.Page, args.PageSize)
	result, err := h.knowledgeService.ListChunks(ctx, datasetID, args.DocumentID, knowledge.ChunkListRequest{Page: page, PageSize: pageSize})
	if err != nil {
		log.Printf("knowledge-mcp: list chunks failed (dataset=%s document=%s page=%d page_size=%d): %v", datasetID, args.DocumentID, page, pageSize, err)
		return mcpErrorResult(id, "知识库分块读取失败，请稍后重试"), nil
	}
	chunks := make([]map[string]any, 0, len(result.Chunks))
	for _, ch := range result.Chunks {
		m := map[string]any(ch)
		item := map[string]any{}
		if v, ok := m["id"]; ok {
			item["chunk_id"] = v
		}
		if v, ok := m["content"]; ok {
			item["content"] = v
		}
		chunks = append(chunks, item)
	}
	docName := ""
	if result.Document != nil {
		if v, ok := map[string]any(result.Document)["name"].(string); ok {
			docName = v
		}
	}
	return mcpJSONResult(id, map[string]any{
		"total": result.Total, "page": page, "page_size": pageSize,
		"document_name": docName, "chunks": chunks,
	})
}

// knowledgeCapabilityHeader 承载 Knowledge MCP 请求的「服务端可验证
// per-Agent capability」（issue #111 重开）。部署侧（agent_deployer.go）
// 构造每个 graph 节点的 knowledge MCP 连接 headers 时用仅服务端持有的
// 密钥签发该头（HMAC，绑定 tenant/deployment/agent/token 指纹），deployer
// 原样写入该节点自己的 agents.yaml 段。可伪造的裸身份头已废弃：
// 不注入、不消费。两个包各持同名常量，改动须同步。
const knowledgeCapabilityHeader = "X-Agent-Capability"

// knowledgeCapabilityDeniedMessage 是 capability 通道一切拒绝的统一中性
// 文案（重复头/空值/验签失败/绑定不匹配/越权 dataset 子集共用），不区分
// 失败原因——不给探测 oracle。
const knowledgeCapabilityDeniedMessage = "无权访问部分知识库 dataset"

// resolveAgentContext 抽取 agent/租户/请求身份提取与绑定 dataset 反查。
// 请求身份取 X-Agent-Capability 连接头（见 knowledgeCapabilityHeader）：
// 重复头（net/http 对 header 名大小写归一后同桶多值）与呈现但空值的头
// 直接中性拒绝，不触发 service；缺失时回退 token agent 自身——存量未
// 注入 capability 的 agents.yaml 回到 #111 前的最严格行为。err 由调用方
// 原样上抛（走 -32603 中性文案）；deny 非 nil 时直接返回 *deny。
func (h *KnowledgeMcpHandler) resolveAgentContext(c *gin.Context, id interface{}) ([]string, *jsonRPCResponse, error) {
	agentCfg, ok := middleware.AgentFromContext(c)
	if !ok {
		return nil, nil, fmt.Errorf("上下文中未找到 Agent 身份")
	}
	tenantID := tenant.GetTenantID(c)
	if tenantID == "" {
		// 理论不可达：AgentRuntimeAuthMiddleware 命中 agents 行后必写 tenant_id。
		// 防御性拒绝，避免空串 tenant 静默查询全表造成跨租户泄漏。
		return nil, nil, fmt.Errorf("知识库 MCP 请求缺少租户上下文")
	}
	capability := ""
	caps := c.Request.Header.Values(knowledgeCapabilityHeader)
	if len(caps) > 1 {
		// Values 按 canonical MIME key 取值：header 名大小写变体天然同桶，
		// 多值即重复头攻击。
		log.Printf("knowledge-mcp: duplicate %s headers rejected (tenant=%s agent=%s count=%d)", knowledgeCapabilityHeader, tenantID, agentCfg.Name, len(caps))
		deny := mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage)
		return nil, &deny, nil
	}
	if len(caps) == 1 {
		capability = strings.TrimSpace(caps[0])
		if capability == "" {
			// 呈现但空值 ≠ 缺失：缺失才回退，空值按拒绝。
			log.Printf("knowledge-mcp: blank %s header rejected (tenant=%s agent=%s)", knowledgeCapabilityHeader, tenantID, agentCfg.Name)
			deny := mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage)
			return nil, &deny, nil
		}
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	allowed, _, err := h.agentService.GetAgentKnowledgeDatasetsForRequest(tenantID, agentCfg.Name, capability, bearer)
	if err != nil {
		if errors.Is(err, services.ErrKnowledgeCapabilityDenied) {
			// 失败原因只进服务端日志；capability 值绝不上日志/响应。
			log.Printf("knowledge-mcp: capability rejected (tenant=%s agent=%s): %v", tenantID, agentCfg.Name, err)
			deny := mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage)
			return nil, &deny, nil
		}
		// 细节（绑定解析失败原因等）只进服务端日志，客户端拿中性文案。
		log.Printf("knowledge-mcp: resolve knowledge datasets failed (tenant=%s agent=%s): %v", tenantID, agentCfg.Name, err)
		return nil, nil, fmt.Errorf("获取 Agent 知识库绑定关系失败")
	}
	return allowed, nil, nil
}

// requireDatasetAccess 是遍历工具的统一 dataset 入口：TrimSpace + 非空校验
// （-32603）、agent/租户上下文解析、绑定子集校验（越权 → isError 中性文案）。
// 授权先于工具自有参数细节校验，未授权调用方拿不到参数校验细节。
// err 由调用方原样上抛（走 -32603）；deny 非 nil 时直接返回 *deny；皆零值即放行。
// 新增遍历工具必须经此入口，防止授权步骤在手写编排中漂移。
func (h *KnowledgeMcpHandler) requireDatasetAccess(c *gin.Context, id interface{}, rawDatasetID string) (datasetID string, deny *jsonRPCResponse, err error) {
	datasetID = strings.TrimSpace(rawDatasetID)
	if datasetID == "" {
		return "", nil, fmt.Errorf("dataset_id 不能为空")
	}
	allowed, capDeny, err := h.resolveAgentContext(c, id)
	if err != nil {
		return "", nil, err
	}
	if capDeny != nil {
		return "", capDeny, nil
	}
	if !isStringSubset([]string{datasetID}, allowed) {
		resp := mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage)
		return datasetID, &resp, nil
	}
	return datasetID, nil, nil
}

// pickFields 白名单拷贝，上游缺失的键直接省略。
func pickFields(obj map[string]any, keys ...string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			out[k] = v
		}
	}
	return out
}

func mcpJSONResult(id interface{}, payload interface{}) (jsonRPCResponse, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("结果序列化失败: %w", err)
	}
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{{"type": "text", "text": string(raw)}},
			"isError": false,
		},
	}, nil
}

// MCP 工具错误码（issue #119）：调用方（runtime/子 Agent）程序化识别用，
// 文案保持中性。码是稳定契约，只增不改。capability 拒绝与 dataset 越权
// 共用 dataset_not_authorized，不区分失败步骤——不给探测 oracle。
const (
	mcpErrDatasetNotAuthorized = "dataset_not_authorized"
	mcpErrNoDatasetBound       = "no_dataset_bound"
	mcpErrRetrievalFailed      = "retrieval_failed"
)

// mcpCodedErrorResult 构造带稳定错误码前缀的 isError 工具结果，
// 文本形态 `[<code>] <中性文案>`。
func mcpCodedErrorResult(id interface{}, code, msg string) jsonRPCResponse {
	return mcpErrorResult(id, "["+code+"] "+msg)
}

func mcpErrorResult(id interface{}, msg string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]interface{}{
			"content": []map[string]interface{}{{"type": "text", "text": msg}},
			"isError": true,
		},
	}
}

func isStringSubset(subset, superset []string) bool {
	set := make(map[string]bool, len(superset))
	for _, s := range superset {
		set[s] = true
	}
	for _, s := range subset {
		if !set[s] {
			return false
		}
	}
	return true
}

func formatRetrievalResult(result *knowledge.RetrievalResult) string {
	if result == nil {
		return "未检索到相关知识库内容。"
	}
	raw := map[string]interface{}(*result)
	chunks, ok := raw["chunks"].([]interface{})
	if !ok || len(chunks) == 0 {
		return "未检索到相关知识库内容。"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("根据知识库检索结果，共找到 %d 条相关分块：\n\n", len(chunks)))
	for _, item := range chunks {
		chunk, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		docName, _ := chunk["document_name"].(string)
		if docName == "" {
			docName = "未知文档"
		}
		similarity, _ := chunk["similarity"].(float64)
		content, _ := chunk["content"].(string)
		sb.WriteString(fmt.Sprintf("[来源：%s | 相似度：%.3f]\n%s\n\n", docName, similarity, content))
	}
	return sb.String()
}
