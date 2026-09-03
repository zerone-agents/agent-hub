package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAgent(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotContentType string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"agentName":     "coder",
				"instanceId":    "a1b2c3d4",
				"containerId":   "9f3c",
				"containerName": "cloud-agent-coder-a1b2c3d4",
				"status":        "running",
				"hostPort":      32768,
				"createdAt":     "2026-06-25T10:00:00Z",
				"yamlPath":      "/var/lib/agent-deployer/coder/agents/agents.yaml",
				"sessionDir":    "/var/lib/agent-deployer/coder/sessions",
				"skillsDir":     "/var/lib/agent-deployer/coder/skills",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	req := &CreateAgentRequest{
		RootAgentID:   "coder",
		DeploymentKey: "acme-coder",
		Agents: []AgentDefinition{
			{
				Name:           "coder",
				Model:          "claude-sonnet-4-6",
				SystemPrompt:   "You are a coding assistant.",
				MaxTurns:       intPtr(10),
				PermissionMode: "auto",
				Tools:          []string{"Read", "Write"},
				Skills:         []SkillSource{{Name: "code-review", URL: "https://example.com/skills/code-review.zip", Hash: "sha256:abc123"}},
				Subagents:      []string{"reviewer"},
			},
		},
		Provider: ProviderConfig{
			Protocol:     "anthropic",
			BaseURL:      "https://api.anthropic.com",
			LockedAPIKey: "sk-ant-xxx",
		},
		RuntimeToken: "rt-caller-provided-token",
	}

	resp, err := client.CreateAgent(context.Background(), req, true)
	if err != nil {
		t.Fatalf("CreateAgent error: %v", err)
	}

	if gotPath != "/api/v1/agents" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents")
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}

	if resp.AgentName != "coder" {
		t.Errorf("AgentName = %q, want %q", resp.AgentName, "coder")
	}
	if resp.Status != "running" {
		t.Errorf("Status = %q, want %q", resp.Status, "running")
	}
	if resp.HostPort != 32768 {
		t.Errorf("HostPort = %d, want %d", resp.HostPort, 32768)
	}
	if resp.CreatedAt != "2026-06-25T10:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", resp.CreatedAt, "2026-06-25T10:00:00Z")
	}

	// Verify force field was sent
	if force, ok := gotBody["force"].(bool); !ok || !force {
		t.Errorf("force = %v, want true", force)
	}

	// Verify the caller-provided runtime token was sent, and the removed
	// rotate_key field is gone from the payload.
	if token, ok := gotBody["runtime_token"].(string); !ok || token != "rt-caller-provided-token" {
		t.Errorf("runtime_token = %v, want %q", gotBody["runtime_token"], "rt-caller-provided-token")
	}
	if _, ok := gotBody["rotate_key"]; ok {
		t.Errorf("rotate_key should not be present in request body, got %v", gotBody["rotate_key"])
	}

	// deploymentKey is serialized as its own top-level field (deployer v3.1,
	// issue #114): tenant-scoped resource key, independent of rootAgentId.
	if dk, ok := gotBody["deploymentKey"].(string); !ok || dk != "acme-coder" {
		t.Errorf("deploymentKey = %v, want %q", gotBody["deploymentKey"], "acme-coder")
	}
}

func TestCreateAgent_ForceFalse(t *testing.T) {
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"agentName":     "coder",
				"instanceId":    "a1b2c3d4",
				"containerId":   "9f3c",
				"containerName": "cloud-agent-coder-a1b2c3d4",
				"status":        "running",
				"hostPort":      32768,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	req := &CreateAgentRequest{
		RootAgentID: "coder",
		Agents: []AgentDefinition{
			{Name: "coder", Model: "claude-sonnet-4-6", SystemPrompt: "You are a coding assistant."},
		},
		Provider: ProviderConfig{
			Protocol:     "anthropic",
			BaseURL:      "https://api.anthropic.com",
			LockedAPIKey: "sk-ant-xxx",
		},
	}

	_, err := client.CreateAgent(context.Background(), req, false)
	if err != nil {
		t.Fatalf("CreateAgent error: %v", err)
	}

	if force, ok := gotBody["force"].(bool); ok && force {
		t.Errorf("force = %v, want false (or omitted)", force)
	}
}

func TestCreateAgent_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "invalid request",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	req := &CreateAgentRequest{
		RootAgentID: "coder",
		Agents: []AgentDefinition{
			{Name: "coder", Model: "claude-sonnet-4-6", SystemPrompt: "You are a coding assistant."},
		},
		Provider: ProviderConfig{
			Protocol:     "anthropic",
			BaseURL:      "https://api.anthropic.com",
			LockedAPIKey: "sk-ant-xxx",
		},
	}

	_, err := client.CreateAgent(context.Background(), req, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("want *HTTPError, got %T: %v", err, err)
	}
	want := "deployer returned HTTP 400: invalid request"
	if httpErr.Error() != want {
		t.Errorf("error = %q, want %q", httpErr.Error(), want)
	}
	if httpErr.Message != "invalid request" {
		t.Errorf("Message = %q, want %q", httpErr.Message, "invalid request")
	}
}

func TestGetAgent(t *testing.T) {
	var gotPath string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"agentName":     "coder",
				"instanceId":    "a1b2c3d4",
				"containerId":   "9f3c",
				"containerName": "cloud-agent-coder-a1b2c3d4",
				"status":        "running",
				"hostPort":      32768,
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resp, err := client.GetAgent(context.Background(), "coder")
	if err != nil {
		t.Fatalf("GetAgent error: %v", err)
	}

	if gotPath != "/api/v1/agents/coder" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents/coder")
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if resp.AgentName != "coder" {
		t.Errorf("AgentName = %q, want %q", resp.AgentName, "coder")
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   `agent "coder": agent not found`,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	_, err := client.GetAgent(context.Background(), "coder")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "deployer returned HTTP 404: agent \"coder\": agent not found"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestGetStatus(t *testing.T) {
	var gotPath string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"agentName":     "coder",
				"containerName": "cloud-agent-coder-a1b2c3d4",
				"containerId":   "9f3c",
				"status":        "running",
				"health":        "healthy",
				"hostPort":      32768,
				"image":         "open-agent-runtime:latest",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	resp, err := client.GetStatus(context.Background(), "coder")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}

	if gotPath != "/api/v1/agents/coder/status" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents/coder/status")
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if resp.Health != "healthy" {
		t.Errorf("Health = %q, want %q", resp.Health, "healthy")
	}
	if resp.Image != "open-agent-runtime:latest" {
		t.Errorf("Image = %q, want %q", resp.Image, "open-agent-runtime:latest")
	}
}

func TestStopAgent(t *testing.T) {
	var gotPath string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.StopAgent(context.Background(), "coder")
	if err != nil {
		t.Fatalf("StopAgent error: %v", err)
	}

	if gotPath != "/api/v1/agents/coder/stop" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents/coder/stop")
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
}

func TestDeleteAgent(t *testing.T) {
	var gotPath, gotAuth, gotPurge string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotPurge = r.URL.Query().Get("purge")
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want %q", r.Method, http.MethodDelete)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.DeleteAgent(context.Background(), "coder", true)
	if err != nil {
		t.Fatalf("DeleteAgent error: %v", err)
	}

	if gotPath != "/api/v1/agents/coder" {
		t.Errorf("path = %q, want %q", gotPath, "/api/v1/agents/coder")
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPurge != "true" {
		t.Errorf("purge = %q, want %q", gotPurge, "true")
	}
}

func TestDeleteAgent_NoPurge(t *testing.T) {
	var gotPurge string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPurge = r.URL.Query().Get("purge")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.DeleteAgent(context.Background(), "coder", false)
	if err != nil {
		t.Fatalf("DeleteAgent error: %v", err)
	}

	if gotPurge != "" {
		t.Errorf("purge = %q, want empty", gotPurge)
	}
}

func intPtr(i int) *int {
	return &i
}

func TestCreateAgent_SendsGraphProtocol(t *testing.T) {
	var gotBody map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": map[string]interface{}{}})
	}))
	defer srv.Close()

	maxQ := 50
	maxTurns := 20
	c := NewClient(srv.URL, "key")
	_, err := c.CreateAgent(context.Background(), &CreateAgentRequest{
		RootAgentID: "org-root",
		Agents: []AgentDefinition{
			{Name: "org-root", Description: "d", Model: "m", SystemPrompt: "p",
				MaxSessionQueries: &maxQ, Subagents: []string{"child-a"}},
			{Name: "child-a", Description: "d2", SystemPrompt: "p2",
				MaxTurns: &maxTurns, Datasets: map[string]string{"ds1": "desc"}},
		},
		Provider:     ProviderConfig{Protocol: "openai", BaseURL: "http://x", LockedAPIKey: "k"},
		RuntimeToken: "tok",
	}, true)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	if _, exists := gotBody["agent"]; exists {
		t.Error("request must not contain legacy top-level \"agent\"")
	}
	if _, exists := gotBody["maxSessionTurns"]; exists {
		t.Error("request must not contain legacy maxSessionTurns")
	}
	var rootID string
	json.Unmarshal(gotBody["rootAgentId"], &rootID)
	if rootID != "org-root" {
		t.Errorf("rootAgentId = %q, want org-root", rootID)
	}
	var agents []map[string]json.RawMessage
	json.Unmarshal(gotBody["agents"], &agents)
	if len(agents) != 2 {
		t.Fatalf("agents len = %d, want 2", len(agents))
	}
	var subs []string
	json.Unmarshal(agents[0]["subagents"], &subs)
	if len(subs) != 1 || subs[0] != "child-a" {
		t.Errorf("root subagents = %v, want [child-a] (pure id refs)", subs)
	}
	// NOTE: the brief's verbatim loop ranged over gotBody["agents"] directly,
	// which iterates the raw json.RawMessage bytes; iterate the decoded
	// elements instead.
	for _, m := range agents {
		if _, exists := m["prompt"]; exists {
			t.Error("agent definition must not contain legacy SubagentDefinition \"prompt\" field")
		}
	}
}

func TestCreateAgent_HTTPErrorCarriesStatusAndMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "rootAgentId not found in agents"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key")
	_, err := c.CreateAgent(context.Background(), &CreateAgentRequest{RootAgentID: "x", Provider: ProviderConfig{}, RuntimeToken: "t"}, false)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("want *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", httpErr.StatusCode)
	}
	if httpErr.Message != "rootAgentId not found in agents" {
		t.Errorf("Message = %q", httpErr.Message)
	}
}

func TestSupportsDeploymentKey_V31Sentinel(t *testing.T) {
	// v3.1.0+ validation order: rootAgentID ok → deploymentKey missing →
	// 400 "deploymentKey is required" (before agents/provider checks).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "deploymentKey is required"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	ok, err := client.SupportsDeploymentKey(context.Background())
	if err != nil || !ok {
		t.Fatalf("SupportsDeploymentKey() = (%v, %v), want (true, nil)", ok, err)
	}
}

func TestSupportsDeploymentKey_LegacyV30(t *testing.T) {
	// v3.0.x has no deploymentKey concept: the same probe trips the
	// empty-agents guard instead → reported as unsupported, not an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "agents must contain at least the root agent definition"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	ok, err := client.SupportsDeploymentKey(context.Background())
	if err != nil || ok {
		t.Fatalf("SupportsDeploymentKey() = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestSupportsDeploymentKey_TransportError(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "test-key")
	ok, err := client.SupportsDeploymentKey(context.Background())
	if err == nil || ok {
		t.Fatalf("SupportsDeploymentKey() = (%v, %v), want (false, error)", ok, err)
	}
}

func TestSupportsDeploymentKey_UnexpectedSuccess(t *testing.T) {
	// No known deployer generation accepts this invalid probe. Fail closed
	// with an error rather than guessing support.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	ok, err := client.SupportsDeploymentKey(context.Background())
	if err == nil || ok {
		t.Fatalf("SupportsDeploymentKey() = (%v, %v), want (false, error)", ok, err)
	}
}
