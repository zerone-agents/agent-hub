package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestDocumentUpdatePreservesPipelineAndHistoricalConfig(t *testing.T) {
	for _, explicitSwitch := range []bool{false, true} {
		t.Run(map[bool]string{false: "snapshot save", true: "explicit builtin switch"}[explicitSwitch], func(t *testing.T) {
			stored := map[string]any{"id": "d", "dataset_id": "kb", "chunk_method": "naive", "pipeline_id": "0123456789abcdef0123456789abcdef", "meta_fields": map[string]any{"old": "v"}, "parser_config": map[string]any{"control_panel": map[string]any{"display_name": "keep"}, "image_table_context_window": 8, "chunk_token_num": 512, "auto_keywords": 0, "raptor": map[string]any{"use_raptor": true, "prompt": "keep prompt"}}}
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == "GET" {
					reads++
					require.Equal(t, "/api/v1/datasets/kb/documents", r.URL.Path)
					require.Equal(t, "d", r.URL.Query().Get("id"))
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{stored}}})
					return
				}
				writes++
				require.Equal(t, "PATCH", r.Method)
				require.Equal(t, "/api/v1/datasets/kb/documents/d", r.URL.Path)
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				config := body["parser_config"].(map[string]any)
				require.Equal(t, map[string]any{"chunk_token_num": float64(1024), "raptor": map[string]any{"use_raptor": false}}, config)
				if explicitSwitch {
					require.Equal(t, "naive", body["chunk_method"])
					require.Equal(t, "", body["pipeline_id"])
					stored["pipeline_id"] = ""
				} else {
					require.NotContains(t, body, "chunk_method")
					require.NotContains(t, body, "pipeline_id")
				}
				storedConfig := stored["parser_config"].(map[string]any)
				storedConfig["chunk_token_num"] = config["chunk_token_num"]
				storedConfig["raptor"].(map[string]any)["use_raptor"] = false
				stored["meta_fields"] = body["meta_fields"]
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": stored})
			}))
			defer server.Close()
			req := domain.DocumentUpdateRequest{"parser_id": "naive", "parser_config": map[string]any{"control_panel": map[string]any{"display_name": "overwritten"}, "image_table_context_window": 9, "chunk_token_num": 1024, "auto_keywords": 0, "raptor": map[string]any{"use_raptor": false, "prompt": "keep prompt"}}, "meta_fields": map[string]any{"version": "v2"}}
			if explicitSwitch {
				req["pipeline_id"] = ""
			}
			doc, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", req)
			require.NoError(t, err)
			require.Equal(t, 2, reads)
			require.Equal(t, 1, writes)
			require.Equal(t, "keep", (*doc)["parser_config"].(map[string]any)["control_panel"].(map[string]any)["display_name"])
			require.Equal(t, float64(8), (*doc)["parser_config"].(map[string]any)["image_table_context_window"])
		})
	}
}

func TestDocumentUpdateFailsClosedOnPreflightOrReadbackMismatch(t *testing.T) {
	for _, failure := range []string{"empty", "foreign", "wrong-id", "permission", "post-empty", "unchanged-name", "lost-pipeline", "post-business-error"} {
		t.Run(failure, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					writes++
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				reads++
				doc := map[string]any{"id": "d", "dataset_id": "kb", "name": "before.pdf", "chunk_method": "naive", "pipeline_id": "0123456789abcdef0123456789abcdef"}
				if failure == "permission" || (failure == "post-business-error" && reads > 1) {
					_, _ = w.Write([]byte(`{"code":109,"data":false}`))
					return
				}
				if failure == "foreign" {
					doc["dataset_id"] = "other"
				}
				if failure == "wrong-id" {
					doc["id"] = "other"
				}
				if reads > 1 && failure != "unchanged-name" {
					doc["name"] = "after.pdf"
				}
				if reads > 1 && failure == "lost-pipeline" {
					doc["pipeline_id"] = ""
				}
				docs, total := []any{doc}, 1
				if failure == "empty" || (failure == "post-empty" && reads > 1) {
					docs, total = []any{}, 0
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": total, "docs": docs}})
			}))
			defer server.Close()
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"name": "after.pdf"})
			require.Error(t, err)
			if reads > 1 {
				var outcome *domain.DocumentUpdateError
				require.ErrorAs(t, err, &outcome)
				require.Equal(t, "unknown", outcome.Outcome)
				require.Equal(t, 1, writes)
			} else {
				require.Zero(t, writes)
			}
		})
	}
}

func TestDocumentUpdateTypedErrorsAreSafeAndKeepOutcome(t *testing.T) {
	for code, status := range map[string]int{"DOCUMENT_UPDATE_INVALID": 400, "DOCUMENT_UPDATE_VALIDATION": 422, "DOCUMENT_UPDATE_FORBIDDEN": 403, "DOCUMENT_UPDATE_UNAVAILABLE": 404, "DOCUMENT_UPDATE_CONFLICT": 409, "DOCUMENT_UPDATE_OUTCOME_UNKNOWN": 500} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"docs":[{"id":"d","name":"before.pdf"}]}}`))
					return
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "retcode": 102, "message": "secret backend trace", "details": map[string]any{"outcome": "unchanged"}, "request_id": "0123456789abcdef0123456789abcdef"})
			}))
			defer server.Close()
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"name": "after.pdf"})
			var typed *domain.DocumentUpdateError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, code, typed.Code)
			require.Equal(t, "unchanged", typed.Outcome)
			require.NotContains(t, typed.Error(), "secret")
			require.Equal(t, "0123456789abcdef0123456789abcdef", typed.RequestID)
		})
	}
}

func TestDocumentMetadataClearRequiresExactReadback(t *testing.T) {
	for _, clears := range []bool{false, true} {
		t.Run(map[bool]string{false: "ignored clear", true: "confirmed clear"}[clears], func(t *testing.T) {
			metadata := map[string]any{"old": "value"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					if clears {
						metadata = map[string]any{}
					}
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "meta_fields": metadata}}}})
			}))
			defer server.Close()
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"meta_fields": map[string]any{}})
			if clears {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestDocumentMetadataListProjectionMatchesReadback(t *testing.T) {
	fields := []any{map[string]any{"key": "version", "type": "list", "description": "version", "enum": []any{"v1", "v2"}, "examples": []any{"v1"}}}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			writes++
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		metadata := map[string]any{}
		if writes > 0 {
			metadata = map[string]any{"type": "object", "properties": map[string]any{"version": map[string]any{"description": "version", "type": "array", "items": map[string]any{"type": "string", "enum": []any{"v1", "v2"}}, "examples": []any{[]any{"v1"}}}}, "additionalProperties": false}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "parser_config": map[string]any{"metadata": metadata}}}}})
	}))
	defer server.Close()
	_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"parser_config": map[string]any{"metadata": fields}})
	require.NoError(t, err)
	require.Equal(t, 1, writes)
}

func TestDocumentMetadataSchemaEditPreservesWholeLegacyProjection(t *testing.T) {
	for _, dropsConstraint := range []bool{false, true} {
		t.Run(map[bool]string{false: "full durable schema", true: "upstream lost unchanged field"}[dropsConstraint], func(t *testing.T) {
			metadata := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"a": map[string]any{"type": "string", "description": "old"}, "b": map[string]any{"type": "number", "description": "keep", "enum": []any{1, 2.5}}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					// A stored historical array is replaced by this object upstream.
					// It must contain the unchanged field and all type constraints.
					metadata = body["parser_config"].(map[string]any)["metadata"].(map[string]any)
					props := metadata["properties"].(map[string]any)
					require.Equal(t, "string", props["a"].(map[string]any)["type"])
					require.Equal(t, "new", props["a"].(map[string]any)["description"])
					require.Equal(t, "number", props["b"].(map[string]any)["type"])
					require.Equal(t, false, metadata["additionalProperties"])
					if dropsConstraint {
						delete(props, "b")
					}
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "parser_config": map[string]any{"metadata": metadata}}}}})
			}))
			defer server.Close()
			complete := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"a": map[string]any{"type": "string", "description": "new"}, "b": map[string]any{"type": "number", "description": "keep", "enum": []any{1, 2.5}}}}
			_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"parser_config": map[string]any{"metadata": complete}})
			if dropsConstraint {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPartialDocumentSchemaNeverCopiesOrOverwritesConcurrentFields(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			writes++
			t.Error("partial schema reached writer")
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"docs":[{"id":"d","parser_config":{"metadata":{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string","description":"old-a"},"b":{"type":"number","description":"concurrent-b"}}}}}]}}`))
	}))
	defer server.Close()
	for _, metadata := range []map[string]any{
		{"properties": map[string]any{"a": map[string]any{"description": "new-a"}}},
		{"type": "object", "additionalProperties": false, "properties": map[string]any{"a": map[string]any{"type": "string", "description": "new-a"}}},
	} {
		_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"parser_config": map[string]any{"metadata": metadata}})
		require.Error(t, err)
		require.Equal(t, 400, domain.StatusCode(err))
	}
	require.Zero(t, writes)
}

func TestDocumentNumericMetadataMatchesPythonProjection(t *testing.T) {
	fields := []any{map[string]any{"key": "score", "type": "number", "enum": []any{"1", "2.5"}, "examples": []any{"1"}}}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			writes++
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		metadata := map[string]any{}
		if writes > 0 {
			metadata = map[string]any{"type": "object", "properties": map[string]any{"score": map[string]any{"description": "", "type": "number", "enum": []any{1, 2.5}, "examples": []any{1}}}, "additionalProperties": false}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "parser_config": map[string]any{"metadata": metadata}}}}})
	}))
	defer server.Close()
	_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"parser_config": map[string]any{"metadata": fields}})
	require.NoError(t, err)
	require.Equal(t, 1, writes)
}

func TestCanonicalChunkMethodExplicitlyLeavesExistingPipeline(t *testing.T) {
	pipeline := "0123456789abcdef0123456789abcdef"
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			writes++
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "naive", body["chunk_method"])
			pipeline = ""
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "chunk_method": "naive", "pipeline_id": pipeline}}}})
	}))
	defer server.Close()
	_, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"chunk_method": "naive"})
	require.NoError(t, err)
	require.Equal(t, 1, writes)
}

// Golden projections were generated by field_schema from the committed
// MultiRAG eb5546c3955050792ec10201f11b73c7da6ac836 source, not this helper.
func TestDocumentMetadataSchemaMatchesCommittedFieldSchema(t *testing.T) {
	data, err := os.ReadFile("testdata/field_schema_eb5546c.json")
	require.NoError(t, err)
	var cases []struct {
		Name   string         `json:"name"`
		Field  map[string]any `json:"field"`
		Schema map[string]any `json:"schema"`
	}
	require.NoError(t, json.Unmarshal(data, &cases))
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			before, err := json.Marshal(tc.Field)
			require.NoError(t, err)
			key, _ := tc.Field["key"].(string)
			if key == "" {
				key, _ = tc.Field["name"].(string)
			}
			want := map[string]any{"type": "object", "properties": map[string]any{key: tc.Schema}, "additionalProperties": false}
			require.Equal(t, want, documentMetadataSchema([]any{tc.Field}))
			after, err := json.Marshal(tc.Field)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after), "projection changed caller-owned fields")
		})
	}
}

func TestDocumentSchemaExtensionsSurviveNormalizeAndConfirmedUpdate(t *testing.T) {
	for _, dropsExtension := range []bool{false, true} {
		t.Run(map[bool]string{false: "durable constraints", true: "lost extension"}[dropsExtension], func(t *testing.T) {
			var schema map[string]any
			require.NoError(t, json.Unmarshal([]byte(`{"type":"object","additionalProperties":false,"properties":{"score":{"type":"number","minimum":0,"maximum":10,"custom":{"keep":false}},"email":{"type":"string","format":"email"},"values":{"type":"array","items":false}}}`), &schema))
			stored := schema
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					writes++
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					stored = body["parser_config"].(map[string]any)["metadata"].(map[string]any)
					if dropsExtension {
						delete(stored["properties"].(map[string]any)["score"].(map[string]any), "custom")
					}
					_, _ = w.Write([]byte(`{"code":0,"data":null}`))
					return
				}
				reads++
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "parser_config": map[string]any{"metadata": stored}}}}})
			}))
			defer server.Close()
			submitted := domain.CloneObject(schema)
			submitted["title"] = "updated"
			doc, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateDocument(context.Background(), "kb", "d", domain.DocumentUpdateRequest{"parser_config": map[string]any{"metadata": submitted}})
			require.Equal(t, 1, writes)
			require.Equal(t, 2, reads)
			if dropsExtension {
				var unknown *domain.DocumentUpdateError
				require.ErrorAs(t, err, &unknown)
				require.Equal(t, "unknown", unknown.Outcome)
			} else {
				require.NoError(t, err)
				require.Equal(t, submitted, (*doc)["parser_config"].(map[string]any)["metadata"])
			}
		})
	}
}
