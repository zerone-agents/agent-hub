package knowledge

import (
	"fmt"
	"net/http"
)

// DocumentUpdateError exposes only feature-local, safe error material. Unknown
// outcomes require a readback before retrying a potentially committed write.
type DocumentUpdateError struct {
	Code       string
	Message    string
	Outcome    string
	RequestID  string
	HTTPStatus int
}

func (e *DocumentUpdateError) Error() string { return e.Message }

func DocumentUpdateUnknown(message string) *DocumentUpdateError {
	return &DocumentUpdateError{Code: "DOCUMENT_UPDATE_OUTCOME_UNKNOWN", Message: message, Outcome: "unknown", HTTPStatus: http.StatusBadGateway}
}

var documentParserConfigKeys = keySet(
	"auto_keywords", "auto_questions", "chunk_token_num", "delimiter", "graphrag", "html4excel", "layout_recognize",
	"parent_child", "enable_children", "children_delimiter", "raptor", "tag_kb_ids", "topn_tags", "filename_embd_weight",
	"task_page_size", "pages", "image_context_size", "table_context_size", "toc_extraction", "overlapped_percent",
	"mineru_parse_method", "mineru_formula_enable", "mineru_table_enable", "mineru_lang", "enable_metadata", "metadata",
	"built_in_metadata", "llm_id", "analyze_hyperlink", "hyperlink_urls", "video_prompt",
)

var documentNestedConfigKeys = map[string]map[string]struct{}{
	"raptor":       keySet("use_raptor", "prompt", "max_token", "threshold", "max_cluster", "random_seed", "auto_disable_for_structured_data", "scope"),
	"graphrag":     keySet("use_graphrag", "entity_types", "method", "community", "resolution"),
	"parent_child": keySet("use_parent_child", "children_delimiter"),
}

func keySet(keys ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		result[key] = struct{}{}
	}
	return result
}

// PrepareDocumentPatch accepts the Hub parser_id alias but never forwards a
// stored DTO wholesale. Historical config keys remain stored upstream; strict
// MultiRAG PATCH models only receive their current write fields. No defaults
// are injected, so explicit false, zero and [] retain their meaning.
func PrepareDocumentPatch(req DocumentUpdateRequest) (map[string]any, error) {
	if req == nil {
		return nil, NewBadRequestError("文档更新必须为JSON对象")
	}
	allowed := keySet("name", "parser_id", "chunk_method", "pipeline_id", "parser_config", "enabled", "meta_fields", "chunk_count", "chunk_num", "token_count", "token_num", "progress")
	for key, value := range req {
		if _, ok := allowed[key]; !ok {
			return nil, NewBadRequestError(fmt.Sprintf("不支持的文档更新字段: %s", key))
		}
		if value == nil {
			return nil, NewBadRequestError(key + " 不能为 null；未修改时请省略")
		}
	}
	if _, alias := req["parser_id"]; alias {
		if _, canonical := req["chunk_method"]; canonical {
			return nil, NewBadRequestError("parser_id 与 chunk_method 不能同时指定")
		}
	}
	for alias, canonical := range map[string]string{"chunk_num": "chunk_count", "token_num": "token_count"} {
		if _, present := req[alias]; present {
			if _, duplicate := req[canonical]; duplicate {
				return nil, NewBadRequestError(alias + " 与 " + canonical + " 不能同时指定")
			}
		}
	}
	for _, key := range []string{"name", "parser_id", "chunk_method", "pipeline_id"} {
		if value, present := req[key]; present {
			s, ok := value.(string)
			if !ok || (key != "pipeline_id" && s == "") {
				return nil, NewBadRequestError(key + " 必须是有效字符串")
			}
		}
	}
	if pipeline, _ := req["pipeline_id"].(string); pipeline != "" {
		if req["parser_id"] != nil || req["chunk_method"] != nil {
			return nil, NewBadRequestError("非空 pipeline_id 与内置解析器不能同时指定")
		}
	}
	body := DocumentUpdateToRemote(req)
	if value, present := req["parser_config"]; present {
		config, ok := value.(map[string]any)
		if !ok {
			return nil, NewBadRequestError("parser_config 必须是JSON对象")
		}
		filtered := sanitizeNestedParserConfig(config, documentParserConfigKeys)
		for key, keys := range documentNestedConfigKeys {
			if value, present := filtered[key]; present {
				if _, ok := value.(map[string]any); !ok {
					return nil, NewBadRequestError("parser_config." + key + " 必须是JSON对象")
				}
				filtered[key] = sanitizeNestedParserConfig(value, keys)
			}
		}
		if len(config) > 0 && len(filtered) == 0 {
			return nil, NewBadRequestError("parser_config 没有可写字段；请只提交已修改的解析参数")
		}
		body["parser_config"] = filtered
	}
	return body, nil
}
