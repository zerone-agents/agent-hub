// H7.1 扩展生命周期管理 API 测试：安装/幂等/404/409 事务无残留、
// 启停、升级迁移与回滚、卸载依赖检查与 force、影响范围预览。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/extension"
	rundomain "control-panel/internal/domain/run"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func extensionLifecycleRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
		&rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{},
	))
	svc := services.NewExtensionService(db)
	h := NewExtensionAdminHandlerWithLifecycle(svc, services.NewExtensionLifecycleService(db))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenantID := c.GetHeader("X-Test-Tenant")
		if tenantID == "" {
			tenantID = "tenant-a"
		}
		c.Set("tenant_id", tenantID)
		c.Set("user_id", "admin-1")
		c.Next()
	})
	// 复用与 main.go 一致的路径结构（租户中间件以 c.Set("tenant_id") 形式注入）
	r.POST("/api/v1/admin/extensions", h.Register)
	r.GET("/api/v1/admin/extensions", h.List)
	r.GET("/api/v1/admin/extensions/:id", h.Get)
	r.POST("/api/v1/admin/extensions/:id/install", h.Install)
	r.POST("/api/v1/admin/extensions/:id/enable", h.Enable)
	r.POST("/api/v1/admin/extensions/:id/disable", h.Disable)
	r.POST("/api/v1/admin/extensions/:id/upgrade", h.Upgrade)
	r.POST("/api/v1/admin/extensions/:id/rollback", h.Rollback)
	r.DELETE("/api/v1/admin/extensions/:id/uninstall", h.Uninstall)
	r.GET("/api/v1/admin/extensions/:id/impact", h.Impact)
	return r
}

func lifecycleManifest(name, version string, minimum int, migrations string, deps string) string {
	mig := ""
	if migrations != "" {
		mig = `,"migrations":` + migrations
	}
	dep := ""
	if deps != "" {
		dep = `,"dependencies":` + deps
	}
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": %q,
	  "displayName": "生命周期 handler 测试",
	  "description": "lifecycle handler test",
	  "stateSchemas": [{"name": "intensity", "payload": {"type": "object", "properties": {"intensity": {"type": "integer", "minimum": %d}}}}]%s%s
	}`, name, version, minimum, mig, dep)
}

func registerExtension(t *testing.T, r *gin.Engine, manifest string) uint64 {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(manifest))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var out struct {
		Data struct {
			Extension struct {
				ID uint64 `json:"id"`
			} `json:"extension"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out.Data.Extension.ID
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

func TestExtensionLifecycleInstallFlowAndIdempotency(t *testing.T) {
	r := extensionLifecycleRouter(t)
	id := registerExtension(t, r, lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))

	w := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"idempotent":false`)

	// 重复安装同版本 → 200 + idempotent:true
	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"idempotent":true`)

	// 安装不存在的版本 → 404
	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), `{"version":"9.9.9"}`)
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "不存在")

	// 详情带 installed 状态
	w = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d", id), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"installed":true`)
	require.Contains(t, w.Body.String(), `"installedVersion":"1.0.0"`)

	// 列表带 installed 徽标数据
	w = doJSON(t, r, http.MethodGet, "/api/v1/admin/extensions", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"installed":true`)
}

func TestExtensionLifecycleInstallDependencyMissing409NoResidue(t *testing.T) {
	r := extensionLifecycleRouter(t)
	child := registerExtension(t, r, lifecycleManifest("io.zerone.child", "1.0.0", 0, "",
		`[{"name":"io.zerone.missing","version":">=1.0.0"}]`))

	w := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", child), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "依赖未满足")
	require.Contains(t, w.Body.String(), "io.zerone.missing")

	// 事务无残留：再次直接查询安装应 404
	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/disable", child), "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestExtensionLifecycleUpgradeRollbackViaAPI(t *testing.T) {
	r := extensionLifecycleRouter(t)
	id := registerExtension(t, r, lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	w := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code)

	registerExtension(t, r, lifecycleManifest("io.zerone.life", "2.0.0", 0, `[
	  {"from":"1.0.0","to":"2.0.0","ops":[{"op":"replace","path":"/properties/intensity/minimum","value":-100}]}
	]`, ""))

	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/upgrade", id), `{"target_version":"2.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"version":"2.0.0"`)
	require.Contains(t, w.Body.String(), `"migrationLog"`)

	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/rollback", id), `{}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"version":"1.0.0"`)
}

func TestExtensionLifecycleEnableDisableViaAPI(t *testing.T) {
	r := extensionLifecycleRouter(t)
	id := registerExtension(t, r, lifecycleManifest("io.zerone.life", "1.0.0", 0, "", ""))
	w := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", id), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code)

	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/disable", id), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"disabled"`)

	// 停用后再启用恢复（幂等）
	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/enable", id), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"enabled"`)

	// 未安装扩展不能停用 → 404
	w = doJSON(t, r, http.MethodPost, "/api/v1/admin/extensions/9999/disable", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestExtensionLifecycleUninstallAndImpactViaAPI(t *testing.T) {
	r := extensionLifecycleRouter(t)
	base := registerExtension(t, r, lifecycleManifest("io.zerone.base", "1.0.0", 0, "", ""))
	w := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", base), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code)
	child := registerExtension(t, r, lifecycleManifest("io.zerone.child", "1.0.0", 0, "",
		`[{"name":"io.zerone.base","version":"1.0.0"}]`))
	w = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/v1/admin/extensions/%d/install", child), `{"version":"1.0.0"}`)
	require.Equal(t, http.StatusOK, w.Code)

	// 影响范围预览：dependents 列出依赖方，dependencies 列出它依赖谁
	w = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d/impact", base), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"io.zerone.child"`)
	require.Contains(t, w.Body.String(), `"installed":true`)

	// 默认卸载 → 409 列出依赖方
	w = doJSON(t, r, http.MethodDelete, fmt.Sprintf("/api/v1/admin/extensions/%d/uninstall", base), "")
	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "io.zerone.child")

	// force 卸载成功
	w = doJSON(t, r, http.MethodDelete, fmt.Sprintf("/api/v1/admin/extensions/%d/uninstall?force=true", base), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"dependentsDisabled":["io.zerone.child"]`)

	// 卸载后详情 installed=false
	w = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/v1/admin/extensions/%d", base), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"installed":false`)
}
