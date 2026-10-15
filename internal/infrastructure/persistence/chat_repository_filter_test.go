package repository

import (
	"testing"
	"time"

	"control-panel/internal/domain/chat"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupFilterDB 复用同包 chat_repository_test.go 的 sqlite 基建（setupChatRepoTestDB）。
func setupFilterDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupChatRepoTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []*chat.Session{
		{UserID: "u1", TenantID: "org-a", ID: "s1", Title: "产品讨论", AgentID: "general", UserName: "alice", DisplayName: "爱丽丝", CreatedAt: base, UpdatedAt: base.Add(1 * time.Hour)},
		{UserID: "u2", TenantID: "org-a", ID: "s2", Title: "100% 命中", AgentID: "coder", UserName: "bob", DisplayName: "鲍勃", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)},
		{UserID: "u1", TenantID: "org-b", ID: "s3", Title: "跨租户会话", AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(3 * time.Hour)},
	}
	for _, s := range rows {
		require.NoError(t, db.Create(s).Error)
	}
	msgs := []*chat.Message{
		{UserID: "u1", TenantID: "org-a", ID: "m1", SessionID: "s1", Role: "user", Content: `[{"type":"text","text":"你好世界"}]`, CreatedAt: base},
		{UserID: "u2", TenantID: "org-a", ID: "m2", SessionID: "s2", Role: "user", Content: `[{"type":"text","text":"secret-token-body"}]`, CreatedAt: base},
	}
	for _, m := range msgs {
		require.NoError(t, db.Create(m).Error)
	}
	return db
}

func TestChatSessionFilter(t *testing.T) {
	setupFilterDB(t)
	repo := NewChatRepository()
	base := func() ChatSessionFilter {
		return ChatSessionFilter{TenantID: "org-a", Page: 1, PageSize: 10}
	}
	ids := func(t *testing.T, filter ChatSessionFilter) []string {
		t.Helper()
		list, total, err := repo.ListSessionsFiltered(filter)
		require.NoError(t, err)
		require.Equal(t, int64(len(list)), total, "COUNT 必须与 rows 命中同一集合")
		out := make([]string, 0, len(list))
		for _, s := range list {
			out = append(out, s.ID)
		}
		return out
	}

	t.Run("租户隔离始终生效", func(t *testing.T) {
		require.Equal(t, []string{"s2", "s1"}, ids(t, base()))
	})
	t.Run("agent_id 精确", func(t *testing.T) {
		f2 := base()
		f2.AgentID = "coder"
		require.Equal(t, []string{"s2"}, ids(t, f2))
	})
	t.Run("user 两列模糊", func(t *testing.T) {
		f2 := base()
		f2.User = "bob"
		require.Equal(t, []string{"s2"}, ids(t, f2))
	})
	t.Run("keyword 只命中标题不命中正文", func(t *testing.T) {
		f2 := base()
		f2.Keyword = "secret-token-body"
		require.Equal(t, []string{}, ids(t, f2))
	})
	t.Run("message_contains 命中正文", func(t *testing.T) {
		f2 := base()
		f2.MessageContains = "secret-token"
		require.Equal(t, []string{"s2"}, ids(t, f2))
	})
	t.Run("LIKE 字面量转义只匹配含百分号的标题", func(t *testing.T) {
		f2 := base()
		f2.Keyword = "%"
		require.Equal(t, []string{"s2"}, ids(t, f2), "未转义会退化成匹配全部")
	})
	t.Run("空串等同未传", func(t *testing.T) {
		f2 := base()
		f2.Keyword = ""
		f2.User = ""
		f2.MessageContains = ""
		require.Equal(t, []string{"s2", "s1"}, ids(t, f2))
	})
	t.Run("ScopeUserID 只收窄不放宽", func(t *testing.T) {
		f2 := base()
		f2.ScopeUserID = "u1"
		f2.User = "bob"
		require.Equal(t, []string{}, ids(t, f2), "member 传他人 user 不得越界")
	})
	t.Run("时间闭区间作用于 updated_at", func(t *testing.T) {
		from := time.Date(2026, 1, 1, 1, 30, 0, 0, time.UTC)
		to := time.Date(2026, 1, 1, 2, 30, 0, 0, time.UTC)
		f2 := base()
		f2.From, f2.To = &from, &to
		require.Equal(t, []string{"s2"}, ids(t, f2))
	})
}
