package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func batchTestRequest(value any) domain.PageRequest {
	return domain.PageRequest{Operation: "document-metadatas", DatasetID: "kb", Body: domain.Object{"selector": map[string]any{"document_ids": []any{"doc"}}, "updates": []any{map[string]any{"key": "flag", "value": value}}, "deletes": []any{}}}
}

func TestTypedBatchRejectsPythonEquivalentTypeChangesBeforeWrite(t *testing.T) {
	for _, tc := range []struct{ before, value, match any }{
		{false, 0, nil}, {true, 1, nil}, {0, false, nil}, {1, true, nil},
		{false, 0, "False"}, {true, 1, "True"}, {0, false, "0.0"}, {1, true, "1.0"},
	} {
		reads, writes := 0, 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPatch {
				writes++
				t.Error("alias update reached writer")
				return
			}
			reads++
			require.Equal(t, "/api/v1/datasets/kb/documents", r.URL.Path)
			require.Equal(t, []string{"doc"}, r.URL.Query()["ids"])
			require.Equal(t, "1", r.URL.Query().Get("page"))
			require.Equal(t, "100", r.URL.Query().Get("page_size"))
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "doc", "dataset_id": "kb", "meta_fields": map[string]any{"flag": tc.before}}}}})
		}))
		request := batchTestRequest(tc.value)
		if tc.match != nil {
			request.Body["updates"].([]any)[0].(map[string]any)["match"] = tc.match
		}
		_, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), request)
		upstream.Close()
		require.Equal(t, 400, domain.StatusCode(err))
		require.Equal(t, 1, reads)
		require.Zero(t, writes)
	}
}

func TestTypedBatchConfirmsExactTypesAndClosesRaceOrIncompleteEvidence(t *testing.T) {
	for _, scenario := range []string{"new-field", "nonalias", "already-correct", "match-miss", "race-noop", "lost-type", "foreign", "wrong-id", "missing-metadata", "incomplete", "post-incomplete", "missing-counts", "bad-counts", "wrong-matched", "business-denied", "false-ack"} {
		t.Run(scenario, func(t *testing.T) {
			before := map[string]any{"flag": "old"}
			wanted := any(false)
			if scenario == "new-field" {
				before = map[string]any{}
			}
			if scenario == "nonalias" {
				before["flag"] = 2
			}
			if scenario == "already-correct" || scenario == "match-miss" {
				before["flag"] = false
			}
			if scenario == "match-miss" {
				wanted = true
			}
			if scenario == "race-noop" || scenario == "lost-type" {
				wanted = 0
			}
			request := batchTestRequest(wanted)
			if scenario == "match-miss" {
				request.Body["updates"].([]any)[0].(map[string]any)["match"] = "True"
			}
			original, err := json.Marshal(request.Body)
			require.NoError(t, err)
			reads, writes := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == http.MethodPatch {
					writes++
					require.Equal(t, "/api/v1/datasets/kb/documents/metadatas", r.URL.Path)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					encoded, err := json.Marshal(body)
					require.NoError(t, err)
					require.JSONEq(t, string(original), string(encoded), "write scope changed")
					if scenario == "business-denied" {
						_, _ = w.Write([]byte(`{"code":109,"data":false}`))
						return
					}
					if scenario == "false-ack" {
						_, _ = w.Write([]byte(`{"code":0,"data":false}`))
						return
					}
					if scenario == "missing-counts" {
						_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
						return
					}
					if scenario == "bad-counts" {
						_, _ = w.Write([]byte(`{"code":0,"data":{"updated":2,"matched_docs":1}}`))
						return
					}
					if scenario == "wrong-matched" {
						_, _ = w.Write([]byte(`{"code":0,"data":{"updated":0,"matched_docs":0}}`))
						return
					}
					updated := 1
					if scenario == "race-noop" || scenario == "lost-type" {
						before["flag"] = false
						updated = 0
					} else if scenario == "already-correct" || scenario == "match-miss" {
						updated = 0
					} else {
						before["flag"] = wanted
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"updated": updated, "matched_docs": 1}})
					return
				}
				reads++
				require.Equal(t, []string{"doc"}, r.URL.Query()["ids"])
				doc := map[string]any{"id": "doc", "dataset_id": "kb", "meta_fields": before}
				if scenario == "foreign" {
					doc["dataset_id"] = "other"
				}
				if scenario == "wrong-id" {
					doc["id"] = "other"
				}
				if scenario == "missing-metadata" {
					delete(doc, "meta_fields")
				}
				total, docs := 1, []any{doc}
				if scenario == "incomplete" || (scenario == "post-incomplete" && reads > 1) {
					total, docs = 0, []any{}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": total, "docs": docs}})
			}))
			defer upstream.Close()
			_, err = NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), request)
			switch scenario {
			case "new-field", "nonalias", "already-correct", "match-miss":
				require.NoError(t, err)
				require.Equal(t, 2, reads)
				require.Equal(t, 1, writes)
			case "foreign", "wrong-id", "missing-metadata", "incomplete":
				require.Error(t, err)
				require.Equal(t, 1, reads)
				require.Zero(t, writes)
			default:
				require.Error(t, err)
				require.Equal(t, 1, writes)
			}
		})
	}
}

func TestTypedBatchLimitsNeverExpandSelectorOrFetchWholeDataset(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	engine := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour)
	for _, kind := range []string{"all", "condition", "multiple", "deletes", "nested", "list-value", "too-many"} {
		request := batchTestRequest(false)
		switch kind {
		case "all":
			request.Body["selector"] = map[string]any{}
		case "condition":
			request.Body["selector"].(map[string]any)["metadata_condition"] = map[string]any{"conditions": []any{map[string]any{"name": "category", "comparison_operator": "=", "value": "v"}}}
		case "multiple":
			request.Body["updates"] = append(request.Body["updates"].([]any), map[string]any{"key": "other", "value": "new"})
		case "deletes":
			request.Body["deletes"] = []any{map[string]any{"key": "other"}}
		case "nested":
			request.Body["updates"].([]any)[0].(map[string]any)["value"] = map[string]any{"nested": false}
		case "list-value":
			request.Body["updates"].([]any)[0].(map[string]any)["value"] = []any{false}
		case "too-many":
			ids := []any{}
			for i := 0; i < 101; i++ {
				ids = append(ids, fmt.Sprint(i))
			}
			request.Body["selector"].(map[string]any)["document_ids"] = ids
		}
		_, err := engine.KnowledgePage(context.Background(), request)
		require.Equal(t, 400, domain.StatusCode(err), kind)
	}
	require.Zero(t, calls)
}

func TestOrdinaryBatchAddsNoReadsAndValidatesCounts(t *testing.T) {
	for _, value := range []any{"new", 2, nil} {
		for _, counts := range []string{`{"updated":1,"matched_docs":2}`, `{"updated":0,"matched_docs":0}`, `{"updated":-1,"matched_docs":2}`, `{"updated":0.5,"matched_docs":2}`, `{"updated":"1","matched_docs":2}`, `{}`} {
			reads, writes := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch {
					reads++
					t.Error("ordinary batch performed extra read")
				}
				writes++
				_, _ = w.Write([]byte(`{"code":0,"data":` + counts + `}`))
			}))
			_, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), batchTestRequest(value))
			upstream.Close()
			if counts == `{"updated":1,"matched_docs":2}` || counts == `{"updated":0,"matched_docs":0}` {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Zero(t, reads)
			require.Equal(t, 1, writes)
		}
	}
}

func TestTypedBatchNumericConditionalMatchNeverReachesWriter(t *testing.T) {
	for _, tc := range []struct {
		metadata     string
		match, value any
	}{
		{`{"flag":2.0}`, "2.0", false}, {`{"flag":1e-07}`, "1e-07", false},
		{`{"flag":0}`, "0.0", true}, {`{"flag":"2"}`, 2, false},
	} {
		reads, writes := 0, 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPatch {
				writes++
				t.Error("numeric conditional match reached writer")
				return
			}
			reads++
			fmt.Fprintf(w, `{"code":0,"data":{"total":1,"docs":[{"id":"doc","dataset_id":"kb","meta_fields":%s}]}}`, tc.metadata)
		}))
		request := batchTestRequest(tc.value)
		request.Body["updates"].([]any)[0].(map[string]any)["match"] = tc.match
		_, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), request)
		upstream.Close()
		require.Equal(t, 400, domain.StatusCode(err))
		require.Equal(t, 1, reads)
		require.Zero(t, writes)
	}
}

func TestTypedBatchUnconditionalFirstWritesAndBooleanMatchesRemainSupported(t *testing.T) {
	for _, tc := range []struct {
		before       map[string]any
		value, match any
	}{
		{map[string]any{}, false, nil}, {map[string]any{}, true, nil}, {map[string]any{}, 0, nil}, {map[string]any{}, 1, nil},
		{map[string]any{"flag": true}, false, "True"}, {map[string]any{"flag": "old"}, false, "old"},
		{map[string]any{"flag": "old"}, 0, "old"}, {map[string]any{"flag": false}, 1, "False"}, {map[string]any{"flag": true}, 0, "True"},
	} {
		metadata := tc.before
		reads, writes := 0, 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPatch {
				writes++
				metadata["flag"] = tc.value
				_, _ = w.Write([]byte(`{"code":0,"data":{"updated":1,"matched_docs":1}}`))
				return
			}
			reads++
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "doc", "dataset_id": "kb", "meta_fields": metadata}}}})
		}))
		request := batchTestRequest(tc.value)
		if tc.match != nil {
			request.Body["updates"].([]any)[0].(map[string]any)["match"] = tc.match
		}
		_, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), request)
		upstream.Close()
		require.NoError(t, err)
		require.Equal(t, 2, reads)
		require.Equal(t, 1, writes)
	}
}
