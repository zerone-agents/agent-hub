// H7.5 UsageAdminHandler 测试：统计 / 预算 / 健康 / 导出端点的端到端行为、
// 租户隔离与参数校验。
package handler

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/systemsetting"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/domain/usage"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func usageAdminRouter(t *testing.T, tenantID string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:usage_admin_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&usage.UsageRecord{}, &usage.UsageBudget{}, &usage.UsageAlert{}, &usage.UsageAlertEvent{}, &systemsetting.SystemSetting{}, &extension.Extension{}))
	svc := services.NewUsageService(db)
	t.Cleanup(svc.Close)
	h := NewUsageAdminHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		tenant.SetTenantID(c, tenantID)
		c.Set("user_id", "admin-1")
		c.Next()
	})
	// 埋点直接落库（跳过 channel）：测试统计正确性。
	r.POST("/seed", func(c *gin.Context) {
		var rec usage.UsageRecord
		if err := c.ShouldBindJSON(&rec); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
		rec.TenantID = tenantID
		if err := db.Create(&rec).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		respondMessage(c, http.StatusOK, "seeded")
	})
	r.GET("/usage/summary", h.Summary)
	r.GET("/usage/by-extension", h.ByExtension)
	r.GET("/usage/by-agent", h.ByAgent)
	r.GET("/usage/by-run", h.ByRun)
	r.GET("/usage/by-model", h.ByModel)
	r.GET("/usage/errors", h.Errors)
	r.GET("/usage/storage", h.Storage)
	r.GET("/usage/health", h.Health)
	r.GET("/usage/export", h.Export)
	r.GET("/usage/budgets", h.ListBudgets)
	r.PUT("/usage/budgets", h.UpsertBudget)
	r.DELETE("/usage/budgets/:id", h.DeleteBudget)
	r.GET("/usage/alerts", h.ListAlerts)
	r.PUT("/usage/alerts", h.UpsertAlert)
	r.DELETE("/usage/alerts/:id", h.DeleteAlert)
	r.GET("/usage/alerts/events", h.AlertEvents)
	r.GET("/usage/pricing", h.GetPricing)
	r.PUT("/usage/pricing", h.PutPricing)
	return r
}

func seedUsage(t *testing.T, r *gin.Engine, body string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/seed", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestUsageAdminSummaryAndTrends(t *testing.T) {
	r := usageAdminRouter(t, "ta")
	day := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	seedUsage(t, r, `{"kind":"model_call","model":"m1","tokensIn":100,"tokensOut":50,"createdAt":"2026-09-10T10:00:00Z"}`)
	seedUsage(t, r, `{"kind":"model_call","model":"m1","error":"boom","createdAt":"2026-09-10T11:00:00Z"}`)
	seedUsage(t, r, `{"kind":"message","createdAt":"2026-09-11T09:00:00Z"}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/usage/summary?from=2026-09-10&to=2026-09-11", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"model_call"`)
	require.Contains(t, w.Body.String(), `"totalCalls":2`)
	require.Contains(t, w.Body.String(), `"totalTokens":150`)

	// 缺省范围（近 7 天）也覆盖这两天。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/usage/summary", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"trends"`)

	// 非法 from → 400。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/usage/summary?from=not-a-date", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	_ = day
}

func TestUsageAdminDimensionsAndErrors(t *testing.T) {
	r := usageAdminRouter(t, "ta")
	now := time.Now().UTC()
	seedUsage(t, r, `{"kind":"extension_call","extensionName":"ext.a","createdAt":"`+now.Add(-time.Hour).Format(time.RFC3339)+`"}`)
	// Keep the error inside the default seven-day report window but outside
	// today's health window, so this test can independently assert both views.
	seedUsage(t, r, `{"kind":"model_call","model":"m1","error":"oops","createdAt":"`+now.Add(-25*time.Hour).Format(time.RFC3339)+`"}`)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/by-extension", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"ext.a"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/by-model", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"m1"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/errors", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"oops"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/storage", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"usage_records"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/health", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"green"`)
}

func TestUsageAdminBudgetCRUDAndProgress(t *testing.T) {
	r := usageAdminRouter(t, "ta")
	seedUsage(t, r, `{"kind":"model_call","createdAt":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)

	body := `{"kind":"model_call","limitValue":10,"period":"daily","alertThresholdPct":80}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/usage/budgets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/budgets", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"usedPct":10`)

	// 非法 kind → 400。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/usage/budgets", strings.NewReader(`{"kind":"bad","limitValue":1,"period":"daily"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	// 删除。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/usage/budgets/1", nil))
	require.Equal(t, http.StatusOK, w.Code)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/usage/budgets/1", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestUsageAdminExportCSV(t *testing.T) {
	r := usageAdminRouter(t, "ta")
	seedUsage(t, r, `{"kind":"model_call","model":"m1","tokensIn":5,"error":"e1","createdAt":"2026-09-10T10:00:00Z"}`)
	seedUsage(t, r, `{"kind":"message","createdAt":"2026-09-10T11:00:00Z"}`)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/export?from=2026-09-10&to=2026-09-10", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")))
	records, err := reader.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3) // 表头 + 2 行
	require.Equal(t, "id", records[0][0])
	require.Equal(t, "kind", records[0][1])

	// kind 过滤：只导 model_call。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/export?from=2026-09-10&to=2026-09-10&kind=model_call", nil))
	reader = csv.NewReader(strings.NewReader(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")))
	records, err = reader.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.Equal(t, "model_call", records[1][1])
}

func TestUsageAdminAlertsAndPricing(t *testing.T) {
	r := usageAdminRouter(t, "ta")
	body := `{"rule":"error_rate_spike","channel":"log","threshold":5}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/usage/alerts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/alerts", nil))
	require.Contains(t, w.Body.String(), `"error_rate_spike"`)

	// webhook 缺 URL → 400。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/usage/alerts", strings.NewReader(`{"rule":"budget_pct","channel":"webhook"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/alerts/events", nil))
	require.Equal(t, http.StatusOK, w.Code)

	// 单价配置读写。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/usage/pricing", strings.NewReader(`{"value":"{\"m1\":{\"input_per_million\":1,\"output_per_million\":2}}"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/pricing", nil))
	require.Contains(t, w.Body.String(), `input_per_million`)

	// 非法 JSON → 400。
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/usage/pricing", strings.NewReader(`{"value":"not-json"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUsageAdminTenantIsolation(t *testing.T) {
	ra := usageAdminRouter(t, "ta")
	rb := usageAdminRouter(t, "tb") // 独立内存库：各自只见自己的数据
	now := time.Now().UTC().Format(time.RFC3339)
	seedUsage(t, ra, `{"kind":"model_call","model":"m-a","createdAt":"`+now+`"}`)
	seedUsage(t, rb, `{"kind":"model_call","model":"m-b","createdAt":"`+now+`"}`)

	w := httptest.NewRecorder()
	ra.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/by-model", nil))
	require.Contains(t, w.Body.String(), `"m-a"`)
	require.NotContains(t, w.Body.String(), `"m-b"`)

	// 预算隔离：ta 建预算，tb 无预算。
	body := `{"kind":"model_call","limitValue":5,"period":"monthly"}`
	req := httptest.NewRequest(http.MethodPut, "/usage/budgets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	ra.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	rb.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/usage/budgets", nil))
	require.NotContains(t, w.Body.String(), `"limitValue"`)
}
