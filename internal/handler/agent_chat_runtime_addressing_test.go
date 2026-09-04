package handler

import (
	"fmt"
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
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.UploadRecord{}, &agent.AgentConfig{}))
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
	t.Helper()

	// Fake runtime: asserts the agent name used in the URL path and streams
	// a minimal SSE response that ends with a success result event.
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*runtimeHitPath = r.URL.Path
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
		switch {
		case r.URL.Path == "/api/v1/agents/"+deployKey && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","hostPort":%d,"runtimeToken":"rt-secret","containerId":"ctr-1"}}`, deployKey, port)))
		case r.URL.Path == "/api/v1/agents/"+deployKey+"/status" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","health":"healthy","hostPort":%d,"containerId":"ctr-1"}}`, deployKey, port)))
		default:
			t.Errorf("unexpected deployer request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
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

// Session-bound-to-other-agent requests must 404 BEFORE persisting anything
// or dialing the runtime (issue #94 acceptance #1; previously :name was only
// used for runtime addressing, so a message could land in A's session while
// streaming from B's runtime).
func TestSendMessage_RejectsSessionBoundToOtherAgent(t *testing.T) {
	var runtimePath string
	h := newAgentChatHandlerWithFakes(t, &runtimePath)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"content":"hi"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	// s-run is bound to agent "min"; address it under a different agent name.
	c.Params = gin.Params{{Key: "name", Value: "other"}, {Key: "id", Value: "s-run"}}

	h.SendMessage(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Empty(t, runtimePath, "runtime must not be dialed")
	var count int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ?", "s-run").Count(&count).Error)
	require.Zero(t, count, "no message may be persisted for a binding mismatch")
}
