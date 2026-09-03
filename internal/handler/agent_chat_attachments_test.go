// internal/handler/agent_chat_attachments_test.go
package handler

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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
	// （探测失败 → 附件不可用）。版本号仅作展示——hub 判定只看 capabilities。
	runtimeVersion string
	// capabilityOff 模拟未声明附件代次校验能力的 runtime（< v2.7.0：
	// /health 无 capabilities 字段，hub 解析为 false；显式 false 与缺字段
	// 对 hub 等价）。默认 false（零值）→ 报 attachmentExpectedGeneration:true。
	capabilityOff bool
	// uploadHandler serves POST /v1/files/uploads. nil → 404（旧 runtime）。
	uploadHandler http.HandlerFunc
	// runsHandler serves POST /v1/agents/{key}/runs. nil → 最小成功 SSE 流。
	runsHandler http.HandlerFunc
	// contentHandler serves GET /v1/files/content. nil → 404.
	contentHandler http.HandlerFunc
}

// 部署代次常量（issue #94 review R3）：fake deployer 报告的容器代。
// Gen1 → Gen2 = 重部署换新容器（container id 必变）；改回 Gen1 = 同容器
// 原地重启（container id 不变，文件仍在）。
const (
	attachmentContainerGen1 = "ctr-gen-1"
	attachmentContainerGen2 = "ctr-gen-2"
)

// attachmentChatEnv wires the production handler against fake deployer +
// fake runtime + in-memory sqlite（复刻 agent_chat_runtime_addressing_test.go
// 模式）。Seeds：agent "min"（running，fake runtime 端口）、session
// "s-att"（u1 所有，绑定 min）、"s-u2"（u2 所有，非 owner 路径）。
type attachmentChatEnv struct {
	handler *AgentChatHandler
	runtime *httptest.Server
	// containerID 是 fake deployer 报告的可变容器代（issue #94 review R3：
	// 部署代次绑定改用不可变 container id）。测试中途 *env.containerID =
	// Gen2 即模拟重部署换容器；改回 Gen1 即模拟同容器原地重启。
	containerID *string
}

func newAttachmentChatEnv(t *testing.T, opts attachmentFakeOpts) *attachmentChatEnv {
	t.Helper()

	mux := http.NewServeMux()
	if opts.runtimeVersion != "" {
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// runtime v2.7.0 契约：/health 顶层声明
			// capabilities.attachmentExpectedGeneration。capabilityOff 时整个
			// capabilities 字段缺省（模拟 < v2.7.0 旧 runtime 的真实形态）。
			caps := ""
			if !opts.capabilityOff {
				caps = `,"capabilities":{"attachmentExpectedGeneration":true}`
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","version":%q%s}`, opts.runtimeVersion, caps)))
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

	// 可变容器代：deployer 对 agent/status 查询实时报告 containerID 变量
	//（闭包捕获引用），测试中途改值即模拟重部署/原地重启换容器代次。
	containerID := attachmentContainerGen1
	deployKey := services.DeployKey(chatTestTenant, "min")
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/agents/"+deployKey && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","hostPort":%d,"runtimeToken":"rt-secret","containerId":%q}}`, deployKey, port, containerID)))
		case r.URL.Path == "/api/v1/agents/"+deployKey+"/status" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(fmt.Sprintf(`{"success":true,"data":{"agentName":%q,"status":"running","health":"healthy","hostPort":%d,"containerId":%q}}`, deployKey, port, containerID)))
		default:
			t.Errorf("unexpected deployer request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(deployerSrv.Close)

	deployerSvc := services.NewAgentDeployerService(services.AgentDeployerConfig{
		Client:       deployer.NewClient(deployerSrv.URL, ""),
		PublicHost:   "127.0.0.1",
		UpstreamHost: "127.0.0.1", // no-Kong 回源 → fake runtime
		// CDNHost/EncryptionKey/RuntimeAPIKey/CapabilitySecret/knowledge/
		// aigc/chat push 均留空：无 knowledge MCP、无 agent graph 下发
		AuthMode: services.ModeCasdoor, // casdoor 形寻址（同 agent_chat_runtime_addressing_test）
	})
	chatSvc := services.NewAgentChatService(
		repository.NewChatRepository(),
		repository.NewAgentRepository(),
		deployerSvc,
		runtime.NewClient(),
		"127.0.0.1", "", "127.0.0.1",
	)
	return &attachmentChatEnv{handler: NewAgentChatHandler(chatSvc), runtime: runtimeSrv, containerID: &containerID}
}

func setupAttachmentDB(t *testing.T, hostPort int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.UploadRecord{}, &agent.AgentConfig{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	require.NoError(t, db.Create(&agent.AgentConfig{
		Name: "min", TenantID: chatTestTenant, ContentHash: "hash-1",
		SystemPrompt: "you are min", DeploymentStatus: "running",
		RuntimePort: hostPort, RuntimeToken: "rt-secret",
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u1", TenantID: chatTestTenant, ID: "s-att", AgentID: "min",
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

func TestCapabilities_EnabledWhenCapabilityDeclared(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.7.0"})
	w := doCapabilities(t, env)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"attachmentsEnabled":true`)
}

// 能力缺失回归（runtime v2.7.0 契约）：版本号不再参与判定——即便 /health
// 报一个更新的版本，未声明 attachmentExpectedGeneration（无 capabilities
// 字段）即 false。
func TestCapabilities_DisabledWhenCapabilityUndeclared(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "9.9.9", capabilityOff: true})
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

// F2 回归（runtime #61 Hub 跟进契约）：附件入口 = runtime capability 声明
// && deployer 报告非空 containerId。capability true 但 deployer 未报告
// containerId（空代次）→ 不探测直接 false；两者齐备 → true。两个独立 env
// 断言——AttachmentsAvailable 有 15s TTL probe cache，同一 service 实例内
// 第二次调用会命中缓存；*env.containerID 可变，构造后置空即模拟空代次。
func TestCapabilities_RequireNonEmptyContainerGeneration(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.7.0"})
	*env.containerID = ""
	w := doCapabilities(t, env)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"attachmentsEnabled":false`)

	envOK := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.7.0"})
	wOK := doCapabilities(t, envOK)
	require.Equal(t, http.StatusOK, wOK.Code)
	require.Contains(t, wOK.Body.String(), `"attachmentsEnabled":true`)
}

func sendMessageForAttachments(t *testing.T, env *attachmentChatEnv, body string, name, sessionID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", userID)
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: name}, {Key: "id", Value: sessionID}}
	env.handler.SendMessage(c)
	return w
}

func TestSendMessage_RejectsEmptyMessage(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	w := sendMessageForAttachments(t, env, `{"content":""}`, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
}

func TestSendMessage_RejectsUnsafeAttachmentPath(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	body := `{"content":"x","attachments":[{"id":"f","name":"n","mime":"text/plain","size":1,"path":"/etc/passwd"}]}`
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
}

func TestSendMessage_RejectsTooManyAttachments(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	atts := `[` + strings.Repeat(`{"id":"f","name":"n","mime":"text/plain","size":1,"path":".zerone-uploads/n.txt"},`, 11)
	atts = strings.TrimSuffix(atts, ",") + `]`
	w := sendMessageForAttachments(t, env, `{"content":"x","attachments":`+atts+`}`, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSendMessage_PassesAttachmentsToRuntimeRun(t *testing.T) {
	var gotBody []byte
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\"}\n\n"))
		},
	})
	body := `{"content":"总结","attachments":[{"id":"f-1","name":"r.pdf","mime":"application/pdf","size":3,"path":".zerone-uploads/r.pdf"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "r.pdf", Mime: "application/pdf", Size: 3, Path: ".zerone-uploads/r.pdf"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, string(gotBody), `"attachments":[{`)
	require.Contains(t, string(gotBody), `.zerone-uploads/r.pdf`)
	require.Contains(t, string(gotBody), `"message":"总结"`)
}

func TestSendMessage_TitleFromFirstAttachmentName(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	body := `{"content":"","attachments":[{"id":"f-1","name":"report.pdf","mime":"application/pdf","size":3,"path":".zerone-uploads/report.pdf"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "report.pdf", Mime: "application/pdf", Size: 3, Path: ".zerone-uploads/report.pdf"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	var sess chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-att").First(&sess).Error)
	require.Equal(t, "report.pdf", sess.Title)
}

// F1：伪造描述符（无服务端上传记录）必须在落库前被拒——`.zerone-uploads` 是
// 同 Agent runtime 容器内所有用户共享的目录，语法合法的他人 path 不能成为
// 自己会话的授权依据。
func TestSendMessage_RejectsForgedAttachmentDescriptors(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	body := `{"content":"x","attachments":[{"id":"f-1","name":"secret.txt","mime":"text/plain","size":3,"path":".zerone-uploads/secret.txt"}]}`
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "forged descriptors must not persist a user message")
}

// F1 真实链路：fake 上传（hub 落记录）→ SendMessage 描述符与记录全等 → 200。
func TestSendMessage_AttachmentAfterRealUpload(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusCreated, w.Code)
	var recCount int64
	require.NoError(t, database.DB.Model(&chat.UploadRecord{}).Where("session_id = ?", "s-att").Count(&recCount).Error)
	require.Equal(t, int64(1), recCount, "successful upload must persist a server-side record")

	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	w = sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
}

// F3：未声明附件能力的 runtime（< v2.7.0，/health 无 capabilities 字段）上
// 含附件 SendMessage → 501；同环境纯文本消息不受影响（200）。旧 runtime 会
// 静默丢弃 run 的 attachments 字段——宁可拒绝也不静默丢附件。
func TestSendMessage_AttachmentsRequireNewRuntime(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.4.0", capabilityOff: true})
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt"})
	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusNotImplemented, w.Code)
	require.Contains(t, w.Body.String(), `"code":"runtime_attachment_unsupported"`)
	// B3（Wave A F3 遗留）：版本门控拒绝必须回滚已落库的 user message，
	// 否则历史里留下没有 assistant 回复的孤儿 user turn（与 F2 attachment_missing 同款）。
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "runtime_attachment_unsupported must roll back the persisted user message")

	w = sendMessageForAttachments(t, env, `{"content":"hi"}`, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code, "text-only messages must not be gated")
}

// F4：未知字段（如伪造的 base64 内联数据）严格解码 400，不静默接受。
func TestSendMessage_RejectsUnknownFields(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	body := `{"content":"x","attachments":[{"id":"f","name":"n","mime":"text/plain","size":1,"path":".zerone-uploads/n.txt","base64":"..."}]}`
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
}

// R2 F2：首个 JSON 值之后的尾随对象（拼接 body）必须 400——严格解码不能
// 静默取首个值放过剩余字节。
func TestSendMessage_RejectsTrailingJsonValue(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	w := sendMessageForAttachments(t, env, `{"content":"hi"}{"base64":"x"}`, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
}

func TestSendMessage_AttachmentMissingMappedToEnvelope(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Attachment not found: .zerone-uploads/x.txt","code":"attachment_missing","path":".zerone-uploads/x.txt"}`))
		},
	})
	body := `{"content":"","attachments":[{"id":"f-1","name":"x.txt","mime":"text/plain","size":1,"path":".zerone-uploads/x.txt"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "x.txt", Mime: "text/plain", Size: 1, Path: ".zerone-uploads/x.txt"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"attachment_missing"`)
	require.Contains(t, w.Body.String(), "附件已失效，请重新上传")
	var errCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "system").Count(&errCount).Error)
	require.Zero(t, errCount, "attachment_missing must NOT be persisted as a system error message")
	// F2：乐观持久化的 user message 必须被回滚，重发才不会重复 user turn。
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "attachment_missing must roll back the persisted user message")
}

// runtime v2.7.0 契约（issue #94 / runtime #61）：带附件的 run 必须携带
// X-Expected-Container-Id == 当前 deployer 报告的容器代次；纯文本 run 不携带
// （无代次可断言，向后兼容）。
func TestSendMessage_AttachmentRunSendsExpectedContainerIDHeader(t *testing.T) {
	var gotGen string
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			gotGen = r.Header.Get(runtime.HeaderExpectedContainerID)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\"}\n\n"))
		},
	})
	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, attachmentContainerGen1, gotGen, "attachment-bearing run must carry the deployer-reported container generation")
}

func TestSendMessage_TextOnlyRunOmitsExpectedContainerIDHeader(t *testing.T) {
	headerPresent := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			_, headerPresent = r.Header[runtime.HeaderExpectedContainerID]
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\"}\n\n"))
		},
	})
	w := sendMessageForAttachments(t, env, `{"content":"hi"}`, "min", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, headerPresent, "text-only runs must not carry X-Expected-Container-Id")
}

// 发送侧 412（runtime v2.7.0 原子代次校验不通过）：透传 412 + 域码 +
// user message 回滚——与 attachment_missing 同款「附件失效可重试」语义。
// 契约明示：412 后禁止去掉 header 降级重试（hub 只 respond，不自动重发）。
func TestSendMessage_GenerationMismatch412RollsBackUserMessage(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"error":"container generation mismatch","code":"generation_mismatch"}`))
		},
	})
	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusPreconditionFailed, w.Code)
	require.Contains(t, w.Body.String(), `"code":"generation_mismatch"`)
	require.Contains(t, w.Body.String(), "附件已过期（部署代次变更），请重新上传")
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "generation_mismatch must roll back the persisted user message")
	var sysCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "system").Count(&sysCount).Error)
	require.Zero(t, sysCount, "generation_mismatch must not be persisted as a system error message")
}

// generation_unavailable（503，runtime 无法确定自身容器身份）：透传 503，
// user message 回滚——review F3：所有已识别的附件 pre-run 拒绝（白名单五码）
// 统一回滚，user turn 从未生效，历史不留注定失败的重复记录；恢复后重新发送。
func TestSendMessage_GenerationUnavailable503RollsBackUserMessage(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		runsHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"container identity unavailable","code":"generation_unavailable"}`))
		},
	})
	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	seedUploadRecords(t, "s-att", services.AttachmentDesc{ID: "f-1", Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt"})
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), `"code":"generation_unavailable"`)
	require.Contains(t, w.Body.String(), "Runtime 部署状态异常，请稍后重试")
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "generation_unavailable must roll back the persisted user message (review F3: all pre-run rejects roll back)")
	var sysCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "system").Count(&sysCount).Error)
	require.Zero(t, sysCount, "generation_unavailable must not be persisted as a system error message")
}

func uploadRequestForAttachments(t *testing.T, env *attachmentChatEnv, files []struct{ name, body string }, sessionID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		fw, err := mw.CreateFormFile("files", f.name)
		require.NoError(t, err)
		_, _ = fw.Write([]byte(f.body))
	}
	require.NoError(t, mw.Close())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user_id", userID)
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: sessionID}}
	env.handler.UploadAttachments(c)
	return w
}

func fakeUploadOK(w http.ResponseWriter, r *http.Request) {
	_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err != nil {
			break // io.EOF 或 hub 限额主动断开
		}
		if p.FileName() != "" {
			_, _ = io.Copy(io.Discard, p)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"files":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`))
}

func TestUploadAttachments_RelaysWithServerSideKey(t *testing.T) {
	var gotAuth string
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		fakeUploadOK(w, r)
	}
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: handler})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "rt-secret", gotAuth, "runtime token must be injected server-side")
	require.Contains(t, w.Body.String(), ".zerone-uploads/a.txt")
}

func TestUploadAttachments_NonOwner404(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-u2", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUploadAttachments_AgentBindingMismatch404(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	// s-att 绑定 min；用 other 名字寻址
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "a.txt")
	_, _ = fw.Write([]byte("abc"))
	require.NoError(t, mw.Close())
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "other"}, {Key: "id", Value: "s-att"}}
	env.handler.UploadAttachments(c)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUploadAttachments_OldRuntime501(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.4.0", capabilityOff: true}) // 能力未声明
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusNotImplemented, w.Code)
	require.Contains(t, w.Body.String(), `"code":"runtime_attachment_unsupported"`)
	// 501 文案（runtime v2.7.0 契约）：提示升级到支持代次校验的版本。
	require.Contains(t, w.Body.String(), "2.7.0")
}

func TestUploadAttachments_OldRuntimeUpload404Mapped501(t *testing.T) {
	// /health 声明能力但上传端点 404（防御性：能力探测与实际端点不一致）
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.7.0"}) // 无 uploadHandler → mux 404
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusNotImplemented, w.Code)
	require.Contains(t, w.Body.String(), `"code":"runtime_attachment_unsupported"`)
}

// runtime v2.7.0 契约：上传必须携带 X-Expected-Container-Id == 当前
// deployer 报告代次（runtime 在任何写入之前原子校验，不匹配 412）。
func TestUploadAttachments_SendsExpectedContainerIDHeader(t *testing.T) {
	var gotGen string
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			gotGen = r.Header.Get(runtime.HeaderExpectedContainerID)
			fakeUploadOK(w, r)
		},
	})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, attachmentContainerGen1, gotGen, "upload must carry the deployer-reported container generation")
}

// 上传侧 412（代次不匹配）：透传 412 + 域码 + 中文文案；412 拒绝发生在
// runtime 写入之前 → hub 不得落任何上传记录。
func TestUploadAttachments_GenerationMismatch412(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"error":"container generation mismatch","code":"generation_mismatch"}`))
		},
	})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusPreconditionFailed, w.Code)
	require.Contains(t, w.Body.String(), `"code":"generation_mismatch"`)
	require.Contains(t, w.Body.String(), "附件已过期（部署代次变更），请重新上传")
	var recCount int64
	require.NoError(t, database.DB.Model(&chat.UploadRecord{}).Where("session_id = ?", "s-att").Count(&recCount).Error)
	require.Zero(t, recCount, "a 412-rejected upload must not persist upload records")
}

// 上传侧 503（runtime 无法确定自身容器身份）：透传 503 + 域码。
func TestUploadAttachments_GenerationUnavailable503(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"container identity unavailable","code":"generation_unavailable"}`))
		},
	})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), `"code":"generation_unavailable"`)
	require.Contains(t, w.Body.String(), "Runtime 部署状态异常，请稍后重试")
}

func TestUploadAttachments_RuntimeLimitExceededPassthrough(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"error":"File \"x\" exceeds the 20MB single-file limit","code":"upload_limit_exceeded"}`))
		},
	})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Contains(t, w.Body.String(), `"code":"upload_limit_exceeded"`)
}

func TestUploadAttachments_HubRejectsTooManyFiles(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	files := make([]struct{ name, body string }, 11)
	for i := range files {
		files[i] = struct{ name, body string }{fmt.Sprintf("f%d.txt", i), "x"}
	}
	w := uploadRequestForAttachments(t, env, files, "s-att", "u1")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Contains(t, w.Body.String(), `"code":"upload_limit_exceeded"`)
}

func TestUploadAttachments_HubRejectsOversizeSingleFile(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "big.bin")
	_, _ = fw.Write(bytes.Repeat([]byte{0}, (20<<20)+16))
	require.NoError(t, mw.Close())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: "s-att"}}
	env.handler.UploadAttachments(c)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Contains(t, w.Body.String(), `"code":"upload_limit_exceeded"`)
}

// 整个请求体总量上限（终审 Finding 2）：非 file part 虽被 relayMultipart 跳过，
// 但读穿它才能定位下一个 boundary——若无总量上限，已认证用户可用「若干小文件 +
// 巨型 text field」造成无界 ingress。MaxBytesReader 超限的读错误经
// relayMultipart 落 400 invalid_multipart。
func TestUploadAttachments_HubRejectsOversizeRequestBody(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0", uploadHandler: fakeUploadOK})
	old := uploadMaxRequestBytes
	uploadMaxRequestBytes = 1 << 10 // 1KB：逼停 2KB text field 的读穿
	t.Cleanup(func() { uploadMaxRequestBytes = old })

	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormField("note") // 非 file part：绕开单文件/文件数限额
	require.NoError(t, err)
	_, err = fw.Write(bytes.Repeat([]byte("x"), 2<<10))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", &buf)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: "s-att"}}
	env.handler.UploadAttachments(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_multipart"`)
}

// 流式性断言：runtime 收到第一个 part 的字节时，客户端尚未写完整个请求
// （knowledge_test.go 的 io.Pipe 慢生产者范式）。
func TestUploadAttachments_StreamsPartByPart(t *testing.T) {
	runtimeSawFirst := make(chan struct{})
	allowSecond := make(chan struct{})
	filesReceived := make(chan int, 1)
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			mr := multipart.NewReader(r.Body, params["boundary"])
			count := 0
			sawFirst := false
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				if p.FileName() == "" {
					continue
				}
				count++
				if !sawFirst {
					sawFirst = true
					close(runtimeSawFirst)
				}
				_, _ = io.Copy(io.Discard, p)
			}
			filesReceived <- count
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"files":[{"id":"f-1","name":"one.bin","mime":"application/octet-stream","size":5,"path":".zerone-uploads/one.bin"},{"id":"f-2","name":"two.bin","mime":"application/octet-stream","size":6,"path":".zerone-uploads/two.bin"}]}`))
		},
	})

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		fw, _ := mw.CreateFormFile("files", "one.bin")
		_, _ = fw.Write([]byte("first"))
		<-allowSecond
		fw2, _ := mw.CreateFormFile("files", "two.bin")
		_, _ = fw2.Write([]byte("second"))
		_ = mw.Close()
		_ = pw.Close()
	}()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/uploads", pr)
	c.Request.Header.Set("Content-Type", mw.FormDataContentType())
	c.Set("user_id", "u1")
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: "s-att"}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		env.handler.UploadAttachments(c)
	}()

	select {
	case <-runtimeSawFirst:
		// good：handler 在请求体未写完时已把第一个 part 转发到 runtime
	case <-time.After(5 * time.Second):
		t.Fatal("handler buffered the whole request before relaying (not streaming)")
	}
	close(allowSecond)
	<-done
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, 2, <-filesReceived)
}

// runtime 在连接建立后、响应前断开（Hijack 后立即关闭）→ 传输失败应映射 502，
// 而不是把 pipe 写错误误判为客户端 multipart 解析错误（400 invalid_multipart）。
func TestUploadAttachments_RuntimeUnreachable502(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
		},
	})
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", strings.Repeat("x", 20<<20)}}, "s-att", "u1")
	require.Equal(t, http.StatusBadGateway, w.Code)
}

// runtime 读少量 body 后直接回 413 不排空（>256KB 请求体迫使 net/http 服务端
// 丢弃剩余 body 关闭连接 → 触发 hub 侧 mid-stream 中断）→ hub 必须透传 runtime
// 的 413 upload_limit_exceeded，而不是 400 invalid_multipart。
func TestUploadAttachments_Runtime413MidStreamPassthrough(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 64)
			_, _ = r.Body.Read(buf) // 只读一点，不排空 → 触发 mid-stream 中断
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"error":"File limit","code":"upload_limit_exceeded"}`))
		},
	})
	// 20MB body 远大于 net/http 服务端 handler 返回后的排空上限（256KB）
	// 和 loopback 内核缓冲：服务端放弃排空关闭连接 → hub 的 io.Copy 必然在
	// pipe 写处失败（确定性命中 relay-fail 分支，而非服务端把整个小 body
	// 缓冲掉）、恰好等于单文件限额上限（20MB）不会触发 hub 侧限额检查。
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", strings.Repeat("x", 20<<20)}}, "s-att", "u1")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Contains(t, w.Body.String(), `"code":"upload_limit_exceeded"`)
	require.Contains(t, w.Body.String(), "附件大小超出限制")
}

// seedUploadRecords inserts server-side upload records (the authorization
// anchor) for u1-owned descriptors, as the hub would after a real upload on
// the default container generation (attachmentContainerGen1).
func seedUploadRecords(t *testing.T, sessionID string, descs ...services.AttachmentDesc) {
	t.Helper()
	for _, d := range descs {
		require.NoError(t, database.DB.Create(&chat.UploadRecord{
			ID: d.ID, TenantID: chatTestTenant, SessionID: sessionID, UserID: "u1",
			Name: d.Name, Mime: d.Mime, Size: d.Size, Path: d.Path,
			ContainerID: attachmentContainerGen1,
		}).Error)
	}
}

// seedAttachmentMessage seeds the full legit chain for the content-proxy
// tests: a server-side upload record (authorization) + a user message carrying
// the file part (display-only).
func seedAttachmentMessage(t *testing.T, sessionID, path string) {
	t.Helper()
	var sess chat.Session
	require.NoError(t, database.DB.Where("id = ?", sessionID).First(&sess).Error)
	require.NoError(t, database.DB.Create(&chat.UploadRecord{
		ID: "f-1", TenantID: chatTestTenant, SessionID: sessionID, UserID: sess.UserID,
		Name: "a.png", Mime: "image/png", Size: 3, Path: path,
		ContainerID: attachmentContainerGen1,
	}).Error)
	require.NoError(t, database.DB.Create(&chat.Message{
		UserID: sess.UserID, TenantID: chatTestTenant, ID: "m-" + path, SessionID: sessionID,
		Role:    "user",
		Content: `[{"type":"file","id":"f-1","name":"a.png","mime":"image/png","size":3,"path":"` + path + `"},{"type":"text","text":"看图"}]`,
	}).Error)
}

func doGetAttachmentContent(t *testing.T, env *attachmentChatEnv, rawPath, sessionID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/content?path="+url.QueryEscape(rawPath), nil)
	c.Set("user_id", userID)
	c.Set("tenant_id", chatTestTenant)
	c.Params = gin.Params{{Key: "name", Value: "min"}, {Key: "id", Value: sessionID}}
	env.handler.AttachmentContent(c)
	return w
}

func TestAttachmentContent_StreamsKnownAttachment(t *testing.T) {
	var gotAuth, gotQuery string
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("x-api-key")
			gotQuery = r.URL.Query().Get("path")
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''a.png`)
			_, _ = w.Write([]byte("png!"))
		},
	})
	seedAttachmentMessage(t, "s-att", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "png!", w.Body.String())
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Equal(t, "rt-secret", gotAuth)
	require.Equal(t, ".zerone-uploads/a.png", gotQuery)
}

func TestAttachmentContent_NonOwner404(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("x")) },
	})
	seedAttachmentMessage(t, "s-u2", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-u2", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestAttachmentContent_RejectsPathOutsideUploadsDir(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})
	w := doGetAttachmentContent(t, env, "package.json", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
}

// F1：消息 file part 不再是授权依据——只插消息不插服务端上传记录必须 404
// （否则伪造一条消息就能授权任意 path 的内容代理）。
func TestAttachmentContent_MessagePartWithoutRecord404(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			_, _ = w.Write([]byte("x"))
		},
	})
	require.NoError(t, database.DB.Create(&chat.Message{
		UserID: "u1", TenantID: chatTestTenant, ID: "m-only", SessionID: "s-att",
		Role:    "user",
		Content: `[{"type":"file","id":"f-1","name":"a.png","mime":"image/png","size":3,"path":".zerone-uploads/a.png"}]`,
	}).Error)
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, runtimeHit, "message parts are display-only; runtime must not be dialed without an upload record")
}

// 交叉核验：合法前缀但未在该 session 上传记录中出现过的 path 一律 404——
// runtime /v1/files/content 能读整个工作区，不能把该能力透给用户态。
func TestAttachmentContent_UnknownPathInSession404(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			_, _ = w.Write([]byte("x"))
		},
	})
	seedAttachmentMessage(t, "s-att", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/other.png", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, runtimeHit, "runtime must not be dialed for unregistered paths")
}

func TestAttachmentContent_RuntimeFileGone404(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		},
	})
	seedAttachmentMessage(t, "s-att", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
}

// 下载侧 header 透传（runtime v2.7.0 契约）：内容代理请求携带
// X-Expected-Container-Id == 当前代次（runtime 在文件读取之前原子校验）。
func TestAttachmentContent_SendsExpectedContainerIDHeader(t *testing.T) {
	var gotGen string
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			gotGen = r.Header.Get(runtime.HeaderExpectedContainerID)
			_, _ = w.Write([]byte("bytes"))
		},
	})
	seedAttachmentMessage(t, "s-att", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, attachmentContainerGen1, gotGen, "content proxy must carry the deployer-reported container generation")
}

// 下载侧 412（代次不匹配）→ hub 412 + 域码 + 中文文案。
func TestAttachmentContent_GenerationMismatch412(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.7.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = w.Write([]byte(`{"error":"container generation mismatch","code":"generation_mismatch"}`))
		},
	})
	seedAttachmentMessage(t, "s-att", ".zerone-uploads/a.png")
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.png", "s-att", "u1")
	require.Equal(t, http.StatusPreconditionFailed, w.Code)
	require.Contains(t, w.Body.String(), `"code":"generation_mismatch"`)
	require.Contains(t, w.Body.String(), "附件已过期（部署代次变更），请重新上传")
}

// R3（部署代次绑定，ContainerID 版）：上传记录只授权创建它的容器代——
// deployer 报告的不可变 container id 精确相等，零时间容差。重部署清空
// .zerone-uploads 后，新容器里同名文件属于新的上传者——旧代记录必须整体
// 失效：①当前代真实上传 → 内容代理 200；②换容器代 + 手插旧代记录
// （CreatedAt=now，不是一小时前——证明代次判定与时间无关，直接覆盖
// reviewer 点名的旧实现 <60s 容差窗口）→ 404 且不拨号 runtime；
// ③ SendMessage 对旧代记录 → 400（落库前拒绝）；④切回原代（restart
// 语义：同容器原地重启，container id 不变，文件仍在）→ ①的记录下载
// 恢复 200（正向回归，不误伤文件确实还在的场景）。
func TestAttachmentContent_StaleRecordAfterRuntimeRecreate404(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler:  fakeUploadOK,
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			_, _ = w.Write([]byte("bytes"))
		},
	})

	// ① 当前代（ctr-gen-1）真实上传落记录（container_id=ctr-gen-1）→
	// 内容代理 200。
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusCreated, w.Code)
	w = doGetAttachmentContent(t, env, ".zerone-uploads/a.txt", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, runtimeHit, "current-generation record must reach the runtime")

	// ② 模拟重部署：deployer 改报新容器代 ctr-gen-2。手插一条上一代
	//（ctr-gen-1）落库的记录——CreatedAt 设为 now（不是一小时前）：
	// 代次绑定与时间无关，即便落库时间晚于新容器启动也必须失败关闭，
	// 且不得拨号 runtime。
	*env.containerID = attachmentContainerGen2
	runtimeHit = false
	require.NoError(t, database.DB.Create(&chat.UploadRecord{
		ID: "f-old", TenantID: chatTestTenant, SessionID: "s-att", UserID: "u1",
		Name: "old.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/old.txt",
		ContainerID: attachmentContainerGen1, // 上一代容器落库的记录
		CreatedAt:   time.Now(),              // 时间无关性：now 也不放行
	}).Error)
	w = doGetAttachmentContent(t, env, ".zerone-uploads/old.txt", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, runtimeHit, "stale-generation record must fail closed before dialing the runtime")

	// ③ SendMessage 同款：描述符与旧代记录四字段全等也 → 400 invalid_attachment，
	// 且零 user message 落库。
	body := `{"content":"","attachments":[{"id":"f-old","name":"old.txt","mime":"text/plain","size":3,"path":".zerone-uploads/old.txt"}]}`
	w = sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "stale descriptors must not persist a user message")

	// ④ 正向回归（restart 语义）：容器原地重启——Docker container id 不变，
	// `.zerone-uploads` 文件仍在。deployer 改回报 ctr-gen-1，①的记录下载
	// 恢复 200。
	*env.containerID = attachmentContainerGen1
	runtimeHit = false
	w = doGetAttachmentContent(t, env, ".zerone-uploads/a.txt", "s-att", "u1")
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, runtimeHit, "same-container restart must re-authorize the still-existing file")
}

// R3（上传→落库间重建竞态，reviewer 点名）：上传成功时记录必然标上传
// 发起刻的容器代（ctr-gen-1，ResolveRuntime 在上传请求前取得）；随后容器
// 重建（ctr-gen-2）——SendMessage 与 AttachmentContent 都必须拒绝（记录
// 代次 ≠ 当前代次），即使描述符/路径与记录四字段全等、落库时间就是 now。
// 用「真实上传 + 立即切换代」显式走通整条链路，不手插记录。
func TestUploadRecords_RecreateBetweenUploadAndUseRejected(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler:  fakeUploadOK,
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			_, _ = w.Write([]byte("bytes"))
		},
	})

	// 真实上传：fake runtime 201 → hub 落记录（container_id=ctr-gen-1）。
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusCreated, w.Code)

	// 上传成功后、使用前容器重建（新代）。
	*env.containerID = attachmentContainerGen2

	// SendMessage：描述符与记录全等，但代次不符 → 400 + 零 user message。
	body := `{"content":"","attachments":[{"id":"f-1","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	w = sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "recreate-between-upload-and-send must not persist a user message")

	// AttachmentContent 同理 → 404 且不拨号 runtime。
	runtimeHit = false
	w = doGetAttachmentContent(t, env, ".zerone-uploads/a.txt", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, runtimeHit, "stale-generation record must fail closed before dialing the runtime")
}

// R4 P1-2（空代次统一失败关闭）：deployer 未报告容器代次（containerId 空）
// 时上传入口失败关闭——503、不发起上传（runtime 零拨号）、不落任何记录
// （空代次记录会让后续代次判等出现「空对空」放行的 fail-open 口子）。
func TestUploadAttachments_EmptyContainerIDRejected(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		uploadHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			fakeUploadOK(w, r)
		},
	})
	*env.containerID = ""
	w := uploadRequestForAttachments(t, env, []struct{ name, body string }{{"a.txt", "abc"}}, "s-att", "u1")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "部署状态异常")
	require.False(t, runtimeHit, "upload must not be relayed when the container generation is unknown")
	var recCount int64
	require.NoError(t, database.DB.Model(&chat.UploadRecord{}).Where("session_id = ?", "s-att").Count(&recCount).Error)
	require.Zero(t, recCount, "no upload record may be persisted for an unknown generation")
}

// R4 P1-2（空代次统一失败关闭，SendMessage 两段）：
// ① deployer 报告 containerId 空 + 手插 container_id="" 的历史空代记录，
// 发送带描述符 → 入口先拒 503（版本探测/记录校验之前），零 user message；
// ② deployer 报告非空代次但记录是空代 → 记录校验恒拒 400 invalid_attachment
// （历史空代记录一律无效，「空对空」判相等的放行口子从记录侧也封死）。
func TestSendMessage_EmptyContainerIDAndLegacyEmptyRecordFailClosed(t *testing.T) {
	env := newAttachmentChatEnv(t, attachmentFakeOpts{runtimeVersion: "2.5.0"})

	// ① 当前代次未知：手插升级前落库的历史空代记录，入口 503。
	*env.containerID = ""
	require.NoError(t, database.DB.Create(&chat.UploadRecord{
		ID: "f-legacy", TenantID: chatTestTenant, SessionID: "s-att", UserID: "u1",
		Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt",
		ContainerID: "", // 历史空代记录（升级前落库）
		CreatedAt:   time.Now(),
	}).Error)
	body := `{"content":"","attachments":[{"id":"f-legacy","name":"a.txt","mime":"text/plain","size":3,"path":".zerone-uploads/a.txt"}]}`
	w := sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "部署状态异常")
	var userCount int64
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "unknown generation must not persist a user message")

	// ② 当前代次已知（ctr-gen-1）但记录仍是空代 → 400，历史空记录恒无效。
	*env.containerID = attachmentContainerGen1
	w = sendMessageForAttachments(t, env, body, "min", "s-att", "u1")
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"code":"invalid_attachment"`)
	require.NoError(t, database.DB.Model(&chat.Message{}).Where("session_id = ? AND role = ?", "s-att", "user").Count(&userCount).Error)
	require.Zero(t, userCount, "legacy empty-generation records must never authorize a send")
}

// R4 P1-2（下载侧直达回归）：空 containerID + 手插空代记录 → 404 且不拨号
// runtime。SessionHasAttachment 对空代次本就失败关闭，此测试把「空对空不判
// 相等」钉成显式契约，并覆盖 handler 层显式早退路径。
func TestAttachmentContent_EmptyContainerID404(t *testing.T) {
	runtimeHit := false
	env := newAttachmentChatEnv(t, attachmentFakeOpts{
		runtimeVersion: "2.5.0",
		contentHandler: func(w http.ResponseWriter, r *http.Request) {
			runtimeHit = true
			_, _ = w.Write([]byte("bytes"))
		},
	})
	require.NoError(t, database.DB.Create(&chat.UploadRecord{
		ID: "f-legacy", TenantID: chatTestTenant, SessionID: "s-att", UserID: "u1",
		Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt",
		ContainerID: "", // 空代记录 + 当前空代：不得判相等放行
		CreatedAt:   time.Now(),
	}).Error)
	*env.containerID = ""
	w := doGetAttachmentContent(t, env, ".zerone-uploads/a.txt", "s-att", "u1")
	require.Equal(t, http.StatusNotFound, w.Code)
	require.False(t, runtimeHit, "unknown generation must fail closed before dialing the runtime")
}
