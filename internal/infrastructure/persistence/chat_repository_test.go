package repository

import (
	"errors"
	"testing"
	"time"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupChatRepoTestDB 起 sqlite 内存库并替换 database.DB 包级变量
// （与 internal/handler/chat_handler_test.go 同一基建模式）。
func setupChatRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.UploadRecord{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

// seedChatTenantData 造两个租户各一条 session + message。
func seedChatTenantData(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u-a", TenantID: "org-a", ID: "s-a", Title: "org-a 会话",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u-b", TenantID: "org-b", ID: "s-b", Title: "org-b 会话",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&chat.Message{
		UserID: "u-a", TenantID: "org-a", ID: "m-a", SessionID: "s-a", Role: "user", Content: "a-secret",
	}).Error)
	require.NoError(t, db.Create(&chat.Message{
		UserID: "u-b", TenantID: "org-b", ID: "m-b", SessionID: "s-b", Role: "user", Content: "b-secret",
	}).Error)
}

func TestChatRepository_ListSessions_TenantIsolation(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	sessions, total, err := repo.ListSessions("org-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, sessions, 1)
	require.Equal(t, "s-a", sessions[0].ID)

	sessions, total, err = repo.ListSessions("org-b", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, sessions, 1)
	require.Equal(t, "s-b", sessions[0].ID)
}

func TestChatRepository_ListSessionsByUser_TenantIsolation(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	// org-b 里不存在 u-a：同 user_id 跨租户也不可探测
	sessions, total, err := repo.ListSessionsByUser("org-b", "u-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Empty(t, sessions)

	sessions, total, err = repo.ListSessionsByUser("org-a", "u-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, sessions, 1)
}

func TestChatRepository_GetSession_CrossTenantNotFound(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	_, err := repo.GetSession("org-b", "s-a")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)

	sess, err := repo.GetSession("org-a", "s-a")
	require.NoError(t, err)
	require.Equal(t, "s-a", sess.ID)
}

func TestChatRepository_GetSessionForUser_CrossTenantNotFound(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	_, err := repo.GetSessionForUser("org-b", "s-a", "u-a")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "跨租户读必须返回 ErrRecordNotFound, got %v", err)

	sess, err := repo.GetSessionForUser("org-a", "s-a", "u-a")
	require.NoError(t, err)
	require.Equal(t, "s-a", sess.ID)
}

func TestChatRepository_ListMessages_TenantIsolation(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	msgs, total, err := repo.ListMessages("org-b", "s-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Empty(t, msgs)

	msgs, total, err = repo.ListMessages("org-a", "s-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, msgs, 1)
	require.Equal(t, "m-a", msgs[0].ID)
}

func TestChatRepository_DeleteSession_CrossTenantNoop(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()
	// issue #94 review R2 F3：上传记录（附件授权锚点）必须随会话在同一删除
	// 事务内一并清除。
	require.NoError(t, db.Create(&chat.UploadRecord{
		ID: "up-a", TenantID: "org-a", SessionID: "s-a", UserID: "u-a",
		Name: "a.txt", Mime: "text/plain", Size: 3, Path: ".zerone-uploads/a.txt",
	}).Error)

	// org-b 删 org-a 的会话：不删任何行
	require.NoError(t, repo.DeleteSession("org-b", "s-a"))
	var cnt int64
	require.NoError(t, db.Model(&chat.Session{}).Where("id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响目标行")
	require.NoError(t, db.Model(&chat.Message{}).Where("session_id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响消息行")
	require.NoError(t, db.Model(&chat.UploadRecord{}).Where("session_id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt, "跨租户删除不得影响上传记录行")

	// 同租户删除生效（session + message + upload record 都删）
	require.NoError(t, repo.DeleteSession("org-a", "s-a"))
	require.NoError(t, db.Model(&chat.Session{}).Where("id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
	require.NoError(t, db.Model(&chat.Message{}).Where("session_id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
	require.NoError(t, db.Model(&chat.UploadRecord{}).Where("session_id = ?", "s-a").Count(&cnt).Error)
	require.Equal(t, int64(0), cnt, "上传记录必须随会话在删除事务内一并清除")
}

func TestChatRepository_CreateSession_StampsTenant(t *testing.T) {
	setupChatRepoTestDB(t)
	repo := NewChatRepository()

	// 调用方传入的 TenantID 不可信，repository 必须强制盖章
	sess := &chat.Session{UserID: "u-a", TenantID: "forged", ID: "s-new", Title: "t"}
	require.NoError(t, repo.CreateSession("org-a", sess))
	require.Equal(t, "org-a", sess.TenantID)

	got, err := repo.GetSession("org-a", "s-new")
	require.NoError(t, err)
	require.Equal(t, "org-a", got.TenantID)

	// 盖章后其他租户不可见
	_, err = repo.GetSession("org-b", "s-new")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestChatRepository_CreateMessage_StampsTenant(t *testing.T) {
	db := setupChatRepoTestDB(t)
	require.NoError(t, db.Create(&chat.Session{UserID: "u-a", TenantID: "org-a", ID: "s-a"}).Error)
	repo := NewChatRepository()

	msg := &chat.Message{UserID: "u-a", TenantID: "forged", ID: "m-new", SessionID: "s-a", Role: "user"}
	require.NoError(t, repo.CreateMessage("org-a", msg))
	require.Equal(t, "org-a", msg.TenantID)

	msgs, total, err := repo.ListMessages("org-a", "s-a", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "org-a", msgs[0].TenantID)
}

func TestChatRepository_PushSessions_StampsTenant(t *testing.T) {
	db := setupChatRepoTestDB(t)
	repo := NewChatRepository()

	now := time.Now().UTC()
	sess := &chat.Session{TenantID: "forged", ID: "s-push", Title: "pushed", CreatedAt: now, UpdatedAt: now}
	msg := &chat.Message{TenantID: "forged", ID: "m-push", Role: "user", Content: "hi", CreatedAt: now}

	result, err := repo.PushSessions("org-a", "u-a", []*chat.Session{sess}, [][]*chat.Message{{msg}})
	require.NoError(t, err)
	require.Equal(t, 1, result.SyncedSessions)
	require.Equal(t, 1, result.SyncedMessages)

	require.Equal(t, "org-a", sess.TenantID)
	require.Equal(t, "org-a", msg.TenantID)

	// 落库值也是盖章后的租户
	var stored chat.Session
	require.NoError(t, db.Where("id = ?", "s-push").First(&stored).Error)
	require.Equal(t, "org-a", stored.TenantID)
	var storedMsg chat.Message
	require.NoError(t, db.Where("id = ?", "m-push").First(&storedMsg).Error)
	require.Equal(t, "org-a", storedMsg.TenantID)

	// org-b 视角不可见
	_, err = repo.GetSession("org-b", "s-push")
	require.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestChatRepository_PushSessions_NormalizesMessageOrder(t *testing.T) {
	db := setupChatRepoTestDB(t)
	repo := NewChatRepository()

	// 模拟 agent-runtime 快照推送：同一会话所有消息共用 session 级时间戳
	//（transcript 无逐条时间），数组顺序 = 时序真源（user 在前）
	ts := time.Now().UTC().Truncate(time.Second)
	sess := &chat.Session{ID: "s-order", Title: "order", CreatedAt: ts, UpdatedAt: ts}
	msgs := []*chat.Message{
		{ID: "m-user", Role: "user", Content: "question", CreatedAt: ts},
		{ID: "m-asst", Role: "assistant", Content: "answer", CreatedAt: ts},
		{ID: "m-asst2", Role: "assistant", Content: "answer2", CreatedAt: ts},
	}

	result, err := repo.PushSessions("org-a", "u-a", []*chat.Session{sess}, [][]*chat.Message{msgs})
	require.NoError(t, err)
	require.Equal(t, 1, result.SyncedSessions)
	require.Equal(t, 3, result.SyncedMessages)

	// 读路径（created_at ASC）必须保持数组顺序：相同时间戳被顶成严格递增
	list, total, err := repo.ListMessages("org-a", "s-order", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Equal(t, []string{"m-user", "m-asst", "m-asst2"}, []string{list[0].ID, list[1].ID, list[2].ID})
	for i := 1; i < len(list); i++ {
		require.True(t, list[i].CreatedAt.After(list[i-1].CreatedAt), "msg[%d] must be after msg[%d]", i, i-1)
	}

	// 幂等：重推同一载荷顺序不变
	_, err = repo.PushSessions("org-a", "u-a", []*chat.Session{sess}, [][]*chat.Message{msgs})
	require.NoError(t, err)
	list2, _, err := repo.ListMessages("org-a", "s-order", 1, 10)
	require.NoError(t, err)
	require.Equal(t, []string{"m-user", "m-asst", "m-asst2"}, []string{list2[0].ID, list2[1].ID, list2[2].ID})

	_ = db
}

func TestChatRepository_ListSessionsByAgentAndUser_TenantIsolation(t *testing.T) {
	db := setupChatRepoTestDB(t)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u-a", TenantID: "org-a", ID: "s-a", AgentID: "coder", Source: SourceAgentChatPageForTest,
	}).Error)
	require.NoError(t, db.Create(&chat.Session{
		UserID: "u-a", TenantID: "org-b", ID: "s-b", AgentID: "coder", Source: SourceAgentChatPageForTest,
	}).Error)
	repo := NewChatRepository()

	sessions, total, err := repo.ListSessionsByAgentAndUser("org-a", "coder", "u-a", "", 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, sessions, 1)
	require.Equal(t, "s-a", sessions[0].ID)
}

func TestChatRepository_UpdateSession_CrossTenantNoop(t *testing.T) {
	db := setupChatRepoTestDB(t)
	seedChatTenantData(t, db)
	repo := NewChatRepository()

	// 跨租户 Update 不得命中
	require.NoError(t, repo.UpdateSessionTitle("org-b", "s-a", "hijacked"))
	require.NoError(t, repo.UpdateSessionRuntimeSessionID("org-b", "s-a", "rt-hijacked"))

	sess, err := repo.GetSession("org-a", "s-a")
	require.NoError(t, err)
	require.NotEqual(t, "hijacked", sess.Title)
	require.Empty(t, sess.RuntimeSessionID)

	// 同租户 Update 生效
	require.NoError(t, repo.UpdateSessionRuntimeSessionID("org-a", "s-a", "rt-1"))
	sess, err = repo.GetSession("org-a", "s-a")
	require.NoError(t, err)
	require.Equal(t, "rt-1", sess.RuntimeSessionID)
}

// SourceAgentChatPageForTest 与 services.SourceAgentChatPage 同值，
// 避免 repository 测试反向依赖 services 包。
const SourceAgentChatPageForTest = "agent_chat_page"
