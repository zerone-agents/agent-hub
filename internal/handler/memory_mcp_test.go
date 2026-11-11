package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/memory"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ── 测试替身 ──────────────────────────────────────────────────────

type fakeAgentMemoryStore struct {
	ensureCalls int
	lastScope   memory.Scope
	lastInput   memory.AgentMemoryInput
	lastUpdate  struct {
		id string
		v  uint64
	}
	rememberResult memory.AgentMemoryRecord
	rememberErr    error
	updateErr      error
	recallResult   []memory.AgentMemoryRecord
	listResult     []memory.AgentMemoryRecord
}

func (f *fakeAgentMemoryStore) EnsureAgentLifecycle(ctx context.Context, sc memory.Scope) error {
	f.ensureCalls++
	f.lastScope = sc
	return nil
}

func (f *fakeAgentMemoryStore) RememberAgentFact(ctx context.Context, sc memory.Scope, in memory.AgentMemoryInput) (memory.AgentMemoryRecord, error) {
	f.lastScope = sc
	f.lastInput = in
	return f.rememberResult, f.rememberErr
}

func (f *fakeAgentMemoryStore) RecallAgentMemories(ctx context.Context, sc memory.Scope, query string, limit int) ([]memory.AgentMemoryRecord, error) {
	f.lastScope = sc
	return f.recallResult, nil
}

func (f *fakeAgentMemoryStore) ListAgentMemories(ctx context.Context, sc memory.Scope, category, memoryID string, limit int) ([]memory.AgentMemoryRecord, error) {
	f.lastScope = sc
	return f.listResult, nil
}

func (f *fakeAgentMemoryStore) UpdateAgentFact(ctx context.Context, sc memory.Scope, id string, expectedVersion uint64, in memory.AgentMemoryInput) (memory.AgentMemoryRecord, error) {
	f.lastScope = sc
	f.lastInput = in
	f.lastUpdate.id, f.lastUpdate.v = id, expectedVersion
	return memory.AgentMemoryRecord{ID: id, Version: expectedVersion + 1}, f.updateErr
}

type staticAgentMemoryResolver struct{ store AgentMemoryStore }

func (r staticAgentMemoryResolver) AgentMemoryStore(string) (AgentMemoryStore, error) {
	return r.store, nil
}

// memoryMcpTokenFP 与 services.tokenFingerprint 同式（sha256hex[:16]）：
// 测试钉住公式，两处漂移会在此红。
func memoryMcpTokenFP(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:16]
}

var memoryMcpAllTools = []string{"memory_remember", "memory_recall", "memory_list", "memory_update"}

func memoryMcpCapability(t *testing.T, caps *services.MemoryCapabilityService, agentName, scopeMode string) string {
	t.Helper()
	raw, err := caps.Sign(services.MemoryCapabilityPayload{
		TenantID: tenant.DefaultID, AgentName: agentName, ScopeMode: scopeMode,
		Tools: memoryMcpAllTools, TokenFingerprint: memoryMcpTokenFP(testValidToken),
	})
	require.NoError(t, err)
	return raw
}

func setupMemoryMcpRouter(store AgentMemoryStore, caps *services.MemoryCapabilityService, recorder *services.AuditRecorder) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewMemoryMcpHandler(staticAgentMemoryResolver{store}, caps, recorder)
	r := gin.New()
	r.POST("/api/v1/memory/mcp", testAgentAuthMiddleware(testValidToken), h.HandleMessage)
	return r
}

func memoryMcpPost(t *testing.T, router *gin.Engine, method string, params json.RawMessage, headers map[string][]string) *httptest.ResponseRecorder {
	t.Helper()
	body := jsonRPCRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/memory/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testValidToken)
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec
}

func memoryMcpCall(t *testing.T, router *gin.Engine, tool, args, capability string) jsonRPCResponse {
	t.Helper()
	params := json.RawMessage(fmt.Sprintf(`{"name":%q,"arguments":%s}`, tool, args))
	headers := map[string][]string{}
	if capability != "" {
		headers["X-Agent-Capability"] = []string{capability}
	}
	rec := memoryMcpPost(t, router, "tools/call", params, headers)
	var resp jsonRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func memoryMcpResultText(t *testing.T, resp jsonRPCResponse) string {
	t.Helper()
	raw, err := json.Marshal(resp.Result)
	require.NoError(t, err)
	var parsed struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.NotEmpty(t, parsed.Content)
	return parsed.Content[0].Text
}

// ── 测试 ──────────────────────────────────────────────────────────

func TestMemoryMcpInitializeAndToolsList(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	router := setupMemoryMcpRouter(&fakeAgentMemoryStore{}, caps, nil)

	rec := memoryMcpPost(t, router, "initialize", nil, nil)
	var initResp struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
	require.Equal(t, "memory-mcp", initResp.Result.ServerInfo.Name)

	rec = memoryMcpPost(t, router, "tools/list", nil, nil)
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	names := make([]string, 0, len(listResp.Result.Tools))
	for _, tl := range listResp.Result.Tools {
		names = append(names, tl.Name)
	}
	require.ElementsMatch(t, memoryMcpAllTools, names)
}

func TestMemoryMcpRememberPinsScopeIgnoringIdentityArgs(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	store := &fakeAgentMemoryStore{rememberResult: memory.AgentMemoryRecord{ID: "m-1", Version: 1}}
	router := setupMemoryMcpRouter(store, caps, nil)
	capability := memoryMcpCapability(t, caps, "test-agent", "agent")

	// 参数夹带 userId/tenantId/applicationId → 一律忽略（spec §5 验收反例 2）
	resp := memoryMcpCall(t, router, "memory_remember",
		`{"subject":"用户","category":"情绪","content":"对工作变动焦虑","quote":"想到换工作就睡不着","userId":"victim","tenantId":"other","applicationId":"evil"}`,
		capability)
	require.False(t, isErrorResult(resp.Result), memoryMcpResultText(t, resp))

	require.Equal(t, memory.Scope{
		TenantID:      tenant.DefaultID,
		ApplicationID: "hub-agent",
		UserID:        "agent:test-agent",
		LifecycleID:   "agent-memory-v1",
	}, store.lastScope, "Scope 由 capability 服务端钉死")
	require.Equal(t, "对工作变动焦虑", store.lastInput.Content)
	require.Equal(t, 1, store.ensureCalls, "首次调用完成 lifecycle 幂等开通")
	require.Contains(t, memoryMcpResultText(t, resp), "m-1")
}

func TestMemoryMcpCapabilityFailClosed(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	store := &fakeAgentMemoryStore{}
	router := setupMemoryMcpRouter(store, caps, nil)
	valid := memoryMcpCapability(t, caps, "test-agent", "agent")
	wrongAgent := memoryMcpCapability(t, caps, "other-agent", "agent")
	userMode := memoryMcpCapability(t, caps, "test-agent", "user")

	cases := []struct {
		name       string
		capability string
		headers    map[string][]string
	}{
		{name: "缺失", capability: ""},
		{name: "篡改", capability: "v1.tampered.sig"},
		{name: "错误 agent 绑定", capability: wrongAgent},
		{name: "user 模式断言", capability: userMode},
		{name: "重复头", capability: valid, headers: map[string][]string{"X-Agent-Capability": {valid, valid}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := tc.headers
			if headers == nil {
				headers = map[string][]string{}
				if tc.capability != "" {
					headers["X-Agent-Capability"] = []string{tc.capability}
				}
			}
			params := json.RawMessage(`{"name":"memory_recall","arguments":{"query":"x"}}`)
			rec := memoryMcpPost(t, router, "tools/call", params, headers)
			var resp jsonRPCResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.True(t, isErrorResult(resp.Result))
			require.Contains(t, memoryMcpResultText(t, resp), "memory_not_authorized")
		})
	}
	require.Zero(t, store.ensureCalls, "capability 失败不得触达存储（fail closed）")
}

func TestMemoryMcpStoreErrorMapping(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	capability := memoryMcpCapability(t, caps, "test-agent", "agent")

	poolFull := &fakeAgentMemoryStore{rememberErr: memory.ErrPoolFull}
	resp := memoryMcpCall(t, setupMemoryMcpRouter(poolFull, caps, nil), "memory_remember",
		`{"subject":"s","category":"c","content":"x","quote":"q"}`, capability)
	require.True(t, isErrorResult(resp.Result))
	require.Contains(t, memoryMcpResultText(t, resp), "memory_pool_full")
	require.Contains(t, memoryMcpResultText(t, resp), "容量已满", "池满须可诊断（spec §7 F1）")

	forbidden := &fakeAgentMemoryStore{updateErr: memory.ErrForbidden}
	resp = memoryMcpCall(t, setupMemoryMcpRouter(forbidden, caps, nil), "memory_update",
		`{"memoryId":"m-1","expectedVersion":1,"content":"新","quote":"新quote"}`, capability)
	require.True(t, isErrorResult(resp.Result))
	require.Contains(t, memoryMcpResultText(t, resp), "无权修改该记忆")

	conflict := &fakeAgentMemoryStore{updateErr: memory.ErrConflict}
	resp = memoryMcpCall(t, setupMemoryMcpRouter(conflict, caps, nil), "memory_update",
		`{"memoryId":"m-1","expectedVersion":1,"content":"新","quote":"新quote"}`, capability)
	require.True(t, isErrorResult(resp.Result))
	require.Contains(t, memoryMcpResultText(t, resp), "版本冲突")
}

func TestMemoryMcpAuditRecordsWithoutContent(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	store := &fakeAgentMemoryStore{rememberResult: memory.AgentMemoryRecord{ID: "m-audit", Version: 1}}
	auditStore := &memoryAdminAuditStore{}
	router := setupMemoryMcpRouter(store, caps, services.NewAuditRecorder(auditStore))
	capability := memoryMcpCapability(t, caps, "test-agent", "agent")

	resp := memoryMcpCall(t, router, "memory_remember",
		`{"subject":"用户","category":"情绪","content":"PRIVATE_CONTENT_X","quote":"PRIVATE_QUOTE_Y"}`, capability)
	require.False(t, isErrorResult(resp.Result))

	require.NotEmpty(t, auditStore.rows)
	require.Equal(t, "memory.mcp.remember", string(auditStore.rows[0].Action))
	require.Equal(t, "memory", string(auditStore.rows[0].TargetType))
	raw, err := json.Marshal(auditStore.rows[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "PRIVATE_CONTENT_X", "审计不含正文（spec §7）")
	require.NotContains(t, string(raw), "PRIVATE_QUOTE_Y", "审计不含摘录（spec §7）")
}

func TestMemoryMcpRecallAndListFlow(t *testing.T) {
	caps := services.NewMemoryCapabilityService("unit-secret")
	store := &fakeAgentMemoryStore{recallResult: []memory.AgentMemoryRecord{{ID: "r-1", Content: "喜欢无糖乌龙茶"}}}
	router := setupMemoryMcpRouter(store, caps, nil)
	capability := memoryMcpCapability(t, caps, "test-agent", "agent")

	resp := memoryMcpCall(t, router, "memory_recall", `{"query":"睡眠","limit":3}`, capability)
	require.False(t, isErrorResult(resp.Result))
	require.Contains(t, memoryMcpResultText(t, resp), "r-1")

	resp = memoryMcpCall(t, router, "memory_list", `{"category":"情绪"}`, capability)
	require.False(t, isErrorResult(resp.Result))

	// 参数缺 content/quote → ErrInvalid 映射（服务层校验在真实现，fake 层仅验
	// 参数透传形状：空对象合法 schema，不在此测校验）
	resp = memoryMcpCall(t, router, "memory_update", `{"memoryId":"r-1","expectedVersion":2,"content":"新","quote":"新q"}`, capability)
	require.False(t, isErrorResult(resp.Result))
	require.Equal(t, "r-1", store.lastUpdate.id)
	require.Equal(t, uint64(2), store.lastUpdate.v)
}
