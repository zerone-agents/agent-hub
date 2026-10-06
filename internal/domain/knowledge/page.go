package knowledge

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// PageEngine extends the knowledge page without changing the chat/MCP engine
// contract. Operations are resolved by a fixed allowlist in the remote adapter.
type PageEngine interface {
	KnowledgePage(ctx context.Context, req PageRequest) (any, error)
}
type PageRequest struct {
	Operation     string
	OAuthProvider string
	DatasetID     string
	DocumentID    string
	ConnectorID   string
	LogID         string
	Query         url.Values
	Body          Object
}
type DocumentStatusResult struct {
	Succeeded []string `json:"succeeded"`
	Failed    []string `json:"failed"`
}

type DocumentFilterResult struct {
	Total  int                  `json:"total"`
	Filter DocumentFilterCounts `json:"filter"`
}

type DocumentFilterCounts struct {
	Suffix    map[string]int            `json:"suffix"`
	RunStatus map[string]int            `json:"run_status"`
	Metadata  map[string]map[string]int `json:"metadata"`
}

type IndexCancelResult struct {
	TaskID          string `json:"task_id"`
	RequestAccepted bool   `json:"request_accepted"`
	CancelRequested bool   `json:"cancel_requested"`
	Task            Object `json:"task"`
}

// MetadataDatasetsQuery folds repeated selections into MultiRAG's CSV string
// contract. Reject missing/empty members instead of silently dropping scope.
func MetadataDatasetsQuery(query url.Values) (url.Values, error) {
	ids := []string{}
	for _, value := range query["dataset_ids"] {
		for _, id := range strings.Split(value, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil, NewBadRequestError("dataset_ids 必须是非空知识库ID列表")
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, NewBadRequestError("dataset_ids 不能为空")
	}
	return url.Values{"dataset_ids": []string{strings.Join(ids, ",")}}, nil
}

// The dataset ID storage contract is String(32). Keep IDs as opaque, safe
// tokens and deduplicate explicit selections, never default to every dataset.
var tagAggregationDatasetID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func TagsAggregationDatasetsQuery(query url.Values) (url.Values, error) {
	ids := []string{}
	seen := map[string]bool{}
	for _, value := range query["dataset_ids"] {
		for _, id := range strings.Split(value, ",") {
			id = strings.TrimSpace(id)
			if !tagAggregationDatasetID.MatchString(id) {
				return nil, NewBadRequestError("dataset_ids 必须为非空知识库ID列表；每个ID限32位字母、数字、下划线或连字符")
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	if len(ids) == 0 {
		return nil, NewBadRequestError("dataset_ids 不能为空，不能省略所选知识库范围")
	}
	return url.Values{"dataset_ids": []string{strings.Join(ids, ",")}}, nil
}

type TagAggregationItem struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Preserve the upstream business status without exposing raw service errors
// or changing the Hub's existing string error-code convention.
type TagAggregationUpstreamError struct {
	UpstreamCode int
	cause        *Error
}

func NewTagAggregationUpstreamError(code int) *TagAggregationUpstreamError {
	return &TagAggregationUpstreamError{UpstreamCode: code, cause: NewUpstreamError("跨知识库标签聚合失败，请核对所选知识库权限后重试", nil)}
}
func (e *TagAggregationUpstreamError) Error() string { return e.cause.Error() }
func (e *TagAggregationUpstreamError) Unwrap() error { return e.cause }
