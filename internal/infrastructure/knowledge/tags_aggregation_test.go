package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestTagsAggregationPinnedPathDeduplicatedCSVAndShape(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/datasets/tags/aggregation", r.URL.Path)
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		require.Equal(t, "dataset_ids=kb1%2Ckb2", r.URL.RawQuery)
		require.Equal(t, []string{"kb1,kb2"}, r.URL.Query()["dataset_ids"])
		_, _ = w.Write([]byte(`{"code":0,"data":[{"value":"alpha","count":2},{"value":"large","count":9007199254740993}]}`))
	}))
	defer upstream.Close()
	result, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), domain.PageRequest{Operation: "tags-aggregation", Query: url.Values{"dataset_ids": []string{" kb1,kb2 ", "kb1"}, "tenant_id": []string{"foreign"}, "all": []string{"true"}}})
	require.NoError(t, err)
	require.Equal(t, []domain.TagAggregationItem{{Value: "alpha", Count: 2}, {Value: "large", Count: 9007199254740993}}, result)
	require.Equal(t, 1, calls)
}

func TestTagsAggregationInvalidScopeNeverCallsRemote(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer upstream.Close()
	engine := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour)
	for _, query := range []url.Values{nil, {}, {"dataset_ids": []string{""}}, {"dataset_ids": []string{"kb1,,kb2"}}, {"dataset_ids": []string{"kb1", ""}}, {"dataset_ids": []string{"kb/other"}}, {"dataset_ids[]": []string{"kb1"}}} {
		_, err := engine.KnowledgePage(context.Background(), domain.PageRequest{Operation: "tags-aggregation", Query: query})
		require.Equal(t, 400, domain.StatusCode(err))
	}
	require.Zero(t, calls)
}

func TestTagsAggregationBusinessFailuresAndMalformedResultsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		body    string
		code    int
		allowed bool
	}{
		{`{"code":0,"data":[]}`, 0, true},
		{`{"retcode":0,"data":[{"value":"zero","count":0}]}`, 0, true},
		{`{"code":102,"message":"No authorization for dataset foreign","data":[{"value":"private","count":42}]}`, 102, false},
		{`{"retcode":109,"data":false}`, 109, false},
		{`{"code":0,"retcode":109,"data":[]}`, 0, false},
		{`{"data":[]}`, 0, false}, {`{"code":0,"data":null}`, 0, false}, {`{"code":0,"data":false}`, 0, false},
		{`{"code":0,"data":{}}`, 0, false}, {`{"code":0,"data":[["tag",2]]}`, 0, false},
		{`{"code":0,"data":[{"value":"tag"}]}`, 0, false}, {`{"code":0,"data":[{"count":2}]}`, 0, false},
		{`{"code":0,"data":[{"value":"tag","count":null}]}`, 0, false}, {`{"code":0,"data":[{"value":"tag","count":-1}]}`, 0, false},
		{`{"code":0,"data":[{"value":1,"count":2}]}`, 0, false}, {`{"code":0,"data":[{"value":"tag","count":0.5}]}`, 0, false},
		{`{"code":0,"data":[{"value":"tag","count":"2"}]}`, 0, false}, {`{"code":0,"data":[{"value":"tag","count":9223372036854775808}]}`, 0, false},
		{`{"code":0,"data":[{"value":"tag","count":2},{"value":"tag","count":3}]}`, 0, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer upstream.Close()
			result, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).KnowledgePage(context.Background(), domain.PageRequest{Operation: "tags-aggregation", Query: url.Values{"dataset_ids": []string{"kb1,foreign"}}})
			if tc.allowed {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.True(t, result == nil, "error returned a typed nil instead of nil")
				require.Equal(t, 502, domain.StatusCode(err))
			}
			if tc.code != 0 {
				var upstreamError *domain.TagAggregationUpstreamError
				require.ErrorAs(t, err, &upstreamError)
				require.Equal(t, tc.code, upstreamError.UpstreamCode)
				require.NotContains(t, err.Error(), "private")
			}
		})
	}
}
