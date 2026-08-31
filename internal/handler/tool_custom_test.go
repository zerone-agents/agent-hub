package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"
	"control-panel/pkg/database"

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

func setupToolHandlerRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&agent.Tool{}, &agent.AgentConfig{}, &agent.AgentTool{}))
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	h := NewToolHandler(services.NewToolService(&toolUploaderMock{data: map[string][]byte{}}, ""))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 生产租户来自 JWT 中间件 c.Set("tenant_id")（chat_handler_test 同款注入）
	r.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	r.POST("/api/v1/admin/tools", h.Create)
	r.PUT("/api/v1/admin/tools/:name", h.Update)
	r.PUT("/api/v1/admin/tools/:name/file", h.UploadFile)
	r.GET("/api/v1/admin/tools/:name/download", h.Download)
	r.DELETE("/api/v1/admin/tools/:name", h.Delete)
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
