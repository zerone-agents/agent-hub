package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	authdom "control-panel/internal/domain/auth"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/scene"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/infrastructure/runtime"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ---------- guest 读路径可见性（spec 4.3 / 7）----------
//
// 沿用 agent_error_test.go 的「真实 sqlite + 真实 service」搭建模式；DB 铺
// 设额外补齐 buildAgentsDTO（GetChatAgents/GetDesktopAgents → GetAll* 聚合）
// 触碰的全部关联表（裸 SQL，模式同 services 包 setupToolTenantServiceTestDB），
// 并 AutoMigrate chat.Session / scene.Scene 供 chat 与 scene 路径使用。
//
// 四个 agent 的组合矩阵（与 brief Step 5 一致）：
//
//	agent-a: guest=true,  desktop=true
//	agent-b: guest=true,  desktop=false
//	agent-c: guest=false, desktop=true
//	agent-d: guest=false, desktop=false

func setupGuestVisibilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, stmt := range []string{
		`CREATE TABLE agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT 'default',
			content_hash VARCHAR(128) NOT NULL DEFAULT '',
			system_prompt TEXT NOT NULL DEFAULT '',
			permission_mode VARCHAR(32) NOT NULL DEFAULT 'auto',
			max_turns INTEGER NOT NULL DEFAULT 50,
			title JSON,
			description JSON,
			icon VARCHAR(512) DEFAULT '',
			icon_name VARCHAR(64) DEFAULT '',
			icon_color VARCHAR(32) DEFAULT '',
			icon_bg_color VARCHAR(64) DEFAULT '',
			provider_id INTEGER,
			model_id VARCHAR(64) DEFAULT '',
			model_selection_id VARCHAR(128) DEFAULT '',
			field_overrides TEXT,
			source VARCHAR(16) NOT NULL DEFAULT 'remote',
			desktop_enabled INTEGER NOT NULL DEFAULT 0,
			mobile_enabled INTEGER NOT NULL DEFAULT 0,
			guest_enabled INTEGER NOT NULL DEFAULT 0,
			is_default INTEGER DEFAULT 0,
			group_name VARCHAR(64) DEFAULT '',
			behavior_profile JSON,
			personality_template_name VARCHAR(128) DEFAULT '',
			personality_template_version INTEGER DEFAULT 0,
			personality_prompt TEXT DEFAULT '',
			max_session_queries INTEGER,
			disallowed_tools TEXT,
			runtime_port INTEGER DEFAULT 0,
			deployment_status VARCHAR(32) DEFAULT '',
			deployed_at DATETIME,
			runtime_token TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX agents_gvuk_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(64) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(128) DEFAULT '',
			description TEXT,
			description_en TEXT,
			is_default INTEGER NOT NULL DEFAULT 0,
			source VARCHAR(16) NOT NULL DEFAULT 'custom',
			file_name VARCHAR(255),
			file_url VARCHAR(512),
			file_hash VARCHAR(128),
			file_size INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX tools_gvuk_tenant_name ON tools(tenant_id, name)`,
		`CREATE TABLE agent_tools (
			agent_id INTEGER NOT NULL,
			tool_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, tool_id)
		)`,
		`CREATE TABLE agent_subagents (
			agent_id INTEGER NOT NULL,
			subagent_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, subagent_id)
		)`,
		`CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(128) NOT NULL,
			tenant_id VARCHAR(64) NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE agent_skills (
			agent_id INTEGER NOT NULL,
			skill_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, skill_id)
		)`,
		`CREATE TABLE mcp_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(128) NOT NULL
		)`,
		`CREATE TABLE agent_mcp_servers (
			agent_id INTEGER NOT NULL,
			mcp_server_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, mcp_server_id)
		)`,
		`CREATE TABLE agent_knowledge_datasets (
			agent_id INTEGER NOT NULL,
			dataset_id VARCHAR(128) NOT NULL DEFAULT '',
			created_at DATETIME
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.UploadRecord{}, &scene.Scene{}))

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

// guestVisEnv 持有已铺设 A/B/C/D 四 agent 与两条场景的路由环境。
// roles 经中间件注入（与 jwtutil.IsGuest 的读取键一致），模拟真实
// 认证中间件落下的身份；user_id 供 chat handler MustGet。
type guestVisEnv struct {
	r                              *gin.Engine
	db                             *gorm.DB
	agentA, agentB, agentC, agentD agent.AgentConfig
}

func newGuestVisEnv(t *testing.T, guest bool) *guestVisEnv {
	t.Helper()
	db := setupGuestVisibilityTestDB(t)

	seedAgent := func(name string, guestEnabled, desktop bool, deployStatus string) agent.AgentConfig {
		t.Helper()
		row := agent.AgentConfig{
			Name:             name,
			TenantID:         chatTestTenant,
			GuestEnabled:     guestEnabled,
			DesktopEnabled:   desktop,
			DeploymentStatus: deployStatus,
		}
		require.NoError(t, db.Create(&row).Error)
		return row
	}
	env := &guestVisEnv{
		db:     db,
		agentA: seedAgent("agent-a", true, true, "running"),
		agentB: seedAgent("agent-b", true, false, "running"),
		agentC: seedAgent("agent-c", false, true, "running"),
		agentD: seedAgent("agent-d", false, false, "stopped"),
	}
	require.NoError(t, db.Create(&scene.Scene{
		Name: "scene-a", TenantID: chatTestTenant, AgentID: env.agentA.ID,
		Title: "ta", Prompt: "pa", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&scene.Scene{
		Name: "scene-c", TenantID: chatTestTenant, AgentID: env.agentC.ID,
		Title: "tc", Prompt: "pc", Enabled: true,
	}).Error)

	roles := []string{"member"}
	if guest {
		roles = []string{authdom.RoleGuest}
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", chatTestTenant)
		c.Set("user_id", "u-guest-vis")
		c.Set("roles", roles)
	})

	// deployer service 用零值 config 的真实实例：formal Get 成功路径会触达
	// ComputePendingArtifacts（快照表缺失 → fail-open 记日志不阻塞，200）；
	// 传 nil 会在该分支 nil 解引用（agent_error_test.go 的 nil 约定只适用
	// 于错误先行的路径）。
	// audit recorder 用包内共享的测试 helper（独立 sqlite，main #150 起
	// NewAgentHandler 必传；本测试的读路径不产生审计，但保持真实 recorder
	// 让未来扩展写路径时同样安全）。
	agentH := NewAgentHandler(services.NewAgentService("", ""), services.NewAgentDeployerService(services.AgentDeployerConfig{}), newHandlerTestAuditRecorder(t))
	chatH := NewAgentChatHandler(services.NewAgentChatService(
		repository.NewChatRepository(),
		repository.NewAgentRepository(),
		services.NewAgentDeployerService(services.AgentDeployerConfig{}),
		runtime.NewClient(),
		"127.0.0.1", "", "127.0.0.1",
	))
	sceneH := NewSceneHandler(services.NewSceneService())

	r.GET("/api/v1/agents", agentH.List)
	r.GET("/api/v1/agents/:name", agentH.Get)
	r.POST("/api/v1/agents/:name/chat/sessions", chatH.CreateSession)
	r.GET("/api/v1/agents/:name/chat/sessions/:id/messages", chatH.ListMessages)
	r.DELETE("/api/v1/agents/:name/chat/sessions/:id", chatH.DeleteSession)
	r.GET("/api/v1/scenes", sceneH.List)

	env.r = r
	return env
}

func agentListNames(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var resp struct {
		Data struct {
			Agents []struct {
				Name string `json:"name"`
			} `json:"agents"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	names := make([]string, 0, len(resp.Data.Agents))
	for _, a := range resp.Data.Agents {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	return names
}

func sceneListNames(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body=%s", w.Body.String())
	names := make([]string, 0, len(resp.Data))
	for _, s := range resp.Data {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

func doGet(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// TestAgentList_GuestViews：view=chat + guest → [A,B]；默认 + guest → [A]
// （desktop ∧ guest）；view=chat + formal → running 的 [A,B,C]（D stopped
// 被部署过滤滤掉——chat 视图只显示线上完成部署的，不看 platform）。
func TestAgentList_GuestViews(t *testing.T) {
	t.Run("guest view=chat 只看 guest-enabled", func(t *testing.T) {
		env := newGuestVisEnv(t, true)
		w := doGet(t, env.r, "/api/v1/agents?view=chat")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Equal(t, []string{"agent-a", "agent-b"}, agentListNames(t, w))
	})
	t.Run("guest 默认视图 = desktop ∧ guest", func(t *testing.T) {
		env := newGuestVisEnv(t, true)
		w := doGet(t, env.r, "/api/v1/agents")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Equal(t, []string{"agent-a"}, agentListNames(t, w))
	})
	t.Run("formal view=chat 只含已完成部署", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		w := doGet(t, env.r, "/api/v1/agents?view=chat")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Equal(t, []string{"agent-a", "agent-b", "agent-c"}, agentListNames(t, w))
	})
}

// TestAgentGet_GuestInvisible404：guest 访问未开放 agent-c → 404 且响应体与
// 真 not-found（name=missing）逐字节相同（防枚举）；formal 访问 agent-c 不受
// 影响（200）。
func TestAgentGet_GuestInvisible404(t *testing.T) {
	t.Run("guest 不可见 404 与真不存在逐字节同形", func(t *testing.T) {
		env := newGuestVisEnv(t, true)

		wInvisible := doGet(t, env.r, "/api/v1/agents/agent-c")
		require.Equal(t, http.StatusNotFound, wInvisible.Code, "body=%s", wInvisible.Body.String())
		wMissing := doGet(t, env.r, "/api/v1/agents/no-such-agent")
		require.Equal(t, http.StatusNotFound, wMissing.Code, "body=%s", wMissing.Body.String())

		require.Equal(t, wMissing.Body.Bytes(), wInvisible.Body.Bytes(),
			"guest 不可见与真不存在的 404 响应体必须逐字节相同（防枚举）")
		require.Contains(t, wInvisible.Body.String(), "Agent 不存在")
		require.NotContains(t, wInvisible.Body.String(), "record not found", "gorm 英文诊断不得泄漏")
		require.NotContains(t, wInvisible.Body.String(), "agent-c", "错误体不得回显被探测的 agent 名")
	})
	t.Run("formal 访问未开放 agent 不受影响", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		w := doGet(t, env.r, "/api/v1/agents/agent-c")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	})
}

// TestAgentChat_GuestPrecheck：guest 对未开放 agent 的 chat 入口 → 中性 404
// "agent not found"，与真不存在的 agent 逐字节同形；formal 请求进入业务
// handler（会话创建成功）。
func TestAgentChat_GuestPrecheck(t *testing.T) {
	postSession := func(t *testing.T, r *gin.Engine, agentName string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentName+"/chat/sessions", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("guest 未开放与真不存在同形 404", func(t *testing.T) {
		env := newGuestVisEnv(t, true)

		wInvisible := postSession(t, env.r, "agent-c")
		require.Equal(t, http.StatusNotFound, wInvisible.Code, "body=%s", wInvisible.Body.String())
		require.Contains(t, wInvisible.Body.String(), "agent not found")

		wMissing := postSession(t, env.r, "no-such-agent")
		require.Equal(t, http.StatusNotFound, wMissing.Code, "body=%s", wMissing.Body.String())
		require.Equal(t, wMissing.Body.Bytes(), wInvisible.Body.Bytes(),
			"chat 入口 guest 不可见与真不存在的 404 必须逐字节相同（防枚举）")
	})
	t.Run("formal 进入业务 handler", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		w := postSession(t, env.r, "agent-c")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Contains(t, w.Body.String(), `"success":true`, "formal 请求不得被 guest 前置校验拦截")
	})
}

// TestAgentChat_SessionAgentBinding（PR #151 review P1 回归探针）：URL :name 必须
// 与会话真实归属 Agent 一致——用户（含 guest）经开放 Agent 的 URL 读取/删除自己
// 名下属于其他 Agent 的会话 → 404 且会话不被删除；正确的 :name 下 owner 正常
// 读写删（guest 访问未开放 Agent 仍被前置 guest 校验拦截）。
func TestAgentChat_SessionAgentBinding(t *testing.T) {
	seedSession := func(t *testing.T, env *guestVisEnv, agentName, sessionID string) {
		t.Helper()
		require.NoError(t, env.db.Create(&chat.Session{
			UserID:   "u-guest-vis",
			TenantID: chatTestTenant,
			ID:       sessionID,
			AgentID:  agentName,
			Title:    "sess",
		}).Error)
	}

	t.Run("guest 经开放 Agent URL 读未开放 Agent 会话 → 404", func(t *testing.T) {
		env := newGuestVisEnv(t, true)
		seedSession(t, env, "agent-c", "sess-x")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-a/chat/sessions/sess-x/messages", nil)
		w := httptest.NewRecorder()
		env.r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	})
	t.Run("guest 经开放 Agent URL 删未开放 Agent 会话 → 404 且不删除", func(t *testing.T) {
		env := newGuestVisEnv(t, true)
		seedSession(t, env, "agent-c", "sess-x")

		req := httptest.NewRequest(http.MethodDelete, "/api/v1/agents/agent-a/chat/sessions/sess-x", nil)
		w := httptest.NewRecorder()
		env.r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())

		var count int64
		require.NoError(t, env.db.Model(&chat.Session{}).Where("id = ?", "sess-x").Count(&count).Error)
		require.Equal(t, int64(1), count, "跨 Agent 删除必须被拒绝，会话保留")
	})
	t.Run("formal 跨 Agent URL 同样 404（绑定校验对所有用户生效）", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		seedSession(t, env, "agent-c", "sess-x")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-a/chat/sessions/sess-x/messages", nil)
		w := httptest.NewRecorder()
		env.r.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	})
	t.Run("正确 :name 下 owner 读写删正常（回归）", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		seedSession(t, env, "agent-c", "sess-x")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-c/chat/sessions/sess-x/messages", nil)
		w := httptest.NewRecorder()
		env.r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		req = httptest.NewRequest(http.MethodDelete, "/api/v1/agents/agent-c/chat/sessions/sess-x", nil)
		w = httptest.NewRecorder()
		env.r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

		var count int64
		require.NoError(t, env.db.Model(&chat.Session{}).Where("id = ?", "sess-x").Count(&count).Error)
		require.Equal(t, int64(0), count)
	})
}

// TestScenes_GuestFilter：guest 公开场景列表只保留 guest-enabled agent 的
// 场景；formal 列表不变。
func TestScenes_GuestFilter(t *testing.T) {
	t.Run("guest 只见 guest-enabled agent 的场景", func(t *testing.T) {
		env := newGuestVisEnv(t, true)
		w := doGet(t, env.r, "/api/v1/scenes")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Equal(t, []string{"scene-a"}, sceneListNames(t, w))
	})
	t.Run("formal 场景列表不变", func(t *testing.T) {
		env := newGuestVisEnv(t, false)
		w := doGet(t, env.r, "/api/v1/scenes")
		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		require.Equal(t, []string{"scene-a", "scene-c"}, sceneListNames(t, w))
	})
}
