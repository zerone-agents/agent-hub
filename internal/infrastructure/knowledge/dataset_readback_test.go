package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain "control-panel/internal/domain/knowledge"
)

func TestDatasetReadbackConfirmsCanonicalExtensionsAndReplacedRoles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested map[string]any
		stored    map[string]any
		wantError bool
	}{
		{"extension-wins", map[string]any{"table_column_roles": map[string]any{"old": "metadata"}, "ext": map[string]any{"table_column_roles": map[string]any{"a.b": "both"}, "graphrag": map[string]any{"method": "ner"}}}, map[string]any{"table_column_roles": map[string]any{"a.b": "both"}, "graphrag": map[string]any{"method": "ner", "use_graphrag": true}}, false},
		{"extension-aliases", map[string]any{"ext": map[string]any{"graphrag": map[string]any{"enabled": true, "method": "ner"}, "parent_child": map[string]any{"use_parent_child": false}}}, map[string]any{"graphrag": map[string]any{"use_graphrag": true, "method": "ner"}, "parent_child": map[string]any{}}, false},
		{"removed-column", map[string]any{"table_column_roles": map[string]any{"A": "metadata"}}, map[string]any{"table_column_roles": map[string]any{"A": "metadata"}}, false},
		{"removed-column-still-present", map[string]any{"table_column_roles": map[string]any{"A": "metadata"}}, map[string]any{"table_column_roles": map[string]any{"A": "metadata", "B": "indexing"}}, true},
		{"clear-roles", map[string]any{"ext": map[string]any{"table_column_roles": map[string]any{}}}, map[string]any{"table_column_roles": map[string]any{}}, false},
		{"clear-roles-retained-stale", map[string]any{"ext": map[string]any{"table_column_roles": map[string]any{}}}, map[string]any{"table_column_roles": map[string]any{"B": "metadata"}}, true},
		{"legacy-role-normalized", map[string]any{"ext": map[string]any{"table_column_roles": map[string]any{"A": "vectorize"}}}, map[string]any{"table_column_roles": map[string]any{"A": "indexing"}}, false},
		{"unrelated-graph-keys-preserved", map[string]any{"ext": map[string]any{"graphrag": map[string]any{"method": "ner"}}}, map[string]any{"graphrag": map[string]any{"method": "ner", "timeout": 300}}, false},
		{"graph-mismatch", map[string]any{"ext": map[string]any{"graphrag": map[string]any{"method": "ner"}}}, map[string]any{"graphrag": map[string]any{"method": "light"}}, true},
		{"disabled-child-still-enabled", map[string]any{"parent_child": map[string]any{"use_parent_child": false}}, map[string]any{"parent_child": map[string]any{"use_parent_child": true}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			puts, reads := 0, 0
			before, _ := json.Marshal(tc.requested)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					puts++
					if r.URL.Path != "/api/v1/datasets/kb" {
						t.Errorf("unexpected write path: %s", r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					} else if config, ok := body["parser_config"].(map[string]any); ok {
						if _, retained := config["ext"]; retained {
							t.Error("legacy extension was not canonicalized before write")
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{}})
				case http.MethodGet:
					reads++
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "total_datasets": 1, "data": []map[string]any{{"id": "kb", "name": "physical", "parser_config": tc.stored}}})
				default:
					t.Errorf("unexpected method: %s", r.Method)
				}
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			_, err := client.UpdateDataset(context.Background(), "kb", domain.DatasetMutationRequest{"parser_config": tc.requested})
			after, _ := json.Marshal(tc.requested)
			if (err != nil) != tc.wantError || puts != 1 || reads != 1 || string(before) != string(after) {
				t.Fatalf("err=%v wantError=%v writes=%d reads=%d requestMutated=%v", err, tc.wantError, puts, reads, string(before) != string(after))
			}
		})
	}
}
