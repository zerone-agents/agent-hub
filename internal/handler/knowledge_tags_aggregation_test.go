package handler

import (
	"control-panel/internal/application/services"
	remote "control-panel/internal/infrastructure/knowledge"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKnowledgeTagsAggregationStaticRouteExplicitScopeAndEmptyShape(t *testing.T) {
	for _, data := range []string{`[]`, `[{"value":"alpha","count":2},{"value":"beta","count":0}]`} {
		calls := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			require.Equal(t, http.MethodGet, r.Method)
			require.Equal(t, "/api/v1/datasets/tags/aggregation", r.URL.Path)
			require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
			require.Equal(t, []string{"kb1,kb2"}, r.URL.Query()["dataset_ids"])
			require.Len(t, r.URL.Query(), 1)
			_, _ = w.Write([]byte(`{"code":0,"data":` + data + `}`))
		}))
		router := setupKnowledgeRouter(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/tags/aggregation?dataset_ids=kb1,kb2&dataset_ids=kb1&all=true&tenant_id=foreign", nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var response struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.True(t, response.Success)
		require.JSONEq(t, data, string(response.Data))
		require.Equal(t, 1, calls)
		upstream.Close()
	}
}

func TestKnowledgeTagsAggregationRejectsInvalidScopeBeforeRemoteIO(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	router := setupKnowledgeRouter(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour))
	for _, query := range []string{"", "?dataset_ids=", "?dataset_ids=kb1,,kb2", "?dataset_ids=kb1&dataset_ids=", "?dataset_ids=kb%2Fother", "?dataset_ids=%5B%22kb1%22%5D", "?dataset_ids%5B%5D=kb1"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/tags/aggregation"+query, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"success":false`)
	}
	require.Zero(t, calls)
}

func TestKnowledgeTagsAggregationUpstreamDenialPreservesBusinessCodeNoPartialTags(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "kb1,foreign", r.URL.Query().Get("dataset_ids"))
		_, _ = w.Write([]byte(`{"code":102,"message":"No authorization for dataset foreign","data":[{"value":"private","count":42}]}`))
	}))
	defer upstream.Close()
	router := setupKnowledgeRouter(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/datasets/tags/aggregation?dataset_ids=kb1,foreign", nil))
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Contains(t, w.Body.String(), `"upstream_code":102`)
	require.NotContains(t, w.Body.String(), "private")
	require.NotContains(t, w.Body.String(), `"count":42`)
	require.Equal(t, 1, calls)
}

func TestKnowledgeTagsAggregationUsesExistingReadAuthorization(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"code":0,"data":[]}`)) }))
	defer upstream.Close()
	router := gin.New()
	read := router.Group("/api/v1/admin", func(c *gin.Context) {
		if c.GetHeader("X-Test-Role") != "reader" {
			c.AbortWithStatus(http.StatusForbidden)
		}
	})
	write := router.Group("/api/v1/admin", func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })
	RegisterKnowledgeRoutes(write, read, NewKnowledgeHandler(services.NewKnowledgeService(remote.NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour), nil), nil))
	path := "/api/v1/admin/knowledge/datasets/tags/aggregation?dataset_ids=kb1"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Zero(t, calls)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Test-Role", "reader")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls)
}
