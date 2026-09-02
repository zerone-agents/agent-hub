package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	providerdomain "control-panel/internal/domain/provider"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestNormalizeAgentName must mirror deployer's naming.SanitizeName exactly.
// Any drift between here and deployer breaks state reconciliation silently —
// control-panel sends "My.Agent", deployer registers "my-agent", every
// subsequent GetStatus / Stop / Start / Delete 404s.
//
// Lock the examples deployer documents (see its SanitizeName comment) so a
// future refactor of either side trips this test.
func TestNormalizeAgentName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "camel-cased", in: "MyAgent", want: "myagent"},
		{name: "already hyphenated", in: "my-agent", want: "my-agent"},
		{name: "underscore → hyphen", in: "my_agent", want: "my-agent"},
		{name: "dot → hyphen", in: "My.Agent", want: "my-agent"},
		{name: "leading/trailing spaces", in: " agent ", want: "agent"},
		{name: "collapse repeated separators", in: "my--agent", want: "my-agent"},
		{name: "mixed separators collapse", in: "My_.Agent", want: "my-agent"},
		{name: "fully lowercase passthrough", in: "coder", want: "coder"},
		{name: "digits preserved", in: "agent-01", want: "agent-01"},
		{name: "only-separators string trims to empty", in: "---", want: ""},
		{name: "empty in, empty out", in: "", want: ""},
		{name: "non-ascii chars replaced", in: "技能", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeAgentName(tc.in); got != tc.want {
				t.Errorf("NormalizeAgentName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestValidateAgentName enforces the stricter agent identifier charset: only
// lowercase letters, digits and hyphens. This is the canonical form that the
// deployer/runtime uses, so accepting anything else would create a mismatch.
func TestValidateAgentName(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "lowercase and hyphen", value: "my-agent", wantErr: false},
		{name: "digits in middle segment", value: "agent-zero-one", wantErr: false},
		{name: "digits at end", value: "agent-01", wantErr: false},
		{name: "single char", value: "a", wantErr: false},
		{name: "exactly 64", value: strings.Repeat("a", 64), wantErr: false},
		{name: "uppercase rejected", value: "MyAgent", wantErr: true},
		{name: "underscore rejected", value: "my_agent", wantErr: true},
		{name: "dot rejected", value: "my.agent", wantErr: true},
		{name: "space rejected", value: "my agent", wantErr: true},
		{name: "chinese rejected", value: "技能", wantErr: true},
		{name: "empty rejected", value: "", wantErr: true},
		{name: "too long", value: strings.Repeat("a", 65), wantErr: true},
		{name: "digit at start", value: "1agent", wantErr: true},
		{name: "hyphen at start", value: "-agent", wantErr: true},
		{name: "hyphen at end", value: "agent-", wantErr: true},
		{name: "consecutive hyphens", value: "my--agent", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAgentName(tc.value)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateAgentName(%q) expected error, got nil", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateAgentName(%q) returned unexpected error: %v", tc.value, err)
			}
		})
	}
}

// TestAgentDeployerService_NormalizesNameOnBoundary is the regression test
// for the original symptom: control-panel stored the user's original casing
// but deployer registered the lowercased/hyphenated form, so subsequent
// status queries 404'd. Hits every public AgentDeployerService entry point
// with a mixed-case input and asserts the path the deployer sees is the
// sanitised form.
func TestAgentDeployerService_NormalizesNameOnBoundary(t *testing.T) {
	tests := []struct {
		entry string
		run   func(svc *AgentDeployerService)
	}{
		{entry: "GetStatus", run: func(svc *AgentDeployerService) { _, _ = svc.GetStatus("tenant-a", "My.Agent") }},
		{entry: "Stop", run: func(svc *AgentDeployerService) { _ = svc.Stop("tenant-a", "My.Agent") }},
		{entry: "Start", run: func(svc *AgentDeployerService) { _, _ = svc.Start("tenant-a", "My.Agent") }},
		{entry: "Delete", run: func(svc *AgentDeployerService) { _ = svc.Delete("tenant-a", "My.Agent") }},
		{entry: "Purge", run: func(svc *AgentDeployerService) { _ = svc.Purge("tenant-a", "My.Agent") }},
	}

	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			var captured string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Method + " " + r.URL.Path
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"data": map[string]any{
						"status":        "running",
						"hostPort":      32768,
						"containerName": "mg-my-agent",
						"runtimeToken":  "rt-test",
					},
				})
			}))
			defer server.Close()

			now := time.Now()
			agentRepo := &mockAgentRepo{
				getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
					return &agent.AgentConfig{
						ID: 1, Name: name,
						ProviderID: uint64Ptr(1), ModelID: "m",
						CreatedAt: now, UpdatedAt: now,
					}, nil
				},
				updateFunc: func(tenantID string, a *agent.AgentConfig) error { return nil },
			}
			providerSvc := &mockProviderSvc{
				getByIDFunc: func(id uint64) (providerdomain.Provider, error) {
					p := providerdomain.NewGenericProvider("anthropic")
					if err := p.Base().SetSummary(&providerdomain.ProviderSummary{ID: 1, BaseURL: "x", Protocol: "anthropic"}); err != nil {
						return nil, err
					}
					return p, nil
				},
				getRawKeyFunc: func(id uint64) (string, error) {
					return "k", nil
				},
			}
			svc := newTestAgentDeployerService(t, server.URL, agentRepo, providerSvc)

			tt.run(svc)

			// Every deployer route uses /api/v1/agents/<deployKey>...; the
			// captured path must contain the tenant-scoped sanitised key
			// "tenant-a-my-agent" (DeployKey("tenant-a", "My.Agent")), never
			// the original "My.Agent" (which would contain a literal dot in
			// the URL).
			if captured == "" {
				t.Fatalf("no request captured for %s", tt.entry)
			}
			const wantSegment = "/api/v1/agents/tenant-a-my-agent"
			if !strings.Contains(captured, wantSegment) {
				t.Errorf("%s: deployer saw %q, want it to contain %q (original \"My.Agent\" should be sanitised)",
					tt.entry, captured, wantSegment)
			}
			if strings.Contains(captured, "My.Agent") || strings.Contains(captured, "My") {
				t.Errorf("%s: deployer saw original casing %q — name was not sanitised before crossing the boundary",
					tt.entry, captured)
			}
		})
	}
}

// setupAgentValidatorTestDB spins up an in-memory sqlite DB with the
// provider tables migrated, and points the package-global database.DB at
// it for the duration of the test. agent_validator.go reaches directly
// into database.DB (same pattern as validateFieldOverridesKeys), so the
// tests have to do the same.
func setupAgentValidatorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&providerdomain.ProviderSummary{}, &providerdomain.ProviderAttribute{}, &providerdomain.ProviderModel{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	return db
}

// TestValidateConfig_MaxTurnsBounds pins the maxTurns range contract shared
// with the frontend form (1..500). 0/negative and >500 must be rejected;
// the boundary value 500 and an absent field must pass.
func TestValidateConfig_MaxTurnsBounds(t *testing.T) {
	cases := []struct {
		name    string
		turns   float64
		wantErr string
	}{
		{"boundary max ok", 500, ""},
		{"above max rejected", 501, "maxTurns 不能超过 500"},
		{"negative rejected", -1, "maxTurns 不能为负数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfig(map[string]interface{}{"maxTurns": tc.turns})
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}

	// Absent field is not validated (legacy payloads may omit it).
	require.NoError(t, ValidateConfig(map[string]interface{}{}))
}

// TestValidateConfig_RejectsNonLLMModel is the Task 6 regression: the
// validator must check the bound model's type via provider_models, not
// the provider's top-level type. Under the old validator, binding an
// agent to an embedding-class model under an LLM-typed provider slipped
// through; under the new validator it must be rejected.
func TestValidateConfig_RejectsNonLLMModel(t *testing.T) {
	db := setupAgentValidatorTestDB(t)

	// Create provider
	require.NoError(t, db.Create(&providerdomain.ProviderSummary{
		Key: "glm", Name: "GLM", Protocol: "anthropic",
		AuthStyle: "api_key", BaseURL: "http://x",
	}).Error)
	var providerID uint64 = 1

	// Create two models: one llm, one embedding
	require.NoError(t, db.Create(&providerdomain.ProviderModel{
		ProviderID: providerID, SelectionID: "glm-4", ModelID: "glm-4",
		DisplayName: "GLM-4", ModelType: "llm", Status: "1",
	}).Error)
	require.NoError(t, db.Create(&providerdomain.ProviderModel{
		ProviderID: providerID, SelectionID: "emb", ModelID: "embedding-3",
		DisplayName: "Emb", ModelType: "embedding", Status: "1",
	}).Error)

	// Binding to llm model: OK
	err := ValidateConfig(map[string]interface{}{
		"systemPrompt": "x", "providerId": float64(providerID), "modelId": "glm-4",
	})
	require.NoError(t, err)

	// Binding to embedding model: rejected
	err = ValidateConfig(map[string]interface{}{
		"systemPrompt": "x", "providerId": float64(providerID), "modelId": "embedding-3",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "不是 LLM")

	// Binding to non-existent model: rejected
	err = ValidateConfig(map[string]interface{}{
		"systemPrompt": "x", "providerId": float64(providerID), "modelId": "ghost",
	})
	require.Error(t, err)
}

// TestValidateConfig_ProviderModelEdgeCases nails down the remaining
// branches of validateProviderModel: missing provider, empty modelId
// (preserves legacy "modelId optional" behaviour), and the
// database.DB == nil fast path.
func TestValidateConfig_ProviderModelEdgeCases(t *testing.T) {
	t.Run("provider does not exist", func(t *testing.T) {
		db := setupAgentValidatorTestDB(t)
		_ = db
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "providerId": float64(999), "modelId": "any",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "providerId 999 不存在")
	})

	t.Run("empty modelId skips model check", func(t *testing.T) {
		db := setupAgentValidatorTestDB(t)
		require.NoError(t, db.Create(&providerdomain.ProviderSummary{
			ID: 1, Key: "glm", Name: "GLM", Protocol: "anthropic",
			AuthStyle: "api_key", BaseURL: "http://x",
		}).Error)
		// No models row at all, but modelId is empty → must pass.
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "providerId": float64(1),
		})
		require.NoError(t, err)
	})

	t.Run("no providerId skips check entirely", func(t *testing.T) {
		setupAgentValidatorTestDB(t)
		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x",
		})
		require.NoError(t, err)
	})

	t.Run("database.DB nil is a no-op", func(t *testing.T) {
		previousDB := database.DB
		database.DB = nil
		t.Cleanup(func() { database.DB = previousDB })

		err := ValidateConfig(map[string]interface{}{
			"systemPrompt": "x", "providerId": float64(1), "modelId": "anything",
		})
		require.NoError(t, err)
	})
}
