package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/middleware"

	"github.com/gin-gonic/gin"
)

// KnowledgeMcpService abstracts the knowledge operations needed by the MCP handler.
type KnowledgeMcpService interface {
	Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error)
}

// AgentMcpService abstracts the agent operations needed by the MCP handler.
type AgentMcpService interface {
	GetAgentKnowledgeDatasets(tenantID, agentName string) ([]string, error)
	CanAgentUseSubagent(tenantID, agentName, subagentName string) (bool, error)
}

const knowledgeAgentHeader = "X-Zerone-Knowledge-Agent"

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
	Question               string   `json:"question"`
	DatasetIDs             []string `json:"dataset_ids"`
	TopK                   *int     `json:"top_k"`
	SimilarityThreshold    *float64 `json:"similarity_threshold"`
	VectorSimilarityWeight *float64 `json:"vector_similarity_weight"`
	Highlight              *bool    `json:"highlight"`
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
					"annotations": map[string]interface{}{
						"readOnlyHint": true,
					},
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"question": map[string]interface{}{
								"type":        "string",
								"description": "A search-optimized question for the knowledge base. Keep core entities, keywords, and intent; remove conversational filler words to retrieve more relevant snippets.",
							},
							"dataset_ids": map[string]interface{}{
								"type":        "array",
								"items":       map[string]interface{}{"type": "string"},
								"description": "Optional. Restrict retrieval to specific knowledge bases. Valid dataset IDs can be found in the <datasets> block of the system prompt. If omitted, all knowledge bases bound to this agent are used automatically.",
							},
							"top_k": map[string]interface{}{
								"type":        "number",
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
						"required": []string{"question"},
					},
				},
			},
		},
	}
}

func (h *KnowledgeMcpHandler) handleToolsCall(ctx context.Context, c *gin.Context, id interface{}, params json.RawMessage) (jsonRPCResponse, error) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("invalid params: %w", err)
	}

	if p.Name != "knowledge_search" {
		return jsonRPCResponse{}, fmt.Errorf("tool not found: %s", p.Name)
	}

	var args knowledgeSearchArgs
	if err := json.Unmarshal(p.Arguments, &args); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("invalid arguments: %w", err)
	}
	args.Question = strings.TrimSpace(args.Question)
	if args.Question == "" {
		return jsonRPCResponse{}, fmt.Errorf("question is required")
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

	agentCfg, ok := middleware.AgentFromContext(c)
	if !ok {
		return jsonRPCResponse{}, fmt.Errorf("agent not found in context")
	}

	tenantID := tenant.GetTenantID(c)
	if tenantID == "" {
		// 理论不可达：AgentRuntimeAuthMiddleware 命中 agents 行后必写 tenant_id。
		// 防御性拒绝，避免空串 tenant 静默查询全表造成跨租户泄漏。
		return jsonRPCResponse{}, fmt.Errorf("tenant context missing on knowledge MCP request")
	}

	knowledgeAgentName := strings.TrimSpace(c.GetHeader(knowledgeAgentHeader))
	if knowledgeAgentName == "" {
		knowledgeAgentName = agentCfg.Name
	}
	if knowledgeAgentName != agentCfg.Name {
		allowed, err := h.agentService.CanAgentUseSubagent(tenantID, agentCfg.Name, knowledgeAgentName)
		if err != nil {
			return jsonRPCResponse{}, fmt.Errorf("failed to validate knowledge subagent: %w", err)
		}
		if !allowed {
			return jsonRPCResponse{}, fmt.Errorf("knowledge agent %q is not mounted by %q", knowledgeAgentName, agentCfg.Name)
		}
	}

	allowedDatasetIDs, err := h.agentService.GetAgentKnowledgeDatasets(tenantID, knowledgeAgentName)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("failed to get agent knowledge datasets: %w", err)
	}
	if len(allowedDatasetIDs) == 0 {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "当前 Agent 未启用知识库 MCP"},
				},
				"isError": true,
			},
		}, nil
	}

	datasetIDs := args.DatasetIDs
	if len(datasetIDs) == 0 {
		datasetIDs = allowedDatasetIDs
	}
	if !isStringSubset(datasetIDs, allowedDatasetIDs) {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "无权访问部分知识库 dataset"},
				},
				"isError": true,
			},
		}, nil
	}

	req := knowledge.RetrievalRequest{
		"question":                 args.Question,
		"dataset_ids":              datasetIDs,
		"top_k":                    *args.TopK,
		"similarity_threshold":     *args.SimilarityThreshold,
		"vector_similarity_weight": *args.VectorSimilarityWeight,
		"highlight":                *args.Highlight,
	}

	result, err := h.knowledgeService.Retrieval(ctx, req)
	if err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": fmt.Sprintf("知识库检索失败: %v", err)},
				},
				"isError": true,
			},
		}, nil
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
