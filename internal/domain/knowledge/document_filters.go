package knowledge

import (
	"encoding/json"
	"strings"
	"unicode"
)

func ValidateDocumentFilters(req DocumentListRequest) error {
	if req.Page < 0 || req.PageSize < 0 || req.PageSize > 100 {
		return NewBadRequestError("文档分页必须为正整数，page_size 最大为 100")
	}
	if req.ID != "" && len(req.IDs) > 0 {
		return NewBadRequestError("id 与 ids 不能同时指定")
	}
	for _, id := range req.IDs {
		if strings.TrimSpace(id) == "" {
			return NewBadRequestError("ids 必须是非空文档ID数组")
		}
	}
	for name, value := range map[string]string{"metadata": req.Metadata, "metadata_condition": req.MetadataCondition} {
		if value == "" {
			continue
		}
		var object map[string]any
		if json.Unmarshal([]byte(value), &object) != nil || object == nil {
			return NewBadRequestError(name + " 必须是JSON对象")
		}
		if name == "metadata" {
			if _, reserved := object["empty_metadata"]; reserved {
				return NewBadRequestError("empty_metadata 是服务保留标记，不能作为精确元数据字段筛选；请使用 return_empty_metadata 查询空元数据")
			}
			// MultiRAG skips null/empty normalized values. Reject such explicit
			// conditions instead of silently broadening the requested scope.
			for _, raw := range object {
				values := []any{raw}
				if array, ok := raw.([]any); ok {
					values = array
				}
				if len(values) == 0 {
					return NewBadRequestError("精确元数据筛选不支持 null、空数组或空白字符串；请显式移除该条件")
				}
				for _, value := range values {
					if value == nil {
						return NewBadRequestError("精确元数据筛选不支持 null、空数组或空白字符串；请显式移除该条件")
					}
					if text, ok := value.(string); ok && pythonMetadataValueIsBlank(text) {
						return NewBadRequestError("精确元数据筛选不支持 null、空数组或空白字符串；请显式移除该条件")
					}
				}
			}
		}
		if name == "metadata_condition" {
			if err := ValidateMetadataConditionMembership(object); err != nil {
				return err
			}
		}
	}
	return nil
}

// Python str.strip() includes the four ASCII information separators in
// addition to Unicode White_Space. Only test emptiness; preserve nonblank
// literal values because MultiRAG uses the original string as the exact key.
func pythonMetadataValueIsBlank(text string) bool {
	return strings.TrimFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || (r >= '\u001c' && r <= '\u001f')
	}) == ""
}
