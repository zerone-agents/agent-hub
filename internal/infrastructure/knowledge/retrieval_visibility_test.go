package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetrievalParentVisibilityFailsClosedBeforeNormalization(t *testing.T) {
	for _, scenario := range []string{
		"enabled", "disabled", "missing", "denied", "http-error", "transport-error", "missing-status", "enabled-false", "status-number", "null-data", "missing-total", "wrong-total", "duplicate-doc", "outside-batch",
		"source-kb-alias", "source-doc-alias", "source-chunk-alias", "source-missing-kb", "source-missing-doc", "source-missing-chunk", "source-null-alias", "source-wrong-kb", "source-disabled", "source-outside-doc-scope",
		"parent-kb-alias", "parent-doc-alias", "parent-wrong-kb", "parent-wrong-doc", "ambiguous-owner", "wrong-owner", "aggregate-disabled", "aggregate-alias", "aggregate-kb-alias", "aggregate-other-kb", "aggregate-only",
	} {
		t.Run(scenario, func(t *testing.T) {
			searches, reads := 0, 0
			chunk := map[string]any{"chunk_id": "c", "kb_id": "kb1", "doc_id": "d", "content_with_weight": "hidden-chunk-text"}
			agg := map[string]any{"doc_id": "d", "doc_name": "hidden-document-name", "count": 1}
			badSource := true
			switch scenario {
			case "source-kb-alias":
				chunk["dataset_id"] = "kb2"
			case "source-doc-alias":
				chunk["document_id"] = "other"
			case "source-chunk-alias":
				chunk["id"] = "other"
			case "source-missing-kb":
				delete(chunk, "kb_id")
			case "source-missing-doc":
				delete(chunk, "doc_id")
			case "source-missing-chunk":
				delete(chunk, "chunk_id")
			case "source-null-alias":
				chunk["document_id"] = nil
			case "source-wrong-kb":
				chunk["kb_id"] = "foreign"
			case "source-disabled":
				chunk["available_int"] = 0
			case "source-outside-doc-scope":
				chunk["doc_id"] = "outside"
			case "aggregate-alias":
				agg["document_id"] = "other"
			case "aggregate-kb-alias":
				agg["kb_id"], agg["dataset_id"] = "kb1", "kb2"
			case "aggregate-other-kb":
				agg["kb_id"] = "foreign"
			default:
				badSource = false
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == "POST" {
					searches++
					chunks := []any{chunk}
					if scenario == "aggregate-only" {
						chunks = []any{}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "chunks": chunks, "doc_aggs": []any{agg}}})
					return
				}
				reads++
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "1", r.URL.Query().Get("page"))
				require.Equal(t, "100", r.URL.Query().Get("page_size"))
				require.Equal(t, []string{"d"}, r.URL.Query()["ids"])
				require.NotContains(t, r.URL.Query(), "id")
				kb := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/datasets/"), "/documents")
				if scenario == "transport-error" {
					connection, _, err := w.(http.Hijacker).Hijack()
					require.NoError(t, err)
					_ = connection.Close()
					return
				}
				if scenario == "denied" || scenario == "http-error" {
					if scenario == "http-error" {
						w.WriteHeader(403)
					}
					_, _ = w.Write([]byte(`{"code":109,"message":"private-provider-error"}`))
					return
				}
				if scenario == "null-data" {
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				row := map[string]any{"id": "d", "dataset_id": kb, "status": "1"}
				switch scenario {
				case "disabled", "aggregate-disabled", "aggregate-only":
					row["status"] = "0"
				case "missing-status":
					delete(row, "status")
				case "status-number":
					row["status"] = 1
				case "enabled-false":
					row["enabled"] = false
				case "parent-kb-alias":
					row["kb_id"] = "kb2"
				case "parent-doc-alias":
					row["document_id"] = "other"
				case "parent-wrong-kb":
					row["dataset_id"] = "kb2"
				case "parent-wrong-doc", "outside-batch":
					row["id"] = "other"
				}
				docs := []any{}
				if (kb == "kb1" && scenario != "missing" && scenario != "wrong-owner") || (kb == "kb2" && (scenario == "ambiguous-owner" || scenario == "wrong-owner")) {
					docs = append(docs, row)
				}
				if scenario == "duplicate-doc" && kb == "kb1" {
					docs = append(docs, row)
				}
				data := map[string]any{"total": len(docs), "docs": docs}
				if scenario == "missing-total" {
					delete(data, "total")
				}
				if scenario == "wrong-total" {
					data["total"] = 10
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := client.Retrieval(context.Background(), domain.RetrievalRequest{"question": "q", "dataset_ids": []string{"kb1", "kb2"}, "doc_ids": []string{"d"}})
			if scenario == "enabled" {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 2, reads)
			} else {
				require.Error(t, err)
				require.Nil(t, result, "the entire result must be hidden, including chunks, doc_aggs and total")
				for _, text := range []string{"hidden-chunk-text", "hidden-document-name", "private-provider-error"} {
					require.NotContains(t, err.Error(), text)
				}
			}
			if badSource {
				require.Zero(t, reads, "raw alias/scope failures must precede parent reads and normalization")
			}
			require.Equal(t, 1, searches)
		})
	}
}

func TestRetrievalParentReadBudgetAndDeduplication(t *testing.T) {
	for _, tc := range []struct{ datasets, documents, reads int }{
		{2, 1, 2}, {2, 101, 4}, {4, 800, 32}, {4, 801, 0}, {1, 1001, 0}, {33, 1, 0},
	} {
		t.Run(fmt.Sprintf("%dKB-%ddocs", tc.datasets, tc.documents), func(t *testing.T) {
			selected := []string{}
			for i := 0; i < tc.datasets; i++ {
				selected = append(selected, fmt.Sprintf("kb%d", i))
			}
			reads, searches := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					searches++
					aggs := []any{}
					for i := 0; i < tc.documents; i++ {
						aggs = append(aggs, map[string]any{"doc_id": fmt.Sprintf("d%d", i)})
					}
					// Repeated chunks from one document must share its parent read.
					chunk := map[string]any{"chunk_id": "c", "kb_id": "kb0", "doc_id": "d0"}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": tc.documents, "chunks": []any{chunk, chunk}, "doc_aggs": aggs}})
					return
				}
				reads++
				ids := r.URL.Query()["ids"]
				require.NotEmpty(t, ids)
				require.LessOrEqual(t, len(ids), 100)
				rows := []any{}
				if r.URL.Path == "/api/v1/datasets/kb0/documents" {
					for _, id := range ids {
						rows = append(rows, map[string]any{"id": id, "dataset_id": "kb0", "status": "1"})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": len(rows), "docs": rows}})
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			result, err := client.Retrieval(context.Background(), domain.RetrievalRequest{"question": "q", "dataset_ids": selected})
			if tc.reads == 0 {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				// Aggregations lacking dataset IDs are verified, never rewritten.
				require.NotContains(t, (*result)["doc_aggs"].([]any)[0].(map[string]any), "kb_id")
			}
			require.Equal(t, tc.reads, reads)
			require.Equal(t, 1, searches)
		})
	}
}
