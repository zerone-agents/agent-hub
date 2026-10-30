package repository

// H2 回归（issue #232 计划）：claim / 版本比较 / v1 写入隔离。
//
// 覆盖计划清单：新键 claim→claimed-empty/version0 且无可见聊天；imported 认领
// 自己的 v1 会话→claimed-legacy；native 接管 v1→冲突且不留占位行；异 source
// claim/apply 拒绝；跨租户隔离；重复 claim 幂等；apply 状态机（未 claim /
// applied / duplicate / 同版本异摘要 conflict / 降版本 superseded / 跨版本
// 跳跃）；claim 后 v1 写同键被拒。并发反例在 chat_sync_repository_mysql_test.go
//（TEST_MYSQL_DSN 门禁）。

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.SessionSyncState{}, &chat.UploadRecord{}))
	// ChatRepository 经全局 database.GetDB() 取库（setupChatRepoTestDB 同款注入）
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

var (
	syncSrcA = "aaaa0000-0000-4000-8000-00000000a001"
	syncSrcB = "aaaa0000-0000-4000-8000-00000000b002"
)

func syncOwner(tenant, user string) chat.Owner {
	return chat.Owner{TenantID: tenant, UserID: user}
}

// mkEnvelope 直接构造已通过校验语义的 envelope（绕过 ParseEnvelope——
// wire 层已在 H1 向量钉死；此处测状态机）。摘要经 DigestEnvelope 正规计算。
func mkEnvelope(t *testing.T, sourceID, sessionID string, version int64, kind chat.Kind, msgCount int) chat.Envelope {
	t.Helper()
	e := chat.Envelope{
		ProtocolVersion:   chat.ProtocolVersion,
		ProjectionVersion: chat.ProjectionVersion,
		SourceID:          sourceID,
		AppSessionID:      sessionID,
		Version:           version,
		SnapshotID:        fmt.Sprintf("%08d-1111-4222-8333-%012d", version, version%1000000000000),
		Kind:              kind,
	}
	if kind == chat.KindReplace {
		rp := &chat.ReplacePayload{
			Session: chat.SyncSessionPayload{
				ID: sessionID, Title: fmt.Sprintf("t-v%d", version),
				CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
				Model: "glm-5", Status: "active",
			},
			Messages: []chat.SyncMessagePayload{},
		}
		for i := 0; i < msgCount; i++ {
			rp.Messages = append(rp.Messages, chat.SyncMessagePayload{
				ID:        fmt.Sprintf("%s-m%d", sessionID, i),
				Role:      "user",
				Content:   fmt.Sprintf("c%d@v%d", i, version),
				CreatedAt: "2026-10-01T00:00:00Z",
				Ordinal:   int64(i),
			})
		}
		e.Payload = rp
		b, err := json.Marshal(rp)
		require.NoError(t, err)
		e.RawPayload = b
	} else {
		e.RawPayload = json.RawMessage("null")
	}
	d, err := chat.DigestEnvelope(e)
	require.NoError(t, err)
	e.Digest = d
	return e
}

// mkEnvelopeAltDigest：同 version、不同内容的合法 envelope（摘要自然不同）。
func mkEnvelopeAltDigest(t *testing.T, sourceID, sessionID string, version int64, msgCount int) chat.Envelope {
	t.Helper()
	e := mkEnvelope(t, sourceID, sessionID, version, chat.KindReplace, msgCount)
	rp := *e.Payload
	rp.Session.Title = "alt-" + rp.Session.Title
	for i := range rp.Messages {
		rp.Messages[i].Content = "alt-" + rp.Messages[i].Content
	}
	e.Payload = &rp
	b, err := json.Marshal(&rp)
	require.NoError(t, err)
	e.RawPayload = b
	d, err := chat.DigestEnvelope(e)
	require.NoError(t, err)
	e.Digest = d
	return e
}

// pushV1 直接以 v1 仓库写入会话（构造 chat.Session，绕过 service 层转换）。
func pushV1(t *testing.T, r *ChatRepository, owner chat.Owner, sessionID string, msgCount int) *PushResult {
	t.Helper()
	sess := &chat.Session{
		TenantID: owner.TenantID, UserID: owner.UserID, ID: sessionID,
		Title:     "v1-" + sessionID,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	msgs := make([]*chat.Message, msgCount)
	for i := 0; i < msgCount; i++ {
		msgs[i] = &chat.Message{
			TenantID: owner.TenantID, UserID: owner.UserID, SessionID: sessionID,
			ID: fmt.Sprintf("%s-v1m%d", sessionID, i), Role: "user",
			Content: fmt.Sprintf("v1-%d", i), CreatedAt: time.Now().UTC(),
		}
	}
	res, err := r.PushSessions(owner.TenantID, owner.UserID, []*chat.Session{sess}, [][]*chat.Message{msgs})
	require.NoError(t, err)
	return res
}

func msgCount(t *testing.T, db *gorm.DB, owner chat.Owner, sessionID string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&chat.Message{}).
		Where("tenant_id = ? AND user_id = ? AND session_id = ?", owner.TenantID, owner.UserID, sessionID).
		Count(&n).Error)
	return n
}

// ── Claim：新键 / 幂等 / legacy / native 冲突 / 异 source / 跨租户 ─────────

func TestChatSyncClaimNewKeyEmpty(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)

	st, err := repo.Claim(context.Background(), syncOwner("t1", "u1"), "s1", syncSrcA, "imported")
	require.NoError(t, err)
	require.Equal(t, chat.StateClaimedEmpty, st.Status)
	require.Equal(t, "0", st.Version)
	require.Equal(t, syncSrcA, st.SourceID)
	require.Empty(t, st.SnapshotID, "claim 占位无快照身份")

	// 新键 claim 不创建可见聊天
	var sessions int64
	require.NoError(t, db.Model(&chat.Session{}).Where("user_id = ? AND id = ?", "u1", "s1").Count(&sessions).Error)
	require.Zero(t, sessions)

	// 同 source 重复 claim 幂等
	st2, err := repo.Claim(context.Background(), syncOwner("t1", "u1"), "s1", syncSrcA, "imported")
	require.NoError(t, err)
	require.Equal(t, st, st2)
}

func TestChatSyncClaimLegacyImportedAndV1Isolation(t *testing.T) {
	db := setupSyncTestDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")

	res := pushV1(t, chatRepo, owner, "s1", 2)
	require.Equal(t, 1, res.SyncedSessions)

	// imported 认领自己的 v1 会话 → claimed-legacy
	st, err := repo.Claim(context.Background(), owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)
	require.Equal(t, chat.StateClaimedLegacy, st.Status)
	require.Equal(t, "0", st.Version)

	// claim 后 v1 写同键必须被拒：跳过且正文不被覆盖
	res2 := pushV1(t, chatRepo, owner, "s1", 5)
	require.Equal(t, 1, res2.SkippedSessions, "v1 写已 claim 的键须跳过")
	require.Equal(t, int64(2), msgCount(t, db, owner, "s1"), "跳过时不得覆盖既有正文")
}

// 评审修订 B1（PR #233）：runtime/agent_chat 来源的会话不允许 claim（契约
// §3：服务端必须验证它是该用户拥有的 App 来源会话）。来源判定用
// Session.Source——agent chat 页面会话 Source="agent_chat_page"（services
// 常量，跨层引用避免，此处字面量+注释），App v1 push 会话 Source=""。
func TestChatSyncClaimRejectsRuntimeSourceSession(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")

	sess := &chat.Session{
		TenantID: owner.TenantID, UserID: owner.UserID, ID: "s-agent",
		Title: "agent chat", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Source: "agent_chat_page",
	}
	require.NoError(t, db.Create(sess).Error)

	_, err := repo.Claim(context.Background(), owner, "s-agent", syncSrcA, "imported")
	require.ErrorIs(t, err, chat.ErrSyncClaimConflict)

	_, err = repo.GetState(context.Background(), owner, "s-agent")
	require.ErrorIs(t, err, chat.ErrSyncNotFound, "冲突回滚：不留占位行")
}

func TestChatSyncClaimNativeOverV1Conflict(t *testing.T) {
	db := setupSyncTestDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")

	pushV1(t, chatRepo, owner, "s1", 1)

	_, err := repo.Claim(context.Background(), owner, "s1", syncSrcA, "native")
	require.ErrorIs(t, err, chat.ErrSyncClaimConflict)

	// 冲突回滚：不留占位行
	_, err = repo.GetState(context.Background(), owner, "s1")
	require.ErrorIs(t, err, chat.ErrSyncNotFound)
}

func TestChatSyncClaimOtherSourceConflict(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")

	_, err := repo.Claim(context.Background(), owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)

	_, err = repo.Claim(context.Background(), owner, "s1", syncSrcB, "imported")
	require.ErrorIs(t, err, chat.ErrSyncClaimConflict)

	st, err := repo.GetState(context.Background(), owner, "s1")
	require.NoError(t, err)
	require.Equal(t, syncSrcA, st.SourceID, "冲突不得改绑 source")
}

func TestChatSyncCrossTenantIsolation(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)

	_, err := repo.Claim(context.Background(), syncOwner("t1", "u1"), "s1", syncSrcA, "imported")
	require.NoError(t, err)

	// 异租户视角：同 user 同 session 不可见（同步键含租户）
	_, err = repo.GetState(context.Background(), syncOwner("t2", "u1"), "s1")
	require.ErrorIs(t, err, chat.ErrSyncNotFound)

	// 异租户 apply 也不得命中 t1 的键（等价于未 claim）
	r, err := repo.Apply(context.Background(), syncOwner("t2", "u1"), mkEnvelope(t, syncSrcA, "s1", 1, chat.KindReplace, 1))
	require.ErrorIs(t, err, chat.ErrSyncNotClaimed)
	require.Zero(t, r)
}

// ── Apply 状态机 ────────────────────────────────────────────────────────────

func TestChatSyncApplyWithoutClaimRejected(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)

	_, err := repo.Apply(context.Background(), syncOwner("t1", "u1"), mkEnvelope(t, syncSrcA, "s1", 1, chat.KindReplace, 1))
	require.ErrorIs(t, err, chat.ErrSyncNotClaimed, "无 claim 的键不得自行创建覆盖")
}

func TestChatSyncApplyLifecycle(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)

	// v10 applied：内容全量替换
	env10 := mkEnvelope(t, syncSrcA, "s1", 10, chat.KindReplace, 2)
	r, err := repo.Apply(ctx, owner, env10)
	require.NoError(t, err)
	require.Equal(t, chat.StatusApplied, r.Status)
	require.Equal(t, "10", r.State.Version)
	require.Equal(t, chat.StateActive, r.State.Status)
	require.Equal(t, int64(2), msgCount(t, db, owner, "s1"))
	requireContent := func(substr string) {
		var contents []string
		require.NoError(t, db.Model(&chat.Message{}).
			Where("tenant_id = ? AND user_id = ? AND session_id = ?", owner.TenantID, owner.UserID, "s1").
			Order("id").Pluck("content", &contents).Error)
		joined := contents[0] + "," + contents[1]
		require.Contains(t, joined, substr)
	}
	requireContent("@v10")

	// 同 envelope 重放 → duplicate，内容不变
	r, err = repo.Apply(ctx, owner, env10)
	require.NoError(t, err)
	require.Equal(t, chat.StatusDuplicate, r.Status)
	require.Equal(t, int64(2), msgCount(t, db, owner, "s1"))

	// 同 version 异摘要 → conflict，内容不变
	alt := mkEnvelopeAltDigest(t, syncSrcA, "s1", 10, 2)
	r, err = repo.Apply(ctx, owner, alt)
	require.NoError(t, err)
	require.Equal(t, chat.StatusConflict, r.Status)
	require.Equal(t, "10", r.State.Version)
	require.Equal(t, env10.Digest, r.State.Digest, "冲突行返回当前已确认身份")
	requireContent("@v10")

	// 降版本 v9 → superseded，返回当前身份，不重应用旧正文
	r, err = repo.Apply(ctx, owner, mkEnvelope(t, syncSrcA, "s1", 9, chat.KindReplace, 1))
	require.NoError(t, err)
	require.Equal(t, chat.StatusSuperseded, r.Status)
	require.Equal(t, "10", r.State.Version)
	require.Equal(t, env10.Digest, r.State.Digest)
	requireContent("@v10")
	require.Equal(t, int64(2), msgCount(t, db, owner, "s1"))

	// 异 source apply → conflict，内容不变
	r, err = repo.Apply(ctx, owner, mkEnvelope(t, syncSrcB, "s1", 11, chat.KindReplace, 3))
	require.NoError(t, err)
	require.Equal(t, chat.StatusConflict, r.Status)
	require.Equal(t, syncSrcA, r.State.SourceID)
	requireContent("@v10")

	// 跨版本跳跃 v12 → applied（全量快照语义）
	r, err = repo.Apply(ctx, owner, mkEnvelope(t, syncSrcA, "s1", 12, chat.KindReplace, 1))
	require.NoError(t, err)
	require.Equal(t, chat.StatusApplied, r.Status)
	require.Equal(t, "12", r.State.Version)
	require.Equal(t, int64(1), msgCount(t, db, owner, "s1"), "全量替换：旧消息必须清空")
}

func TestChatSyncApplyReplaceSessionMetadata(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)

	env := mkEnvelope(t, syncSrcA, "s1", 1, chat.KindReplace, 0)
	env.Payload.Session.Title = "元数据标题"
	env.Payload.Session.ModelSelectionID = "glm-5-air"
	env.Payload.Session.Hidden = true
	b, err := json.Marshal(env.Payload)
	require.NoError(t, err)
	env.RawPayload = b
	d, err := chat.DigestEnvelope(env)
	require.NoError(t, err)
	env.Digest = d

	_, err = repo.Apply(ctx, owner, env)
	require.NoError(t, err)

	var sess chat.Session
	require.NoError(t, db.Where("tenant_id = ? AND user_id = ? AND id = ?", owner.TenantID, owner.UserID, "s1").First(&sess).Error)
	require.Equal(t, "元数据标题", sess.Title)
	require.Equal(t, "glm-5-air", sess.ModelSelectionID)
	require.True(t, sess.Hidden)
}

// GetState 阶段映射（claim 占位无快照身份三件）。
func TestChatSyncGetStatePhases(t *testing.T) {
	db := setupSyncTestDB(t)
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.GetState(ctx, owner, "s1")
	require.ErrorIs(t, err, chat.ErrSyncNotFound)

	_, err = repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)
	st, err := repo.GetState(ctx, owner, "s1")
	require.NoError(t, err)
	require.Equal(t, chat.StateClaimedEmpty, st.Status)
	require.Equal(t, "0", st.Version)

	_, err = repo.Apply(ctx, owner, mkEnvelope(t, syncSrcA, "s1", 3, chat.KindReplace, 1))
	require.NoError(t, err)
	st, err = repo.GetState(ctx, owner, "s1")
	require.NoError(t, err)
	require.Equal(t, chat.StateActive, st.Status)
	require.Equal(t, "3", st.Version)
	require.Equal(t, chat.KindReplace, chat.Kind(st.Kind))
}
