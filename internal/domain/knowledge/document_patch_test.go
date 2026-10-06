package knowledge

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPrepareDocumentPatchRemovesHistoricalSnapshotFields(t *testing.T) {
	config := map[string]any{
		"control_panel": map[string]any{"display_name": "Keep"}, "image_table_context_window": 8,
		"chunk_token_num": 1024, "enable_children": false, "auto_keywords": 0, "tag_kb_ids": []any{},
		"raptor":   map[string]any{"use_raptor": false, "legacy": true},
		"graphrag": map[string]any{"method": "light", "unknown": "stored"},
	}
	body, err := PrepareDocumentPatch(DocumentUpdateRequest{"parser_config": config, "meta_fields": map[string]any{}})
	require.NoError(t, err)
	filtered := body["parser_config"].(map[string]any)
	require.NotContains(t, filtered, "control_panel")
	require.NotContains(t, filtered, "image_table_context_window")
	require.Equal(t, false, filtered["enable_children"])
	require.Equal(t, 0, filtered["auto_keywords"])
	require.Equal(t, []any{}, filtered["tag_kb_ids"])
	require.Equal(t, map[string]any{"use_raptor": false}, filtered["raptor"])
	require.Contains(t, config, "control_panel", "caller snapshot must remain unchanged")
	require.Contains(t, config["raptor"], "legacy")
	_, hasMode := body["chunk_method"]
	require.False(t, hasMode)
}

func TestPrepareDocumentPatchRejectsAmbiguousAndMalformedWrites(t *testing.T) {
	for _, req := range []DocumentUpdateRequest{
		nil, {"id": "other"}, {"parser_config": nil}, {"parser_config": []any{}}, {"parser_config": map[string]any{"chunk_token_nmu": 5}},
		{"parser_config": map[string]any{"raptor": nil}}, {"parser_id": "naive", "chunk_method": "table"},
		{"pipeline_id": nil}, {"parser_id": "naive", "pipeline_id": "abc"}, {"parser_id": false},
	} {
		_, err := PrepareDocumentPatch(req)
		require.Error(t, err, "%v", req)
		require.Equal(t, 400, StatusCode(err))
	}
	body, err := PrepareDocumentPatch(DocumentUpdateRequest{"parser_id": "naive", "pipeline_id": ""})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"chunk_method": "naive", "pipeline_id": ""}, body)
}
