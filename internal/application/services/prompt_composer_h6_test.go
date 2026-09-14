package services

import (
	"fmt"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	rundomain "control-panel/internal/domain/run"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// addH6Binding 给运行挂一个 H6 能力包绑定，Snapshot.promptFragments 即传入的条目。
func addH6Binding(t *testing.T, svc testComposerFixture, runID string, fragments []any) {
	t.Helper()
	binding := &rundomain.CapabilityBinding{
		TenantID: "t1", RunID: runID, Namespace: "io.zerone",
		PackageName: "relationship-dynamics", Version: "1.2.3", ContentHash: "pkg-hash",
		ManifestYAML: "manifest", Snapshot: map[string]any{"promptFragments": fragments},
	}
	require.NoError(t, svc.db.Create(binding).Error)
}

type testComposerFixture struct {
	db  *gorm.DB
	svc *PromptComposerService
}

func stageIndex(t *testing.T, stages []string, stage string) int {
	t.Helper()
	for i, s := range stages {
		if s == stage {
			return i
		}
	}
	return -1
}

func provenanceStages(snapshot *rundomain.PromptSnapshot) []string {
	stages := make([]string, 0, len(snapshot.Provenance))
	for _, p := range snapshot.Provenance {
		stages = append(stages, p.Stage)
	}
	return stages
}

func TestPromptComposerRecentMemoryStageOrdering(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{"stage": "application_context", "label": "应用规则X", "template": "固定规则文本"},
	})
	svc.SetRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		return []RecentMemoryItem{{Label: "近期记忆", Text: "你记得昨天核验过数据。"}}, nil
	})

	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	stages := provenanceStages(snapshot)
	appIdx, memoryIdx := -1, -1
	for i, stage := range stages {
		if stage == "application_context" {
			appIdx = i
		}
		if stage == "recent_memory" {
			memoryIdx = i
		}
	}
	require.Greater(t, memoryIdx, appIdx, "recent_memory must come after application_context")
	require.Contains(t, snapshot.RenderedText, "近期记忆")
	require.Contains(t, snapshot.RenderedText, "应用规则X")
	require.Greater(t, stageIndex(t, promptStageOrder, "recent_memory"), stageIndex(t, promptStageOrder, "application_context"))
	require.Equal(t, "recent_memory", promptStageOrder[len(promptStageOrder)-1])
}

func TestPromptComposerRecentMemoryProviderProvenance(t *testing.T) {
	_, svc, r, a := setupPromptComposer(t)
	svc.SetRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		require.Equal(t, "t1", tenantID)
		require.Equal(t, r.ID, runID)
		require.Equal(t, a.ID, agentID)
		return []RecentMemoryItem{{Label: "记忆A", Text: "记忆内容甲", SourceVersion: "3.0.0"}}, nil
	})
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	var found bool
	for _, p := range snapshot.Provenance {
		if p.Stage != "recent_memory" {
			continue
		}
		found = true
		require.Equal(t, "capability_package", p.SourceType)
		require.Equal(t, "io.zerone.subjective-memory", p.SourceID)
		require.Equal(t, "3.0.0", p.SourceVersion)
		require.Len(t, p.ContentHash, 64)
	}
	require.True(t, found, "recent_memory provenance missing")
}

func TestPromptComposerNilOrFailingProviderSkipsStage(t *testing.T) {
	// nil provider：无 recent_memory 阶段，无错误。
	_, svc, r, a := setupPromptComposer(t)
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Equal(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))
	require.NotContains(t, snapshot.RenderedText, "近期记忆")

	// 出错 provider：同样静默跳过，不影响合成。
	svc.SetRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		return nil, assertErr("memory backend down")
	})
	snapshot, err = svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Equal(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))

	// 空结果：阶段缺席。
	svc.SetRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		return nil, nil
	})
	snapshot, err = svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Equal(t, -1, stageIndex(t, provenanceStages(snapshot), "recent_memory"))
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

// h6State 快捷创建 run_states 行。
func h6State(t *testing.T, db *gorm.DB, runID, namespace, schema, subjectType, subjectID string, data map[string]any) {
	t.Helper()
	require.NoError(t, db.Create(&rundomain.RunState{
		TenantID: "t1", RunID: runID, Namespace: namespace, SchemaName: schema,
		SchemaVersion: "1.0.0", SchemaHash: "h", SubjectType: subjectType, SubjectID: subjectID,
		Revision: 1, Data: data,
	}).Error)
}

func TestPromptComposerPackFragmentHappyPath(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	h6State(t, db, r.ID, "io.zerone", "relation-attitude", "relation", "7", map[string]any{"stance": "hostile", "score": -40})
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{
			"stage":    "relationship_context",
			"label":    "关系态度",
			"template": "你对目标的态度：{{stance}}（{{score}}）",
			"refs": []any{
				map[string]any{"alias": "stance", "namespace": "io.zerone", "schemaName": "relation-attitude", "subjectType": "relation", "subjectId": "7", "path": "stance"},
				map[string]any{"alias": "score", "namespace": "io.zerone", "schemaName": "relation-attitude", "subjectType": "relation", "subjectId": "7", "path": "score"},
			},
			"audience": "agent:self",
		},
	})
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Contains(t, snapshot.RenderedText, "你对目标的态度：hostile（-40）")
	var found bool
	for _, p := range snapshot.Provenance {
		if p.Label == "关系态度" {
			found = true
			require.Equal(t, "relationship_context", p.Stage)
			require.Equal(t, "capability_package", p.SourceType)
			require.Equal(t, "io.zerone/relationship-dynamics#1", p.SourceID)
			require.Equal(t, "1.2.3", p.SourceVersion)
			require.Len(t, p.ContentHash, 64)
		}
	}
	require.True(t, found)
}

func TestPromptComposerPackFragmentOnMissingSkip(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	// 不创建任何 run_states 行 → 状态缺失。
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{
			"label": "未知事实", "template": "秘密：{{fact}}", "onMissing": "skip",
			"refs": []any{
				map[string]any{"alias": "fact", "namespace": "io.zerone", "schemaName": "belief-state", "subjectType": "agent", "subjectId": "7", "path": "status"},
			},
			"audience": "agent:self",
		},
	})
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	// 未知事实不注入：片段整体缺席。
	require.NotContains(t, snapshot.RenderedText, "秘密：")
	for _, p := range snapshot.Provenance {
		require.NotEqual(t, "未知事实", p.Label)
	}
}

func TestPromptComposerPackFragmentOnMissingPlaceholder(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{
			"label": "占位规则", "template": "缺失时保留 {{fact}} 原样", "onMissing": "placeholder",
			"refs": []any{
				map[string]any{"alias": "fact", "namespace": "io.zerone", "schemaName": "belief-state", "subjectType": "agent", "subjectId": "7", "path": "status"},
			},
		},
	})
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Contains(t, snapshot.RenderedText, "缺失时保留 {{fact}} 原样")
}

func TestPromptComposerPackFragmentMaxTokensTruncation(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	h6State(t, db, r.ID, "io.zerone", "emotion-state", "agent", "7", map[string]any{"narration": "一二三四五六七八九十"})
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{
			"label": "心情叙述", "template": "前缀{{mood}}后缀", "maxTokens": 8,
			"refs": []any{
				map[string]any{"alias": "mood", "namespace": "io.zerone", "schemaName": "emotion-state", "subjectType": "agent", "subjectId": "7", "path": "narration"},
			},
		},
	})
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	// 渲染文本按 rune 截断到 8 并加省略号：前缀一二三四五六…
	require.Contains(t, snapshot.RenderedText, "前缀一二三四五六…")
	var note string
	for _, p := range snapshot.Provenance {
		if p.Label == "心情叙述" {
			note = p.Note
		}
	}
	require.Contains(t, note, "maxTokens")
}

func TestPromptComposerPackFragmentAudienceSelfIsolation(t *testing.T) {
	db, svc, r, a := setupPromptComposer(t)
	fix := testComposerFixture{db, svc}
	b := &agent.AgentConfig{TenantID: "t1", Name: "auditor", ContentHash: "agent-v1", SystemPrompt: "复核。"}
	require.NoError(t, db.Create(b).Error)
	_, err := NewRunService(db).AddAgent("t1", r.ID, b.ID, "审核人")
	require.NoError(t, err)

	// 属于 analyst 的敏感片段；auditor 看不到。同一绑定内放两条片段：
	// 指向 analyst 自身状态的记忆，与显式引用 analyst subjectId 的片段。
	h6State(t, db, r.ID, "io.zerone", "memory-entry", "agent", fmt.Sprint(a.ID), map[string]any{"interpretation": "分析师的私密记忆"})
	addH6Binding(t, fix, r.ID, []any{
		map[string]any{
			"label": "私密记忆", "template": "记忆：{{m}}", "audience": "agent:self",
			"refs": []any{
				map[string]any{"alias": "m", "namespace": "io.zerone", "schemaName": "memory-entry", "subjectType": "agent", "subjectId": fmt.Sprint(a.ID), "path": "interpretation"},
			},
		},
		map[string]any{
			"label": "他人态度", "template": "你看到 {{s}}", "audience": "agent:self",
			"refs": []any{
				map[string]any{"alias": "s", "namespace": "io.zerone", "schemaName": "memory-entry", "subjectType": "agent", "subjectId": fmt.Sprint(a.ID), "path": "interpretation"},
			},
		},
	})

	own, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	require.Contains(t, own.RenderedText, "分析师的私密记忆")

	other, err := svc.Compose("t1", r.ID, b.ID)
	require.NoError(t, err)
	require.NotContains(t, other.RenderedText, "分析师的私密记忆")
	require.NotContains(t, other.RenderedText, "私密记忆")
	// 显式引用他人 subjectId 的 agent:self 片段：对当前 Agent 也不渲染。
	require.NotContains(t, other.RenderedText, "你看到")
}

func TestPromptComposerUnloadSafetyNoBindings(t *testing.T) {
	_, svc, r, a := setupPromptComposer(t)
	// 无绑定、无 provider：输出与 pre-H6 行为一致——无 capability_package 来源片段。
	snapshot, err := svc.Compose("t1", r.ID, a.ID)
	require.NoError(t, err)
	for _, p := range snapshot.Provenance {
		require.NotEqual(t, "capability_package", p.SourceType)
	}
	require.NotContains(t, snapshot.RenderedText, "{{")
	require.False(t, strings.Contains(snapshot.RenderedText, "…"))
}
