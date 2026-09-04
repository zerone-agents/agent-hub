package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
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

func (f *fakeAgentMcpService) GetAgentKnowledgeDatasetsForRequest(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
	return f.datasets, tokenAgentName, f.err
}

func testAgentAuthMiddlewareFor(agentName, validToken string) gin.HandlerFunc {
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
		c.Set("agent", &agent.AgentConfig{Name: agentName})
		tenant.SetTenantID(c, tenant.DefaultID)
		c.Next()
	}
}

func testAgentAuthMiddleware(validToken string) gin.HandlerFunc {
	return testAgentAuthMiddlewareFor("test-agent", validToken)
}

func setupKnowledgeMcpRouter(knowledgeSvc KnowledgeMcpService, agentSvc AgentMcpService) *gin.Engine {
	return setupKnowledgeMcpRouterAsAgent("test-agent", knowledgeSvc, agentSvc)
}

// setupKnowledgeMcpRouterAsAgent 与 setupKnowledgeMcpRouter 相同，但把
// Bearer 命中的 token agent 换成指定名（按身份授权矩阵用 root）。
func setupKnowledgeMcpRouterAsAgent(agentName string, knowledgeSvc KnowledgeMcpService, agentSvc AgentMcpService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewKnowledgeMcpHandler(knowledgeSvc, agentSvc)
	router := gin.New()
	router.POST("/api/v1/knowledge/mcp", testAgentAuthMiddlewareFor(agentName, testValidToken), handler.HandleMessage)
	return router
}

func postJSONRPC(t *testing.T, router *gin.Engine, method string, params json.RawMessage, token string) *httptest.ResponseRecorder {
	return postJSONRPCWithHeaders(t, router, method, params, token, nil)
}

// postJSONRPCWithHeaders 在 postJSONRPC 基础上附加任意请求头。key 经
// net/http 的 MIME 规范化存储（Add 语义：同名 key 多值累加），用于
// capability 攻击矩阵（重复头、大小写变体、裸 X-Agent-Id 等）。
func postJSONRPCWithHeaders(t *testing.T, router *gin.Engine, method string, params json.RawMessage, token string, headers map[string][]string) *httptest.ResponseRecorder {
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
	for k, vs := range headers {
		for _, v := range vs {
			// Add 规范化 key：模拟真实链路里 net/http 对 header 名的
			// 大小写归一（x-agent-capability 与 X-Agent-Capability 同桶）。
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// postJSONRPCWithCapability 附带一个或多个 X-Agent-Capability 请求头
// （部署时 hub 签发并注入 MCP 连接头），用于按身份授权攻击矩阵。
func postJSONRPCWithCapability(t *testing.T, router *gin.Engine, method string, params json.RawMessage, token string, capabilities ...string) *httptest.ResponseRecorder {
	return postJSONRPCWithHeaders(t, router, method, params, token, map[string][]string{
		"X-Agent-Capability": capabilities,
	})
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
	// 整型入参（Go *int）在 schema 中须标 integer，浮点入参（*float64）标 number——
	// 与实际 unmarshal 行为一致，避免客户端按 number 发 2.5 得到莫名 -32603。
	wantTypes := map[string]map[string]string{
		"knowledge_search":    {"top_k": "integer", "similarity_threshold": "number", "vector_similarity_weight": "number"},
		"knowledge_documents": {"page": "integer", "page_size": "integer"},
		"knowledge_chunks":    {"page": "integer", "page_size": "integer"},
	}
	for _, tl := range tools {
		m, ok := tl.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		want, ok := wantTypes[name]
		if !ok {
			continue
		}
		schema, ok := m["inputSchema"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: inputSchema missing or invalid", name)
		}
		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s: properties missing or invalid", name)
		}
		for param, wantType := range want {
			p, ok := props[param].(map[string]interface{})
			if !ok {
				t.Fatalf("%s: property %s missing", name, param)
			}
			if got := p["type"]; got != wantType {
				t.Fatalf("%s.%s type = %v, want %s", name, param, got, wantType)
			}
			// page_size 超上限由服务端钳制（不报错），schema 不得声明 maximum：
			// 做 schema 校验的客户端会在发请求前拒绝 >50 的值，钳制永远走不到。
			if param == "page_size" {
				if _, hasMax := p["maximum"]; hasMax {
					t.Fatalf("%s.page_size must not declare maximum in inputSchema (server clamps instead of rejecting)", name)
				}
			}
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
	text := resultText(resp.Result)
	if !strings.HasPrefix(text, "[no_dataset_bound]") || !strings.Contains(text, "当前 Agent 未绑定任何知识库数据集") {
		t.Fatalf("expected [no_dataset_bound] message, got %q", text)
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

// issue #111（重开）：Knowledge MCP 授权按「服务端可验证的 per-Agent
// capability」逐节点隔离。capability 由 hub 部署时用仅服务端持有的密钥
// 签发（HMAC），绑定 tenant/部署 key/请求身份/token 指纹；裸 X-Agent-Id
// 不再参与授权。child 不得访问 parent/sibling 的 dataset，root 也不得
// 访问 child 的。token agent 固定为 root（fixture：root 绑 ds-root，
// child-a 绑 ds-child——verify 逻辑由 services 包单测锁定，此处的 keyed
// fake 按真实语义模拟其返回）。
func TestKnowledgeMcpHandler_ToolsCall_PerAgentCapabilityAuthorization(t *testing.T) {
	const (
		capRoot   = "v1.cap-root-opaque"
		capChildA = "v1.cap-child-a-opaque"
	)
	// capability 值是敏感凭据：矩阵第 7 组断言其绝不出现在响应文本里。
	allCaps := []string{capRoot, capChildA, "v1.cap-tampered-opaque"}

	resolve := func(capabilityHeader, bearerToken string) ([]string, string, error) {
		// 模拟真实 service 语义：token 指纹不匹配（轮换）→ sentinel deny。
		if bearerToken != testValidToken {
			return nil, "", fmt.Errorf("%w: token fingerprint mismatch", services.ErrKnowledgeCapabilityDenied)
		}
		switch capabilityHeader {
		case "":
			// 缺失 capability → 存量回退：token agent 自身绑定。
			return []string{"ds-root"}, "root", nil
		case capRoot:
			return []string{"ds-root"}, "root", nil
		case capChildA:
			return []string{"ds-child"}, "child-a", nil
		default:
			return nil, "", fmt.Errorf("%w: capability rejected", services.ErrKnowledgeCapabilityDenied)
		}
	}
	newRouter := func(knowledgeSvc KnowledgeMcpService, fn func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error)) *gin.Engine {
		return setupKnowledgeMcpRouterAsAgent("root", knowledgeSvc, &tenantAwareAgentMcpService{fn: fn})
	}
	bindings := func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
		return resolve(capabilityHeader, bearerToken)
	}

	t.Run("root capability → 只授权 root datasets", func(t *testing.T) {
		var gotDatasetIDs []string
		knowledgeSvc := &fakeKnowledgeMcpService{
			retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
				gotDatasetIDs, _ = req["dataset_ids"].([]string)
				return &knowledge.RetrievalResult{}, nil
			},
		}
		rec := postJSONRPCWithCapability(t, newRouter(knowledgeSvc, bindings), "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, capRoot)

		var resp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if isErrorResult(resp.Result) {
			t.Fatalf("root capability requesting its own dataset must pass, got %v", resp.Result)
		}
		if len(gotDatasetIDs) != 1 || gotDatasetIDs[0] != "ds-root" {
			t.Fatalf("retrieval dataset_ids = %v, want [ds-root]", gotDatasetIDs)
		}
	})

	t.Run("child-a capability → 真实检索放行自己的 dataset", func(t *testing.T) {
		var gotDatasetIDs []string
		knowledgeSvc := &fakeKnowledgeMcpService{
			retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
				gotDatasetIDs, _ = req["dataset_ids"].([]string)
				return &knowledge.RetrievalResult{}, nil
			},
		}
		rec := postJSONRPCWithCapability(t, newRouter(knowledgeSvc, bindings), "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-child"]}}`),
			testValidToken, capChildA)

		var resp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if isErrorResult(resp.Result) {
			t.Fatalf("child-a capability requesting its own dataset must pass, got %v", resp.Result)
		}
		if len(gotDatasetIDs) != 1 || gotDatasetIDs[0] != "ds-child" {
			t.Fatalf("retrieval dataset_ids = %v, want [ds-child]", gotDatasetIDs)
		}
	})

	t.Run("child-a capability 请求 parent 的 dataset → 拒绝", func(t *testing.T) {
		rec := postJSONRPCWithCapability(t, newRouter(&fakeKnowledgeMcpService{}, bindings), "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, capChildA)

		var resp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "无权访问") {
			t.Fatalf("child-a capability must not reach parent datasets, got %v", resp.Result)
		}
	})

	t.Run("root token + 裸 X-Agent-Id（无 capability）→ 完全不参与，回退 root 自身", func(t *testing.T) {
		var gotCapabilityHeader string
		capture := func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
			gotCapabilityHeader = capabilityHeader
			return resolve(capabilityHeader, bearerToken)
		}
		router := newRouter(&fakeKnowledgeMcpService{}, capture)

		// 裸 X-Agent-Id: child-a 出现在请求里，但 capability 通道为空。
		rec := postJSONRPCWithHeaders(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-child"]}}`),
			testValidToken, map[string][]string{"X-Agent-Id": {"child-a"}})
		var resp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "无权访问") {
			t.Fatalf("bare X-Agent-Id must not grant child bindings, got %v", resp.Result)
		}
		if gotCapabilityHeader != "" {
			t.Fatalf("service must receive empty capability header; bare X-Agent-Id must never reach it, got %q", gotCapabilityHeader)
		}

		// 同一请求下 root 自身 dataset 放行（回退 = token agent 绑定）。
		rec = postJSONRPCWithHeaders(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, map[string][]string{"X-Agent-Id": {"root"}})
		var passResp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &passResp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if isErrorResult(passResp.Result) {
			t.Fatalf("fallback (no capability) must grant token agent own bindings, got %v", passResp.Result)
		}
	})

	denyTexts := make([]string, 0, 4) // matrix 7: 所有 deny 原因同一中性文案
	collectDeny := func(t *testing.T, rec *httptest.ResponseRecorder) string {
		t.Helper()
		var resp jsonRPCResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if !isErrorResult(resp.Result) {
			t.Fatalf("expected isError deny, got %v", resp.Result)
		}
		text := resultText(resp.Result)
		denyTexts = append(denyTexts, text)
		return text
	}

	t.Run("篡改 capability（payload/大小写）→ 验签失败拒绝", func(t *testing.T) {
		router := newRouter(&fakeKnowledgeMcpService{}, bindings)
		// 未知 capability 值（模拟篡改后验签不过）。
		collectDeny(t, postJSONRPCWithCapability(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, "v1.cap-tampered-opaque"))
		// 内容大小写改动。
		collectDeny(t, postJSONRPCWithCapability(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, "V1.CAP-ROOT-OPAQUE"))
	})

	t.Run("token 轮换：旧 token 签的 capability + 轮换后的新 token 请求 → 拒绝", func(t *testing.T) {
		// 轮换后新 token 本身有效（middleware 放行），但 capability 的
		// TokenFp 绑定签发时的旧 token（fake: capRoot 仅对 testValidToken
		// 有效）→ TokenFp 失配 → sentinel deny。
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/api/v1/knowledge/mcp",
			testAgentAuthMiddlewareFor("root", "new-rotated-token"),
			NewKnowledgeMcpHandler(&fakeKnowledgeMcpService{}, &tenantAwareAgentMcpService{fn: bindings}).HandleMessage)
		collectDeny(t, postJSONRPCWithCapability(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			"new-rotated-token", capRoot))
	})

	t.Run("重复 X-Agent-Capability 头 → 拒绝且不触发 service", func(t *testing.T) {
		called := false
		capture := func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
			called = true
			return resolve(capabilityHeader, bearerToken)
		}
		router := newRouter(&fakeKnowledgeMcpService{}, capture)
		// 两个值（第二个用小写 key 变体——Add 规范化后与第一个同桶，
		// 模拟真实链路里 net/http 对 header 名的大小写归一）都必须按重复拒绝。
		collectDeny(t, postJSONRPCWithHeaders(t, router, "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, map[string][]string{
				"X-Agent-Capability": {capRoot},
				"x-agent-capability": {capChildA},
			}))
		if called {
			t.Fatal("duplicate capability headers must be denied before the service is called")
		}
	})

	t.Run("空值 capability 头 → 拒绝（present-but-blank ≠ 缺失）", func(t *testing.T) {
		collectDeny(t, postJSONRPCWithCapability(t, newRouter(&fakeKnowledgeMcpService{}, bindings), "tools/call",
			json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello","dataset_ids":["ds-root"]}}`),
			testValidToken, "  "))
	})

	// matrix 7：capability 值不出现在任何响应文本；所有 deny 原因同一文案
	// （不给探测 oracle），与 dataset 越权拒绝的文案一致。
	for _, text := range denyTexts {
		if text != "[dataset_not_authorized] 无权访问部分知识库 dataset" {
			t.Fatalf("all capability denies must share the neutral subset-denial message, got %q (all: %v)", text, denyTexts)
		}
	}
	assertNoCapabilityLeak(t, allCaps, denyTexts)
}

// assertNoCapabilityLeak 断言 capability 值不出现在任何客户端可见文本里。
func assertNoCapabilityLeak(t *testing.T, caps, texts []string) {
	t.Helper()
	for _, text := range texts {
		for _, cap := range caps {
			if strings.Contains(text, cap) {
				t.Fatalf("capability value leaked to client-visible text: %q", cap)
			}
		}
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
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "query 不能为空") {
		t.Fatalf("expected 'query 不能为空' error, got %+v", resp)
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

// upstreamLeakMarker 模拟 multirag 错误里会携带的内网拓扑信息，
// 用于断言：上游细节只进服务端日志，绝不进 MCP 响应文本（LLM 可见）。
const upstreamLeakMarker = `Get "http://10.0.0.1:9380/api/v1": dial tcp 10.0.0.1:9380: i/o timeout`

func TestKnowledgeMcpHandler_ToolsCall_ServiceErrorsNeutralized(t *testing.T) {
	leakErr := errors.New(upstreamLeakMarker)
	cases := []struct {
		name    string
		svc     *fakeKnowledgeMcpService
		params  string
		wantMsg string
	}{
		{
			name: "search",
			svc: &fakeKnowledgeMcpService{retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
				return nil, leakErr
			}},
			params:  `{"name":"knowledge_search","arguments":{"query":"hello"}}`,
			wantMsg: "知识库检索失败",
		},
		{
			name: "documents",
			svc: &fakeKnowledgeMcpService{listDocsFunc: func(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
				return nil, leakErr
			}},
			params:  `{"name":"knowledge_documents","arguments":{"dataset_id":"allowed-dataset"}}`,
			wantMsg: "知识库文档列表获取失败",
		},
		{
			name: "chunks",
			svc: &fakeKnowledgeMcpService{listChunksFunc: func(ctx context.Context, datasetID, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
				return nil, leakErr
			}},
			params:  `{"name":"knowledge_chunks","arguments":{"dataset_id":"allowed-dataset","document_id":"doc-1"}}`,
			wantMsg: "知识库分块读取失败",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := setupKnowledgeMcpRouter(tc.svc, &fakeAgentMcpService{datasets: []string{"allowed-dataset"}})
			rec := postJSONRPC(t, router, "tools/call", json.RawMessage(tc.params), testValidToken)
			var resp jsonRPCResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !isErrorResult(resp.Result) {
				t.Fatalf("expected isError=true, got %v", resp.Result)
			}
			text := resultText(resp.Result)
			if !strings.Contains(text, tc.wantMsg) {
				t.Fatalf("expected neutral message %q, got %q", tc.wantMsg, text)
			}
			if strings.Contains(text, "10.0.0.1") || strings.Contains(text, "9380") {
				t.Fatalf("upstream detail leaked to client: %q", text)
			}
		})
	}
}

func TestKnowledgeMcpHandler_DatasetsLookupErrorNeutralized(t *testing.T) {
	router := setupKnowledgeMcpRouter(
		&fakeKnowledgeMcpService{},
		&fakeAgentMcpService{err: errors.New(`Error 1146: Table 'hub.agent_knowledge_datasets' doesn't exist; SELECT * FROM agents WHERE tenant_id='default'`)},
	)
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_datasets","arguments":{}}`), testValidToken)

	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %+v", resp)
	}
	if !strings.Contains(resp.Error.Message, "获取 Agent 知识库绑定关系失败") {
		t.Fatalf("expected neutral error message, got %q", resp.Error.Message)
	}
	if strings.Contains(resp.Error.Message, "SELECT") || strings.Contains(resp.Error.Message, "tenant_id") {
		t.Fatalf("internal error detail leaked: %q", resp.Error.Message)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Datasets_DegradeOnErrorOrNil(t *testing.T) {
	// 错误分支与 (nil,nil) 契约违反分支都必须降级为仅 id，不得 panic、不得泄漏上游细节。
	knowledgeSvc := &fakeKnowledgeMcpService{
		getDatasetFunc: func(ctx context.Context, id string) (*knowledge.Dataset, error) {
			if id == "bad" {
				return nil, errors.New("upstream boom " + upstreamLeakMarker)
			}
			return nil, nil
		},
	}
	router := setupKnowledgeMcpRouter(knowledgeSvc, &fakeAgentMcpService{datasets: []string{"bad", "nil-contract"}})
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_datasets","arguments":{}}`), testValidToken)

	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if isErrorResult(resp.Result) {
		t.Fatalf("expected degrade-not-fail, got %v", resp.Result)
	}
	var payload struct {
		Datasets []map[string]any `json:"datasets"`
	}
	if err := json.Unmarshal([]byte(resultText(resp.Result)), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.Datasets) != 2 {
		t.Fatalf("datasets len = %d, want 2", len(payload.Datasets))
	}
	for _, ds := range payload.Datasets {
		if len(ds) != 1 || ds["id"] == "" {
			t.Fatalf("expected id-only degrade entries, got %v", payload.Datasets)
		}
	}
	if strings.Contains(resultText(resp.Result), "10.0.0.1") {
		t.Fatalf("degrade path leaked upstream detail: %s", resultText(resp.Result))
	}
}

func TestKnowledgeMcpHandler_ToolsCall_MissingTenantContext(t *testing.T) {
	// 理论不可达（middleware 命中 agents 行必写 tenant），防御性验证：
	// 无 tenant 时必须返回明确错误，而非用空串静默查询全表。
	gin.SetMode(gin.TestMode)
	var gotTenantID string
	agentSvc := &tenantAwareAgentMcpService{fn: func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
		gotTenantID = tenantID
		return []string{"allowed-dataset"}, "test-agent", nil
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
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "租户") {
		t.Fatalf("expected explicit tenant-missing error, got %+v", resp)
	}
}

func TestKnowledgeMcpHandler_ToolsCall_TenantScopedDatasets(t *testing.T) {
	// 验证 handler 把 context 里的 tenant_id、token agent 名、capability
	// 头与 Bearer token 原文传给 dataset 反查；无 capability 时回退 token
	// agent 自身绑定（capabilityHeader 为空串）。
	var gotTenantID, gotTokenAgent, gotCapability, gotBearer string
	agentSvc := &tenantAwareAgentMcpService{fn: func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
		gotTenantID, gotTokenAgent, gotCapability, gotBearer = tenantID, tokenAgentName, capabilityHeader, bearerToken
		return []string{"tenant-a-dataset"}, tokenAgentName, nil
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
	if gotTokenAgent != "test-agent" {
		t.Fatalf("tokenAgentName = %q, want test-agent", gotTokenAgent)
	}
	if gotCapability != "" {
		t.Fatalf("capabilityHeader = %q, want empty (legacy fallback without the header)", gotCapability)
	}
	if gotBearer != testValidToken {
		t.Fatalf("bearerToken = %q, want the raw presented token %q", gotBearer, testValidToken)
	}
}

type tenantAwareAgentMcpService struct {
	fn func(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error)
}

func (f *tenantAwareAgentMcpService) GetAgentKnowledgeDatasetsForRequest(tenantID, tokenAgentName, capabilityHeader, bearerToken string) ([]string, string, error) {
	return f.fn(tenantID, tokenAgentName, capabilityHeader, bearerToken)
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
	// ≤0 视为未传 → 回落默认值
	postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_documents","arguments":{"dataset_id":"ds-1","page":0,"page_size":-5}}`), testValidToken)
	if gotReq.Page != 1 || gotReq.PageSize != 20 {
		t.Fatalf("non-positive paging should fall back to defaults, got %+v", gotReq)
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
	if resp2.Error == nil || !strings.Contains(resp2.Error.Message, "dataset_id 不能为空") {
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
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "document_id 不能为空") {
		t.Fatalf("expected document_id required error, got %+v", resp)
	}
	// 缺 dataset_id → JSON-RPC error
	rec2 := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_chunks","arguments":{"document_id":"doc-1"}}`), testValidToken)
	var resp2 jsonRPCResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp2.Error == nil || !strings.Contains(resp2.Error.Message, "dataset_id 不能为空") {
		t.Fatalf("expected dataset_id required error, got %+v", resp2)
	}
}

func TestKnowledgeMcpHandler_ToolsListMatchesBuiltinSeed(t *testing.T) {
	// 种子 ToolsJSON（管理界面展示）与 tools/list 广播（runtime 消费）是两份手写副本，
	// 工具名集合必须一致，防止单边漂移（改了一边忘了另一边）。
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/list", nil, testValidToken)

	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("result is not an object: %v", resp.Result)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatalf("tools missing: %v", result["tools"])
	}
	advertised := map[string]bool{}
	for _, tl := range tools {
		if m, ok := tl.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				advertised[name] = true
			}
		}
	}
	seedNames := services.BuiltinKnowledgeToolNames()
	if len(advertised) != len(seedNames) {
		t.Fatalf("tools/list has %d tools but seed has %d — 两份副本已漂移: advertised=%v seed=%v",
			len(advertised), len(seedNames), advertised, seedNames)
	}
	for _, name := range seedNames {
		if !advertised[name] {
			t.Fatalf("builtin seed tool %q not advertised in tools/list — 两份副本已漂移", name)
		}
	}
}

func TestKnowledgeMcpHandler_ToolsCall_Chunks_AuthzBeforeParamValidation(t *testing.T) {
	// requireDatasetAccess 统一入口的优先级契约：授权先于参数细节校验——
	// 未授权 dataset + 缺 document_id 时返回「无权访问」，不暴露参数校验细节。
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{datasets: []string{"ds-1"}})
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"knowledge_chunks","arguments":{"dataset_id":"other"}}`), testValidToken)
	var resp jsonRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !isErrorResult(resp.Result) || !strings.Contains(resultText(resp.Result), "无权访问") {
		t.Fatalf("expected unauthorized isError before param validation, got %+v", resp)
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

// issue #119：MCP 工具错误文本带稳定机器可识别码前缀 `[<code>] <中性文案>`，
// 调用方（runtime/子 Agent）按码程序化分支，不解析文案。
func TestKnowledgeMcpHandler_ToolsCall_ErrorCodesStable(t *testing.T) {
	cases := []struct {
		name       string
		svc        *fakeKnowledgeMcpService
		agent      *fakeAgentMcpService
		params     string
		wantPrefix string
	}{
		{
			name:       "no dataset bound",
			svc:        &fakeKnowledgeMcpService{},
			agent:      &fakeAgentMcpService{},
			params:     `{"name":"knowledge_search","arguments":{"query":"q"}}`,
			wantPrefix: "[no_dataset_bound]",
		},
		{
			name:       "unauthorized dataset",
			svc:        &fakeKnowledgeMcpService{},
			agent:      &fakeAgentMcpService{datasets: []string{"kb1"}},
			params:     `{"name":"knowledge_search","arguments":{"query":"q","dataset_ids":["kb-other"]}}`,
			wantPrefix: "[dataset_not_authorized]",
		},
		{
			name: "retrieval upstream failure",
			svc: &fakeKnowledgeMcpService{retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
				return nil, errors.New("multirag error 100: boom")
			}},
			agent:      &fakeAgentMcpService{datasets: []string{"kb1"}},
			params:     `{"name":"knowledge_search","arguments":{"query":"q"}}`,
			wantPrefix: "[retrieval_failed]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := setupKnowledgeMcpRouter(tc.svc, tc.agent)
			rec := postJSONRPC(t, router, "tools/call", json.RawMessage(tc.params), testValidToken)
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
			if text := resultText(resp.Result); !strings.HasPrefix(text, tc.wantPrefix) {
				t.Fatalf("text = %q, want prefix %q", text, tc.wantPrefix)
			}
		})
	}
}

// issue #119：检索来源至少含 document_name + document_id；缺 id 时省略该段，
// 缺名回退「未知文档」。
func TestFormatRetrievalResult_SourceAttribution(t *testing.T) {
	result := knowledge.RetrievalResult(map[string]any{
		"chunks": []interface{}{
			map[string]any{"document_name": "PTNB指南.pdf", "document_id": "doc-1", "similarity": 0.9123, "content": "内容一"},
			map[string]any{"document_id": "doc-2", "similarity": 0.5, "content": "内容二"},
			map[string]any{"document_name": "无ID文档.pdf", "similarity": 0.4, "content": "内容三"},
		},
	})
	text := formatRetrievalResult(&result)
	for _, want := range []string{
		"[来源：PTNB指南.pdf | 文档ID：doc-1 | 相似度：0.912]",
		"[来源：未知文档 | 文档ID：doc-2 | 相似度：0.500]",
		"[来源：无ID文档.pdf | 相似度：0.400]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text %q missing %q", text, want)
		}
	}
}

// issue #119 验收项：不传 dataset_ids 时，请求补全为当前身份的全部绑定
// 数据集（顺序保持 service 返回序）。
func TestKnowledgeMcpHandler_ToolsCall_SearchDefaultCompletionUsesAllBoundDatasets(t *testing.T) {
	var gotDatasetIDs any
	svc := &fakeKnowledgeMcpService{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			gotDatasetIDs = map[string]any(req)["dataset_ids"]
			return &knowledge.RetrievalResult{"chunks": []any{}}, nil
		},
	}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-1", "kb-2"}})
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"hello"}}`)
	rec := postJSONRPC(t, router, "tools/call", params, testValidToken)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	ids, ok := gotDatasetIDs.([]string)
	if !ok || len(ids) != 2 || ids[0] != "kb-1" || ids[1] != "kb-2" {
		t.Fatalf("dataset_ids = %#v, want [kb-1 kb-2]", gotDatasetIDs)
	}
}

// issue #119 失败路径诊断：检索失败后在后台逐个探测本次请求的 dataset 并
// 记录存活状态（结果只进服务端日志），客户端仍收 [retrieval_failed] 中性
// 文案。探测经无缓冲 channel 交接：若实现退化为同步（阻塞响应），此处与
// postJSONRPC 互等死锁、由 go test 超时暴露；异步实现则响应先返回。
func TestKnowledgeMcpHandler_ToolsCall_SearchFailureProbesBoundDatasets(t *testing.T) {
	probeStarted := make(chan string)
	svc := &fakeKnowledgeMcpService{
		retrievalFunc: func(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
			return nil, errors.New("multirag error 102: combined retrieval boom")
		},
		getDatasetFunc: func(ctx context.Context, id string) (*knowledge.Dataset, error) {
			probeStarted <- id
			if id == "kb-zombie" {
				return nil, knowledge.NewNotFoundError("dataset 不存在")
			}
			return &knowledge.Dataset{"id": id}, nil
		},
	}
	router := setupKnowledgeMcpRouter(svc, &fakeAgentMcpService{datasets: []string{"kb-live", "kb-zombie"}})
	params := json.RawMessage(`{"name":"knowledge_search","arguments":{"query":"PTNB 术前 血小板"}}`)
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
	if text := resultText(resp.Result); !strings.HasPrefix(text, "[retrieval_failed]") {
		t.Fatalf("text = %q, want [retrieval_failed] prefix", text)
	}
	var got []string
	for i := 0; i < 2; i++ {
		select {
		case id := <-probeStarted:
			got = append(got, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("background probe %d/2 not fired (got %v) — probe must run off the request path", i+1, got)
		}
	}
	if got[0] != "kb-live" || got[1] != "kb-zombie" {
		t.Fatalf("probed = %v, want [kb-live kb-zombie]", got)
	}
}
