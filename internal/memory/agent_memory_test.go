package memory

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// memoryTestService 返回未开通的 agent 形态 Scope——生产流由
// EnsureAgentLifecycle 在首次工具调用时服务端开通（spec §3）。
func memoryTestService(t *testing.T) (*Service, Scope) {
	t.Helper()
	s, _, _ := setup(t) // 独立夹具 scope（alice），与本测试 scope 互不影响
	return s, Scope{TenantID: "tenant", ApplicationID: "app", UserID: "agent:test-agent", LifecycleID: "agent-memory-v1"}
}

func TestEnsureAgentLifecycleIdempotent(t *testing.T) {
	s, sc := memoryTestService(t)
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc))
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc), "重复调用幂等（确定性 opID + 活跃复核）")
}

func TestRememberAgentFactIdempotentAndPoolFull(t *testing.T) {
	s, sc := memoryTestService(t)
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc))
	in := AgentMemoryInput{Subject: "用户", Category: "情绪", Content: "对工作变动焦虑", Quote: "想到换工作就睡不着"}
	first, err := s.RememberAgentFact(testCtx, sc, in)
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Version)
	require.Equal(t, "agent:test-agent", first.Source)
	second, err := s.RememberAgentFact(testCtx, sc, in)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "同 content 指纹幂等返回已有")
	// 灌满至 maxRecords（不同 content 绕开幂等）后须返回专用池满错误
	for i := 0; i < maxRecords; i++ {
		_, err = s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "fill-" + strconv.Itoa(i), Quote: "q", Subject: "s", Category: "c"})
		if err != nil {
			break
		}
	}
	_, err = s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "overflow", Quote: "q", Subject: "s", Category: "c"})
	require.ErrorIs(t, err, ErrPoolFull, "池满是专用错误（拒写不淘汰，Agent 可感知停止重试）")
}

func TestRememberAgentFactValidation(t *testing.T) {
	s, sc := memoryTestService(t)
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc))
	for name, in := range map[string]AgentMemoryInput{
		"无 quote":     {Content: "c"},
		"无 content":   {Quote: "q"},
		"正文超长":        {Content: string(make([]rune, 2001)), Quote: "q"},
		"quote 超长":    {Content: "c", Quote: string(make([]rune, 601))},
		"subject 超长":  {Content: "c", Quote: "q", Subject: string(make([]byte, 129))},
		"category 超长": {Content: "c", Quote: "q", Category: string(make([]byte, 65))},
	} {
		_, err := s.RememberAgentFact(testCtx, sc, in)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
}

func TestUpdateAgentFactCASAndSourceGuard(t *testing.T) {
	s, sc := memoryTestService(t)
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc))
	rec, err := s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "旧内容", Quote: "旧quote", Subject: "用户", Category: "偏好"})
	require.NoError(t, err)
	// CAS 版本错
	_, err = s.UpdateAgentFact(testCtx, sc, rec.ID, rec.Version+5, AgentMemoryInput{Content: "新", Quote: "新quote", Subject: "用户", Category: "偏好"})
	require.ErrorIs(t, err, ErrConflict)
	// 正确 CAS：版本 +1，quote 更新
	upd, err := s.UpdateAgentFact(testCtx, sc, rec.ID, rec.Version, AgentMemoryInput{Content: "新内容", Quote: "新quote", Subject: "用户", Category: "偏好"})
	require.NoError(t, err)
	require.Equal(t, rec.Version+1, upd.Version)
	require.Equal(t, "新quote", upd.Quote)
	require.Equal(t, "新内容", upd.Content)
	// 非 agent-memory kind（人工账本）不可经本工具修改：伪造 kind 后 update → deny
	other, err := s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "他人", Quote: "q", Subject: "s", Category: "c"})
	require.NoError(t, err)
	require.NoError(t, s.db.Model(&recordRow{}).Where("id = ?", other.ID).Update("kind", "fact").Error)
	_, err = s.UpdateAgentFact(testCtx, sc, other.ID, other.Version, AgentMemoryInput{Content: "x", Quote: "q", Subject: "s", Category: "c"})
	require.ErrorIs(t, err, ErrForbidden, "kind 守卫：仅本工具写入的记录可改")
	// 不存在的 id → ErrNotFound（中性映射用）
	_, err = s.UpdateAgentFact(testCtx, sc, "no-such-id", 1, AgentMemoryInput{Content: "x", Quote: "q"})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListAndRecallAgentMemories(t *testing.T) {
	s, sc := memoryTestService(t)
	require.NoError(t, s.EnsureAgentLifecycle(testCtx, sc))
	rec, err := s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "喜欢无糖乌龙茶", Quote: "q", Subject: "用户", Category: "偏好"})
	require.NoError(t, err)
	noise, err := s.RememberAgentFact(testCtx, sc, AgentMemoryInput{Content: "次要记录", Quote: "q", Subject: "用户", Category: "其他"})
	require.NoError(t, err)
	_ = noise
	list, err := s.ListAgentMemories(testCtx, sc, "", "", 50)
	require.NoError(t, err)
	require.Len(t, list, 2)
	direct, err := s.ListAgentMemories(testCtx, sc, "", rec.ID, 50)
	require.NoError(t, err)
	require.Len(t, direct, 1)
	require.Equal(t, rec.ID, direct[0].ID)
	// 类别过滤
	cat, err := s.ListAgentMemories(testCtx, sc, "偏好", "", 50)
	require.NoError(t, err)
	require.Len(t, cat, 1)
	none, err := s.ListAgentMemories(testCtx, sc, "不存在", "", 50)
	require.NoError(t, err)
	require.Empty(t, none)
	// recall：outbox 对账建索引后按 chunk 反查本 kind 记录
	syncAll(t, s)
	hits, err := s.RecallAgentMemories(testCtx, sc, "乌龙茶", 5)
	require.NoError(t, err)
	require.NotEmpty(t, hits)
	found := false
	for _, h := range hits {
		if h.ID == rec.ID && h.Content == "喜欢无糖乌龙茶" {
			found = true
		}
	}
	require.True(t, found, "recall 须能按 chunk 反查到刚写入的记忆")
}
