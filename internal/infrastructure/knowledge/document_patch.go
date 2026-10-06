package knowledge

import (
	"bytes"
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func (c *RemoteMultiragEngine) readDocument(ctx context.Context, datasetID, documentID string) (*domain.Document, error) {
	result, err := c.ListDocuments(ctx, datasetID, domain.DocumentListRequest{ID: documentID, Page: 1, PageSize: 1})
	if err != nil {
		return nil, err
	}
	if result.Total != 1 || len(result.Documents) != 1 || result.Documents[0]["id"] != documentID {
		return nil, domain.NewNotFoundError("本知识库中未找到唯一文档，请刷新核对")
	}
	doc := result.Documents[0]
	for _, key := range []string{"kb_id", "dataset_id"} {
		if id, present := doc[key]; present && id != datasetID {
			return nil, domain.NewUpstreamError("文档回读的知识库归属不一致", nil)
		}
	}
	return &doc, nil
}

func (c *RemoteMultiragEngine) updateDocumentConfirmed(ctx context.Context, datasetID, documentID string, req domain.DocumentUpdateRequest) (*domain.Document, error) {
	body, err := domain.PrepareDocumentPatch(req)
	if err != nil {
		return nil, err
	}
	before, err := c.readDocument(ctx, datasetID, documentID)
	if err != nil {
		return nil, err
	}
	// Old editors sent the retained builtin on every save. Leaving an existing
	// pipeline for that same builtin requires an explicit empty pipeline_id.
	if parser, present := req["parser_id"]; present && parser == (*before)["parser_id"] && (*before)["pipeline_id"] != nil && (*before)["pipeline_id"] != "" {
		if _, explicit := body["pipeline_id"]; !explicit {
			delete(body, "chunk_method")
		}
	}
	if config, ok := body["parser_config"].(map[string]any); ok {
		stored, _ := (*before)["parser_config"].(map[string]any)
		// List reads project a stored legacy metadata array into JSON Schema.
		// Send complete schemas atomically. Never fill a partial edit from our
		// initial read: another editor may change those omitted fields before
		// the upstream transaction, and we cannot supply a revision precondition.
		// Complete caller-supplied schemas retain upstream's last-write semantics
		// for explicitly submitted fields; they do not provide conflict detection.
		if metadata, ok := config["metadata"].(map[string]any); ok && len(metadata) > 0 {
			old, _ := stored["metadata"].(map[string]any)
			_, properties := metadata["properties"].(map[string]any)
			if metadata["type"] != "object" || !properties || !completeDocumentSchema(old, metadata) {
				return nil, domain.NewBadRequestError("文档 PATCH 不支持局部元数据 Schema；请提交完整模板或使用元数据模板接口")
			}
		}
		body["parser_config"] = changedDocumentConfig(stored, config)
		if len(body["parser_config"].(map[string]any)) == 0 {
			delete(body, "parser_config")
		}
	}
	if len(body) == 0 {
		return before, nil
	}
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID)
	if err := c.patchDocument(ctx, path, body); err != nil {
		return nil, err
	}
	after, err := c.readDocument(ctx, datasetID, documentID)
	if err != nil {
		return nil, domain.DocumentUpdateUnknown("文档保存已受理，但回读未确认；请刷新核对后再重试")
	}
	expected := domain.NormalizeDocument(body)
	// Mode is preserved on config/name/metadata-only patches. An explicit
	// builtin switches out of a pipeline; all mode fields are checked on GET.
	parser := (*before)["parser_id"]
	pipeline := (*before)["pipeline_id"]
	if selected, present := body["chunk_method"]; present {
		parser, pipeline = selected, ""
	}
	if selected, present := body["pipeline_id"]; present {
		pipeline = selected
	}
	if parser != nil {
		expected["parser_id"] = parser
	}
	if pipeline != nil {
		expected["pipeline_id"] = pipeline
	}
	// MultiRAG normalizes enabled into numeric status on list readback.
	actual := domain.CloneObject(map[string]any(*after))
	if expected["pipeline_id"] == "" && actual["pipeline_id"] == nil {
		actual["pipeline_id"] = ""
	}
	if _, present := body["enabled"]; present && actual["enabled"] == nil {
		if status, present := actual["status"]; present {
			actual["enabled"] = status == "1"
		}
	}
	if n, ok := expected["enabled"].(float64); ok {
		expected["enabled"] = n == 1
	}
	if n, ok := expected["enabled"].(int); ok {
		expected["enabled"] = n == 1
	}
	if fields, present := body["meta_fields"]; present {
		wanted, _ := fields.(map[string]any)
		a, _ := json.Marshal(actual["meta_fields"])
		b, _ := json.Marshal(canonicalDocumentMetadata(wanted))
		if !bytes.Equal(a, b) {
			return nil, domain.DocumentUpdateUnknown("文档元数据保存回读不一致；请刷新核对后再重试")
		}
		delete(expected, "meta_fields")
	}
	// Disabling parent-child deliberately clears its delimiter upstream.
	if config, ok := expected["parser_config"].(map[string]any); ok {
		config = domain.CloneObject(config)
		if fields, ok := config["metadata"].([]any); ok && len(fields) > 0 {
			config["metadata"] = documentMetadataSchema(fields)
		}
		if metadata, present := config["metadata"]; present {
			actualConfig, _ := actual["parser_config"].(map[string]any)
			if !jsonEqualDocumentValue(actualConfig["metadata"], metadata) {
				return nil, domain.DocumentUpdateUnknown("文档元数据模板回读不一致；请刷新核对后再重试")
			}
		}
		pc, _ := config["parent_child"].(map[string]any)
		if pc["use_parent_child"] == false || config["enable_children"] == false {
			config = domain.CloneObject(config)
			config["parent_child"] = map[string]any{}
			config["enable_children"] = false
			config["children_delimiter"] = ""
		}
		expected["parser_config"] = config
	}
	if !jsonContains(actual, map[string]any(expected)) {
		return nil, domain.DocumentUpdateUnknown("文档保存回读与请求不一致；请刷新核对后再重试")
	}
	return after, nil
}

var documentMetadataSeparator = regexp.MustCompile(`[、,，;；|]+`)

func canonicalDocumentMetadata(fields map[string]any) map[string]any {
	result := domain.CloneObject(fields)
	for key, value := range fields {
		items, ok := value.([]any)
		if !ok {
			continue
		}
		normalized := []any{}
		seen := map[string]bool{}
		appendValue := func(item any) {
			id := fmt.Sprint(item)
			if flag, ok := item.(bool); ok {
				if flag {
					id = "True"
				} else {
					id = "False"
				}
			}
			if !seen[id] {
				seen[id] = true
				normalized = append(normalized, item)
			}
		}
		for _, item := range items {
			if text, ok := item.(string); ok {
				found := false
				for _, part := range documentMetadataSeparator.Split(strings.TrimSpace(text), -1) {
					if part = strings.TrimSpace(part); part != "" {
						found = true
						appendValue(part)
					}
				}
				if !found {
					appendValue(item)
				}
			} else {
				appendValue(item)
			}
		}
		result[key] = normalized
	}
	return result
}

// The document list projects legacy extraction fields with MultiRAG's
// field_schema(). Preserve extension constraints and object/boolean item
// schemas while consuming only the UI control fields. [] remains a clear.
func documentMetadataSchema(fields []any) map[string]any {
	if len(fields) == 0 {
		return map[string]any{}
	}
	properties := map[string]any{}
	for _, item := range fields {
		field, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := field["key"].(string)
		if key == "" {
			key, _ = field["name"].(string)
		}
		description, _ := field["description"].(string)
		if description == "" {
			description, _ = field["descriptions"].(string)
		}
		projection := domain.CloneObject(field)
		for _, control := range []string{"key", "name", "type", "description", "descriptions", "enum", "examples", "restrict_values"} {
			delete(projection, control)
		}
		projection["description"] = description
		kind, _ := field["type"].(string)
		values, _ := field["enum"].([]any)
		if _, present := field["enum"]; !present && field["restrict_values"] == true {
			values, _ = field["examples"].([]any)
		}
		if kind == "number" {
			values = numericDocumentMetadataValues(values)
		}
		if kind == "list" {
			items, present := projection["items"]
			if !present || (items == true && len(values) > 0) {
				items = map[string]any{}
			}
			if object, ok := items.(map[string]any); ok {
				// Clone before adding defaults/enum; request-owned items are shared
				// with the outgoing patch and must remain untouched.
				object = domain.CloneObject(object)
				if _, present := object["type"]; !present {
					object["type"] = "string"
				}
				if len(values) > 0 {
					object["enum"] = values
				}
				items = object
			}
			projection["type"], projection["items"] = "array", items
		} else {
			if kind == "time" {
				kind = "string"
			}
			if kind != "" {
				projection["type"] = kind
			}
			if len(values) > 0 {
				projection["enum"] = values
				if kind == "" {
					projection["type"] = "string"
				}
			}
		}
		if examples, present := field["examples"]; present && examples != nil && field["restrict_values"] != true {
			if kind == "number" {
				items, _ := examples.([]any)
				examples = numericDocumentMetadataValues(items)
			}
			if kind == "list" {
				items, _ := examples.([]any)
				wrapped := make([]any, 0, len(items))
				for _, item := range items {
					wrapped = append(wrapped, []any{item})
				}
				examples = wrapped
			}
			projection["examples"] = examples
		}
		properties[key] = projection
	}
	return map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
}

func changedDocumentConfig(stored, patch map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range patch {
		if key == "metadata" {
			if object, ok := value.(map[string]any); ok && len(object) == 0 {
				continue
			}
			if !jsonEqualDocumentValue(stored[key], value) {
				result[key] = value
			}
			continue
		}
		if nested, ok := value.(map[string]any); ok {
			old, _ := stored[key].(map[string]any)
			if changed := changedDocumentConfig(old, nested); len(changed) > 0 {
				result[key] = changed
			}
		} else if !jsonContains(stored[key], value) {
			result[key] = value
		}
	}
	return result
}

func jsonEqualDocumentValue(a, b any) bool {
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && bytes.Equal(aJSON, bJSON)
}

func completeDocumentSchema(stored, submitted map[string]any) bool {
	for key, value := range stored {
		incoming, present := submitted[key]
		if !present {
			return false
		}
		if object, ok := value.(map[string]any); ok {
			next, ok := incoming.(map[string]any)
			if !ok || !completeDocumentSchema(object, next) {
				return false
			}
		}
	}
	return true
}

func numericDocumentMetadataValues(values []any) []any {
	result := make([]any, len(values))
	for i, value := range values {
		if text, ok := value.(string); ok {
			if number, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil {
				value = number
			}
		}
		result[i] = value
	}
	return result
}

var documentRequestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (c *RemoteMultiragEngine) patchDocument(ctx context.Context, path string, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.NewBadRequestError("文档更新请求无法序列化")
	}
	req, err := c.newRequest(ctx, http.MethodPatch, path, nil, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return domain.DocumentUpdateUnknown("文档保存响应未确认；请刷新核对后再重试")
	}
	defer resp.Body.Close()
	var env struct {
		Code      json.RawMessage `json:"code"`
		RetCode   *int            `json:"retcode"`
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
		Details   struct {
			Outcome string `json:"outcome"`
		} `json:"details"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&env) != nil {
		return domain.DocumentUpdateUnknown("文档保存响应无效；请刷新核对后再重试")
	}
	var code int
	if env.RetCode != nil {
		code = *env.RetCode
	} else if json.Unmarshal(env.Code, &code) != nil {
		code = -1
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && code == 0 && string(env.Data) != "false" {
		return nil
	}
	var typed string
	_ = json.Unmarshal(env.Code, &typed)
	status, message := http.StatusBadGateway, "文档保存未确认；请刷新核对后再重试"
	switch typed {
	case "DOCUMENT_UPDATE_INVALID", "DOCUMENT_UPDATE_VALIDATION":
		status, message = http.StatusBadRequest, "文档字段、解析参数或来源限制不允许此修改"
	case "DOCUMENT_UPDATE_UNAVAILABLE":
		status, message = http.StatusNotFound, "文档或解析资源不可用，请刷新核对"
	case "DOCUMENT_UPDATE_FORBIDDEN", "DOCUMENT_UPDATE_UNAUTHORIZED":
		status, message = http.StatusForbidden, "没有修改文档的权限"
	case "DOCUMENT_UPDATE_CONFLICT":
		status, message = http.StatusConflict, "文档任务或配置已变化；请刷新核对后再重试"
	case "DOCUMENT_UPDATE_FAILED":
		message = "文档更新失败，请刷新后重试"
	case "DOCUMENT_UPDATE_OUTCOME_UNKNOWN":
	default:
		return domain.DocumentUpdateUnknown(message)
	}
	outcome := "unknown"
	if env.Details.Outcome == "unchanged" {
		outcome = "unchanged"
	}
	requestID := ""
	if documentRequestIDPattern.MatchString(env.RequestID) {
		requestID = env.RequestID
	}
	return &domain.DocumentUpdateError{Code: typed, Message: message, HTTPStatus: status, Outcome: outcome, RequestID: requestID}
}
