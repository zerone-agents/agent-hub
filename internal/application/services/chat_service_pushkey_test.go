package services

import (
	"testing"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupPushKeyTestDB 与 handler 测试同款：sqlite 内存库替换 database.DB。
// 注意：必须在替换之后再 NewChatService()（repo 构造时抓取 DB）。
func setupPushKeyTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
}

func pushKeySession(id, userName, org string) SessionInput {
	return SessionInput{
		ID:        id,
		Title:     "t-" + id,
		CreatedAt: "2026-08-24T10:00:00Z",
		UpdatedAt: "2026-08-24T10:00:00Z",
		UserName:  userName,
		Org:       org,
		Messages: []MessageInput{
			{ID: "m-" + id, Role: "user", Content: "hi", CreatedAt: "2026-08-24T10:00:01Z"},
		},
	}
}

func TestPushWithSessionIdentity_AssignsPerSessionIdentity(t *testing.T) {
	setupPushKeyTestDB(t)
	s := NewChatService()

	resp, err := s.PushWithSessionIdentity(&PushRequest{
		Sessions: []SessionInput{
			pushKeySession("s-a", "alice", "zerone"),
			pushKeySession("s-b", "bob", ""), // org 缺省 → default
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, 2, resp.SyncedSessions)
	require.Equal(t, 2, resp.SyncedMessages)

	var sa chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-a").First(&sa).Error)
	require.Equal(t, "alice", sa.UserID)
	require.Equal(t, "alice", sa.UserName)
	require.Equal(t, "alice", sa.DisplayName)
	require.Equal(t, "zerone", sa.TenantID)

	var sb chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-b").First(&sb).Error)
	require.Equal(t, "bob", sb.UserID)
	require.Equal(t, "default", sb.TenantID)

	// 消息的 UserID 跟随分组 user 盖章
	var msg chat.Message
	require.NoError(t, database.DB.Where("id = ?", "m-s-a").First(&msg).Error)
	require.Equal(t, "alice", msg.UserID)
	require.Equal(t, "zerone", msg.TenantID)
}

func TestPushWithSessionIdentity_MissingUserName(t *testing.T) {
	setupPushKeyTestDB(t)
	s := NewChatService()

	noName := pushKeySession("s-x", "someone", "")
	noName.UserName = ""
	resp, err := s.PushWithSessionIdentity(&PushRequest{
		Sessions: []SessionInput{pushKeySession("s-ok", "alice", "zerone"), noName},
	})
	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrPushValidation)
	require.Contains(t, err.Error(), "session[1]")
	require.Contains(t, err.Error(), "user_name is required")

	// 整请求拒绝：任何 session 都不应落库
	var count int64
	require.NoError(t, database.DB.Model(&chat.Session{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestPushWithSessionIdentity_TooManySessions(t *testing.T) {
	setupPushKeyTestDB(t)
	s := NewChatService()

	sessions := make([]SessionInput, 51)
	for i := range sessions {
		sessions[i] = pushKeySession("s-many-"+string(rune('a'+i%26)), "u", "default")
		sessions[i].ID = "s-many-" + string(rune('a'+i%26)) + "-" + string(rune('a'+i/26))
	}
	_, err := s.PushWithSessionIdentity(&PushRequest{Sessions: sessions})
	require.Error(t, err)
	require.Contains(t, err.Error(), "too many sessions")
}

func TestPushWithSessionIdentity_EmptyRequest(t *testing.T) {
	setupPushKeyTestDB(t)
	s := NewChatService()

	resp, err := s.PushWithSessionIdentity(&PushRequest{})
	require.NoError(t, err)
	require.True(t, resp.Success)
}
