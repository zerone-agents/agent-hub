package handler

// mcp_invalid_params_test.go：issue #256——JSON-RPC 参数解析失败应返回
// -32602（invalid params）而非 -32603（internal error）。knowledge_mcp 与
// memory_mcp 两端点必须同改（行为不分叉）；「工具不存在」等非参数类
// 错误保持 -32603（不在本 issue 范围）。

import (
	"encoding/json"
	"testing"

	"control-panel/internal/application/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeMcpInvalidParamsReturns32602(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	// params 非对象形状（数组）→ json.Unmarshal(params, &toolCallParams{}) 失败
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`[]`), testValidToken)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error, "参数解析失败走 JSON-RPC Error 对象")
	require.Equal(t, -32602, resp.Error.Code, "invalid params（issue #256）")
	require.Contains(t, resp.Error.Message, "参数解析失败")
}

func TestKnowledgeMcpUnknownToolStays32603(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`{"name":"no_such_tool","arguments":{}}`), testValidToken)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, -32603, resp.Error.Code, "非参数类错误保持 internal error（本 issue 不改语义）")
}

func TestMemoryMcpInvalidParamsReturns32602(t *testing.T) {
	router := setupMemoryMcpRouter(&fakeAgentMemoryStore{}, services.NewMemoryCapabilityService("unit-secret"), nil)
	rec := memoryMcpPost(t, router, "tools/call", json.RawMessage(`[]`), nil)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error, "参数解析失败走 JSON-RPC Error 对象")
	require.Equal(t, -32602, resp.Error.Code, "invalid params（issue #256）")
	require.Contains(t, resp.Error.Message, "参数解析失败")
}

func TestMemoryMcpUnknownToolStays32603(t *testing.T) {
	router := setupMemoryMcpRouter(&fakeAgentMemoryStore{}, services.NewMemoryCapabilityService("unit-secret"), nil)
	rec := memoryMcpPost(t, router, "tools/call", json.RawMessage(`{"name":"no_such_tool","arguments":{}}`), nil)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.Equal(t, -32603, resp.Error.Code, "非参数类错误保持 internal error")
}

// TestKnowledgeMcpToolArgumentsInvalidParams：工具级 arguments 坏 → -32602
// （评审发现 1：四处 bare 位点——documents/chunks/search 外层 map/search
// typed，此前同文案「参数解析失败」却落 -32603，端点内部分叉）。
func TestKnowledgeMcpToolArgumentsInvalidParams(t *testing.T) {
	// typed 解析用例需带 dataset 绑定（search 的授权解析在 typed 解析之前，
	// 空绑定会先落 no_dataset_bound 的 isError result 而非本测路径）
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	bound := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{datasets: []string{"kb-1"}})
	cases := []struct {
		name   string
		router *gin.Engine
		body   string
	}{
		{"knowledge_documents arguments 非对象", router, `{"name":"knowledge_documents","arguments":[]}`},
		{"knowledge_chunks arguments 非对象", router, `{"name":"knowledge_chunks","arguments":[]}`},
		{"knowledge_search arguments 非对象（外层 map 解析）", router, `{"name":"knowledge_search","arguments":[]}`},
		{"knowledge_search query 类型错（typed 解析）", bound, `{"name":"knowledge_search","arguments":{"query":123}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSONRPC(t, tc.router, "tools/call", json.RawMessage(tc.body), testValidToken)
			var resp jsonRPCResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.NotNil(t, resp.Error, "参数解析失败走 JSON-RPC Error 对象")
			require.Equal(t, -32602, resp.Error.Code)
			require.Contains(t, resp.Error.Message, "参数解析失败")
		})
	}
}

// TestMcpInvalidParamsMessageFormat：wire message 保持纯中文旧格式
// （评审发现 2：sentinel 文本曾是英文 "mcp invalid params"，以 %w 前缀
// 泄入 message，违反 CONTRIBUTING「User-facing errors in Chinese」）。
func TestMcpInvalidParamsMessageFormat(t *testing.T) {
	router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
	rec := postJSONRPC(t, router, "tools/call", json.RawMessage(`[]`), testValidToken)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Error)
	require.NotContains(t, resp.Error.Message, "mcp invalid params", "英文 sentinel 文本不得泄入 wire message")
	require.Contains(t, resp.Error.Message, "参数解析失败")
}

// TestMcpMissingToolName32602（#256 follow-up 备注）：tools/call 的 params
// name 缺失**或空白**（MCP 契约必需字段）→ -32602 invalid params；非字符串
// / null name 经 unmarshal 失败同落 -32602。与「name 非空但未知工具」的
// -32603 语义区分（既有 unknown-tool 测试钉住）。空白断言钉住 TrimSpace
// 语义——防未来「简化」为 p.Name == "" 静默回退（评审发现 1）。
// 两端点同改，行为不分叉。
func TestMcpMissingToolName32602(t *testing.T) {
	bodies := []string{`{}`, `{"name":"   "}`, `{"name":123}`, `{"name":null}`}
	run := func(t *testing.T, post func(t *testing.T, body string) jsonRPCResponse) {
		for _, body := range bodies {
			resp := post(t, body)
			require.NotNil(t, resp.Error, "缺/坏 name 走 JSON-RPC Error 对象，body=%s", body)
			require.Equal(t, -32602, resp.Error.Code, "缺/坏 name → invalid params，body=%s", body)
			require.Contains(t, resp.Error.Message, "参数解析失败", body)
		}
		// 空白 name 的专门断言（unmarshal 失败路径外的 TrimSpace 路径）
		resp := post(t, `{"name":"   "}`)
		require.Contains(t, resp.Error.Message, "name 不能为空", "空白 name 走显式守卫而非 unmarshal 失败")
	}
	t.Run("knowledge_mcp", func(t *testing.T) {
		router := setupKnowledgeMcpRouter(&fakeKnowledgeMcpService{}, &fakeAgentMcpService{})
		run(t, func(t *testing.T, body string) jsonRPCResponse {
			rec := postJSONRPC(t, router, "tools/call", json.RawMessage(body), testValidToken)
			var resp jsonRPCResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			return resp
		})
	})
	t.Run("memory_mcp", func(t *testing.T) {
		router := setupMemoryMcpRouter(&fakeAgentMemoryStore{}, services.NewMemoryCapabilityService("unit-secret"), nil)
		run(t, func(t *testing.T, body string) jsonRPCResponse {
			rec := memoryMcpPost(t, router, "tools/call", json.RawMessage(body), nil)
			var resp jsonRPCResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			return resp
		})
	})
}
