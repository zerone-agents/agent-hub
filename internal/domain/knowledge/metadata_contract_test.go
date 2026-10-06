package knowledge

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMetadataMembershipAcrossSearchListAndBatch(t *testing.T) {
	for _, op := range []string{"in", "not in"} {
		for _, tc := range []struct {
			value   string
			allowed bool
			typed   bool
		}{
			{`1`, false, false}, {`true`, false, false}, {`false`, false, false}, {`[1]`, true, true}, {`[true]`, true, true}, {`["one",2]`, true, true},
			{`["one",false]`, true, true}, {`null`, false, false}, {`[null]`, false, false}, {`{}`, false, false}, {`[["one"]]`, false, false},
			{`"one"`, true, false}, {`["one","two"]`, true, false}, {`[]`, true, false}, {`""`, true, false},
		} {
			t.Run(op+tc.value, func(t *testing.T) {
				var value any
				require.NoError(t, json.Unmarshal([]byte(tc.value), &value))
				condition := map[string]any{"conditions": []any{map[string]any{"name": "field", "comparison_operator": op, "value": value}}}
				encoded, err := json.Marshal(condition)
				require.NoError(t, err)
				_, searchErr := RetrievalToSearch(RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "meta_data_filter": map[string]any{"method": "manual", "manual": []any{map[string]any{"key": "field", "op": op, "value": value}}}})
				_, legacyErr := RetrievalToSearch(RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "metadata_condition": condition})
				listErr := ValidateDocumentFilters(DocumentListRequest{MetadataCondition: string(encoded)})
				batchErr := ValidateMetadataBatchMembership(Object{"selector": map[string]any{"metadata_condition": condition}})
				allowed := tc.allowed && !(op == "not in" && tc.typed)
				if op == "not in" {
					require.Error(t, batchErr)
					require.Equal(t, 400, StatusCode(batchErr))
				} else if allowed {
					require.NoError(t, batchErr)
				} else {
					require.Error(t, batchErr)
				}
				for _, err := range []error{searchErr, legacyErr, listErr} {
					if allowed {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.Equal(t, 400, StatusCode(err))
					}
				}
			})
		}
	}
	// Typed equality/comparison operands retain their existing contract.
	for _, op := range []string{"=", "≠", ">", "≤"} {
		require.NoError(t, validateMetadataMembership(op, 1))
	}
}

func TestDocumentTemplateRejectsIgnoredFieldsAndKeepsExplicitClear(t *testing.T) {
	for _, body := range []Object{
		{"metadata": []any{}, "enabled": false}, {"enabled": true}, {"metadata": []any{}, "built_in_metadata": []any{}},
		{"metadata": nil}, {}, {"metadata": "wrong"},
	} {
		_, err := DocumentMetadataTemplate(body)
		require.Error(t, err)
		require.Equal(t, 400, StatusCode(err))
	}
	for _, template := range []any{[]any{}, map[string]any{}, []map[string]any{{"key": "extended", "custom": false, "items": true}}} {
		got, err := DocumentMetadataTemplate(Object{"metadata": template})
		require.NoError(t, err)
		expected, err := json.Marshal(template)
		require.NoError(t, err)
		actual, err := json.Marshal(got)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	}
}

func TestDocumentTemplateTrimsOnlyFieldDTOIdentifiers(t *testing.T) {
	for _, identifiers := range []map[string]any{
		{"key": " \tcategory\n"}, {"name": "\u3000category\u00a0"}, {"key": " category ", "name": "\tcategory\n"},
	} {
		field := CloneObject(identifiers)
		field["description"] = " description "
		field["custom"] = map[string]any{"key": " keep ", "name": " keep "}
		before, err := json.Marshal(field)
		require.NoError(t, err)
		got, err := DocumentMetadataTemplate(Object{"metadata": []any{field}})
		require.NoError(t, err)
		normalized := got.([]any)[0].(map[string]any)
		for key := range identifiers {
			require.Equal(t, "category", normalized[key])
		}
		require.Equal(t, field["description"], normalized["description"])
		require.Equal(t, field["custom"], normalized["custom"])
		after, err := json.Marshal(field)
		require.NoError(t, err)
		require.Equal(t, string(before), string(after), "DTO normalization mutated caller fields")
	}
	// JSON Schema property names are literals, not MetadataField DTOs.
	schema := map[string]any{"type": "object", "properties": map[string]any{" category ": map[string]any{"type": "string"}}, "key": " keep "}
	got, err := DocumentMetadataTemplate(Object{"metadata": schema})
	require.NoError(t, err)
	require.Equal(t, schema, got)
	// Ordinary document PATCH uses a different upstream field DTO without trim.
	fields := []any{map[string]any{"key": " category "}}
	patch, err := PrepareDocumentPatch(DocumentUpdateRequest{"parser_config": map[string]any{"metadata": fields}})
	require.NoError(t, err)
	require.Equal(t, fields, patch["parser_config"].(map[string]any)["metadata"])
}

func TestBatchMetadataRejectsCommittedExclusionOperatorsAndAliases(t *testing.T) {
	for _, op := range []string{"not in", "not contains", "!=", "≠", "not is"} {
		for _, logic := range []string{"and", "or"} {
			condition := map[string]any{"logic": logic, "conditions": []any{
				map[string]any{"name": "category", "comparison_operator": "=", "value": "book"},
				map[string]any{"name": "author", "comparison_operator": op, "value": "Alice"},
			}}
			err := ValidateMetadataBatchMembership(Object{"selector": map[string]any{"metadata_condition": condition}})
			require.Error(t, err)
			require.Equal(t, 400, StatusCode(err))
			require.Contains(t, err.Error(), op)
		}
	}
	for _, op := range []string{"=", "is", "contains", "in", ">", "<", "≥", "≤", ">=", "<=", "start with", "end with", "empty", "not empty"} {
		condition := map[string]any{"conditions": []any{map[string]any{"name": "author", "comparison_operator": op, "value": "Alice"}}}
		require.NoError(t, ValidateMetadataBatchMembership(Object{"selector": map[string]any{"metadata_condition": condition}}), op)
	}
}

func TestBatchMetadataLogicRejectsImplicitOrFallback(t *testing.T) {
	for _, logic := range []any{"xor", "AND", "OR", "and ", "", nil, true, 1, []any{}, map[string]any{}} {
		condition := map[string]any{"logic": logic, "conditions": []any{map[string]any{"name": "x", "comparison_operator": "=", "value": "yes"}}}
		err := ValidateMetadataBatchMembership(Object{"selector": map[string]any{"metadata_condition": condition}})
		require.Error(t, err)
		require.Equal(t, 400, StatusCode(err))
	}
	for _, condition := range []map[string]any{{}, {"conditions": []any{}}, {"logic": "and", "conditions": []any{}}, {"logic": "or", "conditions": []any{}}} {
		require.NoError(t, ValidateMetadataBatchMembership(Object{"selector": map[string]any{"metadata_condition": condition}}))
	}
}
