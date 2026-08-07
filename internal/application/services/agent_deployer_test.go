package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
)

func TestWaitForHealthy_DockerHealthyPath(t *testing.T) {
	// Mock deployer that returns healthy.
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","health":"healthy","hostPort":3000,"image":"img"}}`))
	}))
	defer srv.Close()

	client := deployer.NewClient(srv.URL, "test-key")
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1"}

	port, err := s.WaitForHealthy(context.Background(), "general", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 3000 {
		t.Fatalf("expected port 3000, got %d", port)
	}
	if !called {
		t.Fatal("expected deployer /status to be called")
	}
}

func TestWaitForHealthy_ActiveProbePath(t *testing.T) {
	// Mock deployer always returns starting; health probe returns true immediately.
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","health":"starting","hostPort":3000,"image":"img"}}`))
	}))
	defer deployerSrv.Close()

	client := deployer.NewClient(deployerSrv.URL, "test-key")
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool {
		return host == "10.0.0.1" && port == 3000
	}}

	gotPort, err := s.WaitForHealthy(context.Background(), "general", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPort != 3000 {
		t.Fatalf("expected port 3000, got %d", gotPort)
	}
}

func TestWaitForHealthy_Timeout(t *testing.T) {
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","health":"starting","hostPort":3000,"image":"img"}}`))
	}))
	defer deployerSrv.Close()

	client := deployer.NewClient(deployerSrv.URL, "test-key")
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool { return false }}

	start := time.Now()
	_, err := s.WaitForHealthy(context.Background(), "general", 500*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Fatalf("expected at least 500ms timeout, got %s", time.Since(start))
	}
}

// Test helpers for the existing agent_validator_test.go.

type mockAgentRepo struct {
	getByNameFunc                 func(name string) (*agent.AgentConfig, error)
	getSubagentsFunc              func(agentID uint64) ([]string, error)
	getKnowledgeDatasetIDsByAgent func(agentID uint64) ([]string, error)
	updateFunc                    func(a *agent.AgentConfig) error
}

func (m *mockAgentRepo) GetByName(name string) (*agent.AgentConfig, error) {
	return m.getByNameFunc(name)
}

func (m *mockAgentRepo) GetSubagents(agentID uint64) ([]string, error) {
	if m.getSubagentsFunc != nil {
		return m.getSubagentsFunc(agentID)
	}
	return nil, nil
}

func (m *mockAgentRepo) GetKnowledgeDatasetIDsByAgent(agentID uint64) ([]string, error) {
	if m.getKnowledgeDatasetIDsByAgent != nil {
		return m.getKnowledgeDatasetIDsByAgent(agentID)
	}
	return nil, nil
}

func (m *mockAgentRepo) Update(a *agent.AgentConfig) error {
	return m.updateFunc(a)
}

type mockProviderSvc struct {
	getByIDFunc   func(id uint64) (providerdomain.Provider, error)
	getRawKeyFunc func(id uint64) (string, error)
}

func (m *mockProviderSvc) GetByID(id uint64) (providerdomain.Provider, error) {
	return m.getByIDFunc(id)
}

func (m *mockProviderSvc) GetRawAPIKey(id uint64) (string, error) {
	return m.getRawKeyFunc(id)
}

type mockToolRepo struct{}

func (m *mockToolRepo) GetToolsByAgent(agentID uint64) ([]string, error) { return nil, nil }

type mockSkillRepo struct{}

func (m *mockSkillRepo) GetAgentSkills(agentID uint64) ([]string, error)           { return nil, nil }
func (m *mockSkillRepo) GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error) { return nil, nil }

type mockMcpSvc struct {
	mcps map[string]*McpClientDTO
	err  error
}

func (m *mockMcpSvc) GetClientMcpsByAgent(name string) (map[string]*McpClientDTO, error) {
	if m.mcps != nil || m.err != nil {
		return m.mcps, m.err
	}
	return nil, nil
}

type mockKnowledgeSvc struct{}

func (m *mockKnowledgeSvc) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	return nil, nil
}

func newTestAgentDeployerService(t *testing.T, deployerURL string, agentRepo agentRepository, providerSvc providerService) *AgentDeployerService {
	t.Helper()
	return &AgentDeployerService{
		client:       deployer.NewClient(deployerURL, "test-key"),
		publicHost:   "10.0.0.1",
		agentRepo:    agentRepo,
		toolRepo:     &mockToolRepo{},
		skillRepo:    &mockSkillRepo{},
		providerSvc:  providerSvc,
		mcpSvc:       &mockMcpSvc{},
		knowledgeSvc: &mockKnowledgeSvc{},
		healthProbe:  func(ctx context.Context, host string, port int) bool { return false },
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }

// deployTokenFixture bundles a mock deployer server that distinguishes the
// GET probe (existing-container check) from the POST create call, plus the
// captured state from the POST.
type deployTokenFixture struct {
	server     *httptest.Server
	postBody   []byte
	postCalled bool
	persisted  string
}

// newDeployTokenServer builds a mock deployer. getFound controls the GET
// /api/v1/agents/<name> probe response (container exists or not); POST
// /api/v1/agents always succeeds and echoes a container payload.
func newDeployTokenServer(t *testing.T, getFound bool, f *deployTokenFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if !getFound {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"success":false,"error":"agent not found"}`))
				return
			}
			w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","hostPort":3000}}`))
			return
		}
		f.postCalled = true
		body, _ := io.ReadAll(r.Body)
		f.postBody = body
		w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","hostPort":3000,"runtimeToken":"echoed"}}`))
	}))
}

func (f *deployTokenFixture) sentToken(t *testing.T) string {
	t.Helper()
	if !f.postCalled {
		t.Fatal("expected deployer create (POST) to be called")
	}
	var parsed struct {
		RuntimeToken string `json:"runtime_token"`
	}
	if err := json.Unmarshal(f.postBody, &parsed); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	return parsed.RuntimeToken
}

func deployTokenAgentRepo(f *deployTokenFixture, runtimeToken string) *mockAgentRepo {
	providerID := uint64(1)
	return &mockAgentRepo{
		getByNameFunc: func(name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID:           1,
				Name:         "general",
				ProviderID:   &providerID,
				ModelID:      "glm-5-turbo",
				RuntimeToken: runtimeToken,
			}, nil
		},
		updateFunc: func(a *agent.AgentConfig) error {
			f.persisted = a.RuntimeToken
			return nil
		},
	}
}

func deployTokenProviderSvc() *mockProviderSvc {
	return &mockProviderSvc{
		getByIDFunc: func(id uint64) (providerdomain.Provider, error) {
			p := providerdomain.NewGenericProvider("openai")
			if err := p.Base().SetSummary(&providerdomain.ProviderSummary{ID: id, BaseURL: "http://example.com", Protocol: "openai"}); err != nil {
				return nil, err
			}
			return p, nil
		},
		getRawKeyFunc: func(id uint64) (string, error) { return "key", nil },
	}
}

// TestDeploy_RuntimeToken covers the control-panel-side token ownership: the
// deployer no longer mints tokens, so Deploy must decide whether to reuse the
// stored token or generate a fresh one, send it as runtime_token, and persist
// the value it sent (not whatever the response echoes).
func TestDeploy_RuntimeToken(t *testing.T) {
	// The test service has an empty encryption key, so Encrypt/Decrypt are
	// pass-through and "persisted" values are plaintext.
	const storedToken = "0123456789abcdef0123456789abcdef"

	t.Run("reuses stored token when not rotating", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		dto, err := s.Deploy("general", true, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := f.sentToken(t); got != storedToken {
			t.Errorf("runtime_token = %q, want stored %q", got, storedToken)
		}
		if dto.APIKey != storedToken {
			t.Errorf("dto.APIKey = %q, want %q", dto.APIKey, storedToken)
		}
		if f.persisted != storedToken {
			t.Errorf("persisted token = %q, want %q", f.persisted, storedToken)
		}
	})

	t.Run("rotate without force still reuses stored token", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		if _, err := s.Deploy("general", false, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := f.sentToken(t); got != storedToken {
			t.Errorf("runtime_token = %q, want stored %q (rotation requires force)", got, storedToken)
		}
	})

	t.Run("mints new token when rotating with force", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		dto, err := s.Deploy("general", true, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := f.sentToken(t)
		if got == "" || got == storedToken {
			t.Errorf("runtime_token = %q, want a fresh token different from stored %q", got, storedToken)
		}
		if len(got) != 32 {
			t.Errorf("runtime_token length = %d, want 32 hex chars", len(got))
		}
		if dto.APIKey != got {
			t.Errorf("dto.APIKey = %q, want sent token %q", dto.APIKey, got)
		}
		if f.persisted != got {
			t.Errorf("persisted token = %q, want sent token %q (must not persist response echo)", f.persisted, got)
		}
	})

	t.Run("mints token on first deploy when nothing stored", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, false, f) // GET probe: no existing container
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, ""), deployTokenProviderSvc())
		dto, err := s.Deploy("general", false, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := f.sentToken(t)
		if len(got) != 32 {
			t.Errorf("runtime_token length = %d, want 32 hex chars", len(got))
		}
		if dto.APIKey != got || f.persisted != got {
			t.Errorf("dto.APIKey=%q persisted=%q, both want sent token %q", dto.APIKey, f.persisted, got)
		}
	})

	t.Run("refuses when container exists but token unrecoverable", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f) // GET probe: container exists
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, ""), deployTokenProviderSvc())
		_, err := s.Deploy("general", false, false)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "无法恢复") {
			t.Errorf("error = %q, want it to mention 无法恢复", err.Error())
		}
		if f.postCalled {
			t.Error("deployer create (POST) must not be called when refusing")
		}
	})

	t.Run("substitutes runtime token placeholder in MCP headers", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		s.mcpSvc = &mockMcpSvc{mcps: map[string]*McpClientDTO{
			"knowledge": {
				Name: "knowledge",
				Type: "http",
				URL:  "http://example.com/api/v1/knowledge/mcp",
				Headers: map[string]string{
					"Authorization": BuiltinKnowledgeAuthHeader,
					"X-Static":      "keep-me",
				},
			},
		}}

		if _, err := s.Deploy("general", true, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		body := string(f.postBody)
		if !strings.Contains(body, `"Authorization":"Bearer `+storedToken+`"`) {
			t.Errorf("request body should contain resolved Authorization header, got %s", body)
		}
		if strings.Contains(body, "$agent_runtime_token") {
			t.Errorf("request body must not contain the placeholder, got %s", body)
		}
		if !strings.Contains(body, `"X-Static":"keep-me"`) {
			t.Errorf("unrelated headers must pass through untouched, got %s", body)
		}
	})
}

// newGetStatusServer builds a mock deployer whose GET
// /api/v1/agents/<name>/status always reports the given Docker status.
func newGetStatusServer(t *testing.T, status string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"` + status + `","health":"none","hostPort":3000,"image":"img"}}`))
	}))
}

func getStatusRepo(persisted *string) *mockAgentRepo {
	return &mockAgentRepo{
		getByNameFunc: func(name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID:               1,
				Name:             "general",
				DeploymentStatus: "running",
				RuntimePort:      3000,
			}, nil
		},
		updateFunc: func(a *agent.AgentConfig) error {
			*persisted = a.DeploymentStatus
			return nil
		},
	}
}

func TestGetStatus_TransientStatusNotPersisted(t *testing.T) {
	for _, status := range []string{"created", "restarting", "paused", "removing", "unknown"} {
		t.Run(status, func(t *testing.T) {
			srv := newGetStatusServer(t, status)
			defer srv.Close()
			var persisted string
			s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())

			dto, err := s.GetStatus("general")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The DTO reflects the live status; only the DB write is filtered.
			if dto.Status != status {
				t.Errorf("dto.Status = %q, want live status %q", dto.Status, status)
			}
			if persisted != "" {
				t.Errorf("transient status %q must not be persisted, DB got %q", status, persisted)
			}
		})
	}
}

func TestGetStatus_StableStatusPersisted(t *testing.T) {
	for _, status := range []string{"running", "exited", "stopped", "dead"} {
		t.Run(status, func(t *testing.T) {
			srv := newGetStatusServer(t, status)
			defer srv.Close()
			var persisted string
			s := newTestAgentDeployerService(t, srv.URL, getStatusRepo(&persisted), deployTokenProviderSvc())

			if _, err := s.GetStatus("general"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status == "running" {
				// Same as DB value: no update needed.
				if persisted != "" {
					t.Errorf("unchanged status should not trigger update, got %q", persisted)
				}
				return
			}
			if persisted != status {
				t.Errorf("stable status should be persisted, got %q want %q", persisted, status)
			}
		})
	}
}
