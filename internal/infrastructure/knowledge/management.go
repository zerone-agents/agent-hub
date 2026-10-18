package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func (c *RemoteMultiragEngine) documentFilters(ctx context.Context, r domain.PageRequest) (*domain.DocumentFilterResult, error) {
	// Aggregation is deliberately independent of pagination and metadata
	// predicates. Its candidates describe the current non-metadata selection.
	query := url.Values{"type": []string{"filter"}}
	for _, key := range []string{"keywords", "run", "run_status", "types", "suffix"} {
		if values, present := r.Query[key]; present {
			query[key] = append([]string(nil), values...)
		}
	}
	var raw struct {
		Total  *int                         `json:"total"`
		Filter *domain.DocumentFilterCounts `json:"filter"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/datasets/"+url.PathEscape(r.DatasetID)+"/documents", query, nil, &raw); err != nil {
		return nil, err
	}
	if raw.Total == nil || *raw.Total < 0 || raw.Filter == nil || raw.Filter.Suffix == nil || raw.Filter.RunStatus == nil || raw.Filter.Metadata == nil {
		return nil, domain.NewUpstreamError("文档筛选统计响应不完整，请重试", nil)
	}
	for _, counts := range []map[string]int{raw.Filter.Suffix, raw.Filter.RunStatus} {
		for _, count := range counts {
			if count < 0 {
				return nil, domain.NewUpstreamError("文档筛选统计无效", nil)
			}
		}
	}
	for _, counts := range raw.Filter.Metadata {
		if counts == nil {
			return nil, domain.NewUpstreamError("文档元数据筛选统计不完整", nil)
		}
		for _, count := range counts {
			if count < 0 {
				return nil, domain.NewUpstreamError("文档元数据筛选统计无效", nil)
			}
		}
	}
	return &domain.DocumentFilterResult{Total: *raw.Total, Filter: *raw.Filter}, nil
}

var indexTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func (c *RemoteMultiragEngine) cancelDatasetIndex(ctx context.Context, r domain.PageRequest) (*domain.IndexCancelResult, error) {
	kind := r.Query.Get("type")
	if len(r.Query["type"]) != 1 || (kind != "graph" && kind != "raptor" && kind != "mindmap") {
		return nil, domain.NewBadRequestError("索引类型无效")
	}
	id, ok := r.Body["task_id"].(string)
	if len(r.Body) != 1 || !ok || !indexTaskIDPattern.MatchString(id) {
		return nil, domain.NewBadRequestError("必须提供当前索引的 task_id")
	}
	query := url.Values{"type": []string{kind}}
	read := func() (domain.Object, error) {
		data, err := c.KnowledgePage(ctx, domain.PageRequest{Operation: "index-get", DatasetID: r.DatasetID, Query: query})
		if err != nil {
			return nil, err
		}
		task, ok := data.(map[string]any)
		if !ok {
			return nil, domain.NewUpstreamError("当前索引任务响应不完整", nil)
		}
		return domain.Object(task), nil
	}
	before, err := read()
	if err != nil {
		return nil, err
	}
	if before["id"] != id {
		return nil, &domain.Error{Kind: domain.ErrorKindBadRequest, HTTPStatus: http.StatusConflict, Message: "本库当前索引任务已变化；请刷新后再请求停止"}
	}
	progress, ok := before["progress"].(float64)
	if !ok {
		return nil, domain.NewUpstreamError("当前索引任务缺少进度状态，请刷新核对", nil)
	}
	if progress < 0 || progress >= 1 {
		return nil, &domain.Error{Kind: domain.ErrorKindBadRequest, HTTPStatus: http.StatusConflict, Message: "索引任务已结束，无需请求停止"}
	}
	// No arbitrary task route is exposed. The ID comes from a freshly read
	// dataset/type binding, and upstream cancellation also checks ownership.
	var acknowledged domain.Object
	pauseQuery := url.Values{"type": []string{kind}, "wipe": []string{"false"}, "task_id": []string{id}}
	if _, err := c.doJSON(ctx, http.MethodDelete, "/api/v1/datasets/"+url.PathEscape(r.DatasetID)+"/index", pauseQuery, nil, &acknowledged); err != nil {
		return nil, domain.NewUpstreamError("索引停止请求未确认；请刷新任务状态后重试", nil)
	}
	if acknowledged == nil {
		return nil, domain.NewUpstreamError("索引停止请求未被确认；请刷新任务状态", nil)
	}
	after, err := read()
	if err != nil || after["id"] != id {
		return nil, domain.NewUpstreamError("索引停止请求已受理，但状态回读未确认；请刷新任务状态", nil)
	}
	afterProgress, ok := after["progress"].(float64)
	if !ok {
		return nil, domain.NewUpstreamError("索引停止请求已受理，但回读缺少进度状态；请刷新任务状态", nil)
	}
	// The dataset pause route persists this exact acknowledgement while its
	// writer drains. A trace at -1 alone could be a natural failure. Neither
	// value proves that the worker has stopped or that a restart is ready.
	afterMessage, _ := after["progress_msg"].(string)
	recorded := afterProgress == -1 && afterMessage == "Pause requested; waiting for the graph writer to settle."
	return &domain.IndexCancelResult{TaskID: strings.TrimSpace(id), RequestAccepted: true, CancelRequested: recorded, Task: after}, nil
}
