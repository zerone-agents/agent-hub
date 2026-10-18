package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIndexMutationRequiresBoundAcknowledgementAndReadback(t *testing.T) {
	for _, tc := range []struct {
		name, operation, expectedID, response, readback string
		wantCalls                                       int
		wantError                                       bool
	}{
		{"submit", "index-run", "", `{"code":0,"data":{"task_id":"current"}}`, `{"code":0,"data":{"id":"current","progress":0}}`, 2, false},
		{"submit-unbound", "index-run", "", `{"code":0,"data":{"task_id":"current"}}`, `{"code":0,"data":{"id":"other","progress":0}}`, 2, true},
		{"submit-empty-ack", "index-run", "", `{"code":0,"data":{}}`, "", 1, true},
		{"resume-writer-draining", "index-run", "", `{"code":102,"message":"Graph writer is still draining."}`, "", 1, true},
		{"clear", "index-delete", "current", `{"code":0,"data":{}}`, `{"code":0,"data":{}}`, 2, false},
		{"clear-unconfirmed", "index-delete", "current", `{"code":0,"data":{}}`, `{"code":0,"data":{"id":"current","progress":1}}`, 2, true},
		{"clear-new-task", "index-delete", "current", `{"code":0,"data":{}}`, `{"code":0,"data":{"id":"other","progress":0}}`, 2, true},
		{"clear-stale", "index-delete", "stale", `{"code":102,"message":"Task binding changed"}`, "", 1, true},
		{"clear-unbound-request", "index-delete", "", "", "", 0, true},
		{"clear-null-ack", "index-delete", "current", `{"code":0,"data":null}`, "", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/api/v1/datasets/kb/index", r.URL.Path)
				require.Equal(t, "graph", r.URL.Query().Get("type"))
				if r.Method == "GET" {
					_, _ = w.Write([]byte(tc.readback))
					return
				}
				if tc.operation == "index-delete" {
					require.Equal(t, "DELETE", r.Method)
					require.Equal(t, "true", r.URL.Query().Get("wipe"))
					require.Equal(t, tc.expectedID, r.URL.Query().Get("task_id"))
				} else {
					require.Equal(t, "POST", r.Method)
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			query := url.Values{"type": []string{"graph"}}
			if tc.expectedID != "" {
				query.Set("task_id", tc.expectedID)
			}
			_, err := c.KnowledgePage(context.Background(), domain.PageRequest{Operation: tc.operation, DatasetID: "kb", Query: query})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantCalls, calls)
		})
	}
}
