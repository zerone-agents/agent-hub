package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func embeddingEvidence(code int) string {
	score := .95
	if code == 10 {
		score = .8
	}
	return fmt.Sprintf(`{"code":%d,"data":{"summary":{"kb_id":"kb","model":"new@Factory","sampled":1,"valid":1,"avg_cos_sim":%g,"min_cos_sim":%g,"max_cos_sim":%g,"match_mode":"content_only"},"results":[{"chunk_id":"chunk","cos_sim":%g}]}}`, code, score, score, score, score)
}

func TestEmbeddingCheckPreservesIncompatibleEvidenceAndRejectsErrors(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		code, status   int
		wantError      bool
	}{
		{"compatible", embeddingEvidence(0), 0, 200, false},
		{"incompatible", embeddingEvidence(10), 10, 200, false},
		{"denied", `{"code":109,"message":"private-secret","data":null}`, 0, 200, true},
		{"missing-status", `{"data":{}}`, 0, 200, true},
		{"false", `{"code":0,"data":false}`, 0, 200, true},
		{"empty-summary", `{"code":0,"data":{"summary":{},"results":[]}}`, 0, 200, true},
		{"http-error", embeddingEvidence(0), 0, 403, true},
		{"conflicting-status", `{"code":0,"retcode":10,"data":{}}`, 0, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "POST", r.Method)
				require.Equal(t, "/api/v1/datasets/kb/embedding/check", r.URL.Path)
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, map[string]any{"embd_id": "new@Factory", "check_num": float64(5)}, body)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := c.CheckEmbedding(context.Background(), "kb", domain.EmbeddingCheckRequest{EmbeddingID: "new@Factory"})
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, result)
				require.NotContains(t, err.Error(), "private-secret")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.code, result.Code)
				require.Equal(t, tc.code == 0, result.Compatible)
				require.Equal(t, "kb", result.Summary["kb_id"])
				require.Equal(t, "chunk", result.Results[0]["chunk_id"])
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestEmbeddingSwitchChecksOnceBeforeWriteAndNeverWritesOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name, old, response string
		count               any
		checks, writes      int
		wantError           bool
	}{
		{"compatible", "old@Factory", embeddingEvidence(0), 4, 1, 1, false},
		{"incompatible", "old@Factory", embeddingEvidence(10), 4, 1, 0, true},
		{"denied", "old@Factory", `{"code":109}`, 4, 1, 0, true},
		{"malformed", "old@Factory", `{"code":0,"data":{}}`, 4, 1, 0, true},
		{"same-model", "new@Factory", "", 4, 0, 1, false},
		{"empty-dataset", "old@Factory", "", 0, 0, 1, false},
		{"unknown-count", "old@Factory", "", nil, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checks, writes := 0, 0
			stored := map[string]any{"id": "kb", "name": "physical", "embedding_model": tc.old, "chunk_count": tc.count}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/api/v1/datasets":
					require.Equal(t, "kb", r.URL.Query().Get("id"))
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{stored}})
				case r.Method == "POST" && r.URL.Path == "/api/v1/datasets/kb/embedding/check":
					checks++
					require.Zero(t, writes)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "new@Factory", body["embd_id"])
					_, _ = w.Write([]byte(tc.response))
				case r.Method == "PUT" && r.URL.Path == "/api/v1/datasets/kb":
					writes++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "new@Factory", body["embedding_model"])
					stored["embedding_model"] = body["embedding_model"]
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": stored})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
			}))
			defer server.Close()
			c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := c.UpdateDataset(context.Background(), "kb", domain.DatasetMutationRequest{"embd_id": "new@Factory"})
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, tc.old, stored["embedding_model"])
			} else {
				require.NoError(t, err)
				require.Equal(t, "new@Factory", (*result)["embd_id"])
			}
			require.Equal(t, tc.checks, checks)
			require.Equal(t, tc.writes, writes)
		})
	}
}
