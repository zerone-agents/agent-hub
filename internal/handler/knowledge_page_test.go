package handler

import (
	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKnowledgeCredentialReadsUseWriteAuthorization(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"code":0,"data":[]}`)) }))
	defer upstream.Close()
	router := gin.New()
	read := router.Group("/api/v1/admin")
	write := router.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("X-Test-Role") == "member" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	RegisterKnowledgeRoutes(write, read, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	for _, path := range []string{"/datasets/kb/sources", "/datasets/kb/connectors", "/datasets/kb/sources/c1", "/datasets/kb/sources/c1/logs"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge"+path, nil)
		req.Header.Set("X-Test-Role", "member")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("path=%s status=%d", path, w.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("unauthorized remote calls=%d", calls)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/kb/tags", nil)
	req.Header.Set("X-Test-Role", "member")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || calls != 1 {
		t.Errorf("readonly tags status=%d calls=%d", w.Code, calls)
	}
}
