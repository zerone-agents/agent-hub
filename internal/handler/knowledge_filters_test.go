package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeMetadataDocumentFiltersAndGraphRoutes(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		require.Equal(t, "GET", r.Method)
		switch r.URL.Path {
		case "/api/v1/datasets/metadata/keys":
			require.Equal(t, "kb,kb2", r.URL.Query().Get("dataset_ids"))
			_, _ = w.Write([]byte(`{"code":0,"data":["version"]}`))
		case "/api/v1/datasets/metadata/flattened":
			require.Equal(t, "kb,kb2", r.URL.Query().Get("dataset_ids"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"version":{"v2":["d"]}}}`))
		case "/api/v1/datasets/kb/documents":
			query := r.URL.Query()
			require.Equal(t, []string{"d", "d2"}, query["ids"])
			require.Equal(t, []string{"pdf", "doc"}, query["types"])
			require.JSONEq(t, `{"version":["v2"]}`, query.Get("metadata"))
			require.Equal(t, "false", query.Get("return_empty_metadata"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"total":1,"docs":[{"id":"d","run":"DONE","chunk_count":2}]}}`))
		case "/api/v1/datasets/kb/graph":
			require.Equal(t, "d", r.URL.Query().Get("doc_id"))
			_, _ = w.Write([]byte(`{"code":0,"data":{"graph":{"nodes":[]},"mind_map":{}}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.String())
		}
	}))
	defer upstream.Close()
	router := gin.New()
	group := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	call := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/knowledge"+path, nil))
		return w
	}
	for _, path := range []string{
		"/datasets/metadata/keys?dataset_ids=kb,kb2",
		"/datasets/metadata/flattened?dataset_ids=kb&dataset_ids=kb2",
		"/datasets/kb/documents?ids=d&ids=d2&types=pdf&types=doc&return_empty_metadata=false&metadata=" + url.QueryEscape(`{"version":["v2"]}`),
		"/datasets/kb/graph?doc_id=d",
	} {
		w := call(path)
		require.Equal(t, 200, w.Code, path+" "+w.Body.String())
		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Equal(t, true, response["success"])
		require.NotNil(t, response["data"])
	}
	require.Equal(t, 4, calls)
	for _, path := range []string{
		"/datasets/metadata/keys",
		"/datasets/metadata/flattened?dataset_ids=kb,",
		"/datasets/kb/documents?id=d&ids=d2",
		"/datasets/kb/documents?metadata=[]",
		"/datasets/kb/documents?return_empty_metadata=maybe",
		"/datasets/kb/graph?doc_id=",
		"/datasets/kb/graph?doc_id=d&doc_id=d2",
	} {
		w := call(path)
		require.Equal(t, 400, w.Code, path+" "+w.Body.String())
	}
	require.Equal(t, 4, calls, "invalid filters must not reach upstream")
}

func TestKnowledgeConnectorDeletionSyncPassesAndReadsBack(t *testing.T) {
	var persisted map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/connectors/c", r.URL.Path)
		if r.Method == "PATCH" {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&persisted))
			require.Equal(t, true, persisted["config"].(map[string]any)["sync_deleted_files"])
		} else {
			require.Equal(t, "GET", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": persisted})
	}))
	defer upstream.Close()
	router := gin.New()
	group := router.Group("/api/v1/admin")
	RegisterKnowledgeRoutes(group, group, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	for _, method := range []string{"PATCH", "GET"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/v1/admin/knowledge/datasets/kb/sources/c", strings.NewReader(`{"config":{"sync_deleted_files":true}}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"sync_deleted_files":true`)
	}
}
