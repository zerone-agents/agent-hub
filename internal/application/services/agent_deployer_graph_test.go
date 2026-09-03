package services

// agent_deployer_graph_test.go covers the issue #111 agent-graph deployment
// construction: Deploy must send the complete closure of the root agent
// (root + directly mounted subagents), each node carrying its own
// capabilities with no inheritance, root-only runtime-global fields, and
// explicit failure on any graph violation (missing/self/cycle/depth/collision).

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/skill"
	repository "control-panel/internal/infrastructure/persistence"
)

// ── map-backed per-agent mocks ────────────────────────────────────────────
// The flat mocks in agent_deployer_test.go return the same payload for every
// agent; graph construction needs per-agent relations, hence these keyed
// variants. They implement the same service interfaces as the flat ones.

type graphToolRepo struct{ byAgent map[uint64][]*agent.Tool }

func (m *graphToolRepo) GetToolRecordsByAgent(agentID uint64) ([]*agent.Tool, error) {
	return m.byAgent[agentID], nil
}

type graphSkillRepo struct{ byAgent map[uint64][]*skill.Skill }

func (m *graphSkillRepo) GetAgentSkills(agentID uint64) ([]string, error) { return nil, nil }
func (m *graphSkillRepo) GetAgentSkillsFull(agentID uint64) ([]*skill.Skill, error) {
	return m.byAgent[agentID], nil
}

type graphMcpSvc struct {
	byAgent map[string]map[string]*McpClientDTO
}

func (m *graphMcpSvc) GetClientMcpsByAgent(tenantID, name string) (map[string]*McpClientDTO, error) {
	return m.byAgent[tenantID+"/"+name], nil
}

type graphKnowledgeSvc struct{ byID map[string]knowledge.Dataset }

func (m *graphKnowledgeSvc) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	if ds, ok := m.byID[id]; ok {
		return &ds, nil
	}
	return nil, fmt.Errorf("dataset %s not found", id)
}

// graphKnowledgeMcp mimics the built-in knowledge MCP as GetClientMcpsByAgent
// serves it per-agent (headers already decrypted). The stale X-Agent-Id decoy
// mimics a same-named key configured on the MCP server — the deployment owns
// that key exclusively and must override it with the node's own identity
// (issue #111 review F1).
func graphKnowledgeMcp() *McpClientDTO {
	return &McpClientDTO{
		Name: "knowledge", Type: "http", URL: "https://hub.example.com/api/v1/knowledge/mcp",
		Headers: map[string]string{
			"Authorization": BuiltinKnowledgeAuthHeader,
			"X-Agent-Id":    "stale-configured-identity",
		},
		Tools: builtinKnowledgeTools,
	}
}

// ── fixture universe ──────────────────────────────────────────────────────

// graphWorld is the in-memory tenant-isolated agent universe: agents keyed by
// "tenant/name", per-agent relations keyed by agent ID.
type graphWorld struct {
	agents      map[string]*agent.AgentConfig
	subagents   map[uint64][]string
	tools       map[uint64][]*agent.Tool
	skills      map[uint64][]*skill.Skill
	mcps        map[string]map[string]*McpClientDTO
	datasets    map[uint64][]string
	datasetMeta map[string]knowledge.Dataset
	nextID      uint64
}

func newGraphWorld() *graphWorld {
	return &graphWorld{
		agents:      map[string]*agent.AgentConfig{},
		subagents:   map[uint64][]string{},
		tools:       map[uint64][]*agent.Tool{},
		skills:      map[uint64][]*skill.Skill{},
		mcps:        map[string]map[string]*McpClientDTO{},
		datasets:    map[uint64][]string{},
		datasetMeta: map[string]knowledge.Dataset{},
	}
}

func (w *graphWorld) addAgent(tenantID, name string, mutate func(*agent.AgentConfig)) *agent.AgentConfig {
	w.nextID++
	providerID := uint64(1)
	cfg := &agent.AgentConfig{ID: w.nextID, Name: name, TenantID: tenantID, ProviderID: &providerID}
	if mutate != nil {
		mutate(cfg)
	}
	if cfg.ProviderID == nil {
		cfg.ProviderID = &providerID
	}
	w.agents[tenantID+"/"+name] = cfg
	return cfg
}

func (w *graphWorld) agentRepo() *mockAgentRepo {
	return &mockAgentRepo{
		getByNameFunc: func(tenantID, name string) (*agent.AgentConfig, error) {
			cfg, ok := w.agents[tenantID+"/"+name]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return cfg, nil
		},
		getSubagentsFunc: func(agentID uint64) ([]string, error) {
			return w.subagents[agentID], nil
		},
		getKnowledgeDatasetIDsByAgent: func(agentID uint64) ([]string, error) {
			return w.datasets[agentID], nil
		},
		updateFunc: func(tenantID string, a *agent.AgentConfig) error { return nil },
	}
}

func (w *graphWorld) service(t *testing.T, deployerURL string) *AgentDeployerService {
	t.Helper()
	s := newTestAgentDeployerService(t, deployerURL, w.agentRepo(), deployTokenProviderSvc())
	s.toolRepo = &graphToolRepo{byAgent: w.tools}
	s.skillRepo = &graphSkillRepo{byAgent: w.skills}
	s.mcpSvc = &graphMcpSvc{byAgent: w.mcps}
	s.knowledgeSvc = &graphKnowledgeSvc{byID: w.datasetMeta}
	s.cdnHost = "https://cdn.example.com"
	return s
}

// graphFixture bundles the brief's matrix universe with named handles.
type graphFixture struct {
	world  *graphWorld
	parent *agent.AgentConfig
	childA *agent.AgentConfig
	childB *agent.AgentConfig
}

// buildGraphFixture builds the brief's universe: tenant-a parent (full
// capabilities), child-a (full capabilities of its own, all distinct from
// parent's), child-b (capability relations all empty); tenant-b holds
// same-name parent/child-a with clearly distinguishable confidential markers.
func buildGraphFixture(t *testing.T) *graphFixture {
	t.Helper()
	w := newGraphWorld()

	parent := w.addAgent("tenant-a", "parent", func(c *agent.AgentConfig) {
		c.SystemPrompt = "parent-system-prompt"
		c.ModelID = "root-model"
		c.PermissionMode = "plan"
		c.MaxSessionQueries = intPtr(42)
		c.Description = map[string]string{"zh": "父 Agent", "en": "parent agent"}
		c.DisallowedTools = []string{"Bash"}
	})
	childA := w.addAgent("tenant-a", "child-a", func(c *agent.AgentConfig) {
		c.SystemPrompt = "child-a-system-prompt"
		// Root-only fields present in DB must NOT reach the wire for children.
		c.ModelID = "child-a-model-must-not-leak"
		c.PermissionMode = "bypassPermissions"
		c.MaxSessionQueries = intPtr(99)
		// Agent-local deny list (issue #111 F3a): children carry their own.
		c.DisallowedTools = []string{"WebSearch", "mcp__knowledge__lookup"}
	})
	childB := w.addAgent("tenant-a", "child-b", func(c *agent.AgentConfig) {
		c.SystemPrompt = "child-b-system-prompt"
	})
	bParent := w.addAgent("tenant-b", "parent", func(c *agent.AgentConfig) {
		c.SystemPrompt = "tenant-b-parent-prompt-CONFIDENTIAL"
		c.ModelID = "tenant-b-model"
	})
	bChild := w.addAgent("tenant-b", "child-a", func(c *agent.AgentConfig) {
		c.SystemPrompt = "tenant-b-child-prompt-CONFIDENTIAL"
	})

	// Mounts: tenant-a parent → [child-a, child-b]; tenant-b parent → [child-a].
	w.subagents[parent.ID] = []string{"child-a", "child-b"}
	w.subagents[bParent.ID] = []string{"child-a"}

	// Parent capabilities.
	w.tools[parent.ID] = []*agent.Tool{
		{Name: "Read", Source: agent.ToolSourceBuiltin},
		{Name: "parent-custom-tool", Source: agent.ToolSourceCustom,
			FileName: "pct.ts", FileURL: "tools/tenant-a/parent-custom-tool/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.ts",
			FileHash: "1111111111111111111111111111111111111111111111111111111111111111", FileSize: 128},
	}
	w.mcps["tenant-a/parent"] = map[string]*McpClientDTO{
		"parent-mcp": {Name: "parent-mcp", Type: "http", URL: "https://mcp.example.com/parent",
			Tools: []McpTool{{Name: "parent_lookup"}}},
		"knowledge": graphKnowledgeMcp(),
	}
	w.skills[parent.ID] = []*skill.Skill{
		{Name: "parent-skill", URL: "skills/tenant-a/parent-skill/phash", FileHash: "2222222222222222222222222222222222222222222222222222222222222222"},
	}
	w.datasets[parent.ID] = []string{"ds-parent"}
	w.datasetMeta["ds-parent"] = knowledge.Dataset{"description": "parent dataset description"}

	// Child-a capabilities: all its own, distinct from parent's.
	w.tools[childA.ID] = []*agent.Tool{
		{Name: "Bash", Source: agent.ToolSourceBuiltin},
		{Name: "child-a-custom-tool", Source: agent.ToolSourceCustom,
			FileName: "cat.ts", FileURL: "tools/tenant-a/child-a-custom-tool/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ts",
			FileHash: "3333333333333333333333333333333333333333333333333333333333333333", FileSize: 128},
	}
	w.mcps["tenant-a/child-a"] = map[string]*McpClientDTO{
		"child-a-mcp": {Name: "child-a-mcp", Type: "sse", URL: "https://mcp.example.com/child-a",
			Tools: []McpTool{{Name: "child_lookup"}}},
		"knowledge": graphKnowledgeMcp(),
	}
	w.skills[childA.ID] = []*skill.Skill{
		{Name: "child-a-skill", URL: "skills/tenant-a/child-a-skill/chash", FileHash: "4444444444444444444444444444444444444444444444444444444444444444"},
	}
	w.datasets[childA.ID] = []string{"ds-child-a"}
	w.datasetMeta["ds-child-a"] = knowledge.Dataset{"description": "child-a dataset description"}

	// Child-b: deliberately no entries — empty relations stay empty.

	// Tenant-b capabilities (must never leak into a tenant-a deploy).
	w.mcps["tenant-b/parent"] = map[string]*McpClientDTO{
		"tenant-b-only-mcp": {Name: "tenant-b-only-mcp", Type: "http", URL: "https://mcp.example.com/tenant-b"},
	}
	w.datasets[bParent.ID] = []string{"ds-tenant-b-CONFIDENTIAL"}
	w.datasetMeta["ds-tenant-b-CONFIDENTIAL"] = knowledge.Dataset{"description": "tenant-b secret dataset"}
	w.skills[bChild.ID] = []*skill.Skill{
		{Name: "tenant-b-skill", URL: "skills/tenant-b/leak", FileHash: "5555555555555555555555555555555555555555555555555555555555555555"},
	}

	return &graphFixture{world: w, parent: parent, childA: childA, childB: childB}
}

// deployGraphParent deploys tenant-a/parent against a recording fake deployer
// and returns the parsed create-request body plus the capture fixture.
func deployGraphParent(t *testing.T, fx *graphFixture) (map[string]any, *deployTokenFixture) {
	t.Helper()
	f := &deployTokenFixture{}
	srv := newDeployTokenServer(t, false, f)
	t.Cleanup(srv.Close)
	s := fx.world.service(t, srv.URL)
	if _, err := s.Deploy("tenant-a", "parent", false, false); err != nil {
		t.Fatalf("deploy tenant-a/parent: %v", err)
	}
	if !f.postCalled {
		t.Fatal("expected deployer create (POST) to be called")
	}
	var body map[string]any
	if err := json.Unmarshal(f.postBody, &body); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	return body, f
}

// graphAgents extracts the agents array as object maps (root first).
func graphAgents(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["agents"].([]any)
	require.True(t, ok, "body should carry an agents array, got %T", body["agents"])
	out := make([]map[string]any, 0, len(raw))
	for _, a := range raw {
		m, ok := a.(map[string]any)
		require.True(t, ok, "agent node should be an object, got %T", a)
		out = append(out, m)
	}
	return out
}

// graphSourceNames extracts the "name" field of an array-of-objects node
// entry (customTools / skills).
func graphSourceNames(t *testing.T, node map[string]any, key string) []string {
	t.Helper()
	raw, ok := node[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		require.True(t, ok, "%s entry should be an object, got %T", key, v)
		out = append(out, fmt.Sprintf("%v", m["name"]))
	}
	return out
}

// TestDeploy_AgentGraph is the happy-path matrix (brief Step 1 assertions
// 1-6 and 8). Assertion 7 lives in TestLoadAgentGraph_ValidationMatrix and
// assertion 9 in TestUpdateSubagents_OneLevelInvariants. Assertion 10 (review
// F1) locks the per-node deployment identity on the knowledge MCP headers.
func TestDeploy_AgentGraph(t *testing.T) {
	t.Run("1: body is rootAgentId + agents[] with no legacy top-level keys", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		require.Contains(t, body, "rootAgentId")
		require.Contains(t, body, "agents")
		for _, legacy := range []string{"agent", "prompt", "maxSessionTurns"} {
			require.NotContains(t, body, legacy, "legacy top-level key %q must be gone", legacy)
		}
	})

	t.Run("2: bare rootAgentId + scoped deploymentKey; agents holds root + child-a + child-b; root.subagents are pure ids", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		require.Equal(t, "parent", body["rootAgentId"])
		require.Equal(t, "tenant-a-parent", body["deploymentKey"])
		nodes := graphAgents(t, body)
		require.Len(t, nodes, 3)
		require.Equal(t, "parent", nodes[0]["name"])
		require.Equal(t, "child-a", nodes[1]["name"])
		require.Equal(t, "child-b", nodes[2]["name"])
		require.Equal(t, []any{"child-a", "child-b"}, nodes[0]["subagents"])
	})

	t.Run("3: per-agent capabilities isolate; child-b keeps empty relations empty", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		nodes := graphAgents(t, body)
		require.Len(t, nodes, 3)
		root, childA, childB := nodes[0], nodes[1], nodes[2]

		// Root carries only its own MCP / customTools / skills / datasets.
		rootMcp := root["mcpServers"].(map[string]any)
		require.Contains(t, rootMcp, "parent-mcp")
		require.NotContains(t, rootMcp, "child-a-mcp")
		require.Contains(t, graphSourceNames(t, root, "customTools"), "parent-custom-tool")
		require.NotContains(t, graphSourceNames(t, root, "customTools"), "child-a-custom-tool")
		require.Contains(t, graphSourceNames(t, root, "skills"), "parent-skill")
		require.Equal(t, map[string]any{"ds-parent": "parent dataset description"}, root["datasets"])
		// Per-agent MCP loading also feeds each allow-list with its own
		// SDK-qualified MCP tool names only.
		require.Contains(t, root["tools"], "mcp__parent-mcp__parent_lookup")
		require.NotContains(t, root["tools"], "mcp__child-a-mcp__child_lookup")

		// Child-a carries only its own.
		childMcp := childA["mcpServers"].(map[string]any)
		require.Contains(t, childMcp, "child-a-mcp")
		require.NotContains(t, childMcp, "parent-mcp")
		require.Contains(t, graphSourceNames(t, childA, "customTools"), "child-a-custom-tool")
		require.NotContains(t, graphSourceNames(t, childA, "customTools"), "parent-custom-tool")
		require.Contains(t, graphSourceNames(t, childA, "skills"), "child-a-skill")
		require.Equal(t, map[string]any{"ds-child-a": "child-a dataset description"}, childA["datasets"])
		require.Contains(t, childA["tools"], "mcp__child-a-mcp__child_lookup")
		require.NotContains(t, childA["tools"], "mcp__parent-mcp__parent_lookup")

		// Child-b: no capability relations at all.
		for _, key := range []string{"mcpServers", "customTools", "skills", "datasets", "settingSources"} {
			require.NotContains(t, childB, key, "child-b key %q must stay absent", key)
		}
	})

	t.Run("4: model/maxSessionQueries/permissionMode are root-only", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		nodes := graphAgents(t, body)
		root := nodes[0]
		require.Equal(t, "root-model", root["model"])
		require.Equal(t, "plan", root["permissionMode"])
		for _, child := range nodes[1:] {
			for _, key := range []string{"model", "maxSessionQueries", "permissionMode"} {
				require.NotContains(t, child, key, "child node must not declare root-only key %q", key)
			}
		}
	})

	t.Run("5: settingSources==[user] iff the node declares skills", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		nodes := graphAgents(t, body)
		require.Equal(t, []any{"user"}, nodes[0]["settingSources"]) // parent has skills
		require.Equal(t, []any{"user"}, nodes[1]["settingSources"]) // child-a has skills
		require.NotContains(t, nodes[2], "settingSources")          // child-b has none
	})

	t.Run("6: tenant-b same-name agents never leak into the request", func(t *testing.T) {
		_, f := deployGraphParent(t, buildGraphFixture(t))
		bodyStr := string(f.postBody)
		for _, marker := range []string{
			"tenant-b-parent-prompt-CONFIDENTIAL",
			"tenant-b-child-prompt-CONFIDENTIAL",
			"tenant-b-only-mcp",
			"ds-tenant-b-CONFIDENTIAL",
			"tenant-b-skill",
			"tenant-b-model",
		} {
			require.NotContains(t, bodyStr, marker)
		}
	})

	t.Run("8: maxSessionQueries flows from parent config to the root node", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		root := graphAgents(t, body)[0]
		require.Equal(t, float64(42), root["maxSessionQueries"])
	})

	t.Run("10: knowledge MCP headers carry each node's deployment identity", func(t *testing.T) {
		body, f := deployGraphParent(t, buildGraphFixture(t))
		nodes := graphAgents(t, body)
		require.Len(t, nodes, 3)

		knowledgeHeaders := func(t *testing.T, node map[string]any) map[string]any {
			t.Helper()
			servers, ok := node["mcpServers"].(map[string]any)
			require.True(t, ok, "node should carry mcpServers, got %T", node["mcpServers"])
			knowledge, ok := servers["knowledge"].(map[string]any)
			require.True(t, ok, "knowledge MCP should be mounted, got %T", servers["knowledge"])
			headers, ok := knowledge["headers"].(map[string]any)
			require.True(t, ok, "knowledge MCP should carry headers, got %T", knowledge["headers"])
			return headers
		}

		// Root carries its DB bare name — NOT the tenant-scoped deploy key
		// "tenant-a-parent": the hub authorizer resolves X-Agent-Id with a
		// tenant-scoped GetByName, and the deploy key is not a DB name.
		// Equality against "parent" also proves the stale same-named key
		// configured on the MCP server (fixture decoy) was overridden.
		require.Equal(t, "parent", knowledgeHeaders(t, nodes[0])["X-Agent-Id"])
		// Child carries its own bare name, never the root's.
		require.Equal(t, "child-a", knowledgeHeaders(t, nodes[1])["X-Agent-Id"])
		// resolveMcpHeaders must preserve the injected identity key while
		// substituting the Authorization placeholder with the real token.
		require.Equal(t, "Bearer "+f.sentToken(t), knowledgeHeaders(t, nodes[0])["Authorization"])
	})

	t.Run("11: disallowedTools is agent-local; empty stays absent; never cross-copied", func(t *testing.T) {
		body, _ := deployGraphParent(t, buildGraphFixture(t))
		nodes := graphAgents(t, body)
		root, childA, childB := nodes[0], nodes[1], nodes[2]

		// Each node carries exactly its own deny list (parent ["Bash"],
		// child-a ["WebSearch","mcp__knowledge__lookup"], child-b none).
		require.Equal(t, []any{"Bash"}, root["disallowedTools"])
		require.Equal(t, []any{"WebSearch", "mcp__knowledge__lookup"}, childA["disallowedTools"])
		// Empty deny list stays absent (omitempty drops the key), never []/null.
		require.NotContains(t, childB, "disallowedTools")
		// Deny lists never cross-copy between sibling/parent nodes.
		require.NotContains(t, root["disallowedTools"], "WebSearch")
		require.NotContains(t, childA["disallowedTools"], "Bash")
	})
}

// TestLoadAgentGraph_ValidationMatrix is brief assertion 7: every graph
// violation fails the deploy explicitly, before any deployer create call.
func TestLoadAgentGraph_ValidationMatrix(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(fx *graphFixture)
		errPart    string
		errComment string
	}{
		{
			name: "missing subagent",
			mutate: func(fx *graphFixture) {
				fx.world.subagents[fx.parent.ID] = append(fx.world.subagents[fx.parent.ID], "ghost")
			},
			errPart:    "不存在",
			errComment: "dangling mount must fail with explicit guidance",
		},
		{
			name: "self mount",
			mutate: func(fx *graphFixture) {
				fx.world.subagents[fx.parent.ID] = []string{"parent"}
			},
			errPart: "不能挂载自己",
		},
		{
			name: "cycle",
			mutate: func(fx *graphFixture) {
				fx.world.subagents[fx.childA.ID] = []string{"parent"}
			},
			errPart: "互相挂载",
		},
		{
			name: "depth: child mounts its own subagent",
			mutate: func(fx *graphFixture) {
				fx.world.subagents[fx.childA.ID] = []string{"child-b"}
			},
			errPart: "仅支持一层委托",
		},
		{
			// The issue #114 analogue of the old deploy-key collision (see
			// TestLoadAgentGraph_BareIDCollision): a child named exactly like
			// the canonical root is caught by the self-mount guard above.
			name: "collision: child bare name equals root name",
			mutate: func(fx *graphFixture) {
				fx.world.subagents[fx.parent.ID] = []string{"parent"}
			},
			errPart: "不能挂载自己",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildGraphFixture(t)
			tc.mutate(fx)

			f := &deployTokenFixture{}
			srv := newDeployTokenServer(t, false, f)
			t.Cleanup(srv.Close)
			s := fx.world.service(t, srv.URL)

			_, err := s.Deploy("tenant-a", "parent", false, false)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart, tc.errComment)
			require.False(t, f.postCalled, "deployer create must not be called on graph violation")
		})
	}
}

// TestDeploy_FailFastOnCapabilityArtifacts is review F4 (issue #111): a node
// whose bound skill lacks artifact metadata, or whose bound dataset metadata
// cannot be resolved, must fail the deploy explicitly instead of silently
// deploying a partial capability closure — and the deployer create call must
// never fire. Errors must name the offending agent's DB bare name plus the
// missing artifact (position placeholder when the skill name itself is empty).
func TestDeploy_FailFastOnCapabilityArtifacts(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(fx *graphFixture)
		errPart string
		comment string
	}{
		{
			name: "child skill missing file hash",
			mutate: func(fx *graphFixture) {
				fx.world.skills[fx.childA.ID] = append(fx.world.skills[fx.childA.ID], &skill.Skill{
					Name: "child-a-broken-skill",
					URL:  "skills/tenant-a/child-a-broken-skill/dirty",
				})
			},
			errPart: `Agent "child-a" 挂载的技能 "child-a-broken-skill" 缺少制品元数据`,
			comment: "incomplete skill artifact must fail naming the child agent and the skill",
		},
		{
			name: "nameless skill reported by position",
			mutate: func(fx *graphFixture) {
				fx.world.skills[fx.parent.ID] = append(fx.world.skills[fx.parent.ID], &skill.Skill{
					URL:      "skills/tenant-a/nameless/dirty",
					FileHash: "6666666666666666666666666666666666666666666666666666666666666666",
				})
			},
			errPart: `Agent "parent" 挂载的技能 "第 2 个技能" 缺少制品元数据`,
			comment: "skill with empty name must be identified by its 1-based position",
		},
		{
			name: "root dataset metadata unavailable",
			mutate: func(fx *graphFixture) {
				fx.world.datasets[fx.parent.ID] = append(fx.world.datasets[fx.parent.ID], "ds-ghost")
			},
			errPart: `Agent "parent" 绑定的知识库 ds-ghost 不存在或元数据不可用`,
			comment: "unresolvable dataset must fail naming the agent and the dataset id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := buildGraphFixture(t)
			tc.mutate(fx)

			f := &deployTokenFixture{}
			srv := newDeployTokenServer(t, false, f)
			t.Cleanup(srv.Close)
			s := fx.world.service(t, srv.URL)

			_, err := s.Deploy("tenant-a", "parent", false, false)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errPart, tc.comment)
			require.False(t, f.postCalled, "deployer create must not be called on incomplete capability metadata")
		})
	}
}

// TestLoadAgentGraph_BareIDCollision pins the issue #114 collision guard: a
// legacy non-canonical root row (only reachable through MySQL's ci collation
// in production — Deploy normalizes its input name before the exact map/SQL
// lookup) whose bare runtime id collides with a mounted child name is
// rejected hub-side; the deployer would otherwise see duplicate agents[]
// entries. Called via loadAgentGraph directly to bypass the Deploy-side
// normalization that makes this unreachable through the public entrypoint.
func TestLoadAgentGraph_BareIDCollision(t *testing.T) {
	fx := buildGraphFixture(t)
	legacy := fx.world.addAgent("tenant-a", "Parent.X", func(cfg *agent.AgentConfig) {
		cfg.ModelID = "m"
		cfg.SystemPrompt = "p"
	})
	fx.world.subagents[legacy.ID] = []string{"parent-x"}

	s := fx.world.service(t, "http://deployer.test")
	_, err := s.loadAgentGraph(context.Background(), "tenant-a", legacy)
	require.Error(t, err)
	require.Contains(t, err.Error(), "冲突")
	require.Contains(t, err.Error(), "parent-x")
}

// TestUpdateSubagents_OneLevelInvariants is brief assertion 9: the mount
// entrypoint keeps the persisted graph within one delegation level.
func TestUpdateSubagents_OneLevelInvariants(t *testing.T) {
	t.Run("mounted parent cannot mount others", func(t *testing.T) {
		setupSubagentToolsTestDB(t)
		agentRepo := repository.NewAgentRepository()
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "grand"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child"}))
		grand, err := agentRepo.GetByName("default", "grand")
		require.NoError(t, err)
		parent, err := agentRepo.GetByName("default", "parent")
		require.NoError(t, err)
		require.NoError(t, agentRepo.ReplaceSubagents(grand.ID, []uint64{parent.ID}))

		svc := NewAgentService("test-encryption-key")
		err = svc.UpdateSubagents("default", "parent", []string{"child"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "已被其他 Agent 挂载")

		subs, err := agentRepo.GetSubagents(parent.ID)
		require.NoError(t, err)
		require.Empty(t, subs, "rejected update must not touch bindings")
	})

	t.Run("cannot mount an agent that already mounts others", func(t *testing.T) {
		setupSubagentToolsTestDB(t)
		agentRepo := repository.NewAgentRepository()
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "grandchild"}))
		child, err := agentRepo.GetByName("default", "child")
		require.NoError(t, err)
		grandchild, err := agentRepo.GetByName("default", "grandchild")
		require.NoError(t, err)
		require.NoError(t, agentRepo.ReplaceSubagents(child.ID, []uint64{grandchild.ID}))

		svc := NewAgentService("test-encryption-key")
		err = svc.UpdateSubagents("default", "parent", []string{"child"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "自身已挂载子 Agent")
	})

	t.Run("clearing a mounted parent's own list stays allowed (remediation)", func(t *testing.T) {
		setupSubagentToolsTestDB(t)
		agentRepo := repository.NewAgentRepository()
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "grand"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "parent"}))
		require.NoError(t, agentRepo.Create("default", &agent.AgentConfig{Name: "child"}))
		grand, err := agentRepo.GetByName("default", "grand")
		require.NoError(t, err)
		parent, err := agentRepo.GetByName("default", "parent")
		require.NoError(t, err)
		child, err := agentRepo.GetByName("default", "child")
		require.NoError(t, err)
		// Legacy state predating the invariant: grand→parent→child.
		require.NoError(t, agentRepo.ReplaceSubagents(grand.ID, []uint64{parent.ID}))
		require.NoError(t, agentRepo.ReplaceSubagents(parent.ID, []uint64{child.ID}))

		svc := NewAgentService("test-encryption-key")
		require.NoError(t, svc.UpdateSubagents("default", "parent", []string{}))

		subs, err := agentRepo.GetSubagents(parent.ID)
		require.NoError(t, err)
		require.Empty(t, subs, "clearing must actually clear the legacy violation")
	})
}
