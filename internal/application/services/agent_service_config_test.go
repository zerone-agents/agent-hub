package services

import (
	"encoding/base64"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMaxSessionQueriesConfigKeys locks the issue #111 rename: the config map
// speaks maxSessionQueries only, and legacy maxSessionTurns requests are
// rejected at the unpack entry before any field is applied (no partial
// unpacking).
func TestMaxSessionQueriesConfigKeys(t *testing.T) {
	t.Run("unpack 新 key", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		if err := unpackConfigToModel(map[string]interface{}{
			"maxSessionQueries": float64(50),
			"systemPrompt":      "p",
		}, cfg, ""); err != nil {
			t.Fatalf("unpackConfigToModel: %v", err)
		}
		if cfg.MaxSessionQueries == nil || *cfg.MaxSessionQueries != 50 {
			t.Fatalf("cfg.MaxSessionQueries = %v, want *50", cfg.MaxSessionQueries)
		}
	})

	t.Run("unpack 旧 key 拒绝", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		err := unpackConfigToModel(map[string]interface{}{
			"maxSessionTurns": float64(50),
			"systemPrompt":    "p",
		}, cfg, "")
		if err == nil {
			t.Fatal("unpackConfigToModel must reject legacy maxSessionTurns key")
		}
		if !strings.Contains(err.Error(), "maxSessionTurns") {
			t.Fatalf("error should mention maxSessionTurns, got: %v", err)
		}
		if cfg.MaxSessionQueries != nil {
			t.Fatalf("cfg.MaxSessionQueries must stay nil (no partial unpack), got %v", *cfg.MaxSessionQueries)
		}
		if cfg.SystemPrompt != "" {
			t.Fatalf("cfg.SystemPrompt must stay empty (no partial unpack), got %q", cfg.SystemPrompt)
		}
	})

	t.Run("pack 新 key", func(t *testing.T) {
		cfg := &agent.AgentConfig{MaxSessionQueries: intPtr(50)}
		m := modelToConfigMap(cfg, "")
		if v, ok := m["maxSessionQueries"].(float64); !ok || v != 50 {
			t.Fatalf(`m["maxSessionQueries"] = %#v, want float64(50)`, m["maxSessionQueries"])
		}
		if _, exists := m["maxSessionTurns"]; exists {
			t.Fatal(`packed map must not contain legacy "maxSessionTurns" key`)
		}
	})

	t.Run("pack nil", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		m := modelToConfigMap(cfg, "")
		v, exists := m["maxSessionQueries"]
		if !exists {
			t.Fatal(`m["maxSessionQueries"] key must be present with explicit nil`)
		}
		if v != nil {
			t.Fatalf(`m["maxSessionQueries"] = %#v, want nil`, v)
		}
	})
}

// TestDisallowedToolsConfigKeys locks the issue #111 agent-local deny list
// config key: unpack accepts a string array and rejects non-string items,
// absent key stays nil; pack writes the array back and keeps unset as
// explicit null (same nil semantics as maxSessionQueries).
func TestDisallowedToolsConfigKeys(t *testing.T) {
	t.Run("unpack 数组", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		if err := unpackConfigToModel(map[string]interface{}{
			"disallowedTools": []interface{}{"Bash", "mcp__knowledge__lookup"},
			"systemPrompt":    "p",
		}, cfg, ""); err != nil {
			t.Fatalf("unpackConfigToModel: %v", err)
		}
		want := []string{"Bash", "mcp__knowledge__lookup"}
		if len(cfg.DisallowedTools) != len(want) || cfg.DisallowedTools[0] != want[0] || cfg.DisallowedTools[1] != want[1] {
			t.Fatalf("cfg.DisallowedTools = %#v, want %#v", cfg.DisallowedTools, want)
		}
	})

	t.Run("unpack 非字符串项报错", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		err := unpackConfigToModel(map[string]interface{}{
			"disallowedTools": []interface{}{"Bash", 42},
		}, cfg, "")
		if err == nil {
			t.Fatal("unpackConfigToModel must reject non-string disallowedTools items")
		}
		if !strings.Contains(err.Error(), "disallowedTools") {
			t.Fatalf("error should mention disallowedTools, got: %v", err)
		}
		if cfg.DisallowedTools != nil {
			t.Fatalf("cfg.DisallowedTools must stay nil on error, got %#v", cfg.DisallowedTools)
		}
	})

	t.Run("unpack 条目 trim 归一", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		if err := unpackConfigToModel(map[string]interface{}{
			"disallowedTools": []interface{}{"  Bash  ", "mcp__knowledge__lookup"},
		}, cfg, ""); err != nil {
			t.Fatalf("unpackConfigToModel: %v", err)
		}
		want := []string{"Bash", "mcp__knowledge__lookup"}
		if len(cfg.DisallowedTools) != len(want) || cfg.DisallowedTools[0] != want[0] || cfg.DisallowedTools[1] != want[1] {
			t.Fatalf("cfg.DisallowedTools = %#v, want %#v (entries must be trimmed so runtime exact-match deny actually applies)", cfg.DisallowedTools, want)
		}
	})

	t.Run("unpack trim 后为空的条目报错", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		err := unpackConfigToModel(map[string]interface{}{
			"disallowedTools": []interface{}{"   "},
		}, cfg, "")
		if err == nil {
			t.Fatal("unpackConfigToModel must reject whitespace-only disallowedTools items")
		}
		if !strings.Contains(err.Error(), "disallowedTools") {
			t.Fatalf("error should mention disallowedTools, got: %v", err)
		}
		if cfg.DisallowedTools != nil {
			t.Fatalf("cfg.DisallowedTools must stay nil on error, got %#v", cfg.DisallowedTools)
		}
	})

	t.Run("unpack 缺省 nil", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		if err := unpackConfigToModel(map[string]interface{}{"systemPrompt": "p"}, cfg, ""); err != nil {
			t.Fatalf("unpackConfigToModel: %v", err)
		}
		if cfg.DisallowedTools != nil {
			t.Fatalf("cfg.DisallowedTools must stay nil when key absent, got %#v", cfg.DisallowedTools)
		}
	})

	t.Run("pack 数组", func(t *testing.T) {
		cfg := &agent.AgentConfig{DisallowedTools: []string{"Bash"}}
		m := modelToConfigMap(cfg, "")
		v, ok := m["disallowedTools"].([]string)
		if !ok || len(v) != 1 || v[0] != "Bash" {
			t.Fatalf(`m["disallowedTools"] = %#v, want []string{"Bash"}`, m["disallowedTools"])
		}
	})

	t.Run("pack nil 显式 null", func(t *testing.T) {
		cfg := &agent.AgentConfig{}
		m := modelToConfigMap(cfg, "")
		v, exists := m["disallowedTools"]
		if !exists {
			t.Fatal(`m["disallowedTools"] key must be present with explicit nil`)
		}
		if v != nil {
			t.Fatalf(`m["disallowedTools"] = %#v, want nil`, v)
		}
	})
}

// setupAgentKnowledgeAuthTestDB 起 sqlite 内存库，建齐
// GetAgentKnowledgeDatasetsForRequest 触碰的三张表：agents、agent_subagents、
// agent_knowledge_datasets。与 setupSubagentToolsTestDB 同款裸 SQL 方案
// （sqlite 索引名全局唯一，AutoMigrate 会撞 uk_name）。
func setupAgentKnowledgeAuthTestDB(t *testing.T) *gorm.DB {
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
			is_default INTEGER DEFAULT 0,
			group_name VARCHAR(64) DEFAULT '',
			max_session_queries INTEGER,
			disallowed_tools TEXT,
			runtime_port INTEGER DEFAULT 0,
			deployment_status VARCHAR(32) DEFAULT '',
			deployed_at DATETIME,
			runtime_token TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX agents_uk_tenant_name ON agents(tenant_id, name)`,
		`CREATE TABLE agent_subagents (
			agent_id INTEGER NOT NULL,
			subagent_id INTEGER NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, subagent_id)
		)`,
		`CREATE TABLE agent_knowledge_datasets (
			agent_id INTEGER NOT NULL,
			dataset_id VARCHAR(64) NOT NULL,
			created_at DATETIME,
			PRIMARY KEY (agent_id, dataset_id)
		)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	return db
}

// mustCreateAgent 建 agent 并断言成功（fixture 种子走与生产相同的 repo 写路径）。
func mustCreateAgent(t *testing.T, tenantID, name string) *agent.AgentConfig {
	t.Helper()
	repo := repository.NewAgentRepository()
	require.NoError(t, repo.Create(tenantID, &agent.AgentConfig{Name: name}))
	cfg, err := repo.GetByName(tenantID, name)
	require.NoError(t, err)
	return cfg
}

// GetAgentKnowledgeDatasetsForRequest 按「服务端可验证的 per-agent
// capability」授权（issue #111 重开）：呈现的 X-Agent-Capability 必须验签
// 通过并绑定 token agent 的 tenant/部署 key/token 指纹，身份必须是 token
// agent 自身或其直接挂载的 subagent；授权集永远只是该身份自己的绑定
// （部署闭包并集语义已废弃——child 不得访问 parent/sibling 的 dataset，
// root 也不得访问 child 的）。缺失 capability 回退 token agent 自身。
func TestGetAgentKnowledgeDatasetsForRequest(t *testing.T) {
	const (
		facadeEncKey = "test-encryption-key"
		runtimeToken = "0123456789abcdef0123456789abcdef"
	)
	// issueCap 按 facade 期望的绑定三元组签发 capability（tenant=default，
	// dep=DeployKey("default","root")，token 指纹默认取 runtimeToken）。
	issueCap := func(agentName, token string) string {
		return issueKnowledgeCapability([]byte(facadeEncKey), knowledgeCapabilityPayload{
			Version: 1,
			Tenant:  "default",
			Dep:     DeployKey("default", "root"),
			Agent:   agentName,
			TokenFp: tokenFingerprint(token),
		})
	}
	newFixture := func(t *testing.T) (*AgentService, *repository.AgentRepository) {
		t.Helper()
		setupAgentKnowledgeAuthTestDB(t)
		agentRepo := repository.NewAgentRepository()
		root := mustCreateAgent(t, "default", "root")
		childA := mustCreateAgent(t, "default", "child-a")
		childB := mustCreateAgent(t, "default", "child-b")
		require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{childA.ID, childB.ID}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root", "ds-shared"}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(childA.ID, []string{"ds-child", "ds-shared"}))
		return NewAgentService(facadeEncKey), agentRepo
	}

	t.Run("root capability → 仅自身绑定", func(t *testing.T) {
		svc, _ := newFixture(t)
		got, requesting, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("root", runtimeToken), runtimeToken)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"ds-root", "ds-shared"}, got)
		assert.Equal(t, "root", requesting)
		assert.NotContains(t, got, "ds-child", "root must not reach child datasets under capability auth")
	})

	t.Run("直接 child capability → 仅 child 自身绑定（parent/sibling 均不可达）", func(t *testing.T) {
		svc, _ := newFixture(t)
		got, requesting, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("child-a", runtimeToken), runtimeToken)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"ds-child", "ds-shared"}, got)
		assert.Equal(t, "child-a", requesting)
		assert.NotContains(t, got, "ds-root", "child must not reach parent datasets")

		// 挂载但无绑定的 sibling：空集而非并集。
		gotB, requestingB, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("child-b", runtimeToken), runtimeToken)
		require.NoError(t, err)
		assert.Empty(t, gotB, "mounted child with no bindings grants nothing")
		assert.Equal(t, "child-b", requestingB)
	})

	t.Run("未挂载的同租户 agent capability → sentinel 拒绝", func(t *testing.T) {
		svc, agentRepo := newFixture(t)
		outsider := mustCreateAgent(t, "default", "outsider")
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(outsider.ID, []string{"ds-outsider"}))

		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("outsider", runtimeToken), runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)
	})

	t.Run("capability 绑定跨租户脏引用身份 → fail-closed 拒绝", func(t *testing.T) {
		setupAgentKnowledgeAuthTestDB(t)
		agentRepo := repository.NewAgentRepository()
		root := mustCreateAgent(t, "default", "root")
		ghost := mustCreateAgent(t, "tenant-b", "ghost")
		// repo 层 ReplaceSubagents 不做租户校验（校验在 service UpdateSubagents），
		// 直接种入脏引用模拟存量数据：GetSubagents 的 JOIN 能取到名字
		// （capability 闭包校验通过），但 default 租户下 GetByName 解析失败
		// ——必须是明确拒绝而非旧闭包的跳过语义。
		require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{ghost.ID}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root"}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(ghost.ID, []string{"ds-ghost"}))

		svc := NewAgentService(facadeEncKey)
		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("ghost", runtimeToken), runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)
	})

	t.Run("同名跨租户身份解析 tenant-scoped", func(t *testing.T) {
		setupAgentKnowledgeAuthTestDB(t)
		agentRepo := repository.NewAgentRepository()
		root := mustCreateAgent(t, "default", "root")
		childDefault := mustCreateAgent(t, "default", "shared-name")
		childTenantB := mustCreateAgent(t, "tenant-b", "shared-name")
		require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{childDefault.ID}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root"}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(childDefault.ID, []string{"ds-child"}))
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(childTenantB.ID, []string{"ds-tenant-b"}))

		svc := NewAgentService(facadeEncKey)
		got, requesting, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("shared-name", runtimeToken), runtimeToken)
		require.NoError(t, err)
		assert.Equal(t, []string{"ds-child"}, got)
		assert.Equal(t, "shared-name", requesting)
		assert.NotContains(t, got, "ds-tenant-b")
	})

	t.Run("capability 缺失回退 token agent 自身", func(t *testing.T) {
		svc, _ := newFixture(t)
		got, requesting, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", "", runtimeToken)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"ds-root", "ds-shared"}, got)
		assert.Equal(t, "root", requesting)
	})

	t.Run("token 轮换：旧 token 签的 capability → TokenFp 失配拒绝", func(t *testing.T) {
		svc, _ := newFixture(t)
		rotated := "ffffffffffffffffffffffffffffffff"
		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("root", rotated), runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)
	})

	t.Run("篡改 capability（payload 换身份保签名）→ 拒绝", func(t *testing.T) {
		svc, _ := newFixture(t)
		orig := issueCap("child-a", runtimeToken)
		parts := strings.Split(orig, ".")
		require.Len(t, parts, 3)
		payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
		require.NoError(t, err)
		// 解码 payload 段换身份后原样保签名重编码——签名覆盖的是原始字节，
		// 篡改必然失配。
		forged := strings.Replace(string(payloadJSON), `"a":"child-a"`, `"a":"root"`, 1)
		require.NotEqual(t, string(payloadJSON), forged, "fixture payload must contain the agent field")
		tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(forged)) + "." + parts[2]
		_, _, err = svc.GetAgentKnowledgeDatasetsForRequest("default", "root", tampered, runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)
	})

	t.Run("跨部署 capability（dep 绑定他人）→ 拒绝", func(t *testing.T) {
		svc, _ := newFixture(t)
		crossDep := issueKnowledgeCapability([]byte(facadeEncKey), knowledgeCapabilityPayload{
			Version: 1, Tenant: "default", Dep: DeployKey("default", "another-root"),
			Agent: "root", TokenFp: tokenFingerprint(runtimeToken),
		})
		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", crossDep, runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)
	})

	t.Run("服务端未配置签名密钥：呈现 capability 一律拒绝（不回退）", func(t *testing.T) {
		setupAgentKnowledgeAuthTestDB(t)
		agentRepo := repository.NewAgentRepository()
		root := mustCreateAgent(t, "default", "root")
		require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root"}))

		svc := NewAgentService("") // dev-mode 无密钥
		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", issueCap("root", runtimeToken), runtimeToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKnowledgeCapabilityDenied)

		// 缺失 capability 的存量回退不受影响（token agent 自身绑定）。
		got, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "root", "", runtimeToken)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"ds-root"}, got)
	})

	t.Run("token agent 不存在", func(t *testing.T) {
		setupAgentKnowledgeAuthTestDB(t)
		svc := NewAgentService(facadeEncKey)
		_, _, err := svc.GetAgentKnowledgeDatasetsForRequest("default", "no-such-agent", issueCap("no-such-agent", runtimeToken), runtimeToken)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不存在")
	})
}
