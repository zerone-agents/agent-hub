package services

import (
	"fmt"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/emotion"
	rundomain "control-panel/internal/domain/run"
	subjectivememory "control-panel/internal/domain/subjectivememory"

	"github.com/stretchr/testify/require"
)

// TestH6StoryBetrayal 是一个端到端"故事"集成测试：两位 Agent 经历同一丑闻、
// 走向不同认知，背叛发生，记忆沉淀，关系影响执行，最后由提示词合成器把
// 各自的主观世界分别缝进各自的提示词。全部走真实 service + 内存 sqlite，
// 无 SQL、无 pack mock。租户固定为 "default"。
func TestH6StoryBetrayal(t *testing.T) {
	runSvc, db := newRunTestService(t)
	// PromptComposer 需要持久化 PromptSnapshot；newRunTestService 未迁移该表，
	// 这里为合成器补上（测试自身的 schema 准备，不碰生产代码）。
	require.NoError(t, db.AutoMigrate(&rundomain.PromptSnapshot{}))
	// 合成器会探测 agent_relations（relationshipFragments），一并备好。
	require.NoError(t, db.AutoMigrate(&agentrelation.AgentRelation{}, &agentrelation.AgentRelationEvent{}))

	tenant := "default"
	now := time.Now().UTC()

	// ---- 演员表：A（阿黎，后来的"背叛者"）与 B（老周，受害者） ----
	aCfg := agent.AgentConfig{TenantID: tenant, Name: "阿黎", ContentHash: "cfg-a", SystemPrompt: "预算分析师。"}
	bCfg := agent.AgentConfig{TenantID: tenant, Name: "老周", ContentHash: "cfg-b", SystemPrompt: "项目经理。"}
	require.NoError(t, db.Create(&aCfg).Error)
	require.NoError(t, db.Create(&bCfg).Error)
	A, B := aCfg.ID, bCfg.ID

	// 各能力服务共用同一个 RunService / DB（与既有 *_test.go 的构造方式一致）。
	beliefSvc := NewBeliefService(runSvc)
	require.NoError(t, beliefSvc.EnsureSchemas())
	emotionSvc := NewEmotionService(runSvc)
	reldynSvc := NewRelationDynamicsService(runSvc)
	memorySvc := NewMemoryService(runSvc)
	composerSvc := NewPromptComposerService(db)

	r1, err := runSvc.Create(tenant, CreateRunInput{Name: "预算丑闻局"})
	require.NoError(t, err)
	R1 := r1.ID
	_, err = runSvc.AddAgent(tenant, R1, A, "预算分析师")
	require.NoError(t, err)
	_, err = runSvc.AddAgent(tenant, R1, B, "项目经理")
	require.NoError(t, err)

	// 私密标记串：aMemoryText 是 B（受害者）的背叛记忆，bMemoryText 是 A 的
	// 作证记忆。各归其主，用于提示词隔离断言。
	aMemoryText := "阿黎在预算会议上背叛了我，我永远记得这一刻"
	bMemoryText := "老周公开替我作证，他说的是真相"

	// 近期记忆 Provider：直接包真实 MemoryService.Recall。
	composerSvc.SetRecentMemoryProvider(func(tenantID, runID string, agentID uint64) ([]RecentMemoryItem, error) {
		entries, err := memorySvc.Recall(tenantID, runID, agentID, "", 5, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		items := make([]RecentMemoryItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, RecentMemoryItem{
				Label:         "近期记忆",
				Text:          fmt.Sprintf("%s（重要性 %d）", e.Interpretation, e.Importance),
				SourceVersion: "1.0.0",
			})
		}
		return items, nil
	})

	// ---------- 第 1 幕：开局，两人都是平静的基线 ----------
	t.Run("开局_平静基线", func(t *testing.T) {
		for _, id := range []uint64{A, B} {
			status, err := emotionSvc.Status(tenant, R1, id)
			require.NoError(t, err)
			require.Equal(t, emotion.MoodCalm, status.Mood)
			require.Equal(t, 0, status.Intensity)
			require.NotEmpty(t, status.Narration)
			t.Logf("开局：agent %d 心情 %s（强度 %d）——%s", id, status.Mood, status.Intensity, status.Narration)
		}
	})

	// ---------- 第 2 幕：同一丑闻，两种认知 ----------
	t.Run("同一丑闻两种认知", func(t *testing.T) {
		fact := "fact:budget-scandal"
		require.NoError(t, beliefSvc.RecordDelivery(tenant, R1, A, fact, now, "story-del-a1"))
		require.NoError(t, beliefSvc.RecordDelivery(tenant, R1, B, fact, now, "story-del-b1"))

		aBeliefs, err := beliefSvc.List(tenant, R1, A, "")
		require.NoError(t, err)
		require.Len(t, aBeliefs, 1)
		require.Equal(t, fact, aBeliefs[0].FactRef)
		bBeliefs, err := beliefSvc.List(tenant, R1, B, "")
		require.NoError(t, err)
		require.Len(t, bBeliefs, 1)
		require.Equal(t, fact, bBeliefs[0].FactRef)
		t.Logf("丑闻送达：A、B 都已知晓 %s（known，置信 %d）", fact, aBeliefs[0].Confidence)

		// A 收到矛盾证据 → 动摇；B 收到确认证据 → 采信。
		require.NoError(t, beliefSvc.AddEvidence(tenant, R1, A, fact, true, now.Add(time.Hour), "story-ev-a-contradict"))
		require.NoError(t, beliefSvc.AddEvidence(tenant, R1, B, fact, false, now.Add(time.Hour), "story-ev-b-confirm"))

		aBeliefs, err = beliefSvc.List(tenant, R1, A, fact)
		require.NoError(t, err)
		require.Equal(t, "doubted", aBeliefs[0].Status)
		require.Equal(t, 25, aBeliefs[0].Confidence)
		bBeliefs, err = beliefSvc.List(tenant, R1, B, fact)
		require.NoError(t, err)
		require.Equal(t, "believed", bBeliefs[0].Status)
		require.Equal(t, 60, bBeliefs[0].Confidence)
		t.Logf("认知分叉：A %s（%d）vs B %s（%d）", aBeliefs[0].Status, aBeliefs[0].Confidence, bBeliefs[0].Status, bBeliefs[0].Confidence)

		disputes, err := beliefSvc.Disputes(tenant, R1)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(disputes), 1)
		found := false
		for _, d := range disputes {
			if d.FactRef != fact || len(d.Entries) < 2 {
				continue
			}
			statuses := map[uint64]string{}
			for _, e := range d.Entries {
				statuses[e.AgentID] = e.Status
			}
			if statuses[A] == "doubted" && statuses[B] == "believed" {
				found = true
			}
		}
		require.True(t, found, "Disputes 应包含 %s 且 A/B 立场实质分歧", fact)
		t.Logf("争议检测：%s 已被标记为争议事实", fact)
	})

	// ---------- 第 3 幕：背叛发生（情绪 + 关系动力学） ----------
	t.Run("背叛发生", func(t *testing.T) {
		at := now.Add(2 * time.Hour)
		// 关系层面：A 对 B 的两次严重背叛。
		require.NoError(t, reldynSvc.OnEvent(tenant, R1, A, B, "betrayed", 3, at, "story-rel-betray-1"))
		require.NoError(t, reldynSvc.OnEvent(tenant, R1, A, B, "betrayed", 3, at.Add(time.Minute), "story-rel-betray-2"))
		// 情绪层面：B 切身体会到背叛。
		require.NoError(t, emotionSvc.OnEvent(tenant, R1, B, "betrayed", 3, at, "story-emo-betray-1"))
		require.NoError(t, emotionSvc.OnEvent(tenant, R1, B, "betrayed", 3, at.Add(time.Minute), "story-emo-betray-2"))

		// 注意：故事视角里 B 是受害者。任务书按 A=受害者描述，这里以真实服务
		// 语义为准：OnEvent(from=A, to=B) 记录的是 A 对 B 的态度；情绪事件落在 B 身上。

		// B 的情绪：两次 severity 3 背叛 → 状态存在、叙述非空。
		status, err := emotionSvc.Status(tenant, R1, B)
		require.NoError(t, err)
		require.NotEmpty(t, status.Narration)
		require.Contains(t, status.Narration, "心情为")
		t.Logf("背叛后：B 心情 %s（强度 %d）——%s", status.Mood, status.Intensity, status.Narration)
		// 两次 severity 3 背叛：-40 × 2 = -80（触及单事件 -40 上限后累加）。
		// 强度为有符号值：负值 = 负面情绪，mood 投影到 grieving。
		require.Equal(t, emotion.MoodGrieving, status.Mood, "负向情绪事件应投影到 grieving")
		require.Equal(t, -80, status.Intensity)
		require.Contains(t, status.Narration, "悲伤")

		// A→B 关系：敌对。
		attitude, err := reldynSvc.View(tenant, R1, A, B)
		require.NoError(t, err)
		require.LessOrEqual(t, attitude.Score, -60)
		require.Equal(t, "hostile", attitude.Stance)
		require.NotEmpty(t, attitude.Narration)
		t.Logf("A 对 B 的态度：%s（%d）——%s", attitude.Stance, attitude.Score, attitude.Narration)

		// 非对称：B→A 从未收到事件，仍是 neutral。
		reverse, err := reldynSvc.View(tenant, R1, B, A)
		require.NoError(t, err)
		require.Equal(t, 0, reverse.Score)
		require.Equal(t, "neutral", reverse.Stance)
		t.Logf("B 对 A 的态度：%s（%d）——背叛的单向性成立", reverse.Stance, reverse.Score)
	})

	// ---------- 第 4 幕：记忆（显式 + 软遗忘） ----------
	t.Run("记忆沉淀与软遗忘", func(t *testing.T) {
		_, err := memorySvc.Record(tenant, R1, B, "event:betrayal-001", aMemoryText, 90, now.Add(3*time.Hour), "story-mem-betrayal")
		require.NoError(t, err)

		got, err := memorySvc.Recall(tenant, R1, B, "背叛", 5, now.Add(4*time.Hour))
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Contains(t, got[0].Interpretation, "背叛")
		t.Logf("B 回忆起：%s", got[0].Interpretation)

		// 一条 30 天前的琐事：importance 5 ≤ 软遗忘上限、age 30 天 ≥ 软遗忘天数、
		// recallCount 0 ≤ 阈值 → 按域规则 SoftForgotten 成立，Recall 不得返回。
		trivial, err := memorySvc.Record(tenant, R1, B, "fact:trivial-coffee", "那天咖啡杯放错了位置", 5, now.Add(-30*24*time.Hour), "story-mem-trivial")
		require.NoError(t, err)
		entry := subjectivememory.MemoryEntry{Importance: trivial.Importance, RecordedAt: trivial.RecordedAt, RecallCount: 0}
		require.True(t, subjectivememory.SoftForgotten(entry, now), "琐事记忆应满足域软遗忘规则")

		again, err := memorySvc.Recall(tenant, R1, B, "", 5, now)
		require.NoError(t, err)
		for _, e := range again {
			require.NotEqual(t, trivial.ID, e.ID, "低重要性 + 陈旧的琐事必须被软遗忘")
		}
		require.Len(t, again, 1, "只有背叛记忆应被回忆起")
		t.Logf("软遗忘生效：琐事记忆（%d 天前，重要性 %d）已沉入水下", 30, trivial.Importance)
	})

	// ---------- 第 5 幕：关系影响执行（Gate） ----------
	t.Run("关系门控执行", func(t *testing.T) {
		// Gate(target=A, from=B)：以 A 对 B 的态度打分（-80，hostile）。
		allowed, confirm, err := reldynSvc.Gate(tenant, R1, A, B, "assign")
		require.NoError(t, err)
		require.True(t, allowed, "GateAction 契约：敌对不阻止，只要求确认")
		require.True(t, confirm, "assign 类动作在敌对下需要目标确认")
		t.Logf("门控：B 向 A 发起 assign → allowed=%v needConfirm=%v（敌对拦截）", allowed, confirm)

		allowed, confirm, err = reldynSvc.Gate(tenant, R1, A, B, "inform")
		require.NoError(t, err)
		require.True(t, allowed)
		require.False(t, confirm, "inform 是纯信息流，敌对不影响")
		t.Logf("门控：B 向 A 发起 inform → allowed=%v needConfirm=%v（畅通）", allowed, confirm)
	})

	// ---------- 第 6 幕：提示词合成（各自的主观世界） ----------
	t.Run("提示词合成与私密隔离", func(t *testing.T) {
		// 给 A 也记一条独特记忆（bMemoryText 归 A），双方各有一枚唯一标记串：
		// aMemoryText（背叛记忆）属于 B，bMemoryText（作证记忆）属于 A。
		_, err := memorySvc.Record(tenant, R1, A, "fact:stood-by-me", bMemoryText, 80, now.Add(3*time.Hour), "story-mem-zhou")
		require.NoError(t, err)

		snapA, deliveryA, err := composerSvc.ComposeForDelivery(tenant, R1, A, "我们接下来怎么办")
		require.NoError(t, err)
		require.NotEmpty(t, deliveryA)
		require.Contains(t, deliveryA, "agenthub.run-context/v1")
		// 近期记忆阶段：A 的提示词里出现 A 自己的记忆。
		require.NotEqual(t, -1, stageIndex(t, provenanceStages(snapA), "recent_memory"))
		require.Contains(t, snapA.RenderedText, bMemoryText)
		// 私密隔离：A 的提示词不得包含 B 的背叛记忆文本。
		require.NotContains(t, snapA.RenderedText, aMemoryText)
		t.Logf("A 的提示词含其自身记忆（%q…），且不含 B 的背叛记忆", bMemoryText)

		snapB, _, err := composerSvc.ComposeForDelivery(tenant, R1, B, "我们接下来怎么办")
		require.NoError(t, err)
		// B 的提示词：含 B 的背叛记忆与 B 的情绪状态叙述，不含 A 的记忆文本。
		require.Contains(t, snapB.RenderedText, aMemoryText)
		// dynamic_state 只收录 agent 作用域状态（subject_id = agent ID，如情绪）；
		// B 有情绪状态行，A 没有——该阶段对 B 出席、对 A 缺席均为正常契约。
		require.NotEqual(t, -1, stageIndex(t, provenanceStages(snapB), "dynamic_state"))
		bEmotion, err := emotionSvc.Status(tenant, R1, B)
		require.NoError(t, err)
		require.Contains(t, snapB.RenderedText, bEmotion.Narration, "B 的情绪状态内容应在 dynamic_state 阶段")
		require.NotContains(t, snapB.RenderedText, bMemoryText, "B 的提示词不得泄露 A 的私密记忆")
		t.Logf("B 的提示词含背叛记忆与情绪叙述（%q…），且不含 A 的记忆——隔离成立", bEmotion.Narration)
	})

	// ---------- 第 7 幕：跨局隔离 ----------
	t.Run("跨局隔离", func(t *testing.T) {
		r2, err := runSvc.Create(tenant, CreateRunInput{Name: "新一局"})
		require.NoError(t, err)
		R2 := r2.ID
		_, err = runSvc.AddAgent(tenant, R2, A, "预算分析师")
		require.NoError(t, err)
		_, err = runSvc.AddAgent(tenant, R2, B, "项目经理")
		require.NoError(t, err)

		got, err := memorySvc.Recall(tenant, R2, B, "", 5, now)
		require.NoError(t, err)
		require.Empty(t, got, "R2 中不应回忆起 R1 的记忆")
		attitude, err := reldynSvc.View(tenant, R2, A, B)
		require.NoError(t, err)
		require.Equal(t, "neutral", attitude.Stance, "R2 中关系应回到中性")
		status, err := emotionSvc.Status(tenant, R2, B)
		require.NoError(t, err)
		require.Equal(t, emotion.MoodCalm, status.Mood, "R2 中情绪应回到平静基线")

		// R1 数据原样保留。
		attitudeR1, err := reldynSvc.View(tenant, R1, A, B)
		require.NoError(t, err)
		require.Equal(t, "hostile", attitudeR1.Stance)
		gotR1, err := memorySvc.Recall(tenant, R1, B, "背叛", 5, now)
		require.NoError(t, err)
		require.Len(t, gotR1, 1)
		t.Logf("跨局隔离：R2 一片空白，R1 的敌对与背叛记忆原封不动")
	})

	// ---------- 第 8 幕：幂等回放 ----------
	t.Run("幂等回放", func(t *testing.T) {
		before, err := emotionSvc.Status(tenant, R1, B)
		require.NoError(t, err)
		// 用第 3 幕完全相同的幂等键重放：不得报错，状态逐字节一致。
		require.NoError(t, emotionSvc.OnEvent(tenant, R1, B, "betrayed", 3, now.Add(2*time.Hour), "story-emo-betray-1"))
		after, err := emotionSvc.Status(tenant, R1, B)
		require.NoError(t, err)
		require.Equal(t, before.Mood, after.Mood)
		require.Equal(t, before.Intensity, after.Intensity)
		require.Equal(t, before.Narration, after.Narration)

		attBefore, err := reldynSvc.View(tenant, R1, A, B)
		require.NoError(t, err)
		require.NoError(t, reldynSvc.OnEvent(tenant, R1, A, B, "betrayed", 3, now.Add(2*time.Hour), "story-rel-betray-1"))
		attAfter, err := reldynSvc.View(tenant, R1, A, B)
		require.NoError(t, err)
		require.Equal(t, attBefore.Score, attAfter.Score)
		require.Equal(t, attBefore.Stance, attAfter.Stance)
		t.Logf("幂等回放：情绪与关系状态均无任何二次效果")
	})
}
