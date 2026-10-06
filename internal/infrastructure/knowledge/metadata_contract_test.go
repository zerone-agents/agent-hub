package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMetadataUnsupportedRequestsNeverReachRemote(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer server.Close()
	engine := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	for _, enabled := range []bool{false, true} {
		_, err := engine.KnowledgePage(context.Background(), domain.PageRequest{Operation: "document-metadata-config", DatasetID: "kb", DocumentID: "doc", Body: domain.Object{"metadata": []any{}, "enabled": enabled}})
		require.Equal(t, 400, domain.StatusCode(err))
	}
	for _, op := range []string{"in", "not in"} {
		values := []any{1, true, nil, map[string]any{}, []any{nil}, []any{[]any{"one"}}}
		if op == "not in" {
			values = append(values, []any{1}, []any{false}, []any{"one", 2})
		}
		for _, value := range values {
			condition := map[string]any{"conditions": []any{map[string]any{"name": "field", "comparison_operator": op, "value": value}}}
			encoded, err := json.Marshal(condition)
			require.NoError(t, err)
			_, err = engine.ListDocuments(context.Background(), "kb", domain.DocumentListRequest{MetadataCondition: string(encoded)})
			require.Equal(t, 400, domain.StatusCode(err))
			_, err = engine.Retrieval(context.Background(), domain.RetrievalRequest{"question": "test", "dataset_ids": []string{"kb"}, "metadata_condition": condition})
			require.Equal(t, 400, domain.StatusCode(err))
			_, err = engine.KnowledgePage(context.Background(), domain.PageRequest{Operation: "document-metadatas", DatasetID: "kb", Body: domain.Object{"selector": map[string]any{"metadata_condition": condition}, "updates": []any{}, "deletes": []any{}}})
			require.Equal(t, 400, domain.StatusCode(err))
		}
	}
	require.Zero(t, calls)
}

func TestDocumentTemplateRequiresIndependentScopedReadback(t *testing.T) {
	data, err := os.ReadFile("testdata/field_schema_eb5546c.json")
	require.NoError(t, err)
	var fixtures []struct {
		Name   string
		Field  map[string]any
		Schema map[string]any
	}
	require.NoError(t, json.Unmarshal(data, &fixtures))
	for _, failure := range []string{"", "lost-extension", "foreign", "wrong-id", "business", "missing", "write-business"} {
		t.Run(failure, func(t *testing.T) {
			writes, reads := 0, 0
			fixture := fixtures[0]
			expected := map[string]any{"type": "object", "properties": map[string]any{"custom": fixture.Schema}, "additionalProperties": false}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == http.MethodPut {
					writes++
					require.Equal(t, "/api/v1/datasets/kb/documents/doc/metadata/config", r.URL.Path)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, []any{fixture.Field}, body["metadata"])
					require.NotContains(t, body, "enabled")
					if failure == "write-business" {
						_, _ = w.Write([]byte(`{"code":109,"data":false}`))
						return
					}
					// A successful acknowledgement alone is insufficient.
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				reads++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/datasets/kb/documents", r.URL.Path)
				require.Equal(t, "doc", r.URL.Query().Get("id"))
				if failure == "business" {
					_, _ = w.Write([]byte(`{"code":109,"data":false}`))
					return
				}
				if failure == "missing" {
					_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"docs":[]}}`))
					return
				}
				schema := expected
				if failure == "lost-extension" {
					schema = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
				}
				doc := map[string]any{"id": "doc", "dataset_id": "kb", "parser_config": map[string]any{"metadata": schema, "enable_metadata": false}}
				if failure == "foreign" {
					doc["dataset_id"] = "other"
				}
				if failure == "wrong-id" {
					doc["id"] = "other"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{doc}}})
			}))
			defer server.Close()
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), domain.PageRequest{Operation: "document-metadata-config", DatasetID: "kb", DocumentID: "doc", Body: domain.Object{"metadata": []any{fixture.Field}}})
			require.Equal(t, 1, writes)
			if failure == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if failure == "write-business" {
				require.Zero(t, reads)
			} else {
				require.Equal(t, 1, reads)
			}
		})
	}
}

func TestDatasetMetadataEnabledOnlyRetainsReadbackContract(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disable", true: "enable"}[enabled], func(t *testing.T) {
			writes, reads := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/datasets/kb/metadata/config", r.URL.Path)
				if r.Method == http.MethodPut {
					writes++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Equal(t, map[string]any{"enabled": enabled}, body)
				} else {
					require.Equal(t, http.MethodGet, r.Method)
					reads++
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"enabled": enabled, "metadata": []any{map[string]any{"key": "keep", "custom": false}}, "built_in_metadata": []any{}}})
			}))
			defer server.Close()
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), domain.PageRequest{Operation: "metadata-config-put", DatasetID: "kb", Body: domain.Object{"enabled": enabled}})
			require.NoError(t, err)
			require.Equal(t, 1, writes)
			require.Equal(t, 1, reads)
		})
	}
}

func TestDocumentTemplateTrimmedKeyAndNameConfirmIndependentReadback(t *testing.T) {
	for _, identifiers := range []map[string]any{
		{"key": " \tcategory\n"}, {"name": "\u3000category\u00a0"}, {"key": " category ", "name": "\tcategory\n"},
	} {
		t.Run(fmt.Sprint(identifiers), func(t *testing.T) {
			field := domain.CloneObject(identifiers)
			field["type"] = "string"
			field["description"] = " description "
			field["format"] = "email"
			field["custom"] = map[string]any{"key": " keep "}
			writes, reads := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == http.MethodPut {
					writes++
					require.Equal(t, "/api/v1/datasets/kb/documents/doc/metadata/config", r.URL.Path)
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					submitted := body["metadata"].([]any)[0].(map[string]any)
					for key := range identifiers {
						if submitted[key] != "category" {
							t.Errorf("outgoing %s = %q, want trimmed category", key, submitted[key])
						}
					}
					require.Equal(t, field["description"], submitted["description"])
					require.Equal(t, field["custom"], submitted["custom"])
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				reads++
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/datasets/kb/documents", r.URL.Path)
				require.Equal(t, "doc", r.URL.Query().Get("id"))
				// Independent GET's static projection matches eb5546c's DTO trim,
				// not the request or Hub's projection helper.
				_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"docs":[{"id":"doc","dataset_id":"kb","parser_config":{"enable_metadata":false,"metadata":{"type":"object","properties":{"category":{"type":"string","description":" description ","format":"email","custom":{"key":" keep "}}},"additionalProperties":false}}}]}}`))
			}))
			defer upstream.Close()
			_, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), domain.PageRequest{Operation: "document-metadata-config", DatasetID: "kb", DocumentID: "doc", Body: domain.Object{"metadata": []any{field}}})
			require.NoError(t, err)
			require.Equal(t, 1, writes)
			require.Equal(t, 1, reads)
		})
	}
}
