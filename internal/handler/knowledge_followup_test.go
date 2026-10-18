package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeFollowupFileContentAndBinding(t *testing.T) {
	for _, tc := range []struct {
		name, action, mime, disposition, content string
		upstreamStatus, wantStatus               int
		badBinding                               bool
	}{
		{"json-download", "download", "application/json", "attachment; filename=x.json", `{"code":109,"business":"file"}`, 200, 200, false},
		{"json-preview", "preview", "application/json", "attachment", `{}`, 200, 200, false},
		{"empty-download", "download", "application/octet-stream", "attachment", "", 200, 200, false},
		{"pdf-preview", "preview", "application/pdf", "", "%PDF", 200, 200, false},
		{"json-error", "download", "application/json", "", `{"code":109,"message":"private-secret"}`, 200, 502, false},
		{"binary-error", "download", "application/octet-stream", "attachment", "private-secret", 403, 502, false},
		{"binding-denied", "download", "application/pdf", "attachment", "private-secret", 200, 502, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileReads := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.URL.Path == "/api/v1/datasets/kb/documents" {
					require.Equal(t, "doc", r.URL.Query().Get("id"))
					owner := "kb"
					if tc.badBinding {
						owner = "other"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "doc", "kb_id": owner}}}})
					return
				}
				fileReads++
				require.Equal(t, "/api/v1/documents/doc/"+tc.action, r.URL.Path)
				w.Header().Set("Content-Type", tc.mime)
				w.Header().Set("Content-Disposition", tc.disposition)
				w.WriteHeader(tc.upstreamStatus)
				_, _ = io.WriteString(w, tc.content)
			}))
			defer upstream.Close()
			w := httptest.NewRecorder()
			managementTestRouter(upstream.URL).ServeHTTP(w, httptest.NewRequest("GET", "/knowledge/datasets/kb/documents/doc/"+tc.action, nil))
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
			if tc.wantStatus == 200 {
				require.Equal(t, tc.content, w.Body.String())
				require.Equal(t, tc.mime, w.Header().Get("Content-Type"))
			} else {
				require.NotContains(t, w.Body.String(), "private-secret")
			}
			if tc.badBinding {
				require.Zero(t, fileReads)
			}
		})
	}
}

func TestKnowledgeFollowupEmbeddingRoute(t *testing.T) {
	for _, code := range []int{0, 10, 109} {
		calls := 0
		score := .95
		if code == 10 {
			score = .8
		}
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "/api/v1/datasets/kb/embedding/check", r.URL.Path)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "private-secret", "data": map[string]any{"summary": map[string]any{"kb_id": "kb", "model": "new@Factory", "sampled": 1, "valid": 1, "avg_cos_sim": score, "min_cos_sim": score, "max_cos_sim": score, "match_mode": "content_only"}, "results": []any{map[string]any{"chunk_id": "chunk"}}}})
		}))
		router := managementTestRouter(upstream.URL)
		for _, body := range []string{`{"embd_id":"new@Factory"}`, `{"embd_id":""}`, `{"embd_id":"new@Factory","check_num":0}`, `{"embd_id":"new@Factory","check_num":101}`, `{"embd_id":"new@Factory","check_num":1.2}`} {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/knowledge/datasets/kb/embedding/check", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			if body != `{"embd_id":"new@Factory"}` {
				require.Equal(t, 400, w.Code, w.Body.String())
			} else if code == 109 {
				require.Equal(t, 502, w.Code)
			} else {
				require.Equal(t, 200, w.Code)
				require.Contains(t, w.Body.String(), `"summary":`)
				require.Contains(t, w.Body.String(), `"compatible":`)
			}
			require.NotContains(t, w.Body.String(), "private-secret")
		}
		require.Equal(t, 1, calls)
		upstream.Close()
	}
}

func TestKnowledgeFollowupIncompatibleSavePreservesEvidenceWithoutWrite(t *testing.T) {
	writes, checks := 0, 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":"kb","name":"physical","embedding_model":"old@Factory","chunk_count":2}]}`))
		case "POST":
			checks++
			_, _ = w.Write([]byte(`{"code":10,"data":{"summary":{"kb_id":"kb","model":"new@Factory","sampled":1,"valid":1,"avg_cos_sim":0.8,"min_cos_sim":0.8,"max_cos_sim":0.8,"match_mode":"content_only"},"results":[{"chunk_id":"c"}]}}`))
		case "PUT":
			writes++
		}
	}))
	defer upstream.Close()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/knowledge/datasets/kb", strings.NewReader(`{"embd_id":"new@Factory"}`))
	req.Header.Set("Content-Type", "application/json")
	managementTestRouter(upstream.URL).ServeHTTP(w, req)
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"code":"knowledge_embedding_incompatible"`)
	require.Contains(t, w.Body.String(), `"code":10`)
	require.Contains(t, w.Body.String(), `"avg_cos_sim":0.8`)
	require.Zero(t, writes)
	require.Equal(t, 1, checks)
}
