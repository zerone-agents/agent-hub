package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type fakeOrganizationMessageService struct {
	relations    []*services.AgentRelationDTO
	sent         *services.AgentMessageDTO
	got          *services.AgentMessageDTO
	inbox        []*services.AgentMessageDTO
	chain        *services.AgentMessageChainDTO
	source       *agent.AgentConfig
	tenantID     string
	input        services.SendAgentMessageInput
	signal       *services.RecordAgentRelationEventInput
	signalResult *services.AgentRelationEventResultDTO
}

func (f *fakeOrganizationMessageService) SignalRelation(tenantID string, source *agent.AgentConfig, _ string, _ string, input *services.RecordAgentRelationEventInput) (*services.AgentRelationEventResultDTO, error) {
	f.tenantID, f.source, f.signal = tenantID, source, input
	return f.signalResult, nil
}

func (f *fakeOrganizationMessageService) Relations(tenantID string, source *agent.AgentConfig) ([]*services.AgentRelationDTO, error) {
	f.tenantID, f.source = tenantID, source
	return f.relations, nil
}

func (f *fakeOrganizationMessageService) Send(_ context.Context, tenantID string, source *agent.AgentConfig, input services.SendAgentMessageInput) (*services.AgentMessageDTO, error) {
	f.tenantID, f.source, f.input = tenantID, source, input
	return f.sent, nil
}

func (f *fakeOrganizationMessageService) Get(tenantID string, source *agent.AgentConfig, _ string) (*services.AgentMessageDTO, error) {
	f.tenantID, f.source = tenantID, source
	return f.got, nil
}

func (f *fakeOrganizationMessageService) Inbox(tenantID string, source *agent.AgentConfig, _ int) ([]*services.AgentMessageDTO, error) {
	f.tenantID, f.source = tenantID, source
	return f.inbox, nil
}

func (f *fakeOrganizationMessageService) MessageChainForAgent(tenantID string, source *agent.AgentConfig, _, _ string, _ int) (*services.AgentMessageChainDTO, error) {
	f.tenantID, f.source = tenantID, source
	return f.chain, nil
}

func setupOrganizationMcpRouter(service OrganizationMessageService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewOrganizationMcpHandler(service)
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer agent-a-token" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("agent", &agent.AgentConfig{ID: 41, Name: "agent-a", TenantID: "tenant-a"})
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	}, h.HandleMessage)
	return router
}

func postOrganizationRPC(t *testing.T, router http.Handler, method string, params json.RawMessage, token string) *httptest.ResponseRecorder {
	return postOrganizationRPCWithSession(t, router, method, params, token, "")
}

func postOrganizationRPCWithSession(t *testing.T, router http.Handler, method string, params json.RawMessage, token, runtimeSessionID string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(jsonRPCRequest{JSONRPC: "2.0", ID: 7, Method: method, Params: params})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organization/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if runtimeSessionID != "" {
		req.Header.Set("X-Zerone-Runtime-Session-ID", runtimeSessionID)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeMcpErrorText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.True(t, response.Result.IsError)
	require.Len(t, response.Result.Content, 1)
	return response.Result.Content[0].Text
}

func decodeMcpTextResult(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.False(t, response.Result.IsError)
	require.Len(t, response.Result.Content, 1)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &payload))
	return payload
}

func TestOrganizationMcpListsRuntimeTools(t *testing.T) {
	router := setupOrganizationMcpRouter(&fakeOrganizationMessageService{})
	rec := postOrganizationRPC(t, router, "tools/list", nil, "agent-a-token")
	require.Equal(t, http.StatusOK, rec.Code)

	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	names := make([]string, 0, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		names = append(names, tool.Name)
	}
	require.Equal(t, []string{"agent_relations", "agent_send", "agent_message_status", "agent_inbox", "agent_message_chain", "group_send", "channel_publish", "group_message_status", "session_start", "session_end", "workflow_start", "approval_vote", "decision_vote", "workflow_step_complete", "workflow_step_fail", "emotion_status", "belief_list", "belief_claim", "memory_record", "memory_recall", "relation_view"}, names)
}

func TestOrganizationMcpMessageChainUsesRuntimeIdentity(t *testing.T) {
	service := &fakeOrganizationMessageService{chain: &services.AgentMessageChainDTO{
		ConversationID: "conv-1", RootMessageID: "root-1", MessageCount: 1, Verified: true,
		Messages: []*services.AgentMessageDTO{{ID: "m1", SourceAgentID: 41, TargetAgentID: 42, Verified: true}},
	}}
	router := setupOrganizationMcpRouter(service)
	rec := postOrganizationRPC(t, router, "tools/call", json.RawMessage(`{"name":"agent_message_chain","arguments":{"conversation_id":"conv-1"}}`), "agent-a-token")
	payload := decodeMcpTextResult(t, rec)
	require.Equal(t, "conv-1", payload["conversationId"])
	require.Equal(t, "tenant-a", service.tenantID)
	require.Equal(t, uint64(41), service.source.ID)
}

func TestOrganizationMcpDoesNotExposeLegacyRelationshipMutation(t *testing.T) {
	router := setupOrganizationMcpRouter(&fakeOrganizationMessageService{})
	params := json.RawMessage(`{"name":"agent_relation_signal","arguments":{"target_agent":"agent-b","event_type":"promise_broken","reason":"承诺后没有交付","idempotency_key":"task-7-promise"}}`)
	rec := postOrganizationRPC(t, router, "tools/call", params, "agent-a-token")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "工具不存在")
}

func TestOrganizationMcpDerivesSourceFromRuntimeToken(t *testing.T) {
	service := &fakeOrganizationMessageService{sent: &services.AgentMessageDTO{
		ID: "message-1", SourceAgent: "agent-a", TargetAgent: "agent-b", Status: "completed", Reply: "B 回复",
	}}
	router := setupOrganizationMcpRouter(service)
	params := json.RawMessage(`{
		"name":"agent_send",
		"arguments":{
			"source_agent":"spoofed-agent",
			"target_agent":"agent-b",
			"scope":"speeding-hq",
			"action":"review",
			"message":"复核收购",
			"context_summary":"估值摘要"
		}
	}`)
	rec := postOrganizationRPC(t, router, "tools/call", params, "agent-a-token")
	require.Equal(t, http.StatusOK, rec.Code)
	payload := decodeMcpTextResult(t, rec)
	require.Equal(t, "message-1", payload["id"])
	require.Equal(t, "B 回复", payload["reply"])

	require.Equal(t, "tenant-a", service.tenantID)
	require.NotNil(t, service.source)
	require.Equal(t, uint64(41), service.source.ID)
	require.Equal(t, "agent-a", service.source.Name, "tool arguments must not be able to spoof the sender")
	require.Equal(t, "agent-b", service.input.TargetAgent)
	require.Equal(t, "review", service.input.Action)
}

func TestOrganizationMcpContinuationDefaultsCallbackForWorkActions(t *testing.T) {
	for _, action := range []string{"consult", "assign", "review", "challenge", "handoff"} {
		t.Run(action, func(t *testing.T) {
			service := &fakeOrganizationMessageService{sent: &services.AgentMessageDTO{ID: "child-1", Status: "queued"}}
			h := NewOrganizationMcpHandler(service)
			// The handler only needs a configured wait capability here; the real
			// service owns durable registration so it can happen before execution.
			h.SetMessageWaitService(services.NewAgentMessageWaitService())
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
				c.Set("agent", &agent.AgentConfig{ID: 41, Name: "agent-a", TenantID: "tenant-a"})
				tenant.SetTenantID(c, "tenant-a")
			}, h.HandleMessage)
			params := json.RawMessage(`{"name":"agent_send","arguments":{"target_agent":"agent-c","action":"` + action + `","message":"continue","run_id":"run-1","parent_message_id":"parent-1"}}`)
			rec := postOrganizationRPCWithSession(t, router, "tools/call", params, "", "runtime-session-1")
			decodeMcpTextResult(t, rec)
			require.Equal(t, "runtime-session-1", service.input.CallbackRuntimeSessionID)
		})
	}
}

func TestOrganizationMcpCallbackRejectsMissingTrustedSessionBeforeSend(t *testing.T) {
	service := &fakeOrganizationMessageService{sent: &services.AgentMessageDTO{ID: "must-not-send"}}
	h := NewOrganizationMcpHandler(service)
	h.SetMessageWaitService(services.NewAgentMessageWaitService())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		c.Set("agent", &agent.AgentConfig{ID: 41, Name: "agent-a", TenantID: "tenant-a"})
		tenant.SetTenantID(c, "tenant-a")
	}, h.HandleMessage)
	params := json.RawMessage(`{"name":"agent_send","arguments":{"target_agent":"agent-c","action":"consult","message":"continue","run_id":"run-1","parent_message_id":"parent-1"}}`)
	rec := postOrganizationRPC(t, router, "tools/call", params, "")
	require.Contains(t, decodeMcpErrorText(t, rec), "可信会话标识")
	require.Empty(t, service.input.TargetAgent, "validation must reject before dispatch")
}

func TestOrganizationMcpExplicitNoneAndNotificationsDoNotRequireSession(t *testing.T) {
	tests := []struct {
		name, action, waitMode string
	}{
		{name: "explicit none", action: "consult", waitMode: `,"wait_mode":"none"`},
		{name: "notification default", action: "inform"},
		{name: "report default", action: "report"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := &fakeOrganizationMessageService{sent: &services.AgentMessageDTO{ID: "child-1", Status: "queued"}}
			router := setupOrganizationMcpRouter(service)
			params := json.RawMessage(`{"name":"agent_send","arguments":{"target_agent":"agent-c","action":"` + tc.action + `","message":"continue","run_id":"run-1","parent_message_id":"parent-1"` + tc.waitMode + `}}`)
			rec := postOrganizationRPC(t, router, "tools/call", params, "agent-a-token")
			decodeMcpTextResult(t, rec)
			require.Empty(t, service.input.CallbackRuntimeSessionID)
			require.Equal(t, "agent-c", service.input.TargetAgent)
		})
	}
}

func TestOrganizationMcpContinuationCannotResetServerOwnedChain(t *testing.T) {
	service := &fakeOrganizationMessageService{sent: &services.AgentMessageDTO{
		ID: "message-2", ConversationID: "server-conversation", RootMessageID: "root-1", ParentMessageID: "parent-1", Hop: 3, MaxHops: 8, Status: "queued",
	}}
	router := setupOrganizationMcpRouter(service)
	params := json.RawMessage(`{
		"name":"agent_send",
		"arguments":{
			"target_agent":"agent-c","action":"handoff","message":"continue","wait_mode":"none",
			"parent_message_id":"parent-1","idempotency_key":"parent-1:handoff:c",
			"conversation_id":"spoofed","root_message_id":"spoofed","hop":1,"max_hops":999,
			"budget":{"max_events":999999,"events_used":0},"trace":{"visited_agent_ids":[],"sync_stack":[]}
		}
	}`)
	rec := postOrganizationRPC(t, router, "tools/call", params, "agent-a-token")
	require.Equal(t, http.StatusOK, rec.Code)
	payload := decodeMcpTextResult(t, rec)
	require.Equal(t, float64(3), payload["hop"])
	require.Equal(t, "server-conversation", payload["conversationId"])
	require.Equal(t, "parent-1", service.input.ParentMessageID)
	require.Equal(t, "parent-1:handoff:c", service.input.IdempotencyKey)
	require.Empty(t, service.input.ConversationID)
	require.Empty(t, service.input.RootMessageID)
	require.Zero(t, service.input.Hop)
	require.Zero(t, service.input.EventBudget)
	require.Empty(t, service.input.VisitedAgentIDs)
}

func TestOrganizationMcpRelationsExposeDirection(t *testing.T) {
	service := &fakeOrganizationMessageService{relations: []*services.AgentRelationDTO{
		{SourceAgentID: 41, SourceAgentName: "agent-a", TargetAgentID: 42, TargetAgentName: "agent-b", Scope: "hq", AllowedActions: []string{"review"}},
		{SourceAgentID: 43, SourceAgentName: "agent-c", TargetAgentID: 41, TargetAgentName: "agent-a", Scope: "hq", AllowedActions: []string{"report"}},
	}}
	router := setupOrganizationMcpRouter(service)
	params := json.RawMessage(`{"name":"agent_relations","arguments":{}}`)
	rec := postOrganizationRPC(t, router, "tools/call", params, "agent-a-token")
	payload := decodeMcpTextResult(t, rec)
	require.Equal(t, "agent-a", payload["self"])
	relations := payload["relations"].([]interface{})
	require.Equal(t, "outgoing", relations[0].(map[string]interface{})["direction"])
	require.Equal(t, "incoming", relations[1].(map[string]interface{})["direction"])
	require.NotContains(t, relations[0].(map[string]interface{}), "relationship_score")
	require.NotContains(t, relations[0].(map[string]interface{}), "stance")
	require.NotContains(t, relations[1].(map[string]interface{}), "relationship_score")
	require.NotContains(t, relations[1].(map[string]interface{}), "current_stance")
	require.NotContains(t, relations[1].(map[string]interface{}), "stance")
}

func TestOrganizationMcpPureConnectionOmitsDynamicRelationship(t *testing.T) {
	service := &fakeOrganizationMessageService{relations: []*services.AgentRelationDTO{
		{SourceAgentID: 41, SourceAgentName: "agent-a", TargetAgentID: 42, TargetAgentName: "agent-b", Scope: "hq", RelationType: "peer", AllowedActions: []string{"review"}},
	}}
	router := setupOrganizationMcpRouter(service)
	rec := postOrganizationRPC(t, router, "tools/call", json.RawMessage(`{"name":"agent_relations","arguments":{}}`), "agent-a-token")
	payload := decodeMcpTextResult(t, rec)
	relation := payload["relations"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, "peer", relation["relation_type"])
	require.NotContains(t, relation, "relationship_score")
	require.NotContains(t, relation, "stance")
}

func TestOrganizationMcpRejectsMissingRuntimeToken(t *testing.T) {
	router := setupOrganizationMcpRouter(&fakeOrganizationMessageService{})
	rec := postOrganizationRPC(t, router, "tools/list", nil, "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
