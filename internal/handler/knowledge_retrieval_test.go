package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeRetrievalStableRouteUsesDatasetSearch(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/api/v1/datasets/kb/search", r.URL.Path)
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if body["meta_data_filter"] != nil {
			require.Equal(t, []any{"d"}, body["doc_ids"])
			filter := body["meta_data_filter"].(map[string]any)
			require.Equal(t, "=", filter["manual"].([]any)[0].(map[string]any)["op"])
			_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"chunks":[],"doc_aggs":[]}}`))
			return
		}
		require.Equal(t, []any{"kb", "kb2"}, body["dataset_ids"])
		require.Equal(t, float64(3), body["page"])
		require.Equal(t, float64(5), body["size"])
		require.NotContains(t, body, "document_ids")
		require.NotContains(t, body, "metadata_condition")
		require.NotNil(t, body["reference_metadata"])
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":14,"chunks":[{"chunk_id":"c","doc_id":"d","docnm_kwd":"name","content_with_weight":"text","document_metadata":{"version":"v2"}}],"doc_aggs":[]}}`))
	}))
	defer upstream.Close()
	router := gin.New()
	group := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	call := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/admin/knowledge/retrieval", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w
	}
	w := call(`{"question":"test","dataset_ids":["kb","kb2"],"document_ids":[],"metadata_condition":{},"page":3,"page_size":5,"reference_metadata":{"include":true}}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"document_metadata":{"version":"v2"}`)
	require.Contains(t, w.Body.String(), `"document_id":"d"`)
	require.Equal(t, 1, calls)
	w = call(`{"question":"test","dataset_ids":["kb"],"doc_ids":[]}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"total":0`)
	require.Equal(t, 1, calls, "empty scope must not reach upstream")
	w = call(`{"question":"test","dataset_ids":["kb"],"doc_ids":["d"],"meta_data_filter":{"method":"manual","manual":[{"key":"version","op":"is","value":"v2"}]}}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"total":0`)
	require.Equal(t, 2, calls, "verified filters must reach dataset search together")
	for _, body := range []string{
		`{"question":"test","dataset_ids":["kb"],"doc_ids":["d"],"use_kg":true}`,
		`{"question":"test","dataset_ids":["kb"],"meta_data_filter":{"method":"auto"},"use_kg":true}`,
	} {
		w = call(body)
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"code":"knowledge_graph_scope_conflict"`)
	}
	require.Equal(t, 2, calls, "graph expansion must not bypass scoped retrieval")
}

func TestKnowledgeRetrievalGraphConstraintsDoNotReachUpstream(t *testing.T) {
	calls := 0
	var got map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/v1/datasets/kb/search", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{"code":0,"data":{"total":0,"chunks":[],"doc_aggs":[]}}`))
	}))
	defer upstream.Close()
	router := gin.New()
	group := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"question":"test","dataset_ids":["kb"],"search_id":"saved","use_kg":true}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"search_id":"saved","use_kg":true,"meta_data_filter":{}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"search_id":"saved","use_kg":true,"meta_data_filter":{"method":"manual","manual":[]}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"search_id":" ","use_kg":true}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"meta_data_filter":{"method":"manual","manual":[]}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"meta_data_filter":{"method":"semi_auto","semi_auto":[]}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"meta_data_filter":{"unused":true}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"metadata_condition":{"conditions":[]}}`, 400},
		{`{"question":"test","dataset_ids":["kb"],"search_id":"saved","use_kg":false}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"search_id":"","use_kg":true}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"search_id":null,"use_kg":true}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"meta_data_filter":{}}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"meta_data_filter":null}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":true,"metadata_condition":{}}`, 200},
		{`{"question":"test","dataset_ids":["kb"],"use_kg":false,"meta_data_filter":{"method":"manual","manual":[]}}`, 200},
	} {
		t.Run(tc.body, func(t *testing.T) {
			before := calls
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/v1/admin/knowledge/retrieval", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			if tc.status == 400 {
				require.Contains(t, w.Body.String(), `"code":"knowledge_graph_scope_conflict"`)
				require.Equal(t, before, calls, "KG constraints must be rejected before upstream")
			} else {
				require.Equal(t, before+1, calls)
				var sent map[string]any
				require.NoError(t, json.Unmarshal([]byte(tc.body), &sent))
				require.Equal(t, sent["search_id"], got["search_id"])
				if value, present := sent["meta_data_filter"]; present {
					require.Equal(t, value, got["meta_data_filter"])
				}
			}
		})
	}
}
