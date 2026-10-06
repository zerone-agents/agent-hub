package handler

import (
	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestKnowledgeDocumentPatchAndPutExposeSafeOutcome(t *testing.T) {
	for _, method := range []string{"PUT", "PATCH"} {
		for _, rejected := range []bool{false, true} {
			t.Run(method+map[bool]string{false: " success", true: " conflict"}[rejected], func(t *testing.T) {
				name := "before.pdf"
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "dataset_id": "kb", "name": name}}}})
						return
					}
					require.Equal(t, "PATCH", r.Method)
					if rejected {
						w.WriteHeader(409)
						_, _ = w.Write([]byte(`{"code":"DOCUMENT_UPDATE_CONFLICT","retcode":102,"message":"private database trace","details":{"outcome":"unchanged"},"request_id":"0123456789abcdef0123456789abcdef"}`))
						return
					}
					name = "after.pdf"
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
				}))
				defer upstream.Close()
				router := gin.New()
				group := router.Group("/api/v1/admin")
				RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
				w := httptest.NewRecorder()
				req := httptest.NewRequest(method, "/api/v1/admin/knowledge/datasets/kb/documents/d", strings.NewReader(`{"name":"after.pdf"}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				if rejected {
					require.Equal(t, 409, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), `"code":"DOCUMENT_UPDATE_CONFLICT"`)
					require.Contains(t, w.Body.String(), `"outcome":"unchanged"`)
					require.NotContains(t, w.Body.String(), "private")
				} else {
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Contains(t, w.Body.String(), `"name":"after.pdf"`)
					require.Contains(t, w.Body.String(), `"success":true`)
				}
			})
		}
	}
}
