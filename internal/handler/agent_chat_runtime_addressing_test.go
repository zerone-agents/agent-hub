package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/chat"
	"control-panel/internal/infrastructure/deployer"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/infrastructure/runtime"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file verifies the runtime addressing contract after the v3.1
// deploymentKey split (issue #114): the runtime container registers agents
// under their bare agent id (e.g. "min"), so the chat handler must call
// StreamRun with the bare name; the tenant-scoped deploy key is a deployer
// resource id only (the deployer fake below keeps scoped lifecycle paths).
//
// The test wires the REAL AgentChatService + AgentDeployerService +
// runtime.Client against:
//   - a fake deployer HTTP server (returns running/healthy, hostPort),
//   - a fake runtime HTTP server (asserts the request path contains the
//     deploy key, replies with a minimal SSE stream),
//   - an in-memory sqlite DB (chat tables + agent config).

// setupRuntimeAddressingDB migrates chat + agent tables and seeds one agent
// ("min", tenant-a, running on the fake deployer) and one chat session.
// encryptionKey is empty so RuntimeToken is stored/read as plaintext.
func setupRuntimeAddressingDB(t *testing.T, hostPort int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &agent.AgentConfig{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	require.NoError(t, db.Create(&agent.AgentConfig{
		Name:             "min",
		TenantID:         chatTestTenant,
		ContentHash:      "hash-1",
		SystemPrompt:     "you are min",
		DeploymentStatus: "running",
		RuntimePort:      hostPort,
		RuntimeToken:     "rt-secret",
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID:   "u1",
		TenantID: chatTestTenant,
		ID:       "s-run",
		Title:    "t",
		AgentID:  "min",
	}).Error)
}

// newAgentChatHandlerWithFakes builds the production handler wired to a fake
// deployer + fake runtime. Returns the handler and the deploy key the runtime
// saw (filled when the runtime is hit).
func newAgentChatHandlerWithFakes(t *testing.T, runtimeHitPath *string) *AgentChatHandler {
	return newAgentChatHandlerWithFakesAndBody(t, runtimeHitPath, nil)
}

// newAgentChatHandlerWithFakesAndBody is the body-capturing variant: the fake
// runtime additionally records the raw run request body into runtimeBody
// (filled on every hit) so tests can assert what the hub sends to the
// runtime — e.g. that maxSessionQueries is carried when configured.
func newAgentChatHandlerWithFakesAndBody(t *testing.T, runtimeHitPath *string, runtimeBody *string) *AgentChatHandler {
	t.Helper()

	// Fake runtime: asserts the agent name used in the URL path and streams
	// a minimal SSE response that ends with a success result event.
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*runtimeHitPath = r.URL.Path
		if runtimeBody != nil {
			b, _ := io.ReadAll(r.Body)
			*runtimeBody = string(b)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: system\ndata: {\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"rs-1\"}\n\n" +
			"event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n"))
	}))
	t.Cleanup(runtimeSrv.Close)

	// The deployer reports the fake runtime's actual host port so the
	// resolved base URL (http://127.0.0.1:<port>) points at runtimeSrv.
	port := portOfServerURL(t, runtimeSrv.URL)
	setupRuntimeAddressingDB(t, port)

	deployKey := services.DeployKey(chatTestTenant, "min")
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Generic agent resolution: any tenant-scoped deploy key resolves to
		// the fake runtime (the addressing fixture seeds "min"; the
		// session-cap fixture seeds additional agents like "min-cap").
		if r.Method == http.MethodGet {
			switch {
			case strings.HasSuffix(r.URL.Path, "/status"):
				name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"), "/status")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","health":"healthy","hostPort":%d}}`, name, port)))
				return
			case strings.HasPrefix(r.URL.Path, "/api/v1/agents/"):
				name := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","hostPort":%d,"runtimeToken":"rt-secret"}}`, name, port)))
				return
			}
		}
		if r.URL.Path == "/api/v1/agents/"+deployKey && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","hostPort":%d,"runtimeToken":"rt-secret"}}`, deployKey, port)))
			return
		}
		if r.URL.Path == "/api/v1/agents/"+deployKey+"/status" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","health":"healthy","hostPort":%d}}`, deployKey, port)))
			return
		}
		t.Errorf("unexpected deployer request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(404)
	}))
	t.Cleanup(deployerSrv.Close)

	deployerSvc := services.NewAgentDeployerService(services.AgentDeployerConfig{
		Client:     deployer.NewClient(deployerSrv.URL, ""),
		PublicHost: "127.0.0.1", // no-Kong fallback host (fake runtime)
		// UpstreamHost/CDNHost/EncryptionKey/RuntimeAPIKey/
		// CapabilitySecret/chat push 均留空：无 knowledge MCP、无 agent graph
		AuthMode: services.ModeCasdoor, // casdoor-shaped addressing fixture
	})
	chatSvc := services.NewAgentChatService(
		repository.NewChatRepository(),
		repository.NewAgentRepository(),
		deployerSvc,
		runtime.NewClient(),
		"127.0.0.1", "",
		"127.0.0.1", // upstreamHost: no-Kong fallback must hit the fake runtime
	)
	return NewAgentChatHandler(chatSvc)
}

func portOfServerURL(t *testing.T, rawURL string) int {
	t.Helper()
	parts := strings.Split(rawURL, ":")
	require.Len(t, parts, 3)
	var port int
	_, err := fmt.Sscanf(parts[2], "%d", &port)
	require.NoError(t, err)
	return port
}

func TestSendMessage_AddressesRuntimeWithBareAgentID(t *testing.T) {
	var runtimePath string
	h := newAgentChatHandlerWithFakes(t, &runtimePath)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"content":"hi"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: "s-run"}}

	h.SendMessage(c)

	require.Equal(t, http.StatusOK, w.Code)
	// The runtime must be addressed by the bare agent id (issue #114); the
	// scoped deploy key never appears in runtime API paths.
	require.Equal(t, "/v1/agents/min/runs", runtimePath)
	// The SSE stream was piped through to the client.
	require.Contains(t, w.Body.String(), `"subtype":"success"`)
}

// TestSendMessage_CarriesMaxSessionQueriesWhenConfigured pins the run-body
// contract the SDK compaction depends on (issue #111): the hub must send the
// agent's maxSessionQueries in the run body when configured. The runtime
// forwards body.maxSessionQueries into the SDK query overrides on EVERY run —
// including an implicit undefined when absent — and the SDK spread merge lets
// that undefined clobber the agents.yaml value, silently disabling session
// compaction (verified end-to-end: with the field the Nth over-limit query
// emits compact/progress+end; without it, never).
func TestSendMessage_CarriesMaxSessionQueriesWhenConfigured(t *testing.T) {
	var runtimeHitPath, runtimeBody string
	h := newAgentChatHandlerWithFakesAndBody(t, &runtimeHitPath, &runtimeBody)

	// Seed a second agent with a session cap; reuse min's runtime port so
	// both agents resolve to the same fake runtime.
	var minCfg agent.AgentConfig
	require.NoError(t, database.DB.Where("name = ?", "min").First(&minCfg).Error)
	capVal := 3
	require.NoError(t, database.DB.Create(&agent.AgentConfig{
		Name:              "min-cap",
		TenantID:          chatTestTenant,
		ContentHash:       "hash-2",
		SystemPrompt:      "you are min-cap",
		DeploymentStatus:  "running",
		RuntimePort:       minCfg.RuntimePort,
		RuntimeToken:      "rt-secret",
		MaxSessionQueries: &capVal,
	}).Error)
	require.NoError(t, database.DB.Create(&chat.Session{
		UserID:   "u1",
		TenantID: chatTestTenant,
		ID:       "s-run-cap",
		Title:    "t",
		AgentID:  "min-cap",
	}).Error)

	send := func(agentName, sessionID string) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"content":"hi"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", "u1")
		c.Set("tenant_id", chatTestTenant)
		c.Params = gin.Params{{Key: "name", Value: agentName}, {Key: "id", Value: sessionID}}
		h.SendMessage(c)
		require.Equal(t, http.StatusOK, w.Code)
	}

	// Unconfigured agent: the key must be ABSENT (sending an explicit
	// undefined would clobber nothing here, but the key must not be sent
	// as a bare null either — the contract is "present only when set").
	send("min", "s-run")
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(runtimeBody), &body))
	_, present := body["maxSessionQueries"]
	require.False(t, present, "run body must not carry maxSessionQueries when unset: %s", runtimeBody)

	// Configured agent: the cap value must reach the runtime verbatim.
	send("min-cap", "s-run-cap")
	require.NoError(t, json.Unmarshal([]byte(runtimeBody), &body))
	got, present := body["maxSessionQueries"]
	require.True(t, present, "run body must carry maxSessionQueries when configured: %s", runtimeBody)
	require.Equal(t, float64(3), got)
}
