package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestPageDocumentPartialStatusAndCreationContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Error("missing bearer")
		}
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		switch r.URL.Path {
		case "/api/v1/datasets/kb/documents/batch-update-status":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["status"] != "0" {
				t.Errorf("body=%v", body)
			}
			_, _ = w.Write([]byte(`{"code":100,"message":"private-path","data":{"one":{"status":"0"},"two":{"error":"private-key"}}}`))
		case "/api/v1/datasets/kb/documents":
			if r.URL.Query().Get("type") != "web" {
				t.Error("creation type")
			}
			if err := r.ParseMultipartForm(1024); err != nil {
				t.Fatal(err)
			}
			if r.FormValue("name") != "Page" || r.FormValue("url") != "https://example.com" {
				t.Errorf("form=%v", r.Form)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"web1"}}`))
		default:
			t.Errorf("unexpected path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	result, err := c.KnowledgePage(context.Background(), domain.PageRequest{Operation: "document-status", DatasetID: "kb", Body: domain.Object{"doc_ids": []any{"one", "two", "missing"}, "status": "0"}})
	if err != nil {
		t.Fatal(err)
	}
	status := result.(domain.DocumentStatusResult)
	if len(status.Succeeded) != 1 || len(status.Failed) != 2 {
		t.Errorf("result=%+v", status)
	}
	if _, err := c.KnowledgePage(context.Background(), domain.PageRequest{Operation: "create-web", DatasetID: "kb", Body: domain.Object{"name": "Page", "url": "https://example.com"}}); err != nil {
		t.Fatal(err)
	}
}
func TestPageRejectsUnknownOperationsWithoutRemoteIO(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	for _, op := range []string{"../../connectors", "create-web", "document-status"} {
		if _, err := c.KnowledgePage(context.Background(), domain.PageRequest{Operation: op, DatasetID: "kb", Body: domain.Object{}}); err == nil {
			t.Errorf("operation %s accepted", op)
		}
	}
	if calls != 0 {
		t.Errorf("remote calls=%d", calls)
	}
}

func TestKnowledgeManagementPinnedRoutes(t *testing.T) {
	cases := []struct {
		op, method, path string
		query            bool
	}{
		{"metadata-config-get", "GET", "/datasets/kb/metadata/config", false},
		{"metadata-config-put", "PUT", "/datasets/kb/metadata/config", false},
		{"tags-list", "GET", "/datasets/kb/tags", false}, {"tags-rename", "PUT", "/datasets/kb/tags", false}, {"tags-delete", "DELETE", "/datasets/kb/tags", false},
		{"ingestions-summary", "GET", "/datasets/kb/ingestions/summary", false}, {"ingestions-list", "GET", "/datasets/kb/ingestions", false}, {"ingestions-detail", "GET", "/datasets/kb/ingestions/log1", false},
		{"index-get", "GET", "/datasets/kb/index", true}, {"index-run", "POST", "/datasets/kb/index", true}, {"index-delete", "DELETE", "/datasets/kb/index", true}, {"graph-search", "GET", "/datasets/kb/graph/search", false},
		{"connectors-linked", "GET", "/datasets/kb/connectors", false}, {"connectors-list", "GET", "/connectors", false}, {"connectors-create", "POST", "/connectors", false},
		{"connector-get", "GET", "/connectors/c1", false}, {"connector-update", "PATCH", "/connectors/c1", false}, {"connector-delete", "DELETE", "/connectors/c1", false}, {"connector-logs", "GET", "/connectors/c1/logs", false}, {"connector-resume", "POST", "/connectors/c1/resume", false}, {"connector-rebuild", "POST", "/connectors/c1/rebuild", false}, {"connector-link", "PUT", "/datasets/kb/connectors/c1", false}, {"connector-unlink", "DELETE", "/datasets/kb/connectors/c1", false},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !(tc.op == "metadata-config-put" && r.Method == http.MethodGet) && r.Method != tc.method || r.URL.Path != "/api/v1"+tc.path || r.Header.Get("Authorization") != "Bearer key" {
					t.Errorf("request=%s %s", r.Method, r.URL.String())
				}
				if tc.query && r.URL.Query().Get("type") != "graph" {
					t.Error("missing index type")
				}
				if tc.op == "connector-rebuild" {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["kb_id"] != "kb" {
						t.Errorf("unbound rebuild=%v", body)
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"kb_id": "foreign"}})
			}))
			defer server.Close()
			c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			_, err := c.KnowledgePage(context.Background(), domain.PageRequest{Operation: tc.op, DatasetID: "kb", ConnectorID: "c1", LogID: "log1", Query: url.Values{"type": []string{"graph"}}, Body: domain.Object{"kb_id": "foreign"}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOAuthPendingUsesPinnedBusinessCode(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/v1/connectors/google/oauth/web/result" || r.URL.Query().Get("source") != "gmail" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("request=%s %s", r.Method, r.URL.String())
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["flow_id"] != "flow1" {
			t.Errorf("body=%v", body)
		}
		if calls == 1 {
			_, _ = w.Write([]byte(`{"code":106,"message":"Authorization is still pending."}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"credentials":"test-credentials"}}`))
	}))
	defer server.Close()
	c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	req := domain.PageRequest{Operation: "oauth-result", DatasetID: "kb", OAuthProvider: "google", Query: url.Values{"source": []string{"gmail"}}, Body: domain.Object{"flow_id": "flow1"}}
	result, err := c.KnowledgePage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["pending"] != true {
		t.Errorf("result=%v", result)
	}
	result, err = c.KnowledgePage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]any)["credentials"] != "test-credentials" {
		t.Errorf("result=%v", result)
	}
}
