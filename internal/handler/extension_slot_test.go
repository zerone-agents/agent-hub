// ExtensionSlotHandler 测试：插槽列表/可见性覆盖端点 + 授权 API 代理
// （正常转发与头注入、未声明 403、超 512KB 截断、上游超时 502）。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/extensionslot"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/extensionmanifest"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type slotTestEnv struct {
	router *gin.Engine
	db     *gorm.DB
}

func extensionSlotRouter(t *testing.T) *slotTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&extension.Extension{}, &extension.Version{}, &extension.Install{}, &extensionslot.Override{}))
	h := NewExtensionSlotHandler(services.NewExtensionSlotService(db))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Set("user_id", "admin-1")
		c.Next()
	})
	r.GET("/api/v1/admin/extensions/slots", h.ListSlots)
	r.POST("/api/v1/admin/extensions/:id/slots/:slot/visible", h.SetSlotVisible)
	r.GET("/api/v1/extensions/:name/*wildcard", h.Proxy)
	return &slotTestEnv{router: r, db: db}
}

// seedSlotExtension 注册带插槽声明的扩展并启用，返回扩展 ID。
func seedSlotExtension(t *testing.T, env *slotTestEnv, manifest string) uint64 {
	t.Helper()
	extSvc := services.NewExtensionService(env.db)
	res, err := extSvc.Register("tenant-a", services.RegisterExtensionInput{Manifest: json.RawMessage(manifest)})
	require.NoError(t, err)
	lifecycle := services.NewExtensionLifecycleService(env.db)
	_, err = lifecycle.Install("tenant-a", res.Extension.ID, res.Version.Version, "admin-1")
	require.NoError(t, err)
	return res.Extension.ID
}

func slotManifestWithAPI(name, upstream string) string {
	return fmt.Sprintf(`{
	  "apiVersion": "agenthub.extension/v1alpha1",
	  "name": %q,
	  "version": "1.0.0",
	  "displayName": "插槽 Handler 测试",
	  "description": "ExtensionSlotHandler 测试用",
	  "ui": {"slots": [
	    {"slot": "dashboard.card", "component": "stat-card", "title": "运行总数", "order": 1,
	     "data": {"value": 7, "description": "今天"}},
	    {"slot": "agent.detail.tab", "component": "link-list", "title": "扩展链接", "order": 2,
	     "data": {"links": [{"label": "文档", "href": "https://example.com"}]}}
	  ]},
	  "apiRoutes": [
	    {"method": "GET", "path": "/api/v1/extensions/%[1]s/metrics", "upstream": %[2]q},
	    {"method": "GET", "path": "/api/v1/extensions/%[1]s/huge", "upstream": %[3]q},
	    {"method": "GET", "path": "/api/v1/extensions/%[1]s/slow", "upstream": %[4]q}
	  ]
	}`, name, upstream+"/v1/metrics", upstream+"/v1/huge", upstream+"/v1/slow")
}

// allowLoopbackUpstreamForTest 临时关闭 upstream 私网地址阻断：
// httptest.NewServer 绑定 127.0.0.1，生产阻断逻辑下无法作为测试上游。
func allowLoopbackUpstreamForTest(t *testing.T) {
	t.Helper()
	extensionmanifest.UpstreamGuardDisabled = true
	t.Cleanup(func() { extensionmanifest.UpstreamGuardDisabled = false })
}

func TestExtensionSlotHandlerListAndVisibleOverride(t *testing.T) {
	allowLoopbackUpstreamForTest(t)
	env := extensionSlotRouter(t)
	extID := seedSlotExtension(t, env, slotManifestWithAPI("io.zerone.slotty", "http://127.0.0.1:1"))

	// 聚合列表：两个插槽各一条
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/slots", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Slot      string `json:"slot"`
				Component string `json:"component"`
				Title     string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.True(t, listResp.Success)
	require.Len(t, listResp.Data.Items, 2)

	// slot 过滤
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/slots?slot=dashboard.card", nil))
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data.Items, 1)
	require.Equal(t, "运行总数", listResp.Data.Items[0].Title)

	// 隐藏 dashboard.card 的 stat-card
	body := `{"component":"stat-card","visible":false}`
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/admin/extensions/%d/slots/dashboard.card/visible", extID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/slots?slot=dashboard.card", nil))
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data.Items, 0)

	// 恢复可见
	body = `{"component":"stat-card","visible":true}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/admin/extensions/%d/slots/dashboard.card/visible", extID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/slots?slot=dashboard.card", nil))
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Data.Items, 1)

	// 非法 component → 400
	body = `{"component":"evil","visible":false}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/admin/extensions/%d/slots/dashboard.card/visible", extID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestExtensionSlotProxy(t *testing.T) {
	allowLoopbackUpstreamForTest(t)
	var gotExt, gotTenant string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotExt = r.Header.Get("X-Extension-Name")
		gotTenant = r.Header.Get("X-Tenant-ID")
		switch r.URL.Path {
		case "/v1/metrics":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"value":7}`))
		case "/v1/huge":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("x", 600*1024)))
		case "/v1/slow":
			time.Sleep(6 * time.Second)
		}
	}))
	defer upstream.Close()

	env := extensionSlotRouter(t)
	seedSlotExtension(t, env, slotManifestWithAPI("io.zerone.slotty", upstream.URL))

	// 正常转发：注入头 + 内容透传
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.slotty/metrics", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"value":7}`, w.Body.String())
	require.Equal(t, "io.zerone.slotty", gotExt)
	require.Equal(t, "tenant-a", gotTenant)

	// 未声明路径 → 403 中文
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.slotty/secrets", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "未声明")

	// 超 512KB → 截断 + 标记头
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.slotty/huge", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 512*1024, w.Body.Len())
	require.Equal(t, "true", w.Header().Get("X-Extension-Truncated"))

	// 上游超时 → 502 中文
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.slotty/slow", nil))
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "扩展服务暂时无法访问")

	// 不存在的扩展 → 403（不泄露内部状态）
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.ghost/x", nil))
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestExtensionSlotProxyRejectsDisallowedUpstream 回归 P1-3 代理层：
// 域名 upstream 通过注册校验后，若解析到环回/内网地址（DNS rebinding），
// 代理转发前必须拒绝。localhost 稳定解析到 127.0.0.1/::1，无需外网。
func TestExtensionSlotProxyRejectsDisallowedUpstream(t *testing.T) {
	env := extensionSlotRouter(t)
	seedSlotExtension(t, env, slotManifestWithAPI("io.zerone.ssrf", "http://localhost:1"))

	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/extensions/io.zerone.ssrf/metrics", nil))
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "拒绝")
}
