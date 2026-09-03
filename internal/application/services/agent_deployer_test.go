package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	"control-panel/internal/infrastructure/kong"
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
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", upstreamHost: "10.0.0.1"}

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
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", upstreamHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool {
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
	s := &AgentDeployerService{client: client, publicHost: "10.0.0.1", upstreamHost: "10.0.0.1", healthProbe: func(ctx context.Context, host string, port int) bool { return false }}

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
	getByNameFunc                 func(tenantID, name string) (*agent.AgentConfig, error)
	getSubagentsFunc              func(agentID uint64) ([]string, error)
	getKnowledgeDatasetIDsByAgent func(agentID uint64) ([]string, error)
	updateFunc                    func(tenantID string, a *agent.AgentConfig) error
}

func (m *mockAgentRepo) GetByName(tenantID, name string) (*agent.AgentConfig, error) {
	return m.getByNameFunc(tenantID, name)
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

func (m *mockAgentRepo) Update(tenantID string, a *agent.AgentConfig) error {
	return m.updateFunc(tenantID, a)
}

type mockProviderSvc struct {
	getByIDFunc   func(id uint64) (providerdomain.Provider, error)
	getRawKeyFunc func(id uint64) (string, error)
}

func (m *mockProviderSvc) GetByID(tenantID string, id uint64) (providerdomain.Provider, error) {
	return m.getByIDFunc(id)
}

func (m *mockProviderSvc) GetRawAPIKey(tenantID string, id uint64) (string, error) {
	return m.getRawKeyFunc(id)
}

type mockToolRepo struct{ tools []*agent.Tool }

func (m *mockToolRepo) GetToolRecordsByAgent(agentID uint64) ([]*agent.Tool, error) {
	return m.tools, nil
}

type mockSkillRepo struct{}

func (m *mockSkillRepo) GetAgentSkills(agentID uint64) ([]string, error)           { return nil, nil }
func (m *mockSkillRepo) GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error) { return nil, nil }

type mockMcpSvc struct {
	mcps map[string]*McpClientDTO
	err  error
}

func (m *mockMcpSvc) GetClientMcpsByAgent(tenantID, name string) (map[string]*McpClientDTO, error) {
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
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID:           1,
				Name:         "general",
				ProviderID:   &providerID,
				ModelID:      "glm-5-turbo",
				RuntimeToken: runtimeToken,
			}, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error {
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
		dto, err := s.Deploy("tenant-a", "general", true, false)
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
		if _, err := s.Deploy("tenant-a", "general", false, true); err != nil {
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
		dto, err := s.Deploy("tenant-a", "general", true, true)
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
		dto, err := s.Deploy("tenant-a", "general", false, false)
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
		_, err := s.Deploy("tenant-a", "general", false, false)
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

		if _, err := s.Deploy("tenant-a", "general", true, false); err != nil {
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

	t.Run("sends description to deployer (zh preferred)", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		repo := deployTokenAgentRepo(f, storedToken)
		baseGet := repo.getByNameFunc
		repo.getByNameFunc = func(tenantID, name string) (*agent.AgentConfig, error) {
			a, err := baseGet(tenantID, name)
			if err != nil {
				return nil, err
			}
			a.Description = map[string]string{"zh": "通用助手", "en": "General assistant"}
			return a, nil
		}

		s := newTestAgentDeployerService(t, srv.URL, repo, deployTokenProviderSvc())
		if _, err := s.Deploy("tenant-a", "general", true, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(f.postBody), `"description":"通用助手"`) {
			t.Errorf("request body should carry the zh description, got %s", f.postBody)
		}
	})

	t.Run("falls back to agent name when description empty", func(t *testing.T) {
		f := &deployTokenFixture{}
		srv := newDeployTokenServer(t, true, f)
		defer srv.Close()

		// deployTokenAgentRepo returns an AgentConfig with no Description —
		// the deployer requires a non-blank description, so the service must
		// fall back to the agent name to keep the request valid.
		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		if _, err := s.Deploy("tenant-a", "general", true, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(f.postBody), `"description":"general"`) {
			t.Errorf("request body should fall back to the agent name, got %s", f.postBody)
		}
	})
}

func TestAppendMcpToolNames(t *testing.T) {
	got := appendMcpToolNames([]string{"Read", "mcp__duplicate__lookup"}, map[string]*McpClientDTO{
		"knowledge": {
			Tools: []McpTool{{Name: "knowledge_search"}, {Name: " "}},
		},
		"duplicate": {
			Tools: []McpTool{{Name: "lookup"}},
		},
	})

	require.ElementsMatch(t, []string{
		"Read",
		"mcp__knowledge__knowledge_search",
		"mcp__duplicate__lookup",
	}, got)
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
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID:               1,
				Name:             "general",
				DeploymentStatus: "running",
				RuntimePort:      3000,
			}, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error {
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

			dto, err := s.GetStatus("tenant-a", "general")
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

			if _, err := s.GetStatus("tenant-a", "general"); err != nil {
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

func customToolRecordsFixture() []*agent.Tool {
	return []*agent.Tool{
		{Name: "Zeta", TenantID: "t", Source: agent.ToolSourceCustom, FileName: "z.mjs",
			FileURL: "tools/t/Zeta/h1.mjs", FileHash: "h1", FileSize: 3},
		{Name: "Alpha", TenantID: "t", Source: agent.ToolSourceCustom, FileName: "a.ts",
			FileURL: "tools/t/Alpha/h2.ts", FileHash: "h2", FileSize: 4},
		{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin},
	}
}

// buildReqTestProvider mirrors deployTokenProviderSvc's construction so
// buildCreateRequest receives a valid providerdomain.Provider.
func buildReqTestProvider(t *testing.T, id uint64) providerdomain.Provider {
	t.Helper()
	p := providerdomain.NewGenericProvider("openai")
	if err := p.Base().SetSummary(&providerdomain.ProviderSummary{ID: id, BaseURL: "http://example.com", Protocol: "openai"}); err != nil {
		t.Fatalf("build provider fixture: %v", err)
	}
	return p
}

func buildReqWithTools(t *testing.T, tools []*agent.Tool, cdnHost string) (*deployer.CreateAgentRequest, error) {
	t.Helper()
	providerID := uint64(1)
	provider := buildReqTestProvider(t, providerID)
	agentRepo := &mockAgentRepo{getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
		return &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "m",
			SystemPrompt: "p", Description: map[string]string{"zh": "d"}}, nil
	}}
	providerSvc := &mockProviderSvc{
		getByIDFunc:   func(id uint64) (providerdomain.Provider, error) { return buildReqTestProvider(t, id), nil },
		getRawKeyFunc: func(id uint64) (string, error) { return "k", nil },
	}
	svc := newTestAgentDeployerService(t, "http://deployer.test", agentRepo, providerSvc)
	svc.toolRepo = &mockToolRepo{tools: tools}
	svc.cdnHost = cdnHost
	cfg := &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "m", SystemPrompt: "p"}
	return svc.buildCreateRequest(context.Background(), "t", cfg, provider)
}

func TestBuildCreateRequest_CustomToolsSortedAndToolsFull(t *testing.T) {
	req, err := buildReqWithTools(t, customToolRecordsFixture(), "https://cdn.example.com")
	require.NoError(t, err)
	// v3 graph shape: rootAgentId must match the (single) root definition.
	require.Equal(t, "t-general", req.RootAgentID)
	require.Len(t, req.Agents, 1)
	// Tools = 全量关联名（含 builtin），排序
	require.Equal(t, []string{"Alpha", "Bash", "Zeta"}, req.Agents[0].Tools)
	// CustomTools = custom+ready 子集，按名排序，URL=CDN+key
	require.Len(t, req.Agents[0].CustomTools, 2)
	require.Equal(t, "Alpha", req.Agents[0].CustomTools[0].Name)
	require.Equal(t, "https://cdn.example.com/tools/t/Alpha/h2.ts", req.Agents[0].CustomTools[0].URL)
	require.Equal(t, "h2", req.Agents[0].CustomTools[0].Hash)
	require.Equal(t, "a.ts", req.Agents[0].CustomTools[0].FileName)
	require.Equal(t, "Zeta", req.Agents[0].CustomTools[1].Name)
}

func TestBuildCreateRequest_MissingCustomToolFailsFast(t *testing.T) {
	tools := append(customToolRecordsFixture(), &agent.Tool{Name: "Legacy", TenantID: "t", Source: agent.ToolSourceCustom})
	_, err := buildReqWithTools(t, tools, "https://cdn.example.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Legacy")
}

func TestBuildCreateRequest_CustomToolsRequireCDNHost(t *testing.T) {
	_, err := buildReqWithTools(t, customToolRecordsFixture(), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "OSS_CDN_HOST")
}

// deployFailureFixture captures what a fake deployer saw during a failed
// Deploy call: create (POST), archive (DELETE) and the DB updates the
// service performed afterwards.
type deployFailureFixture struct {
	postStatus   int  // HTTP status returned to the create (POST) call
	hijackPost   bool // kill the POST at transport level (network error, not *deployer.HTTPError)
	postCalled   bool
	deleteCalled bool
	updates      []*agent.AgentConfig
}

// newDeployFailureServer builds a mock deployer for Deploy failure-path
// tests: the GET probe reports an existing running container, POST returns
// the fixture's status (or dies at transport level when hijackPost is set),
// DELETE records the archive call and succeeds.
func newDeployFailureServer(t *testing.T, f *deployFailureFixture) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`{"success":true,"data":{"agentName":"general","containerName":"c","containerId":"id","status":"running","hostPort":3000}}`))
		case http.MethodPost:
			f.postCalled = true
			if f.hijackPost {
				// Close the connection without a response: the client sees a
				// transport error (not *deployer.HTTPError), which is the
				// "hub cannot know what the deployer did" cleanup case.
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Error("test server does not support hijacking")
					return
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					t.Errorf("hijack failed: %v", err)
					return
				}
				_ = conn.Close()
				return
			}
			w.WriteHeader(f.postStatus)
			w.Write([]byte(`{"success":false,"error":"boom"}`))
		case http.MethodDelete:
			f.deleteCalled = true
			w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

// deployFailureAgentRepo simulates a healthy, currently-running deployment
// (stored token, status running) and records every DB update.
func deployFailureAgentRepo(f *deployFailureFixture, runtimeToken string) *mockAgentRepo {
	providerID := uint64(1)
	return &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			return &agent.AgentConfig{
				ID:               1,
				Name:             "general",
				ProviderID:       &providerID,
				ModelID:          "glm-5-turbo",
				RuntimeToken:     runtimeToken,
				DeploymentStatus: "running",
				RuntimePort:      3000,
			}, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error {
			f.updates = append(f.updates, a)
			return nil
		},
	}
}

// preRegisterKongRoute seeds the fake gateway with the service and routes a
// Deregister call would remove, so tests can tell whether it ran.
func preRegisterKongRoute(fk *fakeKong, key string) {
	fk.services[svcName(key)] = &kong.Service{Name: svcName(key)}
	fk.routes[routeName(key)] = &kong.Route{Name: routeName(key)}
	fk.routes[legacyRouteName(key)] = &kong.Route{Name: legacyRouteName(key)}
}

// attachFakeKong wires a fake gateway pre-registered with the deployment's
// service and routes onto the service, so Deploy tests can assert whether a
// Deregister ran (entries gone) or not (entries intact).
func attachFakeKong(s *AgentDeployerService, key string) *fakeKong {
	fk := newFakeKong()
	preRegisterKongRoute(fk, key)
	s.kongSvc = NewKongGatewayService(fk, "upstream", "public", nil, 0)
	return fk
}

// assertKongEntriesIntact asserts that no Deregister happened: the route,
// legacy route and service seeded via attachFakeKong are all still present.
func assertKongEntriesIntact(t *testing.T, fk *fakeKong, key string) {
	t.Helper()
	if _, ok := fk.routes[routeName(key)]; !ok {
		t.Errorf("Kong route %s must survive (Deregister must not run)", routeName(key))
	}
	if _, ok := fk.routes[legacyRouteName(key)]; !ok {
		t.Errorf("legacy Kong route %s must survive (Deregister must not run)", legacyRouteName(key))
	}
	if _, ok := fk.services[svcName(key)]; !ok {
		t.Errorf("Kong service %s must survive (Deregister must not run)", svcName(key))
	}
}

// TestDeploy_CreateAgentFailure_CleanupPolicy pins the post-review contract:
// pre-rejections (deployer 4xx protocol validation / 503 runtime floor, both
// decided before the deployer touches Docker) must leave the existing
// container and the DB deployment status untouched, while mid-flight
// failures (5xx, network) still archive the half-created container and mark
// the deployment errored.
func TestDeploy_CreateAgentFailure_CleanupPolicy(t *testing.T) {
	// The stored token keeps resolveRuntimeToken from probing the deployer,
	// so the flow under test is: build request → create.
	const storedToken = "0123456789abcdef0123456789abcdef"

	run := func(t *testing.T, f *deployFailureFixture) error {
		srv := newDeployFailureServer(t, f)
		defer srv.Close()
		s := newTestAgentDeployerService(t, srv.URL, deployFailureAgentRepo(f, storedToken), deployTokenProviderSvc())
		_, err := s.Deploy("default", "general", false, false)
		return err
	}

	t.Run("400 pre-rejected keeps container, DB status and route", func(t *testing.T) {
		f := &deployFailureFixture{postStatus: http.StatusBadRequest}
		srv := newDeployFailureServer(t, f)
		defer srv.Close()
		s := newTestAgentDeployerService(t, srv.URL, deployFailureAgentRepo(f, storedToken), deployTokenProviderSvc())
		key := DeployKey("default", "general")
		fk := attachFakeKong(s, key)

		_, err := s.Deploy("default", "general", false, false)
		if err == nil || !strings.Contains(err.Error(), "deploy agent failed") || !strings.Contains(err.Error(), "400") {
			t.Fatalf("expected wrapped HTTP 400 deploy failure, got %v", err)
		}
		if f.deleteCalled {
			t.Error("pre-rejected (400) deploy must not archive the still-running container")
		}
		if len(f.updates) != 0 {
			t.Errorf("pre-rejected (400) deploy must not overwrite DB status; got %d update(s), first status %q", len(f.updates), f.updates[0].DeploymentStatus)
		}
		assertKongEntriesIntact(t, fk, key)
	})

	t.Run("503 pre-rejected keeps container, DB status and route", func(t *testing.T) {
		f := &deployFailureFixture{postStatus: http.StatusServiceUnavailable}
		srv := newDeployFailureServer(t, f)
		defer srv.Close()
		s := newTestAgentDeployerService(t, srv.URL, deployFailureAgentRepo(f, storedToken), deployTokenProviderSvc())
		key := DeployKey("default", "general")
		fk := attachFakeKong(s, key)

		_, err := s.Deploy("default", "general", false, false)
		if err == nil || !strings.Contains(err.Error(), "503") {
			t.Fatalf("expected HTTP 503 deploy failure, got %v", err)
		}
		if f.deleteCalled {
			t.Error("pre-rejected (503) deploy must not archive the still-running container")
		}
		if len(f.updates) != 0 {
			t.Errorf("pre-rejected (503) deploy must not overwrite DB status; got %d update(s), first status %q", len(f.updates), f.updates[0].DeploymentStatus)
		}
		assertKongEntriesIntact(t, fk, key)
	})

	t.Run("500 mid-flight failure still archives, errors and drops route", func(t *testing.T) {
		f := &deployFailureFixture{postStatus: http.StatusInternalServerError}
		srv := newDeployFailureServer(t, f)
		defer srv.Close()
		s := newTestAgentDeployerService(t, srv.URL, deployFailureAgentRepo(f, storedToken), deployTokenProviderSvc())
		key := DeployKey("default", "general")
		fk := attachFakeKong(s, key)

		_, err := s.Deploy("default", "general", false, false)
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Fatalf("expected HTTP 500 deploy failure, got %v", err)
		}
		if !f.deleteCalled {
			t.Error("mid-flight (500) failure must archive the half-created container")
		}
		if len(f.updates) != 1 || f.updates[0].DeploymentStatus != "error" {
			t.Fatalf("expected exactly one DB update setting status to error, got %d update(s)", len(f.updates))
		}
		// The archived container leaves the route without a backend: the
		// mid-flight cleanup must drop it (and this doubles as the positive
		// control proving Deregister really removes the seeded entries).
		if _, ok := fk.routes[routeName(key)]; ok {
			t.Error("mid-flight (500) failure must deregister the backend-less Kong route")
		}
		if _, ok := fk.services[svcName(key)]; ok {
			t.Error("mid-flight (500) failure must deregister the backend-less Kong service")
		}
	})

	t.Run("network failure still archives and errors", func(t *testing.T) {
		f := &deployFailureFixture{hijackPost: true}
		err := run(t, f)
		if err == nil {
			t.Fatal("expected network deploy failure, got nil")
		}
		if strings.Contains(err.Error(), "deployer returned HTTP") {
			t.Fatalf("expected transport error, got HTTPError: %v", err)
		}
		if !f.deleteCalled {
			t.Error("network failure must archive the half-created container")
		}
		if len(f.updates) != 1 || f.updates[0].DeploymentStatus != "error" {
			t.Fatalf("expected exactly one DB update setting status to error, got %d update(s)", len(f.updates))
		}
	})
}

// TestDeploy_Success_DeregistersStaleRouteAfterCreate pins the route-switch
// ordering: the stale Kong route (pointing at the previous container's port)
// is only dropped after the deployer confirms the create succeeded, and the
// async registerWhenHealthy later re-registers against the new container.
// The fixture's status endpoint never reports health "healthy" and the test
// service's active probe always fails, so the async goroutine cannot
// re-register during the test — the assertions below pin exactly the
// synchronous post-create Deregister, with no timing dependency.
func TestDeploy_Success_DeregistersStaleRouteAfterCreate(t *testing.T) {
	const storedToken = "0123456789abcdef0123456789abcdef"
	f := &deployTokenFixture{}
	srv := newDeployTokenServer(t, true, f)
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
	key := DeployKey("tenant-a", "general")
	fk := attachFakeKong(s, key)

	if _, err := s.Deploy("tenant-a", "general", false, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fk.routes[routeName(key)]; ok {
		t.Error("stale Kong route must be dropped after a successful create")
	}
	if _, ok := fk.routes[legacyRouteName(key)]; ok {
		t.Error("stale legacy Kong route must be dropped after a successful create")
	}
	if _, ok := fk.services[svcName(key)]; ok {
		t.Error("stale Kong service must be dropped after a successful create")
	}
}

// TestDeploy_GraphValidationFailure_DoesNotDeregisterKongRoute pins the
// ordering fix: buildCreateRequest (graph construction + capability
// validation, pure DB reads) runs before the Kong deregistration, so a
// validation failure — here a dangling subagent reference — returns without
// touching the gateway while the existing container keeps serving.
func TestDeploy_GraphValidationFailure_DoesNotDeregisterKongRoute(t *testing.T) {
	f := &deployFailureFixture{postStatus: http.StatusOK} // create must never be reached
	srv := newDeployFailureServer(t, f)
	defer srv.Close()

	providerID := uint64(1)
	repo := &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			if name != "general" {
				return nil, fmt.Errorf("agent %q not found", name)
			}
			return &agent.AgentConfig{ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-5-turbo"}, nil
		},
		getSubagentsFunc: func(agentID uint64) ([]string, error) {
			return []string{"missing-sub"}, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error {
			f.updates = append(f.updates, a)
			return nil
		},
	}
	s := newTestAgentDeployerService(t, srv.URL, repo, deployTokenProviderSvc())

	fk := newFakeKong()
	key := DeployKey("default", "general")
	preRegisterKongRoute(fk, key)
	s.kongSvc = NewKongGatewayService(fk, "upstream", "public", nil, 0)

	// force=true skips the existing-container GET probe; the deploy must die
	// inside buildCreateRequest on the dangling subagent reference.
	_, err := s.Deploy("default", "general", true, false)
	if err == nil {
		t.Fatal("expected graph validation error, got nil")
	}
	if !strings.Contains(err.Error(), "不存在") || !strings.Contains(err.Error(), "missing-sub") {
		t.Errorf("expected dangling-subagent error, got: %v", err)
	}
	if f.postCalled {
		t.Error("graph validation failure must not reach the deployer create call")
	}
	if f.deleteCalled {
		t.Error("graph validation failure must not archive anything")
	}
	if len(f.updates) != 0 {
		t.Errorf("graph validation failure must not touch DB status; got %d update(s)", len(f.updates))
	}
	if _, ok := fk.routes[routeName(key)]; !ok {
		t.Error("Kong route must survive a graph validation failure (Deregister must not run)")
	}
	if _, ok := fk.routes[legacyRouteName(key)]; !ok {
		t.Error("legacy Kong route must survive a graph validation failure")
	}
	if _, ok := fk.services[svcName(key)]; !ok {
		t.Error("Kong service must survive a graph validation failure")
	}
}
