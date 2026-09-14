package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPersonaMcpRouter wires the organization MCP handler with the four real
// H6 persona packs over sqlite, with two runtime tokens resolving to two
// distinct agents (identity from token, never from tool arguments).
func setupPersonaMcpRouter(t *testing.T) (*gin.Engine, *services.RunService, agent.AgentConfig, agent.AgentConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}))

	a := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	b := agent.AgentConfig{Name: "agent-b", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	started := time.Now().UTC()
	require.NoError(t, db.Create(&rundomain.Run{ID: "run-h6", TenantID: "tenant-a", Name: "case", Status: rundomain.StatusRunning, StartedAt: &started}).Error)
	for _, item := range []agent.AgentConfig{a, b} {
		require.NoError(t, db.Create(&rundomain.RunAgent{TenantID: "tenant-a", RunID: "run-h6", AgentID: item.ID, AgentNameSnapshot: item.Name}).Error)
	}

	runService := services.NewRunService(db)
	h := NewOrganizationMcpHandler(&fakeOrganizationMessageService{})
	h.SetPersonaServices(
		services.NewEmotionService(runService),
		services.NewBeliefService(runService),
		services.NewMemoryService(runService),
		services.NewRelationDynamicsService(runService),
	)
	h.SetPersonaRunResolver(runService.ActiveRunForAgent)

	agents := map[string]agent.AgentConfig{"token-a": a, "token-b": b}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		auth := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		identity, ok := agents[auth]
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("agent", &identity)
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	}, h.HandleMessage)
	return router, runService, a, b
}

func callPersonaTool(t *testing.T, router http.Handler, token, name, arguments string) (map[string]interface{}, bool) {
	t.Helper()
	rec := postOrganizationRPC(t, router, "tools/call", json.RawMessage(fmt.Sprintf(`{"name":%q,"arguments":%s}`, name, arguments)), token)
	require.Equal(t, http.StatusOK, rec.Code)
	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	if response.Result.IsError {
		return map[string]interface{}{"error": response.Result.Content[0].Text}, true
	}
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &payload))
	return payload, false
}

func TestOrganizationMcpPersonaToolsListed(t *testing.T) {
	router, _, _, _ := setupPersonaMcpRouter(t)
	rec := postOrganizationRPC(t, router, "tools/list", nil, "token-a")
	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	names := make([]string, 0, len(response.Result.Tools))
	for _, tool := range response.Result.Tools {
		names = append(names, tool.Name)
	}
	for _, want := range []string{"emotion_status", "belief_list", "belief_claim", "memory_record", "memory_recall", "relation_view"} {
		require.Contains(t, names, want)
	}
}

func TestOrganizationMcpPersonaToolsDisabledWithoutServices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewOrganizationMcpHandler(&fakeOrganizationMessageService{})
	router := gin.New()
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		c.Set("agent", &agent.AgentConfig{ID: 41, Name: "agent-a", TenantID: "tenant-a"})
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	}, h.HandleMessage)

	for _, tool := range []string{"emotion_status", "belief_list", "belief_claim", "memory_record", "memory_recall", "relation_view"} {
		_, isError := callPersonaTool(t, router, "", tool, `{}`)
		require.True(t, isError, tool)
	}
	// tools requiring args still return 未启用 before argument parsing
	_, isError := callPersonaTool(t, router, "", "relation_view", `{"target_agent_id":42}`)
	require.True(t, isError)
}

func TestOrganizationMcpPersonaToolsRequireRunSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}))
	a := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&a).Error)

	runService := services.NewRunService(db)
	h := NewOrganizationMcpHandler(&fakeOrganizationMessageService{})
	h.SetPersonaServices(services.NewEmotionService(runService), services.NewBeliefService(runService), services.NewMemoryService(runService), services.NewRelationDynamicsService(runService))
	h.SetPersonaRunResolver(runService.ActiveRunForAgent)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		c.Set("agent", &a)
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	}, h.HandleMessage)

	payload, isError := callPersonaTool(t, router, "", "emotion_status", `{}`)
	require.True(t, isError)
	require.Equal(t, "仅运行会话可用", payload["error"])
}

func TestOrganizationMcpPersonaToolsEndToEndAndIsolation(t *testing.T) {
	router, _, _, b := setupPersonaMcpRouter(t)

	// emotion_status returns the baseline default for a fresh agent.
	payload, isError := callPersonaTool(t, router, "token-a", "emotion_status", `{}`)
	require.False(t, isError)
	require.NotEmpty(t, payload["mood"])
	require.Equal(t, "calm", payload["mood"])

	// belief claim → list roundtrip (own beliefs only).
	payload, isError = callPersonaTool(t, router, "token-a", "belief_claim", `{"fact_ref":"fact:scandal","statement":"我听说账目有问题"}`)
	require.False(t, isError)
	require.True(t, strings.HasPrefix(payload["factRef"].(string), "claim:"))
	payload, isError = callPersonaTool(t, router, "token-a", "belief_list", `{"fact_ref":"fact:scandal"}`)
	require.False(t, isError)
	require.Len(t, payload["beliefs"], 0, "claim creates a claim-scoped fact, the parent fact was never delivered")

	// memory record → recall roundtrip.
	_, isError = callPersonaTool(t, router, "token-a", "memory_record", `{"fact_ref":"fact:scandal","interpretation":"那次会议气氛很紧张","importance":80}`)
	require.False(t, isError)
	payload, isError = callPersonaTool(t, router, "token-a", "memory_recall", `{}`)
	require.False(t, isError)
	memories := payload["memories"].([]interface{})
	require.Len(t, memories, 1)
	require.Equal(t, "那次会议气氛很紧张", memories[0].(map[string]interface{})["interpretation"])

	// Isolation: agent-b recalls nothing of agent-a's memory, and reads its
	// own default states (identity comes from the token, not arguments).
	payload, isError = callPersonaTool(t, router, "token-b", "memory_recall", `{}`)
	require.False(t, isError)
	require.Len(t, payload["memories"], 0)
	payload, isError = callPersonaTool(t, router, "token-b", "emotion_status", `{}`)
	require.False(t, isError)
	require.Equal(t, "calm", payload["mood"])

	// relation_view: default neutral attitude toward the counterpart.
	payload, isError = callPersonaTool(t, router, "token-a", "relation_view", fmt.Sprintf(`{"target_agent_id":%d}`, b.ID))
	require.False(t, isError)
	require.Equal(t, "neutral", payload["stance"])
}

// TestOrganizationMcpPersonaToolsDisabledByGate：管理员经 H7 扩展生命周期
// 停用内置人物能力扩展后，对应 MCP 工具立即返回"能力 <pack> 已被停用"，
// 未停用的能力不受影响；重新启用后恢复（H7 P1 门控）。
func TestOrganizationMcpPersonaToolsDisabledByGate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.AgentConfig{}, &rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{}, &extension.Extension{}, &extension.Version{}, &extension.Install{}))

	a := agent.AgentConfig{Name: "agent-a", TenantID: "tenant-a"}
	require.NoError(t, db.Create(&a).Error)
	started := time.Now().UTC()
	require.NoError(t, db.Create(&rundomain.Run{ID: "run-gate", TenantID: "tenant-a", Name: "case", Status: rundomain.StatusRunning, StartedAt: &started}).Error)
	require.NoError(t, db.Create(&rundomain.RunAgent{TenantID: "tenant-a", RunID: "run-gate", AgentID: a.ID, AgentNameSnapshot: a.Name}).Error)

	runService := services.NewRunService(db)
	lifecycle := services.NewExtensionLifecycleService(db)
	require.NoError(t, lifecycle.EnsureBuiltinPersonaPacks())

	h := NewOrganizationMcpHandler(&fakeOrganizationMessageService{})
	h.SetPersonaServices(
		services.NewEmotionService(runService),
		services.NewBeliefService(runService),
		services.NewMemoryService(runService),
		services.NewRelationDynamicsService(runService),
	)
	h.SetPersonaRunResolver(runService.ActiveRunForAgent)
	h.SetPersonaGate(services.NewPersonaCapabilityGate(db))

	agents := map[string]agent.AgentConfig{"token-a": a}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/organization/mcp", func(c *gin.Context) {
		auth := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		identity, ok := agents[auth]
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Set("agent", &identity)
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	}, h.HandleMessage)

	// 默认启用：emotion_status 正常返回。
	_, isErr := callPersonaTool(t, router, "token-a", "emotion_status", `{}`)
	require.False(t, isErr)

	// 停用 emotion 与 memory（default 平台级；运行时租户 tenant-a 回退命中）。
	disable := func(name string) {
		var ext extension.Extension
		require.NoError(t, db.Where("tenant_id=? AND name=?", "default", name).First(&ext).Error)
		_, err := lifecycle.Disable("default", ext.ID)
		require.NoError(t, err)
	}
	disable("io.zerone.emotion")
	disable("io.zerone.memory")

	payload, isErr := callPersonaTool(t, router, "token-a", "emotion_status", `{}`)
	require.True(t, isErr)
	require.Contains(t, payload["error"], "能力 emotion 已被停用")

	payload, isErr = callPersonaTool(t, router, "token-a", "memory_record", `{"fact_ref":"f1","interpretation":"记忆"}`)
	require.True(t, isErr)
	require.Contains(t, payload["error"], "能力 memory 已被停用")

	payload, isErr = callPersonaTool(t, router, "token-a", "memory_recall", `{}`)
	require.True(t, isErr)
	require.Contains(t, payload["error"], "能力 memory 已被停用")

	// 未停用的 belief 能力不受影响。
	_, isErr = callPersonaTool(t, router, "token-a", "belief_list", `{}`)
	require.False(t, isErr)

	// 重新启用 emotion：立即恢复。
	var emotionExt extension.Extension
	require.NoError(t, db.Where("tenant_id=? AND name=?", "default", "io.zerone.emotion").First(&emotionExt).Error)
	_, err = lifecycle.Enable("default", emotionExt.ID)
	require.NoError(t, err)
	_, isErr = callPersonaTool(t, router, "token-a", "emotion_status", `{}`)
	require.False(t, isErr)
}
