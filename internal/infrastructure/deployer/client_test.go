package deployer

import (
	"context"
	"encoding/json"
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
		Agent: AgentDefinition{
			Name:           "coder",
			Model:          "claude-sonnet-4-6",
			SystemPrompt:   "You are a coding assistant.",
			MaxTurns:       intPtr(10),
			PermissionMode: "auto",
			Tools:          []string{"Read", "Write"},
			Skills:         []SkillSource{{Name: "code-review", URL: "https://example.com/skills/code-review.zip", Hash: "sha256:abc123"}},
			Subagents: []SubagentDefinition{
				{
					Name:        "reviewer",
					Description: "Reviews code",
					Prompt:      "You are a code reviewer.",
					Tools:       []string{"Read"},
					MaxTurns:    intPtr(10),
				},
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
		Agent: AgentDefinition{
			Name:         "coder",
			Model:        "claude-sonnet-4-6",
			SystemPrompt: "You are a coding assistant.",
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
		Agent: AgentDefinition{
			Name:         "coder",
			Model:        "claude-sonnet-4-6",
			SystemPrompt: "You are a coding assistant.",
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
	want := "deployer returned HTTP 400: {\"error\":\"invalid request\",\"success\":false}\n"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
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
	want := "deployer returned HTTP 404: {\"error\":\"agent \\\"coder\\\": agent not found\",\"success\":false}\n"
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
