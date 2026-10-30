package handler

// 评审补充（PR #233 复审建议 2）：v2 同步管理的会话收到绕过协议的直接
// 消息时，此前落 404「会话不存在」（误导）——应 409 + 明确文案。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/deployer"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/infrastructure/runtime"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAgentChatSyncManagedSession409(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &chat.Session{}, &chat.Message{}, &chat.SessionSyncState{}, &chat.UploadRecord{}))
	// deployer GetStatus 前置：本地 agents 表需有该 agent 行
	require.NoError(t, db.Create(&agent.AgentConfig{
		Name: "agent-1", TenantID: "t1", SystemPrompt: "p", ContentHash: "h",
	}).Error)
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	owner := chat.Owner{TenantID: "t1", UserID: "u1"}

	// v1 会话（App push 形态：Source 空）→ 认领 → replace v1 → v2 管理态
	sess := &chat.Session{
		TenantID: "t1", UserID: "u1", ID: "s1", AgentID: "agent-1",
		Title: "t", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, db.Create(sess).Error)
	syncRepo := repository.NewChatSyncRepository(db)
	_, err = syncRepo.Claim(context.Background(), owner, "s1", syncHSrcA, "imported")
	require.NoError(t, err)

	e := chat.Envelope{
		ProtocolVersion:   chat.ProtocolVersion,
		ProjectionVersion: chat.ProjectionVersion,
		SourceID:          syncHSrcA, AppSessionID: "s1", Version: 1,
		SnapshotID: "11112222-3333-4444-5555-666677778888", Kind: chat.KindReplace,
	}
	rp := &chat.ReplacePayload{
		Session: chat.SyncSessionPayload{
			ID: "s1", Title: "t", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
			AgentID: "agent-1", // SendMessage 归属校验按会话 agent_id 判定
		},
		Messages: []chat.SyncMessagePayload{
			{ID: "s1-m0", Role: "user", Content: "synced", CreatedAt: "2026-10-01T00:00:00Z", Ordinal: 0},
		},
	}
	e.Payload = rp
	b, err := json.Marshal(rp)
	require.NoError(t, err)
	e.RawPayload = b
	d, err := chat.DigestEnvelope(e)
	require.NoError(t, err)
	e.Digest = d
	_, err = syncRepo.Apply(context.Background(), owner, e)
	require.NoError(t, err)

	// 假 deployer：status=running（SendMessage 在 SaveUserMessage 前先
	// ResolveRuntime——nil deployerSvc 会 panic）；runtime /health 探测无
	// 监听 → 附件不支持 → 非致命继续。
	deployerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"agentName":"agent-1","status":"running","hostPort":20000,"containerId":"c-1"}}`))
	}))
	t.Cleanup(deployerSrv.Close)

	// agent chat 直接发消息 → 409 + 明确文案（修复前 404「会话不存在」）
	svc := services.NewAgentChatService(repository.NewChatRepository(), nil,
		services.NewAgentDeployerService(services.AgentDeployerConfig{
			Client:   deployer.NewClient(deployerSrv.URL, "unused"),
			AuthMode: services.ModeBuiltin,
		}),
		runtime.NewClient(), "", "", "127.0.0.1")
	h := NewAgentChatHandler(svc)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("user_name", "user1")
		tenant.SetTenantID(c, "t1")
	})
	r.POST("/api/v1/agents/:name/chat/sessions/:id/messages", h.SendMessage)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/chat/sessions/s1/messages",
		strings.NewReader(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code, "body=%s", rec.Body.String())
	require.Contains(t, rec.Body.String(), "云同步管理")

	// 正文不变（同步的那条仍在，rogue 消息未写入）
	var msgs []chat.Message
	require.NoError(t, db.Where("user_id = ? AND session_id = ?", "u1", "s1").Order("id").Find(&msgs).Error)
	require.Len(t, msgs, 1)
	require.Equal(t, "synced", msgs[0].Content)
}
