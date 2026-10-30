package repository

// H3 回归（issue #232 计划）：原子 replace / delete-wins / tombstone 围栏。
//
// 覆盖计划清单：
//   - 原子替换：old=[A,B]→new=[A2] 仅剩 A2；中途失败（消息时间非法）回滚
//     保留旧状态；
//   - ordinal 展示排序：同 timestamp / 甚至 created_at 与 ordinal 相逆时，
//     仍按 ordinal 稳定读（同步正文不靠 created_at 排序去重——契约 §6）；
//   - delete-wins：replace→delete 到达顺序、超时重试（重复 delete 返回原
//     删除身份）、delete 后更高版本 replace 拒绝复活；
//   - DeleteSession 审计：普通删除入口不触碰 sync-state——tombstone 永久
//     保留、迟到请求仍被围栏拦截；active 键被管理员删正文后，更高版本
//     快照按「Hub 为可重建展示副本」语义重建。

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"control-panel/internal/domain/chat"

	"github.com/stretchr/testify/require"
)

// mkEnvelopeRawMsgs：直接给定消息列表（允许构造非法时间等 apply 期失败用例；
// 摘要按原始字节计算，不受时间合法性影响——时间格式在 apply 时才解析）。
func mkEnvelopeRawMsgs(t *testing.T, sourceID, sessionID string, version int64, msgs []chat.SyncMessagePayload) chat.Envelope {
	t.Helper()
	e := chat.Envelope{
		ProtocolVersion:   chat.ProtocolVersion,
		ProjectionVersion: chat.ProjectionVersion,
		SourceID:          sourceID,
		AppSessionID:      sessionID,
		Version:           version,
		SnapshotID:        fmt.Sprintf("%08d-1111-4222-8333-%012d", version, version%1000000000000),
		Kind:              chat.KindReplace,
	}
	rp := &chat.ReplacePayload{
		Session: chat.SyncSessionPayload{
			ID: sessionID, Title: fmt.Sprintf("t-v%d", version),
			CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-02T00:00:00Z",
		},
		Messages: msgs,
	}
	e.Payload = rp
	b, err := json.Marshal(rp)
	require.NoError(t, err)
	e.RawPayload = b
	d, err := chat.DigestEnvelope(e)
	require.NoError(t, err)
	e.Digest = d
	return e
}

func syncMsg(id, content, createdAt string, ordinal int64) chat.SyncMessagePayload {
	return chat.SyncMessagePayload{ID: id, Role: "user", Content: content, CreatedAt: createdAt, Ordinal: ordinal}
}

func listMsgIDs(t *testing.T, r *ChatRepository, tenantID, sessionID string) []string {
	t.Helper()
	msgs, _, err := r.ListMessages(tenantID, sessionID, 1, 100)
	require.NoError(t, err)
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}

func TestChatSyncAtomicReplace(t *testing.T) {
	db := setupSyncTestDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)

	// v1：两条消息 [A, B]
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 1, []chat.SyncMessagePayload{
		syncMsg("s1-a", "A", "2026-10-01T00:00:00Z", 0),
		syncMsg("s1-b", "B", "2026-10-01T00:01:00Z", 1),
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"s1-a", "s1-b"}, listMsgIDs(t, chatRepo, "t1", "s1"))

	// v2：全量替换为单条 [A2] → 仅剩 A2
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 2, []chat.SyncMessagePayload{
		syncMsg("s1-a2", "A2", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"s1-a2"}, listMsgIDs(t, chatRepo, "t1", "s1"),
		"全量替换后不得残留旧正文")

	// v3：中途失败（第二条消息时间非法，解析发生在会话 upsert 与旧消息删除
	// 之后）→ 事务回滚，内容与确认身份保持 v2
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 3, []chat.SyncMessagePayload{
		syncMsg("s1-c", "C", "2026-10-01T00:00:00Z", 0),
		syncMsg("s1-bad", "D", "not-a-time", 1),
	}))
	require.Error(t, err, "非法时间必须失败")
	require.Contains(t, err.Error(), "created_at")
	require.Equal(t, []string{"s1-a2"}, listMsgIDs(t, chatRepo, "t1", "s1"),
		"中途失败不得留下半应用状态")
	st, err := repo.GetState(ctx, owner, "s1")
	require.NoError(t, err)
	require.Equal(t, "2", st.Version)

	// v4：ordinal 权威排序——同 timestamp 不换序；created_at 与 ordinal 相逆
	// 时仍按 ordinal 读（当前 created_at 排序会给出相反顺序，此为 H3 RED 点）
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 4, []chat.SyncMessagePayload{
		syncMsg("s1-first", "o0", "2026-10-05T00:00:00Z", 0),  // 时间更晚、序更前
		syncMsg("s1-second", "o1", "2026-10-01T00:00:00Z", 1), // 时间更早、序在后
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"s1-first", "s1-second"},
		listMsgIDs(t, chatRepo, "t1", "s1"), "展示顺序以 ordinal 为权威，不按 created_at")

	// v5：同 timestamp + id 逆序 → 仍按 ordinal
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 5, []chat.SyncMessagePayload{
		syncMsg("z-zeta", "same-ts-0", "2026-10-01T00:00:00Z", 0),
		syncMsg("a-alpha", "same-ts-1", "2026-10-01T00:00:00Z", 1),
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"z-zeta", "a-alpha"},
		listMsgIDs(t, chatRepo, "t1", "s1"), "同 timestamp 消息不得交换顺序")
}

func TestChatSyncDeleteWins(t *testing.T) {
	db := setupSyncTestDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)

	// replace v5（有正文）→ delete v7
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 5, []chat.SyncMessagePayload{
		syncMsg("s1-a", "A", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)

	delEnv := mkEnvelope(t, syncSrcA, "s1", 7, chat.KindDelete, 0)
	r, err := repo.Apply(ctx, owner, delEnv)
	require.NoError(t, err)
	require.Equal(t, chat.StatusApplied, r.Status)
	require.Equal(t, chat.StateDeleted, r.State.Status)
	require.Equal(t, "7", r.State.Version)
	require.Equal(t, chat.KindDelete, chat.Kind(r.State.Kind))
	require.Empty(t, listMsgIDs(t, chatRepo, "t1", "s1"), "delete 后无正文")

	// 超时重试：同一 delete envelope 重发 → duplicate 且返回原删除身份
	r, err = repo.Apply(ctx, owner, delEnv)
	require.NoError(t, err)
	require.Equal(t, chat.StatusDuplicate, r.Status)
	require.Equal(t, delEnv.SnapshotID, r.State.SnapshotID)
	require.Equal(t, delEnv.Digest, r.State.Digest)
	require.Equal(t, "7", r.State.Version)

	// delete 后更高版本 replace → 拒绝复活
	r, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 9, []chat.SyncMessagePayload{
		syncMsg("s1-zombie", "Z", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)
	require.Equal(t, chat.StatusDeleted, r.Status, "tombstone 拒绝复活（含更高版本）")
	require.Empty(t, listMsgIDs(t, chatRepo, "t1", "s1"))

	// 普通 DeleteSession 不清 tombstone：迟到请求仍被围栏拦截
	require.NoError(t, chatRepo.DeleteSession("t1", "s1"))
	r, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 12, []chat.SyncMessagePayload{
		syncMsg("s1-late", "L", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)
	require.Equal(t, chat.StatusDeleted, r.Status)
	st, err := repo.GetState(ctx, owner, "s1")
	require.NoError(t, err)
	require.Equal(t, chat.StateDeleted, st.Status, "tombstone 不得被普通删除入口清除")
	require.Equal(t, "7", st.Version)
}

// TestChatSyncAdminDeleteActiveThenResync：active 键被管理员删除正文后，
// sync-state 不受影响；更高版本快照按「Hub 为可重建展示副本」重建正文。
func TestChatSyncAdminDeleteActiveThenResync(t *testing.T) {
	db := setupSyncTestDB(t)
	chatRepo := NewChatRepository()
	repo := NewChatSyncRepository(db)
	owner := syncOwner("t1", "u1")
	ctx := context.Background()

	_, err := repo.Claim(ctx, owner, "s1", syncSrcA, "imported")
	require.NoError(t, err)
	_, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 3, []chat.SyncMessagePayload{
		syncMsg("s1-a", "A", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)

	// 管理员删除正文（不触发协议删除）：sync-state 保持 active
	require.NoError(t, chatRepo.DeleteSession("t1", "s1"))
	st, err := repo.GetState(ctx, owner, "s1")
	require.NoError(t, err)
	require.Equal(t, chat.StateActive, st.Status)
	require.Equal(t, "3", st.Version)

	// 同版本重放 → duplicate（身份未变，不重建正文）
	r, err := repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 3, []chat.SyncMessagePayload{
		syncMsg("s1-a", "A", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)
	require.Equal(t, chat.StatusDuplicate, r.Status)

	// 更高版本快照 → applied 重建正文
	r, err = repo.Apply(ctx, owner, mkEnvelopeRawMsgs(t, syncSrcA, "s1", 4, []chat.SyncMessagePayload{
		syncMsg("s1-a4", "A4", "2026-10-01T00:00:00Z", 0),
	}))
	require.NoError(t, err)
	require.Equal(t, chat.StatusApplied, r.Status)
	require.Equal(t, []string{"s1-a4"}, listMsgIDs(t, chatRepo, "t1", "s1"))
}
