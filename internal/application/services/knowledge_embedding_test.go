package services

import (
	"context"
	"control-panel/internal/domain/knowledge"
	remote "control-panel/internal/infrastructure/knowledge"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeEmbeddingUsesResolvedReferenceForCheckAndSave(t *testing.T) {
	providers := setupKnowledgeProviderSvc(t)
	for _, field := range []string{"embd_id", "embedding_model"} {
		t.Run(field, func(t *testing.T) {
			checks, writes := 0, 0
			stored := map[string]any{"id": "kb", "name": "physical", "embedding_model": "old@Anthropic", "chunk_count": 2}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "GET":
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": []any{stored}})
				case "POST":
					checks++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "bge-large-zh@Anthropic", body["embd_id"])
					_, _ = w.Write([]byte(`{"code":0,"data":{"summary":{"kb_id":"kb","model":"bge-large-zh@Anthropic","sampled":1,"valid":1,"avg_cos_sim":1,"min_cos_sim":1,"max_cos_sim":1,"match_mode":"content_only"},"results":[{"chunk_id":"chunk"}]}}`))
				case "PUT":
					writes++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, "bge-large-zh@Anthropic", body["embedding_model"])
					stored["embedding_model"] = body["embedding_model"]
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": stored})
				}
			}))
			defer upstream.Close()
			svc := NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), providers)
			_, err := svc.UpdateDataset(context.Background(), "default", "kb", knowledge.DatasetMutationRequest{field: "bge-large-zh"})
			require.NoError(t, err)
			require.Equal(t, 1, checks)
			require.Equal(t, 1, writes)
			_, err = svc.UpdateDataset(context.Background(), "default", "kb", knowledge.DatasetMutationRequest{field: "bge-large-zh"})
			require.NoError(t, err)
			require.Equal(t, 1, checks, "resolved unchanged model must not run another compatibility check")
			_, err = svc.CheckEmbedding(context.Background(), "default", "kb", knowledge.EmbeddingCheckRequest{EmbeddingID: "bge-large-zh"})
			require.NoError(t, err)
			require.Equal(t, 2, checks)
			require.Equal(t, 2, writes, "explicit check must not save settings")
		})
	}
}
