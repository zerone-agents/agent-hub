package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeIngestAcknowledgementAndValidation(t *testing.T) {
	calls := 0
	response := `{"code":0,"data":true}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/v1/documents/ingest", r.URL.Path)
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]any{"doc_ids": []any{"d", "d2"}, "run": "1", "delete": true, "apply_kb": true}, body)
		_, _ = w.Write([]byte(response))
	}))
	defer upstream.Close()
	router := gin.New()
	read := router.Group("/api/v1/admin")
	write := router.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("X-Test-Role") == "member" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	RegisterKnowledgeRoutes(write, read, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	call := func(body, role string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/admin/knowledge/documents/ingest", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		router.ServeHTTP(w, req)
		return w
	}
	body := `{"doc_ids":["d","d2","d"],"run":1,"delete":true,"apply_kb":true}`
	w := call(body, "admin")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.JSONEq(t, `{"success":true,"data":true}`, w.Body.String())
	for _, receipt := range []string{
		`{"code":0,"data":false}`, `{"code":0,"data":null}`, `{"code":0}`, `{"code":0,"data":{}}`,
		`{"code":0,"data":"true"}`, `{"code":0,"data":1}`, `{"code":100,"data":true}`, `{"data":true}`,
	} {
		response = receipt
		w = call(body, "admin")
		require.Equal(t, 502, w.Code, receipt+" "+w.Body.String())
	}
	before := calls
	w = call(body, "member")
	require.Equal(t, 403, w.Code)
	w = call(`{"doc_ids":["d"],"run":2,"apply_kb":true}`, "admin")
	require.Equal(t, 400, w.Code)
	w = call(`{"doc_ids":["d"],"run":1,"force":true}`, "admin")
	require.Equal(t, 400, w.Code)
	w = call(body+` {}`, "admin")
	require.Equal(t, 400, w.Code)
	require.Equal(t, before, calls)
}
