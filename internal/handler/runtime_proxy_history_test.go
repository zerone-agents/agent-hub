package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeProxyHistoryContractPreservesAuthenticationAndPayload(t *testing.T) {
	for _, path := range []string{"messages", "rollback", "revert"} {
		t.Run(path, func(t *testing.T) {
			f := newFakeRuntime(t)
			router := newProxyEngine(portOf(f.srv.URL))
			body := `{"operationId":"op1","expectedRevision":7,"messages":[{"messageId":"m1","role":"assistant","content":"actual reply"}]}`
			req := httptest.NewRequest(http.MethodPost, "/runtime/default/test/v1/sessions/s1/"+path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-api-key", "runtime-only-key")
			req.Header.Set("X-User-Name", "java-user")
			req.Header.Set("Authorization", "Bearer hub-credential")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != http.StatusOK || f.lastPath != "/v1/sessions/s1/"+path || string(f.lastBody) != body {
				t.Fatal("history contract payload was not passed through")
			}
			if f.lastHeader.Get("x-api-key") != "runtime-only-key" || f.lastHeader.Get("X-User-Name") != "java-user" || f.lastHeader.Get("Authorization") != "" {
				t.Fatal("runtime credential policy changed")
			}
		})
	}
}
