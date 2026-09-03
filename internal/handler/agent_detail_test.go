package handler

import (
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
		// Runtime registers agents under the tenant-scoped deploy key.
		if r.URL.Path != "/v1/agents/tenant-a-min" {
			t.Errorf("runtime path = %q, want /v1/agents/tenant-a-min", r.URL.Path)
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
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m","name":"m","model":"x","status":"ready","maxTurns":10,"hasSystemPrompt":true,"mcpServers":{"github":{"transport":"stdio","command":"mcp-server-github","env":{"GITHUB_TOKEN":"***"}}}}`))
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
	// Verify MCP env value passes through verbatim (no second redaction)
	if !strings.Contains(body, `"GITHUB_TOKEN":"***"`) {
		t.Errorf("MCP env value should pass through unchanged, got: %q", body)
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
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("body should mention 'not found', got: %q", rec.Body.String())
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
	if !strings.Contains(rec.Body.String(), "runtime unreachable") && !strings.Contains(rec.Body.String(), "runtime detail request failed") {
		t.Errorf("body should mention runtime failure, got: %q", rec.Body.String())
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
