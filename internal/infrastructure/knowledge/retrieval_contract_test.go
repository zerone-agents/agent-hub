package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetrievalJointScopeThresholdAndDenialNeverBroaden(t *testing.T) {
	for _, denied := range []bool{false, true} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, "/api/v1/datasets/search", r.URL.Path)
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, []any{"kb1", "kb2"}, body["dataset_ids"])
			require.Equal(t, []any{"doc"}, body["doc_ids"])
			require.Equal(t, .7, body["similarity_threshold"])
			require.Equal(t, float64(2), body["page"])
			require.Equal(t, "reranker@Factory", body["rerank_id"])
			filter := body["meta_data_filter"].(map[string]any)
			require.Equal(t, []any{float64(0), false, "F2"}, filter["manual"].([]any)[0].(map[string]any)["value"])
			if denied {
				_, _ = w.Write([]byte(`{"code":109,"data":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"total":9,"chunks":[{"kb_id":"kb2","doc_id":"doc","chunk_id":"c","docnm_kwd":"source","similarity":0.8}],"doc_aggs":[{"doc_id":"doc","count":9}]}}`))
		}))
		c := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
		result, err := c.Retrieval(context.Background(), domain.RetrievalRequest{"question": "test", "dataset_ids": []string{"kb1", "kb2"}, "doc_ids": []string{"doc"}, "similarity_threshold": .7, "page": 2, "rerank_id": "reranker@Factory", "meta_data_filter": map[string]any{"method": "manual", "manual": []any{map[string]any{"key": "typed", "op": "in", "value": []any{0, false, "F2"}}}}})
		if denied {
			require.Error(t, err)
			require.Nil(t, result)
		} else {
			require.NoError(t, err)
			require.Equal(t, float64(9), (*result)["total"])
			chunk := (*result)["chunks"].([]any)[0].(map[string]any)
			require.Equal(t, "kb2", chunk["kb_id"])
			require.Equal(t, "doc", chunk["document_id"])
		}
		require.Equal(t, 1, calls, "a denied selection must never retry another dataset or identity")
		server.Close()
	}
}
