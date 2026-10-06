package knowledge

import (
	"math"
	"strings"
)

// MultiRAG 4ce5dc0 preserves JSON scalar types for list membership. Negative
// membership remains unsafe for multi-valued metadata and must not gain a new
// typed input path through this gateway.
func validateMetadataMembership(op string, value any) error {
	if op != "in" && op != "not in" {
		return nil
	}
	if op == "not in" {
		switch values := value.(type) {
		case string, []string:
			return nil
		case []any:
			for _, item := range values {
				if _, ok := item.(string); !ok {
					return NewBadRequestError("多值元数据的 not in 仍未验证数字、布尔值或混合列表；请使用正向 in")
				}
			}
			return nil
		default:
			return NewBadRequestError("多值元数据的 not in 仍未验证数字、布尔值或混合列表；请使用正向 in")
		}
	}
	switch values := value.(type) {
	case string, []string:
		return nil
	case []any:
		for _, item := range values {
			switch scalar := item.(type) {
			case string, bool, int, int64, uint64:
			case float64:
				if math.IsNaN(scalar) || math.IsInf(scalar, 0) {
					return NewBadRequestError("in 列表数字必须有限")
				}
			case float32:
				if math.IsNaN(float64(scalar)) || math.IsInf(float64(scalar), 0) {
					return NewBadRequestError("in 列表数字必须有限")
				}
			default:
				return NewBadRequestError("in 只支持文本或有限数字、布尔值组成的列表")
			}
		}
		return nil
	}
	return NewBadRequestError("in 只支持文本或有限数字、布尔值组成的列表")
}

// ValidateMetadataConditionMembership covers the legacy conditions envelope
// used by document listing and metadata batch selectors.
func ValidateMetadataConditionMembership(condition map[string]any) error {
	raw, present := condition["conditions"]
	if !present {
		return nil
	}
	conditions, ok := raw.([]any)
	if !ok {
		return NewBadRequestError("metadata_condition.conditions 必须是条件数组")
	}
	for _, raw := range conditions {
		item, ok := raw.(map[string]any)
		if !ok {
			return NewBadRequestError("元数据条件必须是对象")
		}
		op, _ := item["comparison_operator"].(string)
		if err := validateMetadataMembership(op, item["value"]); err != nil {
			return err
		}
	}
	return nil
}

func ValidateMetadataBatchMembership(body Object) error {
	if raw := body["selector"]; raw != nil {
		selector, err := retrievalObject(raw)
		if err != nil {
			return NewBadRequestError("selector 必须是JSON对象")
		}
		if raw := selector["metadata_condition"]; raw != nil {
			condition, err := retrievalObject(raw)
			if err != nil {
				return NewBadRequestError("metadata_condition 必须是JSON对象")
			}
			// The service defaults an omitted logic to and, but treats every
			// other value as or. Never let a typo expand a writer's scope.
			if raw, present := condition["logic"]; present {
				logic, ok := raw.(string)
				if !ok || (logic != "and" && logic != "or") {
					return NewBadRequestError("批量元数据条件 logic 必须是 and 或 or；未指定时默认为 and")
				}
			}
			// MultiRAG's flattened value-to-document index unions negative
			// matches: Alice AND Bob can pass an Alice exclusion through Bob.
			// Include the committed operator aliases, without changing positive
			// predicates or the submitted selector.
			if conditions, ok := condition["conditions"].([]any); ok {
				for _, raw := range conditions {
					if item, ok := raw.(map[string]any); ok {
						op, _ := item["comparison_operator"].(string)
						switch op {
						case "not in", "not contains", "≠", "!=", "not is":
							return NewBadRequestError("当前服务尚未验证批量元数据 " + op + " 的排除范围；请使用显式文档ID及已验证的正向条件")
						}
					}
				}
			}
			return ValidateMetadataConditionMembership(condition)
		}
	}
	return nil
}

// The document template PUT only writes metadata upstream. It silently ignores
// extra fields, so never acknowledge an enabled toggle through this endpoint.
func DocumentMetadataTemplate(body Object) (any, error) {
	for key := range body {
		if key == "enabled" {
			return nil, NewBadRequestError("文档元数据模板接口不支持 enabled；请通过文档 PATCH 的 parser_config.enable_metadata 修改开关")
		}
		if key != "metadata" {
			return nil, NewBadRequestError("文档元数据模板接口仅支持 metadata")
		}
	}
	value, present := body["metadata"]
	if !present || value == nil {
		return nil, NewBadRequestError("metadata 必须是完整模板对象或数组；使用 [] 清空")
	}
	// Roundtrip JSON here so direct Go callers' typed maps/slices use the same
	// public representation as HTTP callers and independent readback.
	wrapped, err := retrievalObject(map[string]any{"metadata": value})
	if err != nil {
		return nil, NewBadRequestError("metadata 必须是完整模板对象或数组")
	}
	switch value := wrapped["metadata"].(type) {
	case map[string]any:
		return value, nil
	case []any:
		// The dedicated template PUT uses MetadataField (trimmed identifiers).
		// Document PATCH uses another DTO, so normalize here rather than in the
		// shared JSON Schema projection. The JSON roundtrip owns these fields.
		for _, raw := range value {
			if field, ok := raw.(map[string]any); ok {
				for _, key := range []string{"key", "name"} {
					if text, ok := field[key].(string); ok {
						field[key] = strings.TrimSpace(text)
					}
				}
			}
		}
		return value, nil
	default:
		return nil, NewBadRequestError("metadata 必须是完整模板对象或数组")
	}
}
