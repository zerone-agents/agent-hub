// internal/handler/agent_chat_attachments_test.go
package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

// attachmentFakeOpts configures the fake runtime in newAttachmentChatEnv.
type attachmentFakeOpts struct {
	// runtimeVersion is what GET /health reports. Empty → 无 /health 路由
	// （探测失败 → 附件不可用）。
	runtimeVersion string
	// uploadHandler serves POST /v1/files/uploads. nil → 404（旧 runtime）。
	uploadHandler http.HandlerFunc
	// runsHandler serves POST /v1/agents/{key}/runs. nil → 最小成功 SSE 流。
	runsHandler http.HandlerFunc
	// contentHandler serves GET /v1/files/content. nil → 404.
	contentHandler http.HandlerFunc
}

// attachmentChatEnv wires the production handler against fake deployer +
// fake runtime + in-memory sqlite（复刻 agent_chat_runtime_addressing_test.go
// 模式）。Seeds：agent "min"（running，fake runtime 端口）、session
// "s-att"（u1 所有，绑定 min）、"s-u2"（u2 所有，非 owner 路径）。
type attachmentChatEnv struct {
	handler *AgentChatHandler
	runtime *httptest.Server
}

func newAttachmentChatEnv(t *testing.T, opts attachmentFakeOpts) *attachmentChatEnv {
	t.Helper()

	mux := http.NewServeMux()
	if opts.runtimeVersion != "" {
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","version":%q}`, opts.runtimeVersion)))
		})
	}
	if opts.uploadHandler != nil {
		mux.HandleFunc("/v1/files/uploads", opts.uploadHandler)
	}
	if opts.contentHandler != nil {
		mux.HandleFunc("/v1/files/content", opts.contentHandler)
	}
	runsHandler := opts.runsHandler
	if runsHandler == nil {
		runsHandler = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: system\ndata: {\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"rs-1\"}\n\n" +
				"event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n"))
		}
	}
	mux.HandleFunc("/v1/agents/", runsHandler)
	runtimeSrv := httptest.NewServer(mux)
	t.Cleanup(runtimeSrv.Close)

	port := portOfServerURL(t, runtimeSrv.URL)
	setupAttachmentDB(t, port)

	deployKey := services.DeployKey(chatTestTenant, "min")
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/agents/"+deployKey && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","hostPort":%d,"runtimeToken":"rt-secret"}}`, deployKey, port)))
		case r.URL.Path == "/api/v1/agents/"+deployKey+"/status" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","health":"healthy","hostPort":%d}}`, deployKey, port)))
		default:
			t.Errorf("unexpected deployer request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(deployerSrv.Close)

	deployerSvc := services.NewAgentDeployerService(
		deployer.NewClient(deployerSrv.URL, ""),
		"127.0.0.1", // publicHost
		"127.0.0.1", // upstreamHost（no-Kong 回源 → fake runtime）
		"", "", "", nil, nil, nil, "", "",
	)
	chatSvc := services.NewAgentChatService(
		repository.NewChatRepository(),
		repository.NewAgentRepository(),
		deployerSvc,
		runtime.NewClient(),
		"127.0.0.1", "", "127.0.0.1",
	)
	return &attachmentChatEnv{handler: NewAgentChatHandler(chatSvc), runtime: runtimeSrv}
}

func setupAttachmentDB(t *testing.T, hostPort int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &agent.AgentConfig{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	require.NoError(t, db.Create(&agent.AgentConfig{
		Name: "min", TenantID: chatTestTenant, ContentHash: "hash-1",
		SystemPrompt: "you are min", DeploymentStatus: "running",
		RuntimePort: hostPort, RuntimeToken: "rt-secret",
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u1", TenantID: chatTestTenant, ID: "s-att", Title: "t", AgentID: "min",
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u2", TenantID: chatTestTenant, ID: "s-u2", Title: "t", AgentID: "min",
	}).Error)
}

func doCapabilities(t *testing.T, env *attachmentChatEnv) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}}
	env.handler.Capabilities(c)
	return w
}

func TestCapabilities_AttachmentsEnabledForNewRuntime(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	w := doCapabilities(t, env)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"attachmentsEnabled":true`)
}

func TestCapabilities_DisabledForOldRuntimeVersion(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.4.0"})
	w := doCapabilities(t, env)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"attachmentsEnabled":false`)
}

func TestCapabilities_DisabledWhenHealthUnreachable(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{}) // 无 /health 路由
	w := doCapabilities(t, env)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"attachmentsEnabled":false`)
}