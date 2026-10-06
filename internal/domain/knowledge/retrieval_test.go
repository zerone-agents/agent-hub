package knowledge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetrievalSearchCompatibilityAndEmptyScope(t *testing.T) {
	var req RetrievalRequest
	require.NoError(t, json.Unmarshal([]byte(`{"question":" test ","kb_ids":["kb"],"document_ids":[],"page_size":7,"metadata_condition":{"logic":"or","conditions":[{"name":"version","comparison_operator":"is","value":"v2"}]},"reference_metadata":{"include":true}}`), &req))
	body, err := RetrievalToSearch(req)
	require.NoError(t, err)
	require.Equal(t, []string{"kb"}, body["dataset_ids"])
	require.Equal(t, "test", body["question"])
	require.Equal(t, float64(7), body["size"])
	require.NotContains(t, body, "document_ids")
	require.NotContains(t, body, "metadata_condition")
	require.NotContains(t, body, "page_size")
	require.Equal(t, "=", body["meta_data_filter"].(map[string]any)["manual"].([]any)[0].(map[string]any)["op"])
	require.NotNil(t, body["reference_metadata"])
	require.False(t, EmptyRetrievalScope(body))
	nullScope, err := RetrievalToSearch(RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "document_ids": nil})
	require.NoError(t, err)
	require.False(t, EmptyRetrievalScope(nullScope))
	// Normalization at both service and adapter boundaries must be idempotent.
	again, err := RetrievalToSearch(body)
	require.NoError(t, err)
	require.Equal(t, body, again)
	require.Contains(t, req, "metadata_condition")
	for _, scope := range []any{[]string{}, []any{}} {
		body, err = RetrievalToSearch(RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "doc_ids": scope})
		require.NoError(t, err)
		require.True(t, EmptyRetrievalScope(body))
	}
}

func TestRetrievalRejectsIgnoredFiltersAndConflicts(t *testing.T) {
	for _, payload := range []string{
		`{"dataset_ids":[],"question":"test"}`,
		`{"dataset_ids":[" "],"question":"test"}`,
		`{"dataset_ids":["kb"],"question":" "}`,
		`{"dataset_ids":["kb"],"kb_ids":["other"],"question":"test"}`,
		`{"dataset_ids":["kb"],"question":"test","doc_ids":[1]}`,
		`{"dataset_ids":["kb"],"question":"test","metadata_condition":{"version":"v2"}}`,
		`{"dataset_ids":["kb"],"question":"test","meta_data_filter":{"method":"unknown"}}`,
		`{"dataset_ids":["kb"],"question":"test","meta_data_filter":{"method":"manual","manual":[{"key":"v","op":"typo","value":"v2"}]}}`,
		`{"dataset_ids":["kb"],"question":"test","use_kg":"true","doc_ids":["doc"]}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var req RetrievalRequest
			require.NoError(t, json.Unmarshal([]byte(payload), &req))
			_, err := RetrievalToSearch(req)
			require.Error(t, err)
			require.Equal(t, 400, StatusCode(err))
		})
	}
}

func TestRetrievalGraphSavedSearchFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name        string
		searchID    any
		useKG       any
		wantBlocked bool
	}{
		{"saved-search", "saved", true, true},
		{"whitespace-id", " ", true, true},
		{"empty-id", "", true, false},
		{"null-id", nil, true, false},
		{"kg-disabled", "saved", false, false},
		{"kg-omitted", "saved", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "search_id": tc.searchID}
			if tc.useKG != nil {
				req["use_kg"] = tc.useKG
			}
			body, err := RetrievalToSearch(req)
			if tc.wantBlocked {
				require.ErrorIs(t, err, ErrRetrievalGraphScopeConflict)
				require.Equal(t, 400, StatusCode(err))
				require.Nil(t, body)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.searchID, body["search_id"])
			}
		})
	}
	for _, value := range []any{1, true, map[string]any{}} {
		_, err := RetrievalToSearch(RetrievalRequest{"dataset_ids": []string{"kb"}, "question": "test", "search_id": value, "use_kg": true})
		require.Error(t, err)
		require.Equal(t, 400, StatusCode(err))
	}
}

func TestRetrievalGraphUsesForwardedFilterObjectBoundary(t *testing.T) {
	for _, tc := range []struct {
		fields      string
		wantBlocked bool
	}{
		{`"use_kg":true,"meta_data_filter":{"method":"manual","manual":[]}`, true},
		{`"use_kg":true,"meta_data_filter":{"method":"semi_auto","semi_auto":[]}`, true},
		{`"use_kg":true,"meta_data_filter":{"unused":true}`, true},
		{`"use_kg":true,"metadata_condition":{"conditions":[]}`, true},
		{`"use_kg":true,"meta_data_filter":{}`, false},
		{`"use_kg":true,"meta_data_filter":null`, false},
		{`"use_kg":true,"metadata_condition":{}`, false},
		{`"use_kg":true`, false},
		{`"use_kg":false,"meta_data_filter":{"method":"manual","manual":[]}`, false},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			var req RetrievalRequest
			require.NoError(t, json.Unmarshal([]byte(`{"question":"test","dataset_ids":["kb"],`+tc.fields+`}`), &req))
			body, err := RetrievalToSearch(req)
			if tc.wantBlocked {
				require.ErrorIs(t, err, ErrRetrievalGraphScopeConflict)
				require.Equal(t, 400, StatusCode(err))
				require.Nil(t, body)
			} else {
				require.NoError(t, err)
				if value, present := req["meta_data_filter"]; present {
					require.Equal(t, value, body["meta_data_filter"])
				}
			}
		})
	}
}
