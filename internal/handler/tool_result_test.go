package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	rundomain "control-panel/internal/domain/run"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newToolResultTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&rundomain.Run{}, &rundomain.ToolResultRecord{}))
	run := rundomain.Run{ID: "run-1", TenantID: "tenant-a", Name: "test", Status: rundomain.StatusDraft}
	require.NoError(t, db.Create(&run).Error)
	h := NewToolResultHandler(services.NewToolResultService(db))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", "tenant-a") })
	router.GET("/runs/:id/tool-results", h.List)
	router.GET("/runs/:id/tool-results/:toolResultId", h.Get)
	router.POST("/runs/:id/tool-results/reject", h.Reject)
	router.POST("/runs/:id/tool-results/validate", h.Validate)
	return router, db, run.ID
}

func TestToolResultHandlerRejectListAndTenantScopedDetail(t *testing.T) {
	router, db, runID := newToolResultTestRouter(t)
	body := []byte(`{"toolName":"publish.report","idempotencyKey":"approval-1","reason":"review denied","toolResult":{"result":{"title":"draft"},"cost":{},"latencyMs":12}}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/runs/"+runID+"/tool-results/reject", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/tool-results", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"rejected"`)

	var saved rundomain.ToolResultRecord
	require.NoError(t, db.Where("tenant_id=?", "tenant-a").First(&saved).Error)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/tool-results/"+saved.ID, nil))
	require.Equal(t, http.StatusOK, w.Code)

	saved.ID = "other-result"
	saved.TenantID = "tenant-b"
	saved.IdempotencyKey = "other-key"
	require.NoError(t, db.Create(&saved).Error)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runs/"+runID+"/tool-results/other-result", nil))
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestToolResultHandlerValidateReportsClearErrors(t *testing.T) {
	router, _, runID := newToolResultTestRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/runs/"+runID+"/tool-results/validate", bytes.NewBufferString(`{"toolResult":{"cost":{"inputTokens":-1}}}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "non-negative")
}
