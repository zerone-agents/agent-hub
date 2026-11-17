package handler

// mcp_invalid_params_test.go：issue #256——JSON-RPC 参数解析失败应返回
// -32602（invalid params）而非 -32603（internal error）。knowledge_mcp 与
// memory_mcp 两端点必须同改（行为不分叉）；「工具不存在」等非参数类
// 错误保持 -32603（不在本 issue 范围）。

import (
	"encoding/json"
	"testing"

	"control-panel/internal/application/services"

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
