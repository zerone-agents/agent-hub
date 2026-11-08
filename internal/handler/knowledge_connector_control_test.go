package handler

import (
	"bytes"
	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectorResumeKeepsWriteAuthorizationAndBooleanContract(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/connectors/c1/resume" || r.Header.Get("Authorization") != "Bearer owned-key" {
			t.Errorf("unexpected connector request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 {
			t.Errorf("unexpected resume payload: %v %v", body, err)
		}
		if _, ok := body["resume"].(bool); !ok {
			t.Errorf("resume must be a boolean: %v", body)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":true}`))
	}))
	defer upstream.Close()
	router := gin.New()
	read := router.Group("/api/v1/admin")
	write := router.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("X-Test-Role") != "maintainer" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	RegisterKnowledgeRoutes(write, read, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "owned-key", time.Second, time.Hour), nil), nil))
	for _, test := range []struct {
		role, body string
		status     int
		outbound   bool
	}{
		{"maintainer", `{"resume":true}`, 200, true},
		{"maintainer", `{"resume":false}`, 200, true},
		{"maintainer", `{}`, 400, false},
		{"maintainer", `{"resume":null}`, 400, false},
		{"maintainer", `{"resume":"true"}`, 400, false},
		{"maintainer", `{"resume":1}`, 400, false},
		{"member", `{"resume":true}`, 403, false},
		{"", `{"resume":false}`, 403, false},
	} {
		before := calls.Load()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/datasets/kb/sources/c1/resume", bytes.NewBufferString(test.body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Role", test.role)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != test.status || (calls.Load() != before) != test.outbound {
			t.Fatalf("role=%q body=%s status=%d calls=%d", test.role, test.body, response.Code, calls.Load()-before)
		}
		if test.outbound {
			var result struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.Success {
				t.Fatal("connector operation did not return business success", response.Body.String(), err)
			}
		}
	}
}
