package knowledge

import (
	"bytes"
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

func (c *RemoteMultiragEngine) KnowledgePage(ctx context.Context, r domain.PageRequest) (any, error) {
	base := "/api/v1/datasets/" + url.PathEscape(r.DatasetID)
	method, path := "", ""
	var typedBatch *typedBatchPlan
	switch r.Operation {
	case "documents-filter":
		return c.documentFilters(ctx, r)
	case "index-cancel":
		return c.cancelDatasetIndex(ctx, r)
	case "tags-aggregation":
		query, err := domain.TagsAggregationDatasetsQuery(r.Query)
		if err != nil {
			return nil, err
		}
		r.Query = query
		method, path = http.MethodGet, "/api/v1/datasets/tags/aggregation"
	case "metadata-keys", "metadata-flattened":
		query, err := domain.MetadataDatasetsQuery(r.Query)
		if err != nil {
			return nil, err
		}
		r.Query = query
		method, path = http.MethodGet, "/api/v1/datasets/metadata/keys"
		if r.Operation == "metadata-flattened" {
			path = "/api/v1/datasets/metadata/flattened"
		}
	case "create-empty", "create-web":
		method, path = http.MethodPost, base+"/documents"
	case "document-status":
		method, path = http.MethodPost, base+"/documents/batch-update-status"
	case "metadata-summary":
		method, path = http.MethodGet, base+"/metadata/summary"
	case "document-metadata-config":
		if strings.TrimSpace(r.DocumentID) == "" {
			return nil, domain.NewBadRequestError("documentId 不能为空")
		}
		metadata, err := domain.DocumentMetadataTemplate(r.Body)
		if err != nil {
			return nil, err
		}
		r.Body = domain.Object{"metadata": metadata}
		method, path = http.MethodPut, base+"/documents/"+url.PathEscape(r.DocumentID)+"/metadata/config"
	case "document-metadatas":
		if err := domain.ValidateMetadataBatchMembership(r.Body); err != nil {
			return nil, err
		}
		var err error
		typedBatch, err = c.prepareTypedBatch(ctx, r)
		if err != nil {
			return nil, err
		}
		method, path = http.MethodPatch, base+"/documents/metadatas"
	case "metadata-config-get":
		method, path = http.MethodGet, base+"/metadata/config"
	case "metadata-config-put":
		method, path = http.MethodPut, base+"/metadata/config"
	case "tags-list":
		method, path = http.MethodGet, base+"/tags"
	case "tags-rename":
		method, path = http.MethodPut, base+"/tags"
	case "tags-delete":
		method, path = http.MethodDelete, base+"/tags"
	case "ingestions-summary":
		method, path = http.MethodGet, base+"/ingestions/summary"
	case "ingestions-list":
		method, path = http.MethodGet, base+"/ingestions"
	case "ingestions-detail":
		method, path = http.MethodGet, base+"/ingestions/"+url.PathEscape(r.LogID)
	case "index-get", "index-run", "index-delete":
		kind := r.Query.Get("type")
		if kind != "graph" && kind != "raptor" && kind != "mindmap" {
			return nil, domain.NewBadRequestError("索引类型无效")
		}
		path = base + "/index"
		method = http.MethodGet
		if r.Operation == "index-run" {
			method = http.MethodPost
		}
		if r.Operation == "index-delete" {
			method = http.MethodDelete
		}
	case "graph-search":
		method, path = http.MethodGet, base+"/graph/search"
	case "graph-get":
		if values, present := r.Query["doc_id"]; present && (len(values) != 1 || strings.TrimSpace(values[0]) == "") {
			return nil, domain.NewBadRequestError("doc_id 必须是单个非空文档ID")
		}
		method, path = http.MethodGet, base+"/graph"
	case "connectors-linked":
		method, path = http.MethodGet, base+"/connectors"
	case "connectors-list":
		method, path = http.MethodGet, "/api/v1/connectors"
	case "connectors-create":
		method, path = http.MethodPost, "/api/v1/connectors"
	case "connector-get", "connector-update", "connector-delete", "connector-logs", "connector-resume", "connector-rebuild", "connector-link", "connector-unlink":
		if strings.TrimSpace(r.ConnectorID) == "" {
			return nil, domain.NewBadRequestError("connectorId 不能为空")
		}
		path = "/api/v1/connectors/" + url.PathEscape(r.ConnectorID)
		method = http.MethodGet
		switch r.Operation {
		case "connector-update":
			method = http.MethodPatch
		case "connector-delete":
			method = http.MethodDelete
		case "connector-logs":
			path += "/logs"
		case "connector-resume":
			path += "/resume"
			method = http.MethodPost
		case "connector-rebuild":
			path += "/rebuild"
			method = http.MethodPost
			// The KB is taken from the route, never from a browser-supplied body.
			r.Body = domain.Object{"kb_id": r.DatasetID}
		case "connector-link":
			path = base + "/connectors/" + url.PathEscape(r.ConnectorID)
			method = http.MethodPut
		case "connector-unlink":
			path = base + "/connectors/" + url.PathEscape(r.ConnectorID)
			method = http.MethodDelete
		}

	case "oauth-start", "oauth-result":
		if r.OAuthProvider != "google" && r.OAuthProvider != "box" {
			return nil, domain.NewBadRequestError("OAuth服务无效")
		}
		if r.OAuthProvider == "google" {
			source := r.Query.Get("source")
			if source != "google-drive" && source != "gmail" {
				return nil, domain.NewBadRequestError("Google数据源类型无效")
			}
		}
		if r.Operation == "oauth-result" {
			flow, _ := r.Body["flow_id"].(string)
			if strings.TrimSpace(flow) == "" {
				return nil, domain.NewBadRequestError("授权会话不能为空")
			}
		}
		action := "start"
		if r.Operation == "oauth-result" {
			action = "result"
		}
		method, path = http.MethodPost, "/api/v1/connectors/"+r.OAuthProvider+"/oauth/web/"+action

	default:
		return nil, domain.NewBadRequestError("不支持的知识库操作")
	}
	query := r.Query
	if query == nil {
		query = url.Values{}
	}
	var body io.Reader
	contentType := "application/json"
	if r.Operation == "create-web" {
		name, _ := r.Body["name"].(string)
		target, _ := r.Body["url"].(string)
		u, err := url.Parse(target)
		if strings.TrimSpace(name) == "" || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, domain.NewBadRequestError("请输入名称与有效的 HTTP/HTTPS 地址")
		}
		var b bytes.Buffer
		form := multipart.NewWriter(&b)
		if err := form.WriteField("name", name); err != nil {
			return nil, err
		}
		if err := form.WriteField("url", target); err != nil {
			return nil, err
		}
		if err := form.Close(); err != nil {
			return nil, err
		}
		body = &b
		contentType = form.FormDataContentType()
		query.Set("type", "web")
	} else {
		if r.Operation == "create-empty" {
			name, _ := r.Body["name"].(string)
			if strings.TrimSpace(name) == "" {
				return nil, domain.NewBadRequestError("文档名称不能为空")
			}
			query.Set("type", "empty")
		}
		if r.Operation == "document-status" {
			ids, ok := r.Body["doc_ids"].([]any)
			status, _ := r.Body["status"].(string)
			if !ok || len(ids) == 0 || (status != "0" && status != "1") {
				return nil, domain.NewBadRequestError("文档ID及启停状态无效")
			}
			for _, id := range ids {
				s, ok := id.(string)
				if !ok || strings.TrimSpace(s) == "" {
					return nil, domain.NewBadRequestError("文档ID不能为空")
				}
			}
		}
		if r.Body != nil {
			data, err := json.Marshal(r.Body)
			if err != nil {
				return nil, domain.NewBadRequestError("请求内容无效")
			}
			body = bytes.NewReader(data)
		}
	}
	req, err := c.newRequest(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	requestClient := c.client
	if r.Operation == "create-web" {
		requestClient = c.uploadClient
	}
	resp, err := requestClient.Do(req)
	if err != nil {
		return nil, domain.NewUpstreamError("知识库操作未完成，请刷新后重试", nil)
	}
	defer resp.Body.Close()
	var env envelope
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&env) != nil {
		return nil, domain.NewUpstreamError("知识库服务响应异常", nil)
	}
	code, ok := env.statusCode()
	if !ok {
		return nil, domain.NewUpstreamError("知识库服务响应缺少业务状态", nil)
	}
	if r.Operation == "document-status" {
		var items map[string]map[string]any
		if json.Unmarshal(env.Data, &items) != nil || len(items) == 0 {
			return nil, domain.NewUpstreamError("批量启停未完成，请刷新后重试", nil)
		}
		result := domain.DocumentStatusResult{Succeeded: []string{}, Failed: []string{}}
		for _, value := range r.Body["doc_ids"].([]any) {
			id := value.(string)
			item := items[id]
			if item != nil && item["error"] == nil && item["status"] == r.Body["status"] {
				result.Succeeded = append(result.Succeeded, id)
			} else {
				result.Failed = append(result.Failed, id)
			}
		}
		// Non-zero is permitted only for the pinned endpoint's explicit partial
		// result. Missing/error acknowledgements always stay in the retry selection.
		if code != 0 && len(result.Failed) == 0 {
			return nil, domain.NewUpstreamError("批量启停业务状态异常", nil)
		}
		return result, nil
	}
	if r.Operation == "oauth-result" && code == 106 {
		return map[string]any{"pending": true}, nil
	}
	if r.Operation == "tags-aggregation" && env.Code != nil && env.RetCode != nil && *env.Code != *env.RetCode {
		return nil, domain.NewUpstreamError("标签聚合响应的业务状态不一致", nil)
	}
	if code != 0 {
		if r.Operation == "tags-aggregation" {
			return nil, domain.NewTagAggregationUpstreamError(code)
		}
		return nil, domain.NewUpstreamError("知识库操作失败，请检查配置后重试", nil)
	}
	if r.Operation == "tags-aggregation" {
		items, err := decodeTagsAggregation(env.Data)
		if err != nil {
			return nil, err
		}
		return items, nil
	}
	var data any
	if len(env.Data) > 0 {
		if json.Unmarshal(env.Data, &data) != nil {
			return nil, domain.NewUpstreamError("知识库返回内容无效", nil)
		}
	}
	if r.Operation == "document-metadatas" {
		if data == false {
			return nil, domain.NewUpstreamError("批量元数据更新未确认，请刷新核对", nil)
		}
		if err := c.confirmTypedBatch(ctx, r, typedBatch, data); err != nil {
			return nil, err
		}
	}
	if r.Operation == "document-metadata-config" {
		if data == false {
			return nil, domain.NewUpstreamError("文档元数据模板保存未确认，请刷新核对", nil)
		}
		persisted, err := c.readDocument(ctx, r.DatasetID, r.DocumentID)
		if err != nil {
			return nil, domain.DocumentUpdateUnknown("文档元数据模板保存已受理，但回读未确认；请刷新核对")
		}
		expected := r.Body["metadata"]
		if fields, ok := expected.([]any); ok && len(fields) > 0 {
			expected = documentMetadataSchema(fields)
		}
		config, _ := (*persisted)["parser_config"].(map[string]any)
		if !jsonEqualDocumentValue(config["metadata"], expected) {
			return nil, domain.DocumentUpdateUnknown("文档元数据模板回读不一致；请刷新核对")
		}
	}
	if r.Operation == "metadata-config-put" {
		persisted, err := c.KnowledgePage(ctx, domain.PageRequest{Operation: "metadata-config-get", DatasetID: r.DatasetID})
		if err != nil || !jsonContains(persisted, map[string]any(r.Body)) {
			return nil, domain.NewUpstreamError("元数据配置回读未确认，请重试", nil)
		}
	}
	if method != http.MethodGet && data == false {
		return nil, domain.NewUpstreamError("知识库操作未确认，请刷新后重试", nil)
	}
	return data, nil
}

func decodeTagsAggregation(data json.RawMessage) ([]domain.TagAggregationItem, error) {
	var rows []struct {
		Value *string `json:"value"`
		Count *int64  `json:"count"`
	}
	if json.Unmarshal(data, &rows) != nil || rows == nil {
		return nil, domain.NewUpstreamError("标签聚合响应必须为标签计数数组", nil)
	}
	items := make([]domain.TagAggregationItem, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if row.Value == nil || row.Count == nil || *row.Count < 0 || seen[*row.Value] {
			return nil, domain.NewUpstreamError("标签聚合响应的标签或计数无效", nil)
		}
		seen[*row.Value] = true
		items = append(items, domain.TagAggregationItem{Value: *row.Value, Count: *row.Count})
	}
	return items, nil
}
