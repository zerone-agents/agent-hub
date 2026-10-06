package knowledge

import (
	"encoding/json"
	"reflect"
	"strings"
)

var ErrRetrievalGraphScopeConflict = NewBadRequestError("图谱检索尚未验证文档、元数据及已保存搜索的范围约束，请关闭 use_kg 后重试")

// RetrievalToSearch preserves the Hub request aliases while producing the
// dataset search contract. Legacy empty document_ids meant all documents;
// explicit new doc_ids:[] means an empty scope and must never search the KB.
func RetrievalToSearch(req RetrievalRequest) (RetrievalRequest, error) {
	body := CloneObject(req)
	if err := retrievalAlias(body, "dataset_ids", "kb_ids"); err != nil {
		return nil, err
	}
	ids, err := retrievalIDs(body["dataset_ids"])
	if err != nil || len(ids) == 0 {
		return nil, NewBadRequestError("dataset_ids 必须是非空文档库ID数组")
	}
	body["dataset_ids"] = ids
	question, ok := body["question"].(string)
	if !ok || strings.TrimSpace(question) == "" {
		return nil, NewBadRequestError("question 不能为空")
	}
	body["question"] = strings.TrimSpace(question)
	if legacy, exists := body["document_ids"]; exists && legacy != nil {
		ids, err := retrievalIDs(legacy)
		if err != nil {
			return nil, NewBadRequestError("document_ids 必须是文档ID数组")
		}
		if len(ids) == 0 {
			delete(body, "document_ids")
		}
	}
	if err := retrievalAlias(body, "doc_ids", "document_ids"); err != nil {
		return nil, err
	}
	if value, exists := body["doc_ids"]; exists && value != nil {
		ids, err := retrievalIDs(value)
		if err != nil {
			return nil, NewBadRequestError("doc_ids 必须是文档ID数组或 null")
		}
		body["doc_ids"] = ids
	}
	if err := retrievalAlias(body, "size", "page_size"); err != nil {
		return nil, err
	}
	if legacy := body["metadata_condition"]; legacy != nil {
		condition, err := retrievalObject(legacy)
		if err != nil {
			return nil, NewBadRequestError("metadata_condition 必须是JSON对象")
		}
		if len(condition) > 0 {
			if filter := body["meta_data_filter"]; filter != nil {
				return nil, NewBadRequestError("不能同时指定 metadata_condition 和 meta_data_filter")
			}
			conditions, exists := condition["conditions"].([]any)
			if !exists {
				return nil, NewBadRequestError("metadata_condition 必须包含 conditions 数组")
			}
			manual := make([]any, 0, len(conditions))
			for _, value := range conditions {
				item, ok := value.(map[string]any)
				if !ok {
					return nil, NewBadRequestError("元数据条件必须是对象")
				}
				manual = append(manual, map[string]any{"key": item["name"], "op": item["comparison_operator"], "value": item["value"]})
			}
			logic := condition["logic"]
			if logic == nil {
				logic = "and"
			}
			body["meta_data_filter"] = map[string]any{"method": "manual", "logic": logic, "manual": manual}
		}
	}
	delete(body, "metadata_condition")
	useKG := false
	if value, present := body["use_kg"]; present {
		var ok bool
		useKG, ok = value.(bool)
		if !ok {
			return nil, NewBadRequestError("use_kg 必须是布尔值")
		}
	}
	if value := body["meta_data_filter"]; value != nil {
		filter, err := retrievalObject(value)
		if err != nil {
			return nil, NewBadRequestError("meta_data_filter 必须是JSON对象")
		}
		if len(filter) > 0 {
			// MultiRAG closes KG for any nonempty forwarded filter object,
			// including manual:[] and legacy conditions:[] after conversion.
			if useKG {
				return nil, ErrRetrievalGraphScopeConflict
			}
			if err := normalizeMetadataFilter(filter); err != nil {
				return nil, err
			}
		}
		body["meta_data_filter"] = filter
	}
	searchID := ""
	if value := body["search_id"]; value != nil {
		var ok bool
		searchID, ok = value.(string)
		if !ok {
			return nil, NewBadRequestError("search_id 必须是字符串或 null")
		}
	}
	if useKG {
		ids, _ := body["doc_ids"].([]string)
		// A saved Search can replace the body filter upstream. Its authorized
		// configuration is unavailable here, so every nonempty ID closes KG.
		if len(ids) > 0 || searchID != "" {
			return nil, ErrRetrievalGraphScopeConflict
		}
	}
	return RetrievalRequest(body), nil
}

func retrievalAlias(body map[string]any, canonical, legacy string) error {
	value, exists := body[legacy]
	if exists {
		if current, present := body[canonical]; present {
			a, _ := json.Marshal(current)
			b, _ := json.Marshal(value)
			if !reflect.DeepEqual(a, b) {
				return NewBadRequestError(canonical + " 与 " + legacy + " 冲突")
			}
		} else {
			body[canonical] = value
		}
	}
	delete(body, legacy)
	return nil
}

func retrievalIDs(value any) ([]string, error) {
	data, err := json.Marshal(value)
	if err != nil || len(data) == 0 || data[0] != '[' {
		return nil, NewBadRequestError("ID必须是数组")
	}
	var ids []string
	if json.Unmarshal(data, &ids) != nil {
		return nil, NewBadRequestError("ID必须是字符串")
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return nil, NewBadRequestError("ID不能为空")
		}
	}
	return ids, nil
}

func retrievalObject(value any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil || len(data) == 0 || data[0] != '{' {
		return nil, NewBadRequestError("必须是JSON对象")
	}
	var result map[string]any
	err = json.Unmarshal(data, &result)
	return result, err
}

func normalizeMetadataFilter(filter map[string]any) error {
	method, _ := filter["method"].(string)
	if method != "manual" && method != "auto" && method != "semi_auto" {
		return NewBadRequestError("元数据过滤 method 必须是 manual、auto 或 semi_auto")
	}
	if logic := filter["logic"]; logic != nil && logic != "and" && logic != "or" {
		return NewBadRequestError("元数据过滤 logic 必须是 and 或 or")
	}
	if method != "manual" {
		if method == "semi_auto" {
			items, ok := filter["semi_auto"].([]any)
			if !ok {
				return NewBadRequestError("semi_auto 必须是元数据字段数组")
			}
			for _, value := range items {
				key, ok := value.(string)
				if !ok {
					item, ok := value.(map[string]any)
					if !ok {
						return NewBadRequestError("semi_auto 字段必须是字符串或对象")
					}
					key, _ = item["key"].(string)
				}
				if strings.TrimSpace(key) == "" {
					return NewBadRequestError("semi_auto 字段不能为空")
				}
			}
		}
		return nil
	}
	manual, ok := filter["manual"].([]any)
	if !ok {
		return NewBadRequestError("manual 必须是元数据条件数组")
	}
	aliases := map[string]string{"is": "=", "not is": "≠", "!=": "≠", ">=": "≥", "<=": "≤"}
	valid := map[string]bool{"=": true, "≠": true, ">": true, "<": true, "≥": true, "≤": true, "contains": true, "not contains": true, "in": true, "not in": true, "start with": true, "end with": true, "empty": true, "not empty": true}
	for _, value := range manual {
		item, ok := value.(map[string]any)
		if !ok {
			return NewBadRequestError("元数据条件必须是对象")
		}
		key, _ := item["key"].(string)
		op, _ := item["op"].(string)
		if canonical := aliases[op]; canonical != "" {
			op = canonical
		}
		if strings.TrimSpace(key) == "" || !valid[op] {
			return NewBadRequestError("元数据条件 key 或 op 无效")
		}
		if _, exists := item["value"]; !exists && op != "empty" && op != "not empty" {
			return NewBadRequestError("元数据条件 value 必填")
		}
		if err := validateMetadataMembership(op, item["value"]); err != nil {
			return err
		}
		item["op"] = op
		if _, exists := item["value"]; !exists {
			item["value"] = ""
		}
	}
	return nil
}

func EmptyRetrievalScope(req RetrievalRequest) bool {
	ids, ok := req["doc_ids"].([]string)
	return ok && len(ids) == 0
}

func EmptyRetrievalResult() *RetrievalResult {
	result := RetrievalResult{"total": 0, "chunks": []any{}, "doc_aggs": []any{}, "labels": map[string]any{}}
	return &result
}
