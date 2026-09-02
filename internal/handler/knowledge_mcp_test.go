package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

const testValidToken = "valid-token"

type fakeKnowledgeMcpService struct {
	retrievalFunc  func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error)
	getDatasetFunc func(ctx context.Context, id string) (*knowledge.Dataset, error)
	listDocsFunc   func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error)
	listChunksFunc func(ctx context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error)
}

func (f *fakeKnowledgeMcpService) Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
	if f.retrievalFunc != nil {
		return f.retrievalFunc(ctx, req)
	}
	return &knowledge.RetrievalResult{}, nil
}

func (f *fakeKnowledgeMcpService) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	if f.getDatasetFunc != nil {
		return f.getDatasetFunc(ctx, id)
	}
	return &knowledge.Dataset{}, nil
}

func (f *fakeKnowledgeMcpService) ListDocuments(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
	if f.listDocsFunc != nil {
		return f.listDocsFunc(ctx, datasetID, req)
	}
	return &knowledge.DocumentListResult{}, nil
}

func (f *fakeKnowledgeMcpService) ListChunks(ctx context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
	if f.listChunksFunc != nil {
		return f.listChunksFunc(ctx, datasetID, documentID, req)
	}
	return &knowledge.ChunkListResult{}, nil
}

type fakeAgentMcpService struct {
	datasets []string
	err      error
}

func (f *fakeAgentMcpService) GetAgentKnowledgeDatasets(tenantID, agentName string) ([]string, error) {
	return f.datasets, f.err
}

func testAgentAuthMiddleware(validToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(authHeader[len(prefix):])
		if token != validToken {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("agent", &agent.AgentConfig{Name: "test-agent"})
		tenant.SetTenantID(c, tenant.DefaultID)
		c.Next()
	}
}

func setupKnowledgeMcpRouter(knowledgeSvc KnowledgeMcpService, agentSvc AgentMcpService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewKnowledgeMcpHandler(knowledgeSvc, agentSvc)
	router := gin.New()
	router.POST("/api/v1/knowledge/mcp", testAgentAuthMiddleware(testValidToken), handler.HandleMessage)
	return router
}

func postJSONRPC(t *testing.T, router *gin.Engine, method string, params json.RawMessage, token string) *httptest.ResponseRecorder {
	body := jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestKnowledgeMcpHandler_Initialize(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "initialize", nil, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not an object: %v", resp.Result)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("protocolVersion = %v, want 2024-11-05", result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok || serverInfo["name"] != "knowledge-mcp" {
		t.Fatalf("serverInfo missing or invalid: %v", result["serverInfo"])
	}
}

func TestKnowledgeMcpHandler_ToolsList(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/list", nil, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not an object: %v", resp.Result)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %v", result["tools"])
	}
	tool, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatalf("tool is not an object: %v", tools[0])
	}
	if tool["name"] != "knowledge_search" {
		t.Fatalf("tool name = %v, want knowledge_search", tool["name"])
	}
	inputSchema, ok := tool["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputSchema missing or invalid: %v", tool["inputSchema"])
	}
	properties, ok := inputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties missing or invalid: %v", inputSchema["properties"])
	}
	if _, ok := properties["query"]; !ok {
		t.Fatalf("inputSchema missing required query property")
	}
	if _, ok := properties["question"]; ok {
		t.Fatalf("inputSchema should not advertise legacy question property")
	}
	required, ok := inputSchema["required"].([]interface{})
	if !ok {
		t.Fatalf("required missing or invalid: %v", inputSchema["required"])
	}
	found := false
	for _, r := range required {
		if s, ok := r.(string); ok && s == "query" {
			found = true
		}
	}
	if !found {
		t.Fatalf("required should contain query, got %v", required)
	}
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		if m, ok := t.(map[string]interface{}); ok {
			names[m["name"].(string)] = true
		}
	}
	for _, want := range []string{"knowledge_search", "knowledge_datasets", "knowledge_documents", "knowledge_chunks"} {
		if !names[want] {
			t.Fatalf("tools/list missing tool %s, got %v", want, names)
		}
	}
}

func TestKnowledgeMcpHandler_ToolsCall_NoBoundDatasets(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{datasets: []string{}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) {
		t.Fatalf("expected isError=true, got result = %v", resp.Result)
	}
	if !strings.Contains(resultText(resp.Result), "未启用知识库 MCP") {
		t.Fatalf("expected disabled message, got %v", resp.Result)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_UnauthorizedDataset(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["unauthorized-dataset"]}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) {
		t.Fatalf("expected isError=true, got result = %v", resp.Result)
	}
	if !strings.Contains(resultText(resp.Result), "无权访问") {
		t.Fatalf("expected unauthorized message, got %v", resp.Result)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Success(t *testing.T) {
	var gotReq knowledge.RetrievalRequest
	knowledgeSvc := &fakeKnowledgeMcpService{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			gotReq = req
			result := knowledge.RetrievalResult{
				"chunks": []interface{}{
					map[string]interface{}{
						"document_name": "doc1",
						"similarity":    0.95,
						"content":       "relevant content",
					},
				},
			}
			return &result, nil
		},
	}
	router := setupKnowledgeMcpRouter(
		knowledgeSvc,
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","top_k":5,"similarity_threshold":0.3,"vector_similarity_weight":0.5,"highlight":true}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected isError=false, got result = %v", resp.Result)
	}
	// 下游 multirag /api/v1/retrieval 的契约 key 仍是 question；只有 MCP 入参改名 query。
	if gotQuestion, _ := gotReq["question"].(string); gotQuestion != "hello" {
		t.Fatalf("question = %q, want hello", gotQuestion)
	}
	if gotDatasetIDs, _ := gotReq["dataset_ids"].([]string); len(gotDatasetIDs) != 1 || gotDatasetIDs[0] != "allowed-dataset" {
		t.Fatalf("dataset_ids = %v, want [allowed-dataset]", gotReq["dataset_ids"])
	}
	if topK, _ := gotReq["top_k"].(int); topK != 5 {
		t.Fatalf("top_k = %v, want 5", gotReq["top_k"])
	}
	text := resultText(resp.Result)
	if !strings.Contains(text, "doc1") || !strings.Contains(text, "relevant content") {
		t.Fatalf("response text missing expected content: %s", text)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Datasets(t *testing.T) {
	knowledgeSvc := &fakeKnowledgeMcpService{
		getDatasetFunc: func(ctx context.Context, id string) (*knowledge.Dataset, error) {
			// 模拟 NormalizeDataset 出口形状：计数键为 canonical doc_num/chunk_num。
			ds := knowledge.Dataset{
				"id": id, "name": "产品知识库", "description": "产品文档",
				"doc_num": 3, "chunk_num": 42,
				"tenant_id": "internal-should-not-leak", // 白名单外字段必须被丢弃
			}
			return &ds, nil
		},
	}
	router := setupKnowledgeMcpRouter(
		knowledgeSvc,
		&fakeAgentMcpService{datasets: []string{"ds-1"}},
	)
	params := json.RawMessage(`{"name":"knowledge_datasets","arguments":{}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected isError=false, got result = %v", resp.Result)
	}
	text := resultText(resp.Result)
	var payload struct {
		Datasets []map[string]interface{} `json:"datasets"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result text is not JSON: %v; text=%s", err, text)
	}
	if len(payload.Datasets) != 1 {
		t.Fatalf("datasets len = %d, want 1", len(payload.Datasets))
	}
	got := payload.Datasets[0]
	if got["id"] != "ds-1" || got["name"] != "产品知识库" {
		t.Fatalf("unexpected dataset entry: %v", got)
	}
	if got["document_count"] != float64(3) || got["chunk_count"] != float64(42) {
		t.Fatalf("count fields not mapped from doc_num/chunk_num: %v", got)
	}
	if _, leaked := got["tenant_id"]; leaked {
		t.Fatalf("whitelist violated: tenant_id leaked in %v", got)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Datasets_NoBindings(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{datasets: []string{}},
	)
	params := json.RawMessage(`{"name":"knowledge_datasets","arguments":{}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("no bindings should return empty list, not error: %v", resp.Result)
	}
	if !strings.Contains(resultText(resp.Result), `"datasets":[]`) {
		t.Fatalf("expected empty datasets array, got %s", resultText(resp.Result))
	}
}

func TestKnowledgeMcpHandler_InvalidToken(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/list", nil, "wrong-token")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestKnowledgeMcpHandler_ToolsCall_LegacyQuestionArg(t *testing.T) {
	// 兼容已部署未重启的 runtime 容器：它们缓存了旧 tools/list schema，
	// 升级 hub 后仍会发 question 参数，须继续接受（schema 只广播 query）。
	var gotReq knowledge.RetrievalRequest
	knowledgeSvc := &fakeKnowledgeMcpService{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			gotReq = req
			return &knowledge.RetrievalResult{}, nil
		},
	}
	router := setupKnowledgeMcpRouter(
		knowledgeSvc,
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"legacy hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected isError=false for legacy question arg, got result = %v", resp.Result)
	}
	if gotQuestion, _ := gotReq["question"].(string); gotQuestion != "legacy hello" {
		t.Fatalf("downstream question = %q, want legacy hello", gotQuestion)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_MissingQuery(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "query is required") {
		t.Fatalf("expected 'query is required' error, got %+v", resp)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Defaults(t *testing.T) {
	var gotReq knowledge.RetrievalRequest
	knowledgeSvc := &fakeKnowledgeMcpService{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			gotReq = req
			return &knowledge.RetrievalResult{}, nil
		},
	}
	router := setupKnowledgeMcpRouter(
		knowledgeSvc,
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if topK, _ := gotReq["top_k"].(int); topK != 8 {
		t.Fatalf("top_k = %v, want 8", gotReq["top_k"])
	}
	if st, _ := gotReq["similarity_threshold"].(float64); st != 0.2 {
		t.Fatalf("similarity_threshold = %v, want 0.2", gotReq["similarity_threshold"])
	}
	if vsw, _ := gotReq["vector_similarity_weight"].(float64); vsw != 0.3 {
		t.Fatalf("vector_similarity_weight = %v, want 0.3", gotReq["vector_similarity_weight"])
	}
	if hl, _ := gotReq["highlight"].(bool); hl != false {
		t.Fatalf("highlight = %v, want false", gotReq["highlight"])
	}
}

func TestKnowledgeMcpHandler_ToolsCall_ServiceError(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{
			retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
				return nil, errors.New("retrieval failed")
			},
		},
		&fakeAgentMcpService{datasets: []string{"allowed-dataset"}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) {
		t.Fatalf("expected isError=true for retrieval error, got result = %v", resp.Result)
	}
	if !strings.Contains(resultText(resp.Result), "检索失败") {
		t.Fatalf("expected retrieval failure message, got %v", resp.Result)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_MissingTenantContext(t *testing.T) {
	// 理论不可达（middleware 命中 agents 行必写 tenant），防御性验证：
	// 无 tenant 时必须返回明确错误，而非用空串静默查询全表。
	gin.SetMode(gin.TestMode)
	var gotTenantID string
	agentSvc := &tenantAwareAgentMcpService{fn: func(tenantID, agentName string) ([]string, error) {
		gotTenantID = tenantID
		return []string{"allowed-dataset"}, nil
	}}
	router := gin.New()
	router.POST("/api/v1/knowledge/mcp", func(c *gin.Context) {
		// deliberately no tenant.SetTenantID
		c.Set("agent", &agent.AgentConfig{Name: "test-agent"})
		c.Next()
	}, NewKnowledgeMcpHandler(&fakeKnowledgeMcpService{}, agentSvc).HandleMessage)

	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if gotTenantID != "" {
		t.Fatalf("agent service should not be called with empty tenant, got %q", gotTenantID)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "tenant") {
		t.Fatalf("expected explicit tenant-missing error, got %+v", resp)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_TenantScopedDatasets(t *testing.T) {
	// 验证 handler 把 context 里的 tenant_id 原样传给 dataset 反查。
	var gotTenantID, gotAgentName string
	agentSvc := &tenantAwareAgentMcpService{fn: func(tenantID, agentName string) ([]string, error) {
		gotTenantID, gotAgentName = tenantID, agentName
		return []string{"tenant-a-dataset"}, nil
	}}
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, agentSvc)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if gotTenantID != tenant.DefaultID {
		t.Fatalf("tenantID = %q, want %q", gotTenantID, tenant.DefaultID)
	}
	if gotAgentName != "test-agent" {
		t.Fatalf("agentName = %q, want test-agent", gotAgentName)
	}
}

type tenantAwareAgentMcpService struct {
	fn func(tenantID, agentName string) ([]string, error)
}

func (f *tenantAwareAgentMcpService) GetAgentKnowledgeDatasets(tenantID, agentName string) ([]string, error) {
	return f.fn(tenantID, agentName)
}

func isErrorResult(result interface{}) bool {
	m, ok := result.(map[string]interface{})
	if !ok {
		return false
	}
	v, ok := m["isError"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func resultText(result interface{}) string {
	m, ok := result.(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := m["content"].([]interface{})
	if !ok || len(content) == 0 {
		return ""
	}
	item, ok := content[0].(map[string]interface{})
	if !ok {
		return ""
	}
	text, _ := item["text"].(string)
	return text
}

func TestKnowledgeMcpHandler_ToolsCall_Documents_Pagination(t *testing.T) {
	var gotReq knowledge.DocumentListRequest
	var gotDataset string
	knowledgeSvc := &fakeKnowledgeMcpService{
		listDocsFunc: func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
			gotDataset, gotReq = datasetID, req
			return &knowledge.DocumentListResult{
				Total: 57,
				Documents: []knowledge.Document{
					// 模拟 NormalizeDocument 出口形状：计数键为 canonical chunk_num；
					// doc-2 故意缺 run 键，锁定缺键省略（而非置零）语义。
					{"id": "doc-1", "name": "使用手册.pdf", "chunk_num": 12, "progress": 1.0, "run": "DONE", "create_time": 1700000000.0, "storage_path": "/secret"},
					{"id": "doc-2", "name": "FAQ.md", "chunk_num": 7, "progress": 0.5, "create_time": 1700000001.0},
				},
			}, nil
		},
	}
	router := setupKnowledgeMcpRouter(knowledgeSvc, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	params := json.RawMessage(`{"name":"knowledge_documents","arguments":{"dataset_id":"ds-1","page":2,"page_size":30}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if gotDataset != "ds-1" || gotReq.Page != 2 || gotReq.PageSize != 30 {
		t.Fatalf("pagination passthrough wrong: dataset=%q req=%+v", gotDataset, gotReq)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected success, got %v", resp.Result)
	}
	text := resultText(resp.Result)
	var payload struct {
		Total     int                      `json:"total"`
		Page      int                      `json:"page"`
		PageSize  int                      `json:"page_size"`
		Documents []map[string]interface{} `json:"documents"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result text is not JSON: %v; text=%s", err, text)
	}
	if payload.Total != 57 || payload.Page != 2 || payload.PageSize != 30 {
		t.Fatalf("payload echo wrong: total=%d page=%d page_size=%d; text=%s", payload.Total, payload.Page, payload.PageSize, text)
	}
	if len(payload.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2; text=%s", len(payload.Documents), text)
	}
	if payload.Documents[0]["chunk_count"] != float64(12) {
		t.Fatalf("chunk_count not mapped from chunk_num: %v", payload.Documents[0])
	}
	if _, exists := payload.Documents[1]["run"]; exists {
		t.Fatalf("missing source key should be omitted, got run=%v", payload.Documents[1]["run"])
	}
	if strings.Contains(text, "storage_path") {
		t.Fatalf("whitelist violated: %s", text)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Documents_DefaultsAndClamp(t *testing.T) {
	var gotReq knowledge.DocumentListRequest
	knowledgeSvc := &fakeKnowledgeMcpService{
		listDocsFunc: func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
			gotReq = req
			return &knowledge.DocumentListResult{}, nil
		},
	}
	router := setupKnowledgeMcpRouter(knowledgeSvc, &fakeAgentMcpService{datasets: []string{"ds-1"}})

	// 缺省 → page=1, page_size=20
	postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_documents","arguments":{"dataset_id":"ds-1"}}`), testValidToken)
	if gotReq.Page != 1 || gotReq.PageSize != 20 {
		t.Fatalf("defaults wrong: %+v", gotReq)
	}
	// 超上限 → 钳制为 50
	postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_documents","arguments":{"dataset_id":"ds-1","page_size":500}}`), testValidToken)
	if gotReq.PageSize != 50 {
		t.Fatalf("clamp wrong: %+v", gotReq)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Documents_Unauthorized(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	params := json.RawMessage(`{"name":"knowledge_documents","arguments":{"dataset_id":"other-tenant-ds"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "无权访问") {
		t.Fatalf("expected unauthorized isError, got %v", resp.Result)
	}

	// 缺 dataset_id → JSON-RPC error
	rec2 := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_documents","arguments":{}}`), testValidToken)
	var resp2 jsonRPCResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp2.Error == nil || !strings.Contains(resp2.Error.Message, "dataset_id is required") {
		t.Fatalf("expected dataset_id required error, got %+v", resp2)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Chunks_ReadPage(t *testing.T) {
	var gotDS, gotDoc string
	var gotReq knowledge.ChunkListRequest
	knowledgeSvc := &fakeKnowledgeMcpService{
		listChunksFunc: func(ctx context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
			gotDS, gotDoc, gotReq = datasetID, documentID, req
			return &knowledge.ChunkListResult{
				Total: 12,
				Chunks: []knowledge.Chunk{
					{"id": "ck-1", "content": "第一章内容", "image_id": "internal"},
				},
				Document: knowledge.Document{"name": "使用手册.pdf"},
			}, nil
		},
	}
	router := setupKnowledgeMcpRouter(knowledgeSvc, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	params := json.RawMessage(`{"name":"knowledge_chunks","arguments":{"dataset_id":"ds-1","document_id":"doc-1","page":1,"page_size":5}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if gotDS != "ds-1" || gotDoc != "doc-1" || gotReq.Page != 1 || gotReq.PageSize != 5 {
		t.Fatalf("args passthrough wrong: ds=%q doc=%q req=%+v", gotDS, gotDoc, gotReq)
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected success, got %v", resp.Result)
	}
	text := resultText(resp.Result)
	if !strings.Contains(text, `"chunk_id":"ck-1"`) || !strings.Contains(text, "第一章内容") {
		t.Fatalf("unexpected payload: %s", text)
	}
	if !strings.Contains(text, `"document_name":"使用手册.pdf"`) {
		t.Fatalf("missing document_name: %s", text)
	}
	if strings.Contains(text, "image_id") {
		t.Fatalf("whitelist violated: %s", text)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Chunks_RequiredArgs(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	// 缺 document_id → JSON-RPC error
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_chunks","arguments":{"dataset_id":"ds-1"}}`), testValidToken)
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "document_id is required") {
		t.Fatalf("expected document_id required error, got %+v", resp)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Chunks_Unauthorized(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	params := json.RawMessage(`{"name":"knowledge_chunks","arguments":{"dataset_id":"other","document_id":"doc-1"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "无权访问") {
		t.Fatalf("expected unauthorized isError, got %v", resp.Result)
	}
}
