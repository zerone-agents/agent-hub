// H7.4 路由层集成测试（外部测试包，避开 middleware→services 的依赖方向）：
// 真实 ExtensionAuthz 中间件 + 真实 services.ExtensionAuthzService（内存
// SQLite，经生命周期服务同步授权行）挂到模拟 admin 路由上，验证：
//  - 带 X-Extension-Name 头但无授权 → 403（默认拒绝）；
//  - 不带头 → 200，既有请求行为零变化；
//  - manifest 声明并已安装同步的权限 → 放行（正常安装的扩展不受影响）。
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

func (f *authzRouteFixture) registerAndInstall(t *testing.T, tenantID, manifest string) *extension.Extension {
	t.Helper()
	res, err := f.registry.Register(tenantID, services.RegisterExtensionInput{Manifest: []byte(manifest), Source: extension.SourceUpload})
	require.NoError(t, err)
	_, err = f.lifecycle.Install(tenantID, res.Extension.ID, res.Version.Version, "admin-1")
	require.NoError(t, err)
	return res.Extension
}

func newAuthzRouteRouter(authz *services.ExtensionAuthzService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Next()
	})
	// 模拟 cmd/server/main.go 中 admin 路由的挂载方式。
	r.GET("/api/v1/admin/runs",
		middleware.ExtensionAuthz(authz, "run", "read", nil),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/api/v1/admin/runs/:id",
		middleware.ExtensionAuthz(authz, "run", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	// state 读路由：manifest 白名单内的权限类，验证已授权扩展可放行。
	r.GET("/api/v1/admin/runs/:id/persona-state",
		middleware.ExtensionAuthz(authz, "state", "read", func(c *gin.Context) string { return c.Param("id") }),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func doRouteRequest(t *testing.T, r *gin.Engine, path, extName string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if extName != "" {
		req.Header.Set(middleware.ExtensionHeaderName, extName)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExtensionAuthzRouteNoHeaderUnchanged(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.authz)
	w := doRouteRequest(t, r, "/api/v1/admin/runs", "")
	require.Equal(t, http.StatusOK, w.Code)
}

func TestExtensionAuthzRouteNoGrantsDenied403(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.authz)
	// 扩展已注册安装但 manifest 未声明 run 权限：grants 无 run 记录 → 默认拒绝。
	ext := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.no-run-grant", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read","write"]}]`))
	w := doRouteRequest(t, r, "/api/v1/admin/runs/42", ext.Name)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "扩展未被授予该权限")

	// 完全不存在的扩展同样默认拒绝。
	w = doRouteRequest(t, r, "/api/v1/admin/runs", "io.zerone.never-installed")
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestExtensionAuthzRouteGrantedExtensionPasses(t *testing.T) {
	f := newAuthzRouteFixture(t)
	r := newAuthzRouteRouter(f.authz)
	// 注意：manifest 权限白名单（H7.0 十类）不含 "run"，run 类端点对扩展
	// 请求恒为默认拒绝；白名单内的 state/read 验证「已授权即放行」链路。
	ext := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-reader", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	w := doRouteRequest(t, r, "/api/v1/admin/runs/42/persona-state", ext.Name)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestEnforceDefaultDenyUnknownExtension(t *testing.T) {
	f := newAuthzRouteFixture(t)
	// 未知扩展、无任何 grants 记录：任何权限类/动作一律拒绝（白名单语义）。
	for _, perm := range []string{"run", "state", "agent", "workflow"} {
		require.False(t, f.authz.Enforce("tenant-a", "io.zerone.unknown", perm, "read", "", "10.0.0.1"),
			fmt.Sprintf("permission %s 应对未知扩展默认拒绝", perm))
	}
	// 已安装但 manifest 未声明该权限类的扩展同样拒绝。
	ext := f.registerAndInstall(t, "tenant-a", authzRouteManifest("io.zerone.state-only", "1.0.0",
		`[{"permission":"state","scope":"mood","actions":["read"]}]`))
	require.False(t, f.authz.Enforce("tenant-a", ext.Name, "run", "read", "", "10.0.0.1"))
	var auditCount int64
	require.NoError(t, f.db.Model(&extension.AccessAudit{}).Where("allowed=?", false).Count(&auditCount).Error)
	require.Greater(t, auditCount, int64(0))
}
