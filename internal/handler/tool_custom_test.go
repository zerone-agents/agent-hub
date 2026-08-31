package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"
	"control-panel/pkg/oss"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type toolUploaderMock struct{ data map[string][]byte }

func (m *toolUploaderMock) Upload(_ context.Context, key string, r io.Reader, _ int64) (string, error) {
	b, _ := io.ReadAll(r)
	m.data[key] = b
	return "fake-hash", nil
}
func (m *toolUploaderMock) GetPresignedURL(_ context.Context, key string) (string, error) {
	return "https://oss.example.com/" + key, nil
}
func (m *toolUploaderMock) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}
func (m *toolUploaderMock) Download(_ context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

// failingUploaderMock 仅 Upload 失败（OSS 故障注入），其余方法无害实现。
type failingUploaderMock struct{}

func (m *failingUploaderMock) Upload(_ context.Context, _ string, _ io.Reader, _ int64) (string, error) {
	return "", fmt.Errorf("forced oss upload failure")
}
func (m *failingUploaderMock) GetPresignedURL(_ context.Context, key string) (string, error) {
	return "https://oss.example.com/" + key, nil
}
func (m *failingUploaderMock) Delete(_ context.Context, _ string) error { return nil }
func (m *failingUploaderMock) Download(_ context.Context, key string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented: %s", key)
}

func setupToolHandlerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	return setupToolHandlerRouterWith(t, &toolUploaderMock{data: map[string][]byte{}})
}

// setupToolHandlerRouterWith 支持注入自定义 uploader（nil=存储未配置、
// failingUploaderMock=OSS 故障），供 5xx 语义回归测试使用；路由对齐
// cmd/server/main.go 的 Tool 领域注册（含 GET /:name），并注册挂载在
// /agents/:name/tools 的 UpdateAgentTools/GetAgentTools 两条。
func setupToolHandlerRouterWith(t *testing.T, uploader oss.OSSUploader) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	h := NewToolHandler(services.NewToolService(uploader))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 生产租户来自 JWT 中间件 c.Set("tenant_id")（chat_handler_test 同款注入）
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	r.POST("/api/v1/admin/tools", h.Create)
	r.GET("/api/v1/admin/tools/:name", h.Get)
	r.PUT("/api/v1/admin/tools/:name", h.Update)
	r.PUT("/api/v1/admin/tools/:name/file", h.UploadFile)
	r.GET("/api/v1/admin/tools/:name/download", h.Download)
	r.DELETE("/api/v1/admin/tools/:name", h.Delete)
	r.PUT("/api/v1/admin/agents/:name/tools", h.UpdateAgentTools)
	r.GET("/api/v1/admin/agents/:name/tools", h.GetAgentTools)
	return r
}

func multipartToolRequest(t *testing.T, fields map[string]string, fileField, fileName, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		require.NoError(t, w.WriteField(k, v))
	}
	if fileField != "" {
		fw, err := w.CreateFormFile(fileField, fileName)
		require.NoError(t, err)
		_, _ = fw.Write([]byte(content))
	}
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestToolHandler_CreateMultipart(t *testing.T) {
	r := setupToolHandlerRouter(t)
	req := multipartToolRequest(t, map[string]string{"name": "SayHello", "title": "问候"}, "file", "say.ts", "export default { name: 'SayHello' }")
	req.URL.Path = "/api/v1/admin/tools"
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusCreated, resp.Code)
	require.Contains(t, resp.Body.String(), "\"source\":\"custom\"")
	require.Contains(t, resp.Body.String(), "\"artifactStatus\":\"ready\"")
}

func TestToolHandler_CreateWithoutFile(t *testing.T) {
	r := setupToolHandlerRouter(t)
	req := multipartToolRequest(t, map[string]string{"name": "X"}, "", "", "")
	req.URL.Path = "/api/v1/admin/tools"
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestToolHandler_UploadFileAndDownloadMissing(t *testing.T) {
	r := setupToolHandlerRouter(t)
	// missing 存量行
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Legacy", TenantID: "tenant-a", Source: agent.ToolSourceCustom}).Error)

	// download missing → 400
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/admin/tools/Legacy/download", nil))
	require.Equal(t, http.StatusBadRequest, resp.Code)

	// 补传 → ready
	req := multipartToolRequest(t, nil, "file", "l.ts", "export default { name: 'Legacy' }")
	req.Method = http.MethodPut
	req.URL.Path = "/api/v1/admin/tools/Legacy/file"
	resp2 := httptest.NewRecorder()
	r.ServeHTTP(resp2, req)
	require.Equal(t, http.StatusOK, resp2.Code)

	resp3 := httptest.NewRecorder()
	r.ServeHTTP(resp3, httptest.NewRequest(http.MethodGet, "/api/v1/admin/tools/Legacy/download", nil))
	require.Equal(t, http.StatusOK, resp3.Code)
	require.Contains(t, resp3.Body.String(), "https://oss.example.com/tools/tenant-a/Legacy/")
}

func TestToolHandler_DeleteConflict409(t *testing.T) {
	r := setupToolHandlerRouter(t)
	tool := &agent.Tool{Name: "SayHello", TenantID: "tenant-a", Source: agent.ToolSourceCustom}
	require.NoError(t, database.GetDB().Create(tool).Error)
	a := &agent.AgentConfig{Name: "bot", TenantID: "tenant-a", ContentHash: "h", SystemPrompt: "p"}
	require.NoError(t, database.GetDB().Create(a).Error)
	require.NoError(t, database.GetDB().Create(&agent.AgentTool{AgentID: a.ID, ToolID: tool.ID}).Error)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tools/SayHello", nil))
	require.Equal(t, http.StatusConflict, resp.Code)
	require.Contains(t, resp.Body.String(), "\"agents\":[\"bot\"]")
}

func TestToolHandler_UpdateBuiltinRejected(t *testing.T) {
	r := setupToolHandlerRouter(t)
	require.NoError(t, database.GetDB().Create(&agent.Tool{Name: "Bash", TenantID: "", Source: agent.ToolSourceBuiltin}).Error)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/tools/Bash", bytes.NewBufferString(`{"title":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

// ---------- expert review round 3：基础设施故障的 5xx 语义回归 ----------

// TestToolHandler_CreateUploadFailure500 锁定 OSS 故障 → 500 中性文案，
// 英文诊断（service 层 "upload tool file failed" 包装）只进服务端日志，
// 不得外泄到响应体。
func TestToolHandler_CreateUploadFailure500(t *testing.T) {
	r := setupToolHandlerRouterWith(t, &failingUploaderMock{})
	req := multipartToolRequest(t, map[string]string{"name": "SayHello", "title": "x"}, "file", "s.ts", "export default {}")
	req.URL.Path = "/api/v1/admin/tools"
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	body := resp.Body.String()
	require.Contains(t, body, "服务器内部错误")
	require.NotContains(t, body, "upload tool file failed")
}

// TestToolHandler_CreateStorageDisabled503 锁定存储未配置（nil uploader）→
// 503 可行动的配置提示，而非 400/500。
func TestToolHandler_CreateStorageDisabled503(t *testing.T) {
	r := setupToolHandlerRouterWith(t, nil)
	req := multipartToolRequest(t, map[string]string{"name": "SayHello"}, "file", "s.ts", "export default {}")
	req.URL.Path = "/api/v1/admin/tools"
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusServiceUnavailable, resp.Code)
	require.Contains(t, resp.Body.String(), "文件存储未配置")
}

// TestToolHandler_GetDBFailure500 锁定 Get 的 DB 故障 → 500（不再一律伪装
// 404）。经 GORM Query 回调注入强制 tools 表查询失败（模式同 service 侧
// Delete/Update 注入）。
func TestToolHandler_GetDBFailure500(t *testing.T) {
	r := setupToolHandlerRouter(t)
	db := database.GetDB()
	forced := errors.New("forced tools query failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:force_tools_query_fail", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "tools" {
			_ = tx.AddError(forced)
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("test:force_tools_query_fail")
	})

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/admin/tools/SayHello", nil))
	require.Equal(t, http.StatusInternalServerError, resp.Code)
	require.Contains(t, resp.Body.String(), "服务器内部错误")
	require.NotContains(t, resp.Body.String(), "query tool failed")
}

// TestToolHandler_CreateInvalidName400 锁定非法工具名（deployer 契约拒绝
// "."）仍落 400 桶——经 ErrInvalidToolName sentinel 映射，且错误文案可读
// （断言经 JSON 解码，规避转义引号）。
func TestToolHandler_CreateInvalidName400(t *testing.T) {
	r := setupToolHandlerRouter(t)
	req := multipartToolRequest(t, map[string]string{"name": "."}, "file", "s.ts", "export default {}")
	req.URL.Path = "/api/v1/admin/tools"
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusBadRequest, resp.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Contains(t, body.Error, "Tool 标识无效")
	require.Contains(t, body.Error, `不能为 "." 或 ".."`)
}

// TestToolHandler_GetMissing404 回归守卫：不存在的工具 → 404 语义不变。
func TestToolHandler_GetMissing404(t *testing.T) {
	r := setupToolHandlerRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/admin/tools/NoSuchTool", nil))
	require.Equal(t, http.StatusNotFound, resp.Code)
	require.Contains(t, resp.Body.String(), "Tool 不存在")
}

// TestToolHandler_AgentToolsAgentMissing404 回归守卫：不存在的 Agent → 404 且
// 文案携带 Agent 名。此前 service 层一律包成 "Agent '%s' 不存在"（非 sentinel），
// UpdateAgentTools 落 500 桶、GetAgentTools 直写 500，均非 not-found 语义。
func TestToolHandler_AgentToolsAgentMissing404(t *testing.T) {
	r := setupToolHandlerRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/agents/NoSuchAgent/tools", bytes.NewBufferString(`{"toolNames":["Bash"]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	require.Equal(t, http.StatusNotFound, resp.Code)
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	require.Contains(t, body.Error, "Agent 不存在")
	require.Contains(t, body.Error, "NoSuchAgent")

	// GET 同契约（该 handler 此前绕过 respondToolError 直写 500）
	resp2 := httptest.NewRecorder()
	r.ServeHTTP(resp2, httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/NoSuchAgent/tools", nil))
	require.Equal(t, http.StatusNotFound, resp2.Code)
}
