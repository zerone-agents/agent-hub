package services

import (
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

// setupAgentKnowledgeClosureTestDB 起 sqlite 内存库，建齐
// GetAgentKnowledgeClosureDatasets 触碰的三张表：agents、agent_subagents、
// agent_knowledge_datasets。与 setupSubagentToolsTestDB 同款裸 SQL 方案
// （sqlite 索引名全局唯一，AutoMigrate 会撞 uk_name）。
func setupAgentKnowledgeClosureTestDB(t *testing.T) *gorm.DB {
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

// 部署闭包并集：root 自身绑定 ∪ 直接 children 绑定，去重 + 排序稳定化；
// 同时锁定原 GetAgentKnowledgeDatasets 语义未动（detail 展示用自身绑定）。
func TestGetAgentKnowledgeClosureDatasets_UnionDedupSorted(t *testing.T) {
	setupAgentKnowledgeClosureTestDB(t)
	agentRepo := repository.NewAgentRepository()
	root := mustCreateAgent(t, "default", "root")
	child := mustCreateAgent(t, "default", "child")
	require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{child.ID}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-shared", "ds-root"}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(child.ID, []string{"ds-child", "ds-shared"}))

	svc := NewAgentService("test-encryption-key")
	got, err := svc.GetAgentKnowledgeClosureDatasets("default", "root")
	require.NoError(t, err)
	assert.Equal(t, []string{"ds-child", "ds-root", "ds-shared"}, got)

	// 原函数保持"自身绑定"语义：不含 child 的 dataset。
	selfOnly, err := svc.GetAgentKnowledgeDatasets("default", "root")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ds-root", "ds-shared"}, selfOnly)
}

// 存量脏引用（agent_subagents 指向别租户 agent）不得让 root 的知识检索整体
// 失败：跳过该成员回退自身绑定，别租户 dataset 不进并集。新数据由部署
// 校验拦截，鉴权路径 fail-safe。
func TestGetAgentKnowledgeClosureDatasets_SkipsUnresolvableSubagent(t *testing.T) {
	setupAgentKnowledgeClosureTestDB(t)
	agentRepo := repository.NewAgentRepository()
	root := mustCreateAgent(t, "default", "root")
	ghost := mustCreateAgent(t, "tenant-b", "ghost")
	// repo 层 ReplaceSubagents 不做租户校验（校验在 service UpdateSubagents），
	// 直接种入脏引用模拟存量数据：GetSubagents 的 JOIN 能取到名字，但
	// default 租户下 GetByName 解析失败。
	require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{ghost.ID}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root"}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(ghost.ID, []string{"ds-ghost"}))

	svc := NewAgentService("test-encryption-key")
	got, err := svc.GetAgentKnowledgeClosureDatasets("default", "root")
	require.NoError(t, err, "single bad reference must not fail the whole closure")
	assert.Equal(t, []string{"ds-root"}, got)
}

// 同名 agent 跨租户隔离：闭包成员解析全部 tenant-scoped，tenant-b 同名
// agent 的 dataset 不得混入 default 租户 root 的授权集。
func TestGetAgentKnowledgeClosureDatasets_TenantScopedResolution(t *testing.T) {
	setupAgentKnowledgeClosureTestDB(t)
	agentRepo := repository.NewAgentRepository()
	root := mustCreateAgent(t, "default", "root")
	childDefault := mustCreateAgent(t, "default", "shared-name")
	childTenantB := mustCreateAgent(t, "tenant-b", "shared-name")
	require.NoError(t, agentRepo.ReplaceSubagents(root.ID, []uint64{childDefault.ID}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(root.ID, []string{"ds-root"}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(childDefault.ID, []string{"ds-child"}))
	require.NoError(t, agentRepo.ReplaceAgentKnowledgeDatasets(childTenantB.ID, []string{"ds-tenant-b"}))

	svc := NewAgentService("test-encryption-key")
	got, err := svc.GetAgentKnowledgeClosureDatasets("default", "root")
	require.NoError(t, err)
	assert.Equal(t, []string{"ds-child", "ds-root"}, got)
	assert.NotContains(t, got, "ds-tenant-b")
}
