package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubProvider 实现 auth.Provider 全接口（镜像 internal/auth/jwtutil/jwt_test.go
// 的 fakeProvider 模式），供 JWT 分支测试使用。
type stubProvider struct {
	user *auth.AuthUser
	mode string
}

func (s *stubProvider) ValidateAccessToken(string) (*auth.AuthUser, error) { return s.user, nil }
func (s *stubProvider) RefreshToken(string) (*auth.TokenPair, error)       { return nil, nil }
func (s *stubProvider) RevokeToken(string) error                           { return nil }
func (s *stubProvider) GetUserIdentity(string) (*auth.AuthUser, bool)      { return s.user, true }
func (s *stubProvider) Mode() string                                       { return s.mode }

// newChatPushRouter 构造挂了 ChatPushAuth 的最小 gin 路由，final handler
// 回显 auth_method（存在时），用于断言中间件行为。
func newChatPushRouter(pushKey string, p auth.Provider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/push", ChatPushAuth(pushKey, nil, p), func(c *gin.Context) {
		am, _ := c.Get("auth_method")
		s, _ := am.(string)
		_, hasUserID := c.Get("user_id")
		c.JSON(http.StatusOK, gin.H{"auth_method": s, "has_user_id": hasUserID})
	})
	return r
}

func serveChatPush(r *gin.Engine, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/push", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestChatPushAuth_KeyUnsetOnServer_Returns401(t *testing.T) {
	r := newChatPushRouter("", nil)
	w := serveChatPush(r, map[string]string{"X-Chat-Push-Key": "anything"})
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChatPushAuth_WrongKey_Returns401(t *testing.T) {
	r := newChatPushRouter("secret", nil)
	w := serveChatPush(r, map[string]string{"X-Chat-Push-Key": "wrong"})
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChatPushAuth_CorrectKey_PassesAndMarksAuthMethod(t *testing.T) {
	r := newChatPushRouter("secret", nil)
	w := serveChatPush(r, map[string]string{"X-Chat-Push-Key": "secret"})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "chat_push_key")
}

func TestChatPushAuth_NoHeader_NoAuth_RejectedByJWTChain(t *testing.T) {
	r := newChatPushRouter("secret", nil)
	w := serveChatPush(r, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChatPushAuth_NoHeader_ValidJWT_PassesWithIdentity(t *testing.T) {
	p := &stubProvider{
		user: &auth.AuthUser{ID: "1", Username: "alice", Roles: []string{"member"}, TenantID: "default"},
		mode: "builtin",
	}
	r := newChatPushRouter("secret", p)
	w := serveChatPush(r, map[string]string{"Authorization": "Bearer valid"})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "builtin")
}

// 关键回归：guard 必须在 final handler 之前拦截待审批用户。
// 若错误地直接调用 AuthMiddlewareWithCLI（其成功路径内部 c.Next() 会先
// 执行 handler 再回来补 guard），本测试失败——这正是组合方式修正的原因。
func TestChatPushAuth_NoHeader_PendingUser_BlockedBeforeHandler(t *testing.T) {
	handlerCalled := false
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/push", ChatPushAuth("secret", nil, &stubProvider{
		user: &auth.AuthUser{ID: "9", Username: "pending-user", Roles: nil},
		mode: "casdoor",
	}), func(c *gin.Context) {
		handlerCalled = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/push", nil)
	req.Header.Set("Authorization", "Bearer pending")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.False(t, handlerCalled)
	require.Contains(t, w.Body.String(), "PENDING_APPROVAL")
}

// 终审建议：双 header 同时存在时 push-key 通道胜出（token 身份不注入）。
func TestChatPushAuth_BothHeaders_PushKeyWins(t *testing.T) {
	p := &stubProvider{
		user: &auth.AuthUser{ID: "1", Username: "alice", Roles: []string{"member"}, TenantID: "default"},
		mode: "builtin",
	}
	r := newChatPushRouter("secret", p)
	req := httptest.NewRequest(http.MethodPost, "/push", nil)
	req.Header.Set("X-Chat-Push-Key", "secret")
	req.Header.Set("Authorization", "Bearer valid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "chat_push_key")
	require.Contains(t, w.Body.String(), `"has_user_id":false`)
}
