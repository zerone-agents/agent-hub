package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/builtin"
	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubTokenProvider：经 builtinTokenProvider 接口缝注入签发/撤销失败路径。
// RefreshToken/RevokeAllForUser 不在本次断言路径上，返回对应错误即可。
type stubTokenProvider struct {
	issueErr  error
	revokeErr error
}

func (s stubTokenProvider) IssueTokenPair(*authdom.User) (*auth.TokenPair, error) {
	return nil, s.issueErr
}
func (s stubTokenProvider) RefreshToken(string) (*auth.TokenPair, error) {
	return nil, s.issueErr
}
func (s stubTokenProvider) RevokeToken(token string) error {
	if token == "" {
		return nil // 镜像真实 provider：空 token 幂等返回 nil（provider.go:142）
	}
	return s.revokeErr
}
func (s stubTokenProvider) RevokeAllForUser(uint64) error {
	return s.revokeErr
}

var errBoomToken = &tokenErr{}

type tokenErr struct{}

func (*tokenErr) Error() string { return "boom token" }

func newAuthAuditEnv(t *testing.T) (*BuiltinAuthHandler, *gorm.DB) {
	t.Helper()
	// audit.Log 时间列带 MySQL 精度 tag，读回需 T4 同款 sqlite 包装器（同包复用）。
	db, err := gorm.Open(auditTestSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.User{}, &authdom.Invite{}, &authdom.RefreshToken{}, &audit.Log{}))
	users := services.NewUserService(db)
	ar := services.NewAuditRecorder(repository.NewAuditRepository(db))
	h := &BuiltinAuthHandler{ // 同包测试：直接构造以注入 stub
		p: builtin.New(db, builtinTestSecret), users: users,
		invites: services.NewInviteService(db), audit: ar,
	}
	return h, db
}

func rowsOf(t *testing.T, db *gorm.DB, action audit.Action) []audit.Log {
	t.Helper()
	var out []audit.Log
	require.NoError(t, db.Where("action = ?", action).Find(&out).Error)
	return out
}

// ginCtxForAudit 构造无租户上下文（未认证端点语义：login/setup）。
// 已认证端点测试自行补 c.Set("tenant_id", "default")——镜像 jwtutil 行为。
func ginCtxForAudit() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/auth/x", bytes.NewReader(nil))
	return c
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestAuditLoginSuccess(t *testing.T) {
	h, db := newAuthAuditEnv(t)
	_, err := h.users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	c := ginCtxForAudit()
	c.Request = httptest.NewRequest("POST", "/auth/login",
		bytes.NewReader(mustJSON(t, map[string]string{"username": "admin", "password": "Passw0rd!"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	require.Equal(t, http.StatusOK, c.Writer.Status())
	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
	require.Equal(t, "default", rows[0].TenantID) // builtin 恒 default（spec §5.6）
	// 尝试的用户名落 LoginDetail（未认证端点无 actor user_name——T3 数据模型）
	require.Contains(t, rows[0].Detail, `"username":"admin"`)
}

func TestAuditLoginInvalidCredentials(t *testing.T) {
	h, db := newAuthAuditEnv(t)
	c := ginCtxForAudit()
	c.Request = httptest.NewRequest("POST", "/auth/login",
		bytes.NewReader(mustJSON(t, map[string]string{"username": "x", "password": "y"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	require.Equal(t, http.StatusUnauthorized, c.Writer.Status())
	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusFailure, rows[0].Status)
	require.Contains(t, rows[0].Detail, "invalid_credentials")
}

func TestAuditLoginTokenIssuanceFailed(t *testing.T) {
	h, db := newAuthAuditEnv(t)
	u, err := h.users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	h.p = stubTokenProvider{issueErr: errBoomToken}
	c := ginCtxForAudit()
	c.Request = httptest.NewRequest("POST", "/auth/login",
		bytes.NewReader(mustJSON(t, map[string]string{"username": "admin", "password": "Passw0rd!"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	require.Equal(t, http.StatusInternalServerError, c.Writer.Status())
	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusFailure, rows[0].Status)
	require.Contains(t, rows[0].Detail, "token_issuance_failed")
	// Authenticate 已通过：uid 已知（区别于 invalid_credentials 的空 uid）
	require.Equal(t, strconv.FormatUint(u.ID, 10), rows[0].UserID)
}

func TestAuditSetupBoundaryBeforeTokenIssue(t *testing.T) {
	// CreateInitialAdmin 成功 + IssueTokenPair 失败 → auth.setup 记录仍存在（spec §3.1）。
	// /auth/setup 是未认证端点（context 无 tenant_id）——埋点须自带 default 租户落库。
	h, db := newAuthAuditEnv(t)
	h.p = stubTokenProvider{issueErr: errBoomToken}
	c := ginCtxForAudit()
	c.Request = httptest.NewRequest("POST", "/auth/setup",
		bytes.NewReader(mustJSON(t, map[string]string{"password": "Passw0rd!", "confirmPassword": "Passw0rd!"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Setup(c)
	require.Equal(t, http.StatusInternalServerError, c.Writer.Status())
	rows := rowsOf(t, db, audit.ActionSetup)
	require.Len(t, rows, 1, "签发失败不得吞掉已生效变更的审计记录")
	require.Equal(t, "default", rows[0].TenantID)
}

func TestAuditLogoutSemantics(t *testing.T) {
	h, db := newAuthAuditEnv(t)
	// 撤销失败 → failure（凭证可能仍有效，如实记录）
	h.p = stubTokenProvider{revokeErr: errBoomToken}
	c := ginCtxForAudit()
	c.Set("tenant_id", "default") // 生产：JWT 中间件（jwtutil）注入
	c.Request = httptest.NewRequest("POST", "/auth/logout",
		bytes.NewReader(mustJSON(t, map[string]string{"refreshToken": "tok"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Logout(c)
	require.Equal(t, http.StatusOK, c.Writer.Status()) // HTTP 响应行为不变
	rows := rowsOf(t, db, audit.ActionLogout)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusFailure, rows[0].Status)

	// 空 token（幂等，无事可撤销）→ success
	db.Exec("DELETE FROM audit_logs")
	c2 := ginCtxForAudit()
	c2.Set("tenant_id", "default")
	c2.Request = httptest.NewRequest("POST", "/auth/logout", bytes.NewReader([]byte("{}")))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Logout(c2)
	require.Equal(t, http.StatusOK, c2.Writer.Status())
	rows = rowsOf(t, db, audit.ActionLogout)
	require.Len(t, rows, 1)
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
}

func TestAuditChangePasswordRow(t *testing.T) {
	h, db := newAuthAuditEnv(t)
	u, err := h.users.CreateInitialAdmin("Passw0rd!")
	require.NoError(t, err)
	c := ginCtxForAudit()
	c.Set("tenant_id", "default") // 生产：JWT 中间件（jwtutil）注入
	c.Set("user_id", strconv.FormatUint(u.ID, 10))
	c.Set("user_name", "admin")
	c.Request = httptest.NewRequest("POST", "/auth/change-password",
		bytes.NewReader(mustJSON(t, map[string]string{"oldPassword": "Passw0rd!", "newPassword": "Newpass123"})))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ChangePassword(c)
	require.Equal(t, http.StatusOK, c.Writer.Status())
	rows := rowsOf(t, db, audit.ActionPasswordChange)
	require.Len(t, rows, 1)
	require.Equal(t, audit.TargetUser, rows[0].TargetType)
	require.Equal(t, strconv.FormatUint(u.ID, 10), rows[0].TargetID)
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
}

func TestAuditCallbackEarlyStagesStdoutOnly(t *testing.T) {
	// 参数缺失 / state 无效 → org 未知 → 仅 stdout、不落库（spec §5.6 空租户规则）
	db, err := gorm.Open(auditTestSqlite{sqlite.Open(":memory:").(*sqlite.Dialector)}, &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	ar := services.NewAuditRecorder(repository.NewAuditRepository(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/callback", Callback(nil, ar)) // 早期失败不触达 provider，nil 安全

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/auth/callback", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/auth/callback?code=x&state=bogus", nil))
	require.Equal(t, http.StatusBadRequest, w2.Code)

	require.Len(t, rowsOf(t, db, audit.ActionLogin), 0) // 零行：未认证端点不得填充 DB
}
