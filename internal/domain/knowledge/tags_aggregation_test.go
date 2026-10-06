package knowledge

import (
	"github.com/stretchr/testify/require"
	"net/url"
	"strings"
	"testing"
)

func TestTagsAggregationQueryKeepsExplicitUniqueScope(t *testing.T) {
	query := url.Values{"dataset_ids": []string{" kb1,kb2 ", "kb1,kb_3", "kb-4"}, "tenant_id": []string{"foreign"}, "all": []string{"true"}}
	normalized, err := TagsAggregationDatasetsQuery(query)
	require.NoError(t, err)
	require.Equal(t, url.Values{"dataset_ids": []string{"kb1,kb2,kb_3,kb-4"}}, normalized)
	require.Equal(t, []string{" kb1,kb2 ", "kb1,kb_3", "kb-4"}, query["dataset_ids"])
	normalized, err = TagsAggregationDatasetsQuery(url.Values{"dataset_ids": []string{"kb1", "kb1"}})
	require.NoError(t, err)
	require.Equal(t, "kb1", normalized.Get("dataset_ids"))
}

func TestTagsAggregationQueryRejectsMissingEmptyOrMalformedIDs(t *testing.T) {
	for _, query := range []url.Values{
		nil, {}, {"dataset_ids": nil}, {"dataset_ids": []string{""}}, {"dataset_ids": []string{" , "}},
		{"dataset_ids": []string{"kb1,,kb2"}}, {"dataset_ids": []string{"kb1,"}}, {"dataset_ids": []string{"kb1", ""}},
		{"dataset_ids": []string{"[\"kb1\"]"}}, {"dataset_ids": []string{"kb/other"}}, {"dataset_ids": []string{".."}},
		{"dataset_ids": []string{"kb 1"}}, {"dataset_ids": []string{"*"}}, {"dataset_ids": []string{"kb1&tenant_id=other"}},
		{"dataset_ids": []string{"kb1\u001c"}}, {"dataset_ids": []string{strings.Repeat("a", 33)}}, {"dataset_ids[]": []string{"kb1"}},
	} {
		_, err := TagsAggregationDatasetsQuery(query)
		require.Error(t, err, query)
		require.Equal(t, 400, StatusCode(err))
	}
	_, err := TagsAggregationDatasetsQuery(url.Values{"dataset_ids": []string{strings.Repeat("a", 32)}})
	require.NoError(t, err)
}
