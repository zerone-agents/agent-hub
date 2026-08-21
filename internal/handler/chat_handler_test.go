package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/chat"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChatHandlerTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	require.NoError(t, db.Create(&chat.Session{UserID: "u1", TenantID: chatTestTenant, ID: "s-own", Title: "自己的会话"}).Error)
	require.NoError(t, db.Create(&chat.Session{UserID: "u2", TenantID: chatTestTenant, ID: "s-other", Title: "他人会话"}).Error)
	require.NoError(t, db.Create(&chat.Message{UserID: "u1", TenantID: chatTestTenant, ID: "m1", SessionID: "s-own", Role: "user", Content: "hi"}).Error)
	require.NoError(t, db.Create(&chat.Message{UserID: "u2", TenantID: chatTestTenant, ID: "m2", SessionID: "s-other", Role: "user", Content: "secret"}).Error)
}

// chatTestTenant 是 handler 测试共用的租户 ID；种子数据与 gin context 必须一致，
// 否则 TenantOwned 过滤后查不到任何行。
const chatTestTenant = "tenant-a"

// newChatContext 构造携带身份信息的 gin 测试 context（绕过中间件直调 handler；
// 中间件角色判定已由 middleware 测试锁定）。
func newChatContext(w *httptest.ResponseRecorder, roles []string, userID string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("roles", roles)
	c.Set("user_id", userID)
	c.Set("tenant_id", chatTestTenant)
	return c
}

func TestChatListSessions_MemberSeesOnlyOwn(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"member"}, "u1")
	h.ListSessions(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Items []chat.Session `json:"items"`
			Total int64          `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, int64(1), body.Data.Total)
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, "s-own", body.Data.Items[0].ID)
}

func TestChatListSessions_AdminSeesAll(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"admin"}, "u0")
	h.ListSessions(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, int64(2), body.Data.Total)
}

func TestChatGetSession_MemberOthersReturns404(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"member"}, "u1")
	c.Params = gin.Params{{Key: "id", Value: "s-other"}}
	h.GetSession(c)
	require.Equal(t, http.StatusNotFound, w.Code)

	w2 := httptest.NewRecorder()
	c2 := newChatContext(w2, []string{"member"}, "u1")
	c2.Params = gin.Params{{Key: "id", Value: "s-own"}}
	h.GetSession(c2)
	require.Equal(t, http.StatusOK, w2.Code)
}

func TestChatListMessages_MemberOthersReturns404(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"member"}, "u1")
	c.Params = gin.Params{{Key: "id", Value: "s-other"}}
	h.ListMessages(c)
	require.Equal(t, http.StatusNotFound, w.Code)

	w2 := httptest.NewRecorder()
	c2 := newChatContext(w2, []string{"member"}, "u1")
	c2.Params = gin.Params{{Key: "id", Value: "s-own"}}
	h.ListMessages(c2)
	require.Equal(t, http.StatusOK, w2.Code)
}

func TestChatDeleteSession_MemberOwnOnly(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	// 删他人会话 → 404，且数据仍在
	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"member"}, "u1")
	c.Params = gin.Params{{Key: "id", Value: "s-other"}}
	h.DeleteSession(c)
	require.Equal(t, http.StatusNotFound, w.Code)
	var cnt int64
	require.NoError(t, database.GetDB().Model(&chat.Session{}).Where("id = ?", "s-other").Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)

	// 删自己的 → 200，且真删了
	w2 := httptest.NewRecorder()
	c2 := newChatContext(w2, []string{"member"}, "u1")
	c2.Params = gin.Params{{Key: "id", Value: "s-own"}}
	h.DeleteSession(c2)
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, database.GetDB().Model(&chat.Session{}).Where("id = ?", "s-own").Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
}

func TestChatDeleteSession_AdminDeletesAny(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler()

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"maintainer"}, "u0")
	c.Params = gin.Params{{Key: "id", Value: "s-other"}}
	h.DeleteSession(c)
	require.Equal(t, http.StatusOK, w.Code)
}
