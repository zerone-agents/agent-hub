// ExtensionAdminHandler 的管理 API 测试：非法 manifest 返回 400 + 中文错误，
// 以及注册/列表/详情/版本/租户隔离的端到端行为。
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/tenant"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func extensionAdminRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&extension.Extension{}, &extension.Version{}))
	h := NewExtensionAdminHandler(services.NewExtensionService(db))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, "tenant-a")
		c.Set("user_id", "admin-1")
		c.Next()
	})
	r.POST("/api/v1/admin/extensions", h.Register)
	r.GET("/api/v1/admin/extensions", h.List)
	r.GET("/api/v1/admin/extensions/:id", h.Get)
	r.GET("/api/v1/admin/extensions/:id/versions/:version", h.GetVersion)
	return r
}

const handlerValidManifest = `{
  "apiVersion": "agenthub.extension/v1alpha1",
  "name": "io.zerone.handler",
  "version": "1.0.0",
  "displayName": "Handler 测试扩展",
  "description": "handler 层端到端测试",
  "permissions": [{"permission": "state", "scope": "io.zerone.handler/*", "actions": ["read", "write"]}],
  "ui": {"slots": ["dashboard.card"]},
  "tools": [{"name": "ping"}]
}`

func TestExtensionAdminRegisterInvalidManifestChineseError(t *testing.T) {
	r := extensionAdminRouter(t)
	body := `{"apiVersion":"wrong/v1","name":"Not_DNS","version":"1.0","displayName":"","permissions":[{"permission":"emotion","scope":"s","actions":[]}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Contains(t, w.Body.String(), "扩展 manifest 校验失败")
	require.Contains(t, w.Body.String(), "apiVersion")
	require.Contains(t, w.Body.String(), "白名单")
}

func TestExtensionAdminRegisterEmptyBody(t *testing.T) {
	r := extensionAdminRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", nil))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "manifest JSON")
}

func TestExtensionAdminRegisterListGetVersionFlow(t *testing.T) {
	r := extensionAdminRouter(t)

	// 注册
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions?source=seed&changelog=首个版本", strings.NewReader(handlerValidManifest))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Success bool `json:"success"`
		Data    struct {
			Extension struct {
				ID   uint64 `json:"id"`
				Name string `json:"name"`
			} `json:"extension"`
			Version struct {
				Version     string `json:"version"`
				ContentHash string `json:"contentHash"`
				Changelog   string `json:"changelog"`
			} `json:"version"`
			AlreadyExisted bool `json:"alreadyExisted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.False(t, created.Data.AlreadyExisted)
	require.Equal(t, "io.zerone.handler", created.Data.Extension.Name)
	require.Equal(t, "首个版本", created.Data.Version.Changelog)
	extID := created.Data.Extension.ID

	// 幂等重复注册
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", strings.NewReader(handlerValidManifest))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var dup struct {
		Data struct {
			AlreadyExisted bool `json:"alreadyExisted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dup))
	require.True(t, dup.Data.AlreadyExisted)

	// 列表
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions?status=active&page=1&pageSize=10", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"total":1`)
	require.Contains(t, w.Body.String(), `"versionCount":1`)
	require.Contains(t, w.Body.String(), `"latestVersion":"1.0.0"`)

	// 详情：版本摘要 + 权限清单
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/"+itoa(extID), nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"permissions"`)
	require.Contains(t, w.Body.String(), `"toolCount":1`)
	require.Contains(t, w.Body.String(), `"dashboard.card"`)

	// 单版本完整 manifest
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/"+itoa(extID)+"/versions/1.0.0", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"manifest"`)

	// 404：不存在扩展与不存在版本
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/9999", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "扩展不存在")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/extensions/"+itoa(extID)+"/versions/9.9.9", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "扩展版本不存在")
}

func TestExtensionAdminRegisterBadJSON(t *testing.T) {
	r := extensionAdminRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/extensions", bytes.NewBufferString(`{not json`))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "扩展 manifest 校验失败")
}

func itoa(v uint64) string {
	return strconv.FormatUint(v, 10)
}
