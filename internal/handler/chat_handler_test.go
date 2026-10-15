package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	require.NoError(t, db.AutoMigrate(&chat.Session{}, &chat.Message{}, &chat.UploadRecord{}))
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
	h := NewChatHandler("builtin", nil)

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
	h := NewChatHandler("builtin", nil)

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
	h := NewChatHandler("builtin", nil)

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
	h := NewChatHandler("builtin", nil)

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
	h := NewChatHandler("builtin", nil)

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
	h := NewChatHandler("builtin", nil)

	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"maintainer"}, "u0")
	c.Params = gin.Params{{Key: "id", Value: "s-other"}}
	h.DeleteSession(c)
	require.Equal(t, http.StatusOK, w.Code)
}

// --- Push: X-Chat-Push-Key 通道（中间件已由 Task 1 锁定，这里直调 handler，
// 通过 auth_method context key 模拟中间件标记） ---

func newPushKeyPushContext(w *httptest.ResponseRecorder, body string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/push", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("auth_method", "chat_push_key")
	return c
}

func TestChatPush_PushKeyMode_AssignsBodyIdentity(t *testing.T) {
	setupChatHandlerTestDB(t)
	// casdoor 模式：显式 org 直用；缺省 org 解析为 tenant_oauth_clients
	// default 行组织（这里用注入的 resolver 模拟，值 "ops-org"）
	h := NewChatHandler("casdoor", func() (string, bool) { return "ops-org", true })

	body := `{"sessions":[` +
		`{"id":"s-k1","created_at":"2026-08-24T10:00:00Z","updated_at":"2026-08-24T10:00:00Z","user_name":"alice","org":"zerone","messages":[{"id":"m-k1","role":"user","content":"hi","created_at":"2026-08-24T10:00:01Z"}]},` +
		`{"id":"s-k2","created_at":"2026-08-24T10:00:00Z","updated_at":"2026-08-24T10:00:00Z","user_name":"bob"}]}`
	w := httptest.NewRecorder()
	h.Push(newPushKeyPushContext(w, body))

	require.Equal(t, http.StatusOK, w.Code)

	var s1 chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-k1").First(&s1).Error)
	require.Equal(t, "alice", s1.UserID)
	require.Equal(t, "alice", s1.UserName)
	require.Equal(t, "alice", s1.DisplayName)
	require.Equal(t, "zerone", s1.TenantID)

	var s2 chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-k2").First(&s2).Error)
	require.Equal(t, "bob", s2.UserID)
	require.Equal(t, "ops-org", s2.TenantID)
}

func TestChatPush_PushKeyMode_MissingUserNameReturns400(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler("builtin", nil)

	body := `{"sessions":[{"id":"s-bad","created_at":"2026-08-24T10:00:00Z","updated_at":"2026-08-24T10:00:00Z"}]}`
	w := httptest.NewRecorder()
	h.Push(newPushKeyPushContext(w, body))

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "user_name is required")
}

// 防伪造回归：token 通道下 body 的 user_name/org 必须被忽略。
func TestChatPush_TokenMode_IgnoresBodyIdentity(t *testing.T) {
	setupChatHandlerTestDB(t)
	h := NewChatHandler("builtin", nil)

	body := `{"sessions":[{"id":"s-evil","created_at":"2026-08-24T10:00:00Z","updated_at":"2026-08-24T10:00:00Z","user_name":"attacker","org":"evil-org"}]}`
	w := httptest.NewRecorder()
	c := newChatContext(w, []string{"member"}, "u1")
	// token 通道依赖的 context 身份（真实链路由 JWT 中间件注入）
	c.Set("user_name", "u1-name")
	c.Set("display_name", "U1")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/chat/push", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Push(c)

	require.Equal(t, http.StatusOK, w.Code)

	var s chat.Session
	require.NoError(t, database.DB.Where("id = ?", "s-evil").First(&s).Error)
	require.Equal(t, "u1", s.UserID)
	require.Equal(t, chatTestTenant, s.TenantID)
	require.Equal(t, "u1-name", s.UserName)
}

// newChatContextQuery 与既有 newChatContext 同一套身份注入，只多带 query string。
func newChatContextQuery(w *httptest.ResponseRecorder, roles []string, userID, rawQuery string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Set("roles", roles)
	c.Set("user_id", userID)
	c.Set("tenant_id", chatTestTenant)
	return c
}

func seedChatFilterFixtures(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&chat.Session{UserID: "u3", TenantID: chatTestTenant, ID: "s-coder", Title: "100% 命中", AgentID: "coder", UserName: "carol", CreatedAt: base, UpdatedAt: base.Add(time.Hour)}).Error)
	require.NoError(t, db.Create(&chat.Message{UserID: "u3", TenantID: chatTestTenant, ID: "m-coder", SessionID: "s-coder", Role: "user", Content: `body-token-here`, CreatedAt: base}).Error)
}

func TestChatListSessionsFilters(t *testing.T) {
	setupChatHandlerTestDB(t)
	seedChatFilterFixtures(t)
	h := NewChatHandler("builtin", func() (string, bool) { return "default", true })
	listIDs := func(roles []string, uid, q string) (int, []string) {
		w := httptest.NewRecorder()
		h.ListSessions(newChatContextQuery(w, roles, uid, q))
		var body struct {
			Data struct {
				Items []chat.Session `json:"items"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		ids := make([]string, 0, len(body.Data.Items))
		for _, s := range body.Data.Items {
			ids = append(ids, s.ID)
		}
		return w.Code, ids
	}

	t.Run("agent_id 精确命中", func(t *testing.T) {
		code, ids := listIDs([]string{"admin"}, "1", "agent_id=coder")
		require.Equal(t, http.StatusOK, code)
		require.Equal(t, []string{"s-coder"}, ids)
	})
	t.Run("keyword 只命中标题", func(t *testing.T) {
		_, ids := listIDs([]string{"admin"}, "1", "keyword=body-token-here")
		require.Equal(t, []string{}, ids)
	})
	t.Run("message_contains 搭配收窄 → 命中", func(t *testing.T) {
		_, ids := listIDs([]string{"admin"}, "1", "message_contains=body-token&agent_id=coder")
		require.Equal(t, []string{"s-coder"}, ids)
	})
	t.Run("message_contains 无收窄（admin）→ 400", func(t *testing.T) {
		code, _ := listIDs([]string{"admin"}, "1", "message_contains=body-token")
		require.Equal(t, http.StatusBadRequest, code)
	})
	t.Run("message_contains 过短 → 400", func(t *testing.T) {
		code, _ := listIDs([]string{"admin"}, "1", "message_contains=a&agent_id=coder")
		require.Equal(t, http.StatusBadRequest, code)
	})
	t.Run("member 豁免收窄闸且范围不被放宽", func(t *testing.T) {
		code, ids := listIDs([]string{"member"}, "u1", "message_contains=hi")
		require.Equal(t, http.StatusOK, code)
		// 正向断言先行：豁免后必须仍命中本人会话（m1 content="hi" 属 u1/s-own）——
		// 只做反向断言的话空结果集也能过，是弱断言（评审建议 8）
		require.Contains(t, ids, "s-own", "member 豁免收窄闸后应仍看到自己的会话")
		for _, id := range ids {
			require.NotEqual(t, "s-coder", id, "member 不得看到他人会话")
		}
	})
	t.Run("非法 from → 400", func(t *testing.T) {
		code, _ := listIDs([]string{"admin"}, "1", "from=not-a-time")
		require.Equal(t, http.StatusBadRequest, code)
	})
	t.Run("不传筛选时与既有行为一致", func(t *testing.T) {
		code, ids := listIDs([]string{"admin"}, "1", "")
		require.Equal(t, http.StatusOK, code)
		require.ElementsMatch(t, []string{"s-own", "s-other", "s-coder"}, ids)
	})
}
