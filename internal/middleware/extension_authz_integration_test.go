// H7.4 路由层集成测试（外部测试包，避开 middleware→services 的依赖方向）：
// 真实 ExtensionIdentity（身份认证）+ 真实 ExtensionAuthz（授权）+ 真实
// services（内存 SQLite，经生命周期服务签发凭据与同步授权行）挂到模拟
// admin 路由上，验证生产接线下的完整语义：
//   - 不带身份头 → 200，既有请求行为零变化；
//   - 只带名字不带凭据 / 名字不存在 → 401（身份认证先于授权，fail-closed）；
//   - 真实凭据 + manifest 未声明该权限类 → 403（默认拒绝）；
//   - 真实凭据 + manifest 声明并已同步 → 放行；
//   - 轮换凭据后旧 token 立即 401；停用扩展后有效 token 也 401。
package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type authzRouteFixture struct {
	db        *gorm.DB
	registry  *services.ExtensionService
	lifecycle *services.ExtensionLifecycleService
	authz     *services.ExtensionAuthzService
	identity  *services.ExtensionIdentityService
}

func newAuthzRouteFixture(t *testing.T) *authzRouteFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
		&extension.Grant{}, &extension.AccessAudit{},
	))
	f := &authzRouteFixture{
		db:        db,
		registry:  services.NewExtensionService(db),
		lifecycle: services.NewExtensionLifecycleService(db),
		authz:     services.NewExtensionAuthzService(db),
		identity:  services.NewExtensionIdentityService(db),
	}
	f.lifecycle.SetAuthzService(f.authz)
	return f
}

func authzRouteManifest(name, version, permissions string) string {
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "路由集成测试扩展",
	  "description": "route integration test",
	  "permissions": %s
	}`, name, version, permissions)
}

// registerAndInstall 注册并安装扩展，返回扩展行与安装时签发的一次性凭据明文。
func (f *authzRouteFixture) registerAndInstall(t *testing.T, tenantID, manifest string) (*extension.Extension, string) {
	t.Helper()
	res, err := f.registry.Register(tenantID, services.RegisterExtensionInput{Manifest: []byte(manifest), Source: extension.SourceUpload})
	require.NoError(t, err)
	installed, err := f.lifecycle.Install(tenantID, res.Extension.ID, res.Version.Version, "admin-1")
	require.NoError(t, err)
	require.NotEmpty(t, installed.ExtensionToken, "安装必须签发扩展身份凭据")
	return res.Extension, installed.ExtensionToken
}

func newAuthzRouteRouter(identity *services.ExtensionIdentityService, authz *services.ExtensionAuthzService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	})
	// 模拟 cmd/server/main.go 的接线：v1group 上先挂身份认证，路由上挂授权。
	group := r.Group("/api/v1", middleware.ExtensionIdentity(identity))
	group.GET("/admin/runs",
		middleware.ExtensionAuthz(authz, "run", "read", nil),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	group.GET("/admin/runs/:id",
		middleware.ExtensionAuthz(authz, "run", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	group.GET("/admin/runs/:id/persona-state",
		middleware.ExtensionAuthz(authz, "state", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func doRouteRequest(t *testing.T, r *gin.Engine, path, extName, extToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if extName != "" {
		req.Header.Set(middleware.ExtensionHeaderName, extName)
	}
	if extToken != "" {
		req.Header.Set(middleware.ExtensionTokenHeaderName, extToken)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExtensionAuthzRouteNoHeaderUnchanged(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	w := doRouteRequest(t, r, "/api/v1/admin/runs", "", "")
	require.Equal(t, http.StatusOK, w.Code)
}

// 回归 P0：自报的名字不再能走到授权判定。未安装的扩展名（无凭据可签）
// 直接 401，而不是"拒绝授权"——说明身份这一层真的挡在前面了。
func TestExtensionAuthzRouteSelfReportedNameRejectedBeforeAuthz(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)

	w := doRouteRequest(t, r, "/api/v1/admin/runs", "io.zerone.never-installed", "")
	require.Equal(t, http.StatusUnauthorized, w.Code, "只带名字不构成身份")
	require.Contains(t, w.Body.String(), "扩展身份不完整")

	w = doRouteRequest(t, r, "/api/v1/admin/runs", "io.zerone.never-installed", "zx1.deadbeef.guess")
	require.Equal(t, http.StatusUnauthorized, w.Code, "未安装扩展没有凭据，任何 token 都不成立")
	require.Contains(t, w.Body.String(), "扩展身份校验失败")
}

func TestExtensionAuthzRouteNoGrantsDenied403(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	// 扩展已注册安装但 manifest 未声明 run 权限：grants 无 run 记录 → 默认拒绝。
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.no-run-grant", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read","write"]}]`))
	w := doRouteRequest(t, r, "/api/v1/admin/runs/42", ext.Name, token)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "扩展未被授予该权限")
}

func TestExtensionAuthzRouteGrantedExtensionPasses(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	// manifest 白名单内的 state/read：已授权即放行。
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-reader", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	w := doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, token)
	require.Equal(t, http.StatusOK, w.Code)
}

// "run" 已补进 manifest 权限白名单（此前该授权检查点用了白名单外的权限类，
// 导致 run 端点对任何扩展恒 403 —— 合法扩展也进不来）。
func TestExtensionAuthzRouteRunGrantDeclarable(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.run-reader", "1.0.0",
		`[{"permission":"run","scope":"runs","actions":["read"]}]`))
	w := doRouteRequest(t, r, "/api/v1/admin/runs", ext.Name, token)
	require.Equal(t, http.StatusOK, w.Code, "声明 run/read 并已同步 grants 的扩展必须能读 run 列表")
	// 只给了 read，写方法不覆盖（挂载表里 POST /runs 用 run/write）。
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "run", "write", "", "10.0.0.1"))
}

func TestExtensionIdentityRotationInvalidatesOldToken(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-reader", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	require.Equal(t, http.StatusOK, doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, token).Code)

	rotated, err := f.identity.IssueCredential("tenant-a", ext.ID)
	require.NoError(t, err)
	require.NotEqual(t, token, rotated.Token)
	require.Equal(t, http.StatusUnauthorized,
		doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, token).Code,
		"轮换后旧凭据必须立即失效")
	require.Equal(t, http.StatusOK,
		doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, rotated.Token).Code)
}

func TestExtensionIdentityDisableInvalidatesToken(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.identity, f.authz)
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-reader", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	require.Equal(t, http.StatusOK, doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, token).Code)

	_, err := f.lifecycle.Disable("tenant-a", ext.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized,
		doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name, token).Code,
		"停用扩展后其身份凭据必须立即失效")
}

// 凭据只在明文签发一次：库里只存摘要，且摘要不等于明文。
func TestExtensionIdentityStoresOnlyHash(t *testing.T) {
	f := newAuthzRouteFixture(t)
	ext, token := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-reader", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	var inst extension.Install
	require.NoError(t, f.db.Where("tenant_id=? AND extension_id=?", "tenant-a", ext.ID).First(&inst).Error)
	require.NotEmpty(t, inst.AuthTokenID)
	require.NotEmpty(t, inst.AuthTokenHash)
	require.NotEqual(t, token, inst.AuthTokenHash)
	require.NotContains(t, inst.AuthTokenHash, token)
	require.Contains(t, token, inst.AuthTokenID)

	// 描述接口只暴露公开信息。
	info, err := f.identity.DescribeCredential("tenant-a", ext.ID)
	require.NoError(t, err)
	require.True(t, info.Issued)
	require.Equal(t, inst.AuthTokenID, info.TokenID)
}

func TestEnforceDefaultDenyUnknownExtension(t *testing.T) {
	f := newAuthzRouteFixture(t)
	// 未知扩展、无任何 grants 记录：任何权限类/动作一律拒绝（白名单语义）。
	for _, perm := range []string{"run", "state", "agent", "workflow"} {
		require.False(t, f.authz.Enforce("tenant-a", "io.zerone.unknown", perm, "read", "", "10.0.0.1"),
			fmt.Sprintf("permission %s 应对未知扩展默认拒绝", perm))
	}
	// 已安装但 manifest 未声明该权限类的扩展同样拒绝。
	ext, _ := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-only", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "run", "read", "", "10.0.0.1"))
	var auditCount int64
	require.NoError(t, f.db.Model(&extension.AccessAudit{}).Where("allowed=?", false).Count(&auditCount).Error)
	require.Greater(t, auditCount, int64(0))
}
