package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This opt-in test consumes owned PostgreSQL/Milvus fixtures from MultiRAG's
// test_dataset_search_http.py. It must never be aimed at business documents.
// The fixture owner supplies ephemeral credentials through the environment.
func TestKnowledgeSearchMultiRAGIntegration(t *testing.T) {
	base := os.Getenv("HUB_KNOWLEDGE_TEST_BASE_URL")
	if base == "" {
		t.Skip("requires an owned MultiRAG SQL/Milvus scratch fixture")
	}
	key := os.Getenv("HUB_KNOWLEDGE_TEST_API_KEY")
	require.NotEmpty(t, key)
	var datasets, documents []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("HUB_KNOWLEDGE_TEST_DATASETS")), &datasets))
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("HUB_KNOWLEDGE_TEST_DOCUMENTS")), &documents))
	require.Len(t, datasets, 2)
	require.Len(t, documents, 2)
	router := gin.New()
	group := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(base, key, 30*time.Second, time.Hour), nil), nil))
	call := func(payload map[string]any, status int) map[string]any {
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/v1/admin/knowledge/retrieval", strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		require.Equal(t, status, w.Code, w.Body.String())
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		if status != 200 {
			require.Equal(t, false, envelope["success"])
			return envelope
		}
		require.Equal(t, true, envelope["success"])
		result, ok := envelope["data"].(map[string]any)
		require.True(t, ok)
		return result
	}
	version := map[string]any{"key": "version", "op": "is", "value": "v2"}
	absent := map[string]any{"key": "version", "op": "is", "value": "absent"}
	missing := map[string]any{"key": "missing", "op": "is", "value": "x"}
	ready := map[string]any{"key": "category", "op": "is", "value": "match"}
	cases := []struct {
		name, logic string
		scope       []string
		conditions  []any
		want        []string
	}{
		{"match", "and", documents, []any{version}, []string{documents[1]}},
		{"disjoint", "and", documents[:1], []any{version}, []string{}},
		{"absent", "and", documents, []any{absent}, []string{}},
		{"missing-and", "and", documents, []any{missing, ready}, []string{}},
		{"missing-or", "or", documents, []any{missing, version}, []string{documents[1]}},
		{"ready-or", "or", documents[:1], []any{version, ready}, []string{documents[0]}},
		{"metadata-only-absent", "and", nil, []any{absent}, []string{}},
	}
	for _, mode := range []string{"dense", "sparse", "hybrid", "fusion"} {
		for _, method := range []string{"manual", "auto", "semi_auto"} {
			for _, tc := range cases {
				t.Run(fmt.Sprintf("%s/%s/%s", mode, method, tc.name), func(t *testing.T) {
					payload := map[string]any{
						"question": "availability " + tc.name, "dataset_ids": datasets, "doc_ids": tc.scope,
						"search_mode": map[string]any{"type": mode}, "similarity_threshold": 0,
						"meta_data_filter":   map[string]any{"method": method, "logic": tc.logic, "manual": tc.conditions, "semi_auto": []string{"category", "version"}},
						"reference_metadata": map[string]any{"include": true}, "page": 1, "size": 30,
					}
					result := call(payload, 200)
					require.Equal(t, float64(len(tc.want)), result["total"])
					chunks, ok := result["chunks"].([]any)
					require.True(t, ok)
					got := []string{}
					for _, value := range chunks {
						chunk := value.(map[string]any)
						got = append(got, chunk["document_id"].(string))
						require.NotEmpty(t, chunk["document_metadata"])
					}
					sort.Strings(got)
					want := append([]string{}, tc.want...)
					sort.Strings(want)
					require.Equal(t, want, got)
					aggs := []string{}
					for _, value := range result["doc_aggs"].([]any) {
						aggs = append(aggs, value.(map[string]any)["doc_id"].(string))
					}
					sort.Strings(aggs)
					require.Equal(t, want, aggs)
				})
			}
		}
	}
	legacy := map[string]any{
		"question": "availability", "dataset_ids": datasets, "document_ids": documents, "page_size": 30,
		"metadata_condition": map[string]any{"logic": "and", "conditions": []any{map[string]any{"name": "version", "comparison_operator": "is", "value": "v2"}}},
	}
	result := call(legacy, 200)
	require.Equal(t, float64(1), result["total"])
	require.Equal(t, documents[1], result["chunks"].([]any)[0].(map[string]any)["document_id"])
	seen := map[string]bool{}
	for page := 1; page <= 2; page++ {
		result = call(map[string]any{"question": "availability", "dataset_ids": datasets, "page": page, "size": 1}, 200)
		require.Equal(t, float64(2), result["total"])
		chunks := result["chunks"].([]any)
		require.Len(t, chunks, 1)
		seen[chunks[0].(map[string]any)["document_id"].(string)] = true
	}
	require.Len(t, seen, 2)
	call(map[string]any{"question": "availability", "dataset_ids": datasets, "doc_ids": documents, "use_kg": true}, 400)
	if foreign := os.Getenv("HUB_KNOWLEDGE_TEST_FOREIGN_DATASET"); foreign != "" {
		call(map[string]any{"question": "availability", "dataset_ids": []string{datasets[0], foreign}}, 502)
	}
}
