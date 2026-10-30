package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupFilterDB 复用同包 chat_repository_test.go 的 sqlite 基建（setupChatRepoTestDB）。
func setupFilterDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupChatRepoTestDB(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []*chat.Session{
		{UserID: "u1", TenantID: "org-a", ID: "s1", Title: "产品讨论", AgentID: "general", UserName: "alice", DisplayName: "爱丽丝", CreatedAt: base, UpdatedAt: base.Add(1 * time.Hour)},
		{UserID: "u2", TenantID: "org-a", ID: "s2", Title: "100% 命中", AgentID: "coder", UserName: "bob", DisplayName: "鲍勃", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)},
		{UserID: "u1", TenantID: "org-a", ID: "s4", Title: "under_score", AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(4 * time.Hour)},
		{UserID: "u1", TenantID: "org-a", ID: "s5", Title: `back\slash`, AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(5 * time.Hour)},
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
		require.Equal(t, []string{"s5", "s4", "s2", "s1"}, ids(t, base()))
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
	t.Run("LIKE 字面量转义只匹配含下划线的标题", func(t *testing.T) {
		f2 := base()
		f2.Keyword = "_"
		require.Equal(t, []string{"s4"}, ids(t, f2), "_ 未转义会退化成单字符通配")
	})
	t.Run("反斜杠按普通字符匹配", func(t *testing.T) {
		f2 := base()
		f2.Keyword = `\`
		require.Equal(t, []string{"s5"}, ids(t, f2))
	})
	t.Run("空串等同未传", func(t *testing.T) {
		f2 := base()
		f2.Keyword = ""
		f2.User = ""
		f2.MessageContains = ""
		require.Equal(t, []string{"s5", "s4", "s2", "s1"}, ids(t, f2))
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

// ─── MySQL 真库冒烟（TEST_MYSQL_DSN 门控；PR #223 评审阻塞 1 的回归） ───

// sqlCapture 记录 GORM 实际发给数据库的 SQL 文本——EXPLAIN 复核必须用这里
// 捕获的语句，而不是手写近似语句（手写曾掩盖 ESCAPE '\' 的 1064）。
type sqlCapture struct{ stmts []string }

func (l *sqlCapture) LogMode(logger.LogLevel) logger.Interface      { return l }
func (l *sqlCapture) Info(context.Context, string, ...interface{})  {}
func (l *sqlCapture) Warn(context.Context, string, ...interface{})  {}
func (l *sqlCapture) Error(context.Context, string, ...interface{}) {}
func (l *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	if sql, _ := fc(); sql != "" {
		l.stmts = append(l.stmts, sql)
	}
}

// TestChatSessionFilter_MySQLSmoke 用生产引擎验证代码实际生成的 LIKE 语句。
// sqlite 不处理 SQL 文本里的反斜杠转义，掩盖过 ESCAPE '\' 的 ERROR 1064
// （MySQL 字面量里 \' 是转义引号、字符串不闭合）。门控范式与 pkg/database
// 的 TEST_MYSQL_DSN 一致；会 Drop/AutoMigrate cloud 两表——只指向一次性实例。
func TestChatSessionFilter_MySQLSmoke(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN 未设置，跳过 MySQL 冒烟")
	}
	capLog := &sqlCapture{}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: capLog})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&chat.Message{}, &chat.Session{}))
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.SessionSyncState{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = old
		_ = db.Migrator().DropTable(&chat.Message{}, &chat.Session{})
	})

	// 造数：与 sqlite fixture 同形状（小写，避开 MySQL 默认 collation 大小写歧义——
	// 大小写敏感性差异已在 spec §8 声明为非契约）。
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions := []*chat.Session{
		{UserID: "u1", TenantID: "org-a", ID: "s1", Title: "chat about products", AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(time.Hour)},
		{UserID: "u2", TenantID: "org-a", ID: "s2", Title: "100% hit", AgentID: "coder", UserName: "bob", CreatedAt: base, UpdatedAt: base.Add(2 * time.Hour)},
		{UserID: "u1", TenantID: "org-a", ID: "s4", Title: "under_score", AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(4 * time.Hour)},
		{UserID: "u1", TenantID: "org-a", ID: "s5", Title: `back\slash`, AgentID: "general", UserName: "alice", CreatedAt: base, UpdatedAt: base.Add(5 * time.Hour)},
	}
	msgs := []*chat.Message{
		{UserID: "u1", TenantID: "org-a", ID: "m1", SessionID: "s1", Role: "user", Content: `hello world`, CreatedAt: base},
		{UserID: "u2", TenantID: "org-a", ID: "m2", SessionID: "s2", Role: "user", Content: `secret-token-body`, CreatedAt: base},
	}
	for _, s := range sessions {
		require.NoError(t, db.Create(s).Error)
	}
	for _, m := range msgs {
		require.NoError(t, db.Create(m).Error)
	}

	repo := NewChatRepository()
	ids := func(t *testing.T, f ChatSessionFilter) []string {
		t.Helper()
		list, _, err := repo.ListSessionsFiltered(f)
		require.NoError(t, err, "代码生成的 SQL 必须在生产引擎上是合法语句")
		out := make([]string, 0, len(list))
		for _, s := range list {
			out = append(out, s.ID)
		}
		return out
	}
	mf := ChatSessionFilter{TenantID: "org-a", Page: 1, PageSize: 10}

	t.Run("user 两列 LIKE", func(t *testing.T) {
		f := mf
		f.User = "bob"
		require.Equal(t, []string{"s2"}, ids(t, f))
	})
	t.Run("keyword % 字面量", func(t *testing.T) {
		f := mf
		f.Keyword = "%"
		require.Equal(t, []string{"s2"}, ids(t, f))
	})
	t.Run("keyword _ 字面量", func(t *testing.T) {
		f := mf
		f.Keyword = "_"
		require.Equal(t, []string{"s4"}, ids(t, f))
	})
	t.Run("keyword 反斜杠字面量", func(t *testing.T) {
		f := mf
		f.Keyword = `\`
		require.Equal(t, []string{"s5"}, ids(t, f))
	})
	t.Run("message_contains", func(t *testing.T) {
		f := mf
		f.MessageContains = "secret-token"
		require.Equal(t, []string{"s2"}, ids(t, f))
	})

	// 捕获的实际 SQL（供 EXPLAIN 复核；每次 t.Logf 会带行号前缀，人工核对时取 SQL 部分）
	for _, sql := range capLog.stmts {
		t.Logf("CAPTURED SQL: %s", sql)
	}
}
