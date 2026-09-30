package handler

// casdoor 登录审计落库缺口：默认组织登录（URL 无 ?org=，session.Org 为空）时
// Callback 成功审计行租户取 session.Org = 空 → persist 跳过落库（stdout-only，
// spec §5.6 空租户规则），审计 UI 完全看不到 auth.login；而已认证请求（logout
// 等）租户来自 token 的 owner，非空 → 正常落库。
// 修复：租户改用 token 解析出的权威 org（user.Owner），回退 session.Org；
// 同时把 user.Id 记入 UserID（此前登录行用户列恒空）。

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/config"
	"control-panel/internal/domain/audit"
	authdom "control-panel/internal/domain/auth"
	repository "control-panel/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubMembershipStore：CasdoorProvider 构造所需（SyncMembership 的 admin
// fetch 在假 casdoor 上失败即返回，不会走到 store；接口方法给无害实现）。
type stubMembershipStore struct{}

func (stubMembershipStore) FindByExternalID(provider, externalID string) (*authdom.UserIdentity, error) {
	return nil, nil
}
func (stubMembershipStore) ListByTenant(tenantID string) ([]authdom.UserIdentity, error) {
	return nil, nil
}
func (stubMembershipStore) ApplyDecision(provider string, au *auth.AuthUser, d auth.MembershipDecision) error {
	return nil
}
func (stubMembershipStore) SetRole(provider, externalID, role, status string) error { return nil }

// genCasdoorTestKey 测试 RSA 私钥（登录取证验签密钥对的一半）。
func genCasdoorTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

// casdoorPubPEM 由私钥导出 PKIX 公钥 PEM（ParseJwtToken RS256 验签用）。
func casdoorPubPEM(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// mintCasdoorJWT 用测试私钥铸造 RS256 JWT（载荷与 casdoor 同构：owner /
// name / id 平铺）。
func mintCasdoorJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(claims))
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

// newCasdoorCallbackAuditEnv 构造回调审计测试环境：假 casdoor（token 端点返回
// accessToken，其余路径 404 → SyncMembership 宽松失败不阻断）+ 全局配置 swap
// （可恢复，含验签公钥）+ 内存审计库 + 真实 provider（stub store）+ 播种一条
// 登录会话（sessionOrg 为空即默认组织登录路径）。返回回调路由、审计库与 state。
func newCasdoorCallbackAuditEnv(t *testing.T, key *rsa.PrivateKey, accessToken, sessionOrg string) (*gin.Engine, *gorm.DB, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login/oauth/access_token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w,
				`{"access_token":%q,"token_type":"Bearer","expires_in":3600,"refresh_token":"rt-1"}`,
				accessToken)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":"error","msg":"not found"}`))
	}))
	t.Cleanup(srv.Close)

	restore := auth.SwapCasdoorForTest(&config.CasdoorConfig{
		Endpoint: srv.URL, ClientID: "cid", ClientSecret: "sec", Certificate: casdoorPubPEM(t, key),
	})
	t.Cleanup(restore)
	// 确定性：显式重建 tenantClientLookup（本包其他测试可能留有旧值）——空 org
	// （默认组织）走全局 env 凭证；非空 org 需要已注册的组织凭证（生产来自
	// tenant_oauth_clients 行），否则 ExchangeCodeForToken 在最早期就被
	// resolveClientCreds 拒绝。
	auth.SetTenantClientLookup(nil)
	if sessionOrg != "" {
		auth.SetTenantClientLookup(func(org string) (*auth.TenantClientCreds, bool) {
			if org != sessionOrg {
				return nil, false
			}
			return &auth.TenantClientCreds{ClientID: "cid", ClientSecret: "sec"}, true
		})
	}
	t.Cleanup(func() { auth.SetTenantClientLookup(nil) })

	db := openAuditEmbedDB(t)
	require.NoError(t, db.AutoMigrate(&audit.Log{}))
	ar := services.NewAuditRecorder(repository.NewAuditRepository(db))

	state, err := auth.GenerateState()
	require.NoError(t, err)
	auth.StoreSession(state, "verifier-1", sessionOrg, "/")

	provider := auth.NewCasdoorProvider(stubMembershipStore{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/auth/callback", Callback(provider, ar))
	return r, db, state
}

// serveCasdoorCallback 触发一次回调并断言 302 放行（重定向携带签发凭证）。
func serveCasdoorCallback(t *testing.T, r *gin.Engine, state string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=c1&state="+state, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code, "body=%s", rec.Body.String())
}

// TestCasdoorCallbackLoginAuditPersistsWhenSessionOrgEmpty：默认组织登录
// （session.Org 空）成功后，审计行必须落库，租户取 token 的权威 org、
// user_id 取 token 的用户 ID——修复前该行被空租户规则跳过（仅 stdout）。
func TestCasdoorCallbackLoginAuditPersistsWhenSessionOrgEmpty(t *testing.T) {
	key := genCasdoorTestKey(t)
	token := mintCasdoorJWT(t, key, map[string]any{
		"owner": "login-audit-org", "name": "zhiheng", "id": "u-1",
	})

	r, db, state := newCasdoorCallbackAuditEnv(t, key, token, "")
	serveCasdoorCallback(t, r, state)

	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1, "默认组织登录必须落库（修复前空租户被 stdout-only 跳过）")
	require.Equal(t, "login-audit-org", rows[0].TenantID)
	require.Equal(t, "u-1", rows[0].UserID)
	require.Equal(t, "zhiheng", rows[0].UserName, "spec §5.6：登录成功行 UserName=用户名")
	require.Equal(t, audit.StatusSuccess, rows[0].Status)
	require.Contains(t, rows[0].Detail, `"username":"zhiheng"`)
}

// TestCasdoorCallbackLoginAuditPrefersTokenOwnerOverSessionOrg：session.Org 非空
// 且 token 可解析时，权威 org（token owner）优先——多组织/默认组织混用时
// 审计行租户与会话租户（后续请求的 tenant）保持一致。
func TestCasdoorCallbackLoginAuditPrefersTokenOwnerOverSessionOrg(t *testing.T) {
	key := genCasdoorTestKey(t)
	token := mintCasdoorJWT(t, key, map[string]any{
		"owner": "acme-org", "name": "zhiheng", "id": "u-2",
	})

	r, db, state := newCasdoorCallbackAuditEnv(t, key, token, "beta-org")
	serveCasdoorCallback(t, r, state)

	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1)
	require.Equal(t, "acme-org", rows[0].TenantID, "token 权威 org 优先于 session.Org")
}

// TestCasdoorCallbackLoginAuditFallsBackToSessionOrg：token 无法解析
// （GetUserInfo 失败）但登录照实放行时，租户回退 session.Org——守卫既有
// 回退行为不被本次修复破坏。
func TestCasdoorCallbackLoginAuditFallsBackToSessionOrg(t *testing.T) {
	key := genCasdoorTestKey(t)
	r, db, state := newCasdoorCallbackAuditEnv(t, key, "not-a-jwt", "beta-org")
	serveCasdoorCallback(t, r, state)

	rows := rowsOf(t, db, audit.ActionLogin)
	require.Len(t, rows, 1)
	require.Equal(t, "beta-org", rows[0].TenantID)
}
