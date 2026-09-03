package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/infrastructure/runtime"

	"github.com/gin-gonic/gin"
)

// fakeAgentDetailSvc is a test-only AgentDetailService implementation.
// It returns a canned baseURL/apiKey so the production handler will
// issue a real HTTP request to our httptest server, exercising the
// real runtime.Client too.
type fakeAgentDetailSvc struct {
	baseURL string
	apiKey  string
	err     error
}

func (f *fakeAgentDetailSvc) ResolveRuntime(tenantID, name string) (string, string, string, error) {
	if f.err != nil {
		return "", "", "", f.err
	}
	return f.baseURL, f.apiKey, "", nil
}
func (f *fakeAgentDetailSvc) RuntimeClient() *runtime.Client { return runtime.NewClient() }

func setupAgentDetailRouter(svc AgentDetailService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAgentDetailHandler(svc)
	r := gin.New()
	// The handler resolves the tenant from the gin context (normally set by
	// the tenant middleware) to build the tenant-scoped runtime address.
	r.Use(func(c *gin.Context) { c.Set("tenant_id", chatTestTenant) })
	r.GET("/api/v1/admin/agents/:name/detail", h.GetAgentDetail)
	return r
}

func TestGetAgentDetail_Success_Minimal(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Runtime registers agents under their bare agent id (issue #114).
		if r.URL.Path != "/v1/agents/min" {
			t.Errorf("runtime path = %q, want /v1/agents/min", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("runtime method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "rt-secret" {
			t.Errorf("x-api-key = %q, want rt-secret", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"min","name":"min","model":"claude-sonnet-4-6","status":"ready","maxTurns":10,"hasSystemPrompt":false}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "rt-secret"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/min/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"min"`) {
		t.Errorf("body should pass through runtime JSON, got: %q", body)
	}
	// Verify no {success, data} wrapping
	if strings.Contains(body, `"success":true`) {
		t.Errorf("body should be raw runtime JSON, not wrapped: %q", body)
	}
}

func TestGetAgentDetail_Success_WithMcp(t *testing.T) {
	// The runtime pre-redacts env values to "***" (those pass through), but
	// its header redaction may miss hub-armed credentials — the hub strips
	// them at egress (issue #111: 日志与 detail 不泄漏 capability).
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m","name":"m","model":"x","status":"ready","maxTurns":10,"hasSystemPrompt":true,` +
			`"mcpServers":{` +
			`"github":{"transport":"stdio","command":"mcp-server-github","env":{"GITHUB_TOKEN":"***"}},` +
			`"knowledge":{"transport":"http","url":"http://hub.internal:8080/api/v1/mcp/knowledge",` +
			`"headers":{"X-Agent-Capability":"v1.eyJ2IjoxfQ.c2ln","Authorization":"Bearer rt-secret","X-Custom":"cfg"}}}}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/m/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// MCP env values (already runtime-redacted) still pass through unchanged.
	if !strings.Contains(body, `"GITHUB_TOKEN":"***"`) {
		t.Errorf("MCP env value should pass through unchanged, got: %q", body)
	}
	// Non-header MCP config passes through unchanged.
	if !strings.Contains(body, `"url":"http://hub.internal:8080/api/v1/mcp/knowledge"`) {
		t.Errorf("MCP url should pass through unchanged, got: %q", body)
	}
	// Credential headers are removed entirely — key and value.
	for _, leaked := range []string{"X-Agent-Capability", "Authorization", "v1.", "Bearer", "rt-secret"} {
		if strings.Contains(body, leaked) {
			t.Errorf("body must not contain %q, got: %q", leaked, body)
		}
	}
	// Remaining headers keep their names but every value is masked.
	if !strings.Contains(body, `"X-Custom":"***"`) {
		t.Errorf("non-sensitive header value should be masked, got: %q", body)
	}
	// The response is still valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Errorf("redacted body should be valid JSON: %v, got: %q", err, body)
	}
}

// TestGetAgentDetail_RedactsCapabilityAndAuthHeaders is the P1-2 regression
// (PR #118 second review round): a runtime whose header redaction misses the
// deploy-time injected credentials must not be able to leak them through the
// hub detail egress.
func TestGetAgentDetail_RedactsCapabilityAndAuthHeaders(t *testing.T) {
	// Realistic capability wire value: v1.<b64url(payloadJSON)>.<b64url(hmac)>
	// (see services.issueKnowledgeCapability).
	payload := base64.URLEncoding.EncodeToString([]byte(`{"v":1,"t":"default","d":"default-child-a","a":"child-a","f":"ab12cd3ef4567890"}`))
	hmacSeg := base64.URLEncoding.EncodeToString([]byte("signature-bytes"))
	capability := "v1." + payload + "." + hmacSeg
	runtimeToken := "rt-9f8e7d6c5b4a3210"

	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		detail := map[string]any{
			"id": "child-a", "name": "child-a", "model": "x", "status": "ready",
			"maxTurns": 10, "hasSystemPrompt": true,
			"mcpServers": map[string]any{
				"knowledge": map[string]any{
					"transport": "http",
					"url":       "http://hub.internal:8080/api/v1/mcp/knowledge",
					"headers": map[string]any{
						"X-Agent-Capability": capability,
						"Authorization":      "Bearer " + runtimeToken,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(detail)
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/child-a/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, leaked := range []string{"v1.", capability, runtimeToken, "Bearer", "X-Agent-Capability", "Authorization"} {
		if strings.Contains(body, leaked) {
			t.Errorf("hub egress leaked %q, got: %q", leaked, body)
		}
	}
	// Redaction removes credentials but keeps the server entry itself.
	if !strings.Contains(body, `"knowledge"`) || !strings.Contains(body, `"url":"http://hub.internal:8080/api/v1/mcp/knowledge"`) {
		t.Errorf("MCP server entry should survive redaction, got: %q", body)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("redacted body should be valid JSON: %v, got: %q", err, body)
	}
}

func TestGetAgentDetail_MalformedRuntimeJSON(t *testing.T) {
	// Fail closed: uninspectable runtime bytes never egress unredacted.
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m",`)) // truncated JSON
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/m/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"id":"m"`) {
		t.Errorf("malformed runtime body must not egress, got: %q", rec.Body.String())
	}
}

func TestRedactAgentDetail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no sensitive content passes through", in: `{"id":"a","maxTurns":10}`, want: `{"id":"a","maxTurns":10}`},
		{name: "empty object", in: `{}`, want: `{}`},
		{
			name: "sensitive keys removed at any depth, including inside arrays",
			in:   `{"a":[{"authorization":"Bearer x"},{"X-Agent-Capability":"v1.x.y"}],"x-agent-capability":"v1.top"}`,
			want: `{"a":[{},{}]}`,
		},
		{
			name: "headers map: sensitive keys removed, other values masked (case-insensitive)",
			in:   `{"mcpServers":{"k":{"headers":{"x-agent-capability":"v1.a.b","AUTHORIZATION":"Bearer t","X-Custom":"v"}}}}`,
			want: `{"mcpServers":{"k":{"headers":{"X-Custom":"***"}}}}`,
		},
		{name: "non-map headers value untouched", in: `{"headers":"not-a-map"}`, want: `{"headers":"not-a-map"}`},
		{name: "integer literals round-trip exactly", in: `{"big":9007199254740993,"ratio":1.5}`, want: `{"big":9007199254740993,"ratio":1.5}`},
		{name: "HTML characters not escaped", in: `{"url":"http://x/?a=1&b=2"}`, want: `{"url":"http://x/?a=1&b=2"}`},
		{name: "non-object root", in: `[1,2]`, want: `[1,2]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := redactAgentDetail([]byte(tt.in))
			if err != nil {
				t.Fatalf("redactAgentDetail(%s) error: %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Errorf("redactAgentDetail(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}

	if _, err := redactAgentDetail([]byte(`not-json`)); err == nil {
		t.Error("redactAgentDetail(malformed) should error so the caller fails closed")
	}
}

func TestGetAgentDetail_RuntimeNotFound(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"Agent not found"}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/missing/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Agent 在运行时不存在") {
		t.Errorf("body should mention runtime not-found, got: %q", rec.Body.String())
	}
}

func TestGetAgentDetail_RuntimeUnreachable(t *testing.T) {
	// Spin up then immediately close to simulate connection refused
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Agent 运行时不可用") {
		t.Errorf("body should mention runtime failure, got: %q", rec.Body.String())
	}
}

// TestGetAgentDetail_Non2xxWithCredentials is the P1 regression: a non-2xx
// runtime response whose body echoes hub-armed MCP credentials (capability /
// Authorization) must NOT leak them through the hub egress error path. The
// runtime client discards upstream bodies; the handler returns a neutral
// Chinese message only.
func TestGetAgentDetail_Non2xxWithCredentials(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","mcpServers":{"knowledge":{"headers":{"Authorization":"Bearer sekrit-token","X-Agent-Capability":"v1.cGF5bG9hZA.s2ln"}}}}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentDetailRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{"sekrit-token", "v1.cGF5bG9hZA", "s2ln", "Bearer", "boom"} {
		if strings.Contains(body, leak) {
			t.Errorf("credential leaked through non-2xx egress: %q in %s", leak, body)
		}
	}
	if !strings.Contains(body, "Agent 运行时不可用") {
		t.Errorf("body should carry the neutral Chinese message, got: %q", body)
	}
}

func TestGetAgentDetail_AgentNotResolved(t *testing.T) {
	router := setupAgentDetailRouter(&fakeAgentDetailSvc{err: fmt.Errorf("agent not deployed")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/ghost/detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}
