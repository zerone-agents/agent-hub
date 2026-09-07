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

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/internal/domain/skill"
	"control-panel/internal/infrastructure/deployer"
	"control-panel/internal/infrastructure/kong"
	repository "control-panel/internal/infrastructure/persistence"
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

type mockToolRepo struct {
	tools   []*agent.Tool
	byAgent map[uint64][]*agent.Tool
}

func (m *mockToolRepo) GetToolRecordsByAgent(agentID uint64) ([]*agent.Tool, error) {
	if m.byAgent != nil {
		return m.byAgent[agentID], nil
	}
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
// /api/v1/agents always succeeds and echoes a container payload unless
// failCreate is set, in which case the create call is answered with a 500
// mid-flight failure (deployerPreRejected must treat it as non-pre-rejected).
// POSTs without a deploymentKey are capability probes (issue #114) — answered
// with the v3.1.0 sentinel and kept out of the create capture.
func newDeployTokenServer(t *testing.T, getFound, failCreate bool, f *deployTokenFixture) *httptest.Server {
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
		body, _ := io.ReadAll(r.Body)
		var probe struct {
			DeploymentKey string `json:"deploymentKey"`
		}
		_ = json.Unmarshal(body, &probe)
		if probe.DeploymentKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"success":false,"error":"deploymentKey is required"}`))
			return
		}
		if failCreate {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"success":false,"error":"boom"}`))
			return
		}
		f.postCalled = true
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
		srv := newDeployTokenServer(t, true, false, f)
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
		srv := newDeployTokenServer(t, true, false, f)
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
		srv := newDeployTokenServer(t, true, false, f)
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
		srv := newDeployTokenServer(t, false, false, f) // GET probe: no existing container
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
		srv := newDeployTokenServer(t, true, false, f) // GET probe: container exists
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
		srv := newDeployTokenServer(t, true, false, f)
		defer srv.Close()

		s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, storedToken), deployTokenProviderSvc())
		// The knowledge MCP mounted below now also receives a signed
		// per-agent capability — issuance requires the server-held
		// capability secret.
		s.capabilitySecret = testCapabilitySecret
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
		srv := newDeployTokenServer(t, true, false, f)
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
		srv := newDeployTokenServer(t, true, false, f)
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
	// v3.1 split (issue #114): rootAgentId is the bare runtime id and must
	// match the (single) root definition; the scoped key moved to the
	// dedicated deploymentKey field.
	require.Equal(t, "general", req.RootAgentID)
	require.Equal(t, "t-general", req.DeploymentKey)
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
			// Capability probes (no deploymentKey, issue #114) get the v3.1.0
			// sentinel so the gate passes; only real creates are recorded and
			// subjected to the fixture's failure behavior.
			body, _ := io.ReadAll(r.Body)
			var probe struct {
				DeploymentKey string `json:"deploymentKey"`
			}
			_ = json.Unmarshal(body, &probe)
			if probe.DeploymentKey == "" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"success":false,"error":"deploymentKey is required"}`))
				return
			}
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
	s.kongSvc = NewKongGatewayService(fk, "upstream", "public", nil, 0, ModeCasdoor)
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
	srv := newDeployTokenServer(t, true, false, f)
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
	s.kongSvc = NewKongGatewayService(fk, "upstream", "public", nil, 0, ModeCasdoor)

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

// newSnapshotTestRepo builds an in-memory sqlite-backed DeploymentSnapshot
// repository for the artifact-snapshot tests below.
func newSnapshotTestRepo(t *testing.T) *repository.DeploymentSnapshotRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&agent.DeploymentSnapshot{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repository.NewDeploymentSnapshotRepositoryWithDB(db)
}

// snapshotSkillRepo returns per-agent skill records; used to verify that a
// subagent node's mounted skills contribute their hashes to the snapshot.
type snapshotSkillRepo struct{ byAgent map[uint64][]*skill.Skill }

func (m *snapshotSkillRepo) GetAgentSkills(agentID uint64) ([]string, error) { return nil, nil }
func (m *snapshotSkillRepo) GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error) {
	return m.byAgent[agentID], nil
}

// TestDeploy_WritesArtifactSnapshot covers the issue #86 success path: a
// successful deploy must record the artifact hashes of everything sent to the
// deployer (collected from the same request graph, root + subagent nodes).
func TestDeploy_WritesArtifactSnapshot(t *testing.T) {
	snapRepo := newSnapshotTestRepo(t)

	f := &deployTokenFixture{}
	srv := newDeployTokenServer(t, true, false, f) // fake deployer returns running
	defer srv.Close()

	providerID := uint64(1)
	agentRepo := &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			if name == "child" {
				return &agent.AgentConfig{ID: 2, Name: "child"}, nil
			}
			return &agent.AgentConfig{
				ID: 1, Name: "general", ProviderID: &providerID, ModelID: "glm-5-turbo",
				RuntimeToken: "0123456789abcdef0123456789abcdef",
			}, nil
		},
		getSubagentsFunc: func(agentID uint64) ([]string, error) {
			if agentID == 1 {
				return []string{"child"}, nil
			}
			return nil, nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error { return nil },
	}
	s := newTestAgentDeployerService(t, srv.URL, agentRepo, deployTokenProviderSvc())
	// 注入可查的工具：custom && ready（FileHash 非空即 ready）
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/abc", FileHash: "abc123", FileSize: 10},
	}}
	s.cdnHost = "https://cdn.example.com" // custom&&ready 工具须有 CDN host 才能通过 buildCreateRequest
	// 仅子 Agent(ID 2) 挂载 skill：SkillHashes 必须来自 subagent 节点
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{
		2: {{Name: "websearch", URL: "https://cdn.example.com/skills/websearch/s1", FileHash: "s1"}},
	}}
	s.snapshotRepo = snapRepo

	dto, err := s.Deploy("tenant-a", "general", true, false)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if dto.Status != "running" {
		t.Fatalf("status = %q", dto.Status)
	}

	snap, err := snapRepo.GetByAgent(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if snap.ToolHashes["calc"] != "abc123" {
		t.Fatalf("tool hash = %q, want abc123", snap.ToolHashes["calc"])
	}
	if snap.SkillHashes["websearch"] != "s1" {
		t.Fatalf("subagent skill hash = %q, want s1", snap.SkillHashes["websearch"])
	}
	if snap.DeployedAt.IsZero() {
		t.Fatal("snapshot DeployedAt must be set")
	}
}

// failingSnapshotRepo always fails Upsert; used to verify that a snapshot
// write failure never blocks the deploy flow itself.
type failingSnapshotRepo struct{}

func (r *failingSnapshotRepo) Upsert(ctx context.Context, s *agent.DeploymentSnapshot) error {
	return fmt.Errorf("boom")
}
func (r *failingSnapshotRepo) GetByAgent(ctx context.Context, tenantID string, agentID uint64) (*agent.DeploymentSnapshot, error) {
	return nil, nil
}

// TestDeploy_SnapshotWriteFailureDoesNotBlockDeploy covers the issue #86
// resilience contract: the snapshot is an auxiliary record — a failed upsert
// must be logged and ignored, not fail the deploy.
func TestDeploy_SnapshotWriteFailureDoesNotBlockDeploy(t *testing.T) {
	f := &deployTokenFixture{}
	srv := newDeployTokenServer(t, true, false, f) // fake deployer returns running
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, "0123456789abcdef0123456789abcdef"), deployTokenProviderSvc())
	s.snapshotRepo = &failingSnapshotRepo{}

	dto, err := s.Deploy("tenant-a", "general", true, false)
	if err != nil {
		t.Fatalf("deploy must succeed despite snapshot write failure: %v", err)
	}
	if dto.Status != "running" {
		t.Fatalf("status = %q", dto.Status)
	}
}

// TestDeploy_FailureKeepsExistingSnapshot covers the issue #86 failure path: a
// failed deploy (mid-flight 5xx, non pre-rejection) must leave a previously
// recorded snapshot untouched — the snapshot only advances on success.
func TestDeploy_FailureKeepsExistingSnapshot(t *testing.T) {
	snapRepo := newSnapshotTestRepo(t)
	if err := snapRepo.Upsert(context.Background(), &agent.DeploymentSnapshot{
		AgentID: 1, TenantID: "tenant-a", DeployedAt: time.Now(),
		ToolHashes: map[string]string{"calc": "old456"},
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	// fake deployer 返回 5xx（非预拒绝）→ Deploy 失败
	f := &deployTokenFixture{}
	srv := newDeployTokenServer(t, false, true, f) // failCreate: POST create returns 500
	defer srv.Close()

	s := newTestAgentDeployerService(t, srv.URL, deployTokenAgentRepo(f, "0123456789abcdef0123456789abcdef"), deployTokenProviderSvc())
	s.snapshotRepo = snapRepo

	if _, err := s.Deploy("tenant-a", "general", true, false); err == nil {
		t.Fatal("expected deploy error")
	}
	snap, err := snapRepo.GetByAgent(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("snapshot lost: %v", err)
	}
	if snap.ToolHashes["calc"] != "old456" {
		t.Fatalf("hash = %q, want old456 (failure must not overwrite)", snap.ToolHashes["calc"])
	}
}

// ── Task 4: ComputePendingArtifacts ──────────────────────────────────

// computePendingFixture 构建 Task 4 的最小 fixture：真实 sqlite snapshot
// repo + mockToolRepo + snapshotSkillRepo，可注入快照与当前绑定。
func computePendingFixture(t *testing.T, snapToolHashes, snapSkillHashes map[string]string) *AgentDeployerService {
	t.Helper()
	snapRepo := newSnapshotTestRepo(t)
	if snapToolHashes != nil || snapSkillHashes != nil {
		if err := snapRepo.Upsert(context.Background(), &agent.DeploymentSnapshot{
			AgentID: 1, TenantID: "tenant-a", DeployedAt: time.Now(),
			ToolHashes: snapToolHashes, SkillHashes: snapSkillHashes,
		}); err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
	}
	// agentRepo 默认空 mock：GetSubagents 返回 nil（无 subagent），保持既有
	// 用例语义；subagent 闭包场景的用例自行覆盖 agentRepo。
	s := &AgentDeployerService{agentRepo: &mockAgentRepo{}, snapshotRepo: snapRepo}
	return s
}

// TestComputePendingArtifacts_NoDifference 覆盖 brief 基础场景：快照与当前
// 绑定哈希一致 → 空结果（且非 nil）。builtin 工具不入比对集合（source 过滤）。
func TestComputePendingArtifacts_NoDifference(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "abc"}, nil)
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "abc", FileSize: 10},
		{Name: "git", Source: agent.ToolSourceBuiltin},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{
		1: {{Name: "qa", URL: "https://cdn.example.com/skills/qa/def", FileHash: "def"}},
	}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("deployed agent with snapshot must return non-nil result")
	}
	if len(got.Tools) != 0 || len(got.Skills) != 1 {
		t.Fatalf("pending = {tools:%v skills:%v}, want {tools:[] skills:[qa]}", got.Tools, got.Skills)
	}
	if len(got.Skills) > 0 && got.Skills[0] != "qa" {
		t.Fatalf("skills = %v, want [qa]", got.Skills)
	}
}

// TestComputePendingArtifacts_NoSnapshotReturnsNilNil 覆盖 brief 语义：
// 无快照行（未部署过）→ (nil, nil)，调用方可据此跳过差异提示。
func TestComputePendingArtifacts_NoSnapshotReturnsNilNil(t *testing.T) {
	s := computePendingFixture(t, nil, nil)
	s.toolRepo = &mockToolRepo{}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("pending = %+v, want nil (no snapshot)", got)
	}
}

// TestComputePendingArtifacts_ContentReplacementReported 覆盖 brief 行为点：
// 快照 calc=old、当前 calc=new → tools=[calc]。
func TestComputePendingArtifacts_ContentReplacementReported(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "old"}, nil)
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "new", FileSize: 10},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "calc" {
		t.Fatalf("tools = %v, want [calc]", got.Tools)
	}
	if len(got.Skills) != 0 {
		t.Fatalf("skills = %v, want []", got.Skills)
	}
}

// TestComputePendingArtifacts_MetadataEditNotReported 覆盖 brief 行为点：
// 仅 title/description 等元数据变化、哈希不变 → 不报差异（比对维度只有哈希）。
func TestComputePendingArtifacts_MetadataEditNotReported(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "abc"}, map[string]string{"qa": "def"})
	// FileName/FileURL 变化不算内容变化（哈希相同）；skill 侧 title 编辑同源。
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc-v2.ts", FileURL: "tools/acme/calc/v2", FileHash: "abc", FileSize: 10},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{
		1: {{Name: "qa", Title: "QA 新标题", URL: "https://cdn.example.com/skills/qa/def", FileHash: "def"}},
	}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tools) != 0 || len(got.Skills) != 0 {
		t.Fatalf("pending = {tools:%v skills:%v}, want empty (metadata edit only)", got.Tools, got.Skills)
	}
}

// TestComputePendingArtifacts_RemovedBindingNotReported 覆盖 brief 行为点：
// 原绑定（快照含 calc/qa）已解绑、当前为空 → 不报（diff 仅回报当前集合中缺
// 失或哈希不同的项）。
func TestComputePendingArtifacts_RemovedBindingNotReported(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "abc"}, map[string]string{"qa": "def"})
	s.toolRepo = &mockToolRepo{}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("non-nil result expected")
	}
	if len(got.Tools) != 0 || len(got.Skills) != 0 {
		t.Fatalf("pending = {tools:%v skills:%v}, want empty (removed bindings not reported)", got.Tools, got.Skills)
	}
}

// TestComputePendingArtifacts_EmptyResultSerializesAsEmptyArray 覆盖 brief
// 修正点：空集合必须序列化为 [] 而非 null（「空数组=无差异」spec 语义，null
// 保留给未部署场景）。
func TestComputePendingArtifacts_EmptyResultSerializesAsEmptyArray(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "abc"}, nil)
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "abc", FileSize: 10},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `{"tools":[],"skills":[]}` {
		t.Fatalf("json = %s, want {\"tools\":[],\"skills\":[]} (empty arrays, not null)", raw)
	}
}

// TestComputePendingArtifacts_CrossTenantIsolation 覆盖 brief 行为点：快照
// 归 tenant-a，tenant-b 查询同一 agentID → 视同无快照 (nil, nil)。
func TestComputePendingArtifacts_CrossTenantIsolation(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "abc"}, nil)
	s.toolRepo = &mockToolRepo{}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-b", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("pending = %+v, want nil (cross-tenant snapshot isolation)", got)
	}
}

// TestComputePendingArtifacts_SubagentArtifactsReported 覆盖结合 review 后的
// 读侧闭包（I-1）：subagent 独有的 tool/skill 绑定哈希与快照不符 → pending
// 报告 subagent 工件名（root 无绑定也能命中）。写侧 collectArtifactHashes 已
// 记录 subagent 哈希，读侧必须同构遍历 subagent closure。
func TestComputePendingArtifacts_SubagentArtifactsReported(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"sub-calc": "old"}, map[string]string{"sub-qa": "old"})
	s.agentRepo = &mockAgentRepo{
		getSubagentsFunc: func(agentID uint64) ([]string, error) {
			if agentID != 1 {
				t.Fatalf("GetSubagents(%d), want 1 (root)", agentID)
			}
			return []string{"research"}, nil
		},
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			if tenantID != "tenant-a" || name != "research" {
				t.Fatalf("GetByName(%q, %q), want (tenant-a, research)", tenantID, name)
			}
			return &agent.AgentConfig{ID: 2, Name: "research"}, nil
		},
	}
	s.toolRepo = &mockToolRepo{byAgent: map[uint64][]*agent.Tool{
		1: nil, // root 无绑定
		2: {{Name: "sub-calc", Source: agent.ToolSourceCustom, FileName: "sub-calc.ts", FileURL: "tools/acme/sub-calc/sub-calc.ts", FileHash: "new", FileSize: 10}},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{
		1: nil,
		2: {{Name: "sub-qa", URL: "https://cdn.example.com/skills/sub-qa/new", FileHash: "new"}},
	}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "sub-calc" {
		t.Fatalf("tools = %v, want [sub-calc] (subagent tool hash drift)", got.Tools)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "sub-qa" {
		t.Fatalf("skills = %v, want [sub-qa] (subagent skill hash drift)", got.Skills)
	}
}

// TestComputePendingArtifacts_DeletedSubagentSkipped 覆盖 fail-open 语义
// （I-1）：GetSubagents 返回的名字中某个已删除（GetByName 未找到）→ 跳过该
// 节点记一行日志，其余节点（root + 存活的 subagent）继续正常比对，不整体失败。
func TestComputePendingArtifacts_DeletedSubagentSkipped(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "old"}, nil)
	s.agentRepo = &mockAgentRepo{
		getSubagentsFunc: func(agentID uint64) ([]string, error) {
			return []string{"ghost", "research2"}, nil
		},
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			if name == "ghost" {
				return nil, gorm.ErrRecordNotFound
			}
			return &agent.AgentConfig{ID: 3, Name: "research2"}, nil
		},
	}
	s.toolRepo = &mockToolRepo{byAgent: map[uint64][]*agent.Tool{
		1: {{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "new", FileSize: 10}},
		3: {{Name: "sub-x", Source: agent.ToolSourceCustom, FileName: "sub-x.ts", FileURL: "tools/acme/sub-x/sub-x.ts", FileHash: "abc", FileSize: 10}},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v (deleted subagent must not fail the diff)", err)
	}
	if len(got.Tools) != 2 || got.Tools[0] != "calc" || got.Tools[1] != "sub-x" {
		t.Fatalf("tools = %v, want [calc sub-x] (root + live subagent still diffed)", got.Tools)
	}
}

// TestComputePendingArtifactsByName_Delegates 覆盖 brief 行为点：ByName 经
// GetByName 解析 agent ID 后委托 ComputePendingArtifacts，结果与直呼一致。
func TestComputePendingArtifactsByName_Delegates(t *testing.T) {
	s := computePendingFixture(t, map[string]string{"calc": "old"}, nil)
	s.agentRepo = &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			if tenantID != "tenant-a" || name != "general" {
				t.Fatalf("GetByName(%q, %q)", tenantID, name)
			}
			return &agent.AgentConfig{ID: 1, Name: "general"}, nil
		},
	}
	s.toolRepo = &mockToolRepo{tools: []*agent.Tool{
		{Name: "calc", Source: agent.ToolSourceCustom, FileName: "calc.ts", FileURL: "tools/acme/calc/calc.ts", FileHash: "new", FileSize: 10},
	}}
	s.skillRepo = &snapshotSkillRepo{byAgent: map[uint64][]*skill.Skill{}}

	got, err := s.ComputePendingArtifactsByName(context.Background(), "tenant-a", "general")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "calc" {
		t.Fatalf("tools = %v, want [calc] (delegated by name)", got.Tools)
	}
}

// TestComputePendingArtifactsByName_NotFound 覆盖错误路径：GetByName 失败
// → 包装错误返回。
func TestComputePendingArtifactsByName_NotFound(t *testing.T) {
	s := computePendingFixture(t, nil, nil)
	s.agentRepo = &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	got, err := s.ComputePendingArtifactsByName(context.Background(), "tenant-a", "ghost")
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	if got != nil {
		t.Fatalf("pending = %+v, want nil on error", got)
	}
}

// erroringSnapshotRepo 的 GetByAgent 返回非 NotFound 错误，模拟 DB 故障。
type erroringSnapshotRepo struct{}

func (r *erroringSnapshotRepo) Upsert(ctx context.Context, s *agent.DeploymentSnapshot) error {
	return nil
}
func (r *erroringSnapshotRepo) GetByAgent(ctx context.Context, tenantID string, agentID uint64) (*agent.DeploymentSnapshot, error) {
	return nil, fmt.Errorf("snapshot db down")
}

// TestComputePendingArtifacts_RepoErrorPropagates 覆盖错误路径：快照读取的
// 非 NotFound 错误（DB 故障）→ (nil, err)。
func TestComputePendingArtifacts_RepoErrorPropagates(t *testing.T) {
	s := &AgentDeployerService{snapshotRepo: &erroringSnapshotRepo{}}
	got, err := s.ComputePendingArtifacts(context.Background(), "tenant-a", 1)
	if err == nil {
		t.Fatal("expected error from snapshot repo")
	}
	if got != nil {
		t.Fatalf("pending = %+v, want nil on error", got)
	}
}
