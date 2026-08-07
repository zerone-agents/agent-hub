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

	"github.com/gin-gonic/gin"
)

const testValidToken = "valid-token"

type fakeKnowledgeMcpService struct {
	retrievalFunc func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error)
}

func (f *fakeKnowledgeMcpService) Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
	if f.retrievalFunc != nil {
		return f.retrievalFunc(ctx, req)
	}
	return &knowledge.RetrievalResult{}, nil
}

type fakeAgentMcpService struct {
	datasets []string
	err      error
}

func (f *fakeAgentMcpService) GetAgentKnowledgeDatasets(agentName string) ([]string, error) {
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
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", result["tools"])
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
	if _, ok := properties["question"]; !ok {
		t.Fatalf("inputSchema missing required question property")
	}
}

func TestKnowledgeMcpHandler_ToolsCall_NoBoundDatasets(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{datasets: []string{}},
	)
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"hello"}}`)
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
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"hello","dataset_ids":["unauthorized-dataset"]}}`)
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
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"hello","top_k":5,"similarity_threshold":0.3,"vector_similarity_weight":0.5,"highlight":true}}`)
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

func TestKnowledgeMcpHandler_InvalidToken(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/list", nil, "wrong-token")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
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
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"hello"}}`)
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
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"question":"hello"}}`)
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
