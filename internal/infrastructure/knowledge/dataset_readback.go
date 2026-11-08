package knowledge

import (
	"bytes"
	"encoding/json"

	domain "control-panel/internal/domain/knowledge"
)

func datasetParserConfigContains(actual, requested any) bool {
	normalized := domain.DatasetUpdateToRemote(domain.DatasetMutationRequest{"parser_config": requested})["parser_config"]
	expected, ok := normalized.(map[string]any)
	if !ok {
		return jsonContains(actual, normalized)
	}
	expected = domain.CloneObject(expected)
	// The service applies the legacy extension over its outer siblings before
	// merging into storage. It does not retain an ext wrapper in the result.
	extension, _ := expected["ext"].(map[string]any)
	delete(expected, "ext")
	for key, value := range extension {
		expected[key] = value
	}
	if roles, ok := expected["table_column_roles"].(map[string]any); ok {
		roles = domain.CloneObject(roles)
		for name, role := range roles {
			if role == "vectorize" {
				roles[name] = "indexing"
			}
		}
		expected["table_column_roles"] = roles
	}
	if child, ok := expected["parent_child"].(map[string]any); ok && child["use_parent_child"] == false {
		expected["parent_child"] = map[string]any{}
	}
	stored, ok := actual.(map[string]any)
	if !ok {
		return false
	}
	for key, value := range expected {
		// Role maps replace all columns, and disabled parent-child mode clears
		// its object. A subset check would falsely confirm stale stored entries.
		emptyChild, isObject := value.(map[string]any)
		exact := key == "table_column_roles" || (key == "parent_child" && isObject && len(emptyChild) == 0)
		if exact {
			observed, exists := stored[key]
			wantJSON, wantErr := json.Marshal(value)
			gotJSON, gotErr := json.Marshal(observed)
			if !exists || wantErr != nil || gotErr != nil || !bytes.Equal(gotJSON, wantJSON) {
				return false
			}
		} else if !jsonContains(stored[key], value) {
			return false
		}
	}
	return true
}
