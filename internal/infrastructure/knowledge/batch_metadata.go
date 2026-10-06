package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"fmt"
	"strconv"
)

const typedBatchLimit = "当前服务仅能确认最多100个显式文档、单个标量元数据更新，且不能同时指定删除或条件筛选；其他类型操作请使用单文档 meta_fields PATCH"

type typedBatchField struct {
	value   any
	present bool
}
type typedBatchPlan struct {
	ids      []string
	key      string
	expected map[string]typedBatchField
}

func hasBooleanNumberAliasRisk(value any) bool {
	switch value := value.(type) {
	case bool:
		return true
	case float64:
		return value == 0 || value == 1
	case []any:
		for _, item := range value {
			if hasBooleanNumberAliasRisk(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if hasBooleanNumberAliasRisk(item) {
				return true
			}
		}
	}
	return false
}

func booleanNumberAlias(a, b any) bool {
	flag, ok := a.(bool)
	number, numeric := b.(float64)
	if !ok || !numeric {
		flag, ok = b.(bool)
		number, numeric = a.(float64)
	}
	return ok && numeric && ((flag && number == 1) || (!flag && number == 0))
}

func pythonBatchScalarString(value any) string {
	switch value := value.(type) {
	case bool:
		if value {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

// Only alias-risk writes pay for reads. Keep their scope explicit and bounded,
// avoiding a second implementation of the upstream batch selector/writer.
func (c *RemoteMultiragEngine) prepareTypedBatch(ctx context.Context, r domain.PageRequest) (*typedBatchPlan, error) {
	payload, err := json.Marshal(r.Body)
	if err != nil {
		return nil, domain.NewBadRequestError("请求内容无效")
	}
	var body map[string]any
	if json.Unmarshal(payload, &body) != nil {
		return nil, domain.NewBadRequestError("请求内容无效")
	}
	updates, _ := body["updates"].([]any)
	risky := false
	for _, raw := range updates {
		if update, ok := raw.(map[string]any); ok && hasBooleanNumberAliasRisk(update["value"]) {
			risky = true
		}
	}
	if !risky {
		return nil, nil
	}
	if len(updates) != 1 {
		return nil, domain.NewBadRequestError(typedBatchLimit)
	}
	update := updates[0].(map[string]any)
	value := update["value"]
	switch value.(type) {
	case bool, float64:
	default:
		return nil, domain.NewBadRequestError(typedBatchLimit)
	}
	if raw := body["deletes"]; raw != nil {
		deletes, ok := raw.([]any)
		if !ok || len(deletes) != 0 {
			return nil, domain.NewBadRequestError(typedBatchLimit)
		}
	}
	selector, ok := body["selector"].(map[string]any)
	if !ok {
		return nil, domain.NewBadRequestError(typedBatchLimit)
	}
	if raw := selector["metadata_condition"]; raw != nil {
		condition, ok := raw.(map[string]any)
		if !ok || len(condition) != 0 {
			return nil, domain.NewBadRequestError(typedBatchLimit)
		}
	}
	rawIDs, ok := selector["document_ids"].([]any)
	if !ok {
		return nil, domain.NewBadRequestError(typedBatchLimit)
	}
	plan := &typedBatchPlan{expected: map[string]typedBatchField{}}
	plan.key, ok = update["key"].(string)
	if !ok || plan.key == "" {
		return nil, domain.NewBadRequestError("元数据字段 key 必须是非空字符串")
	}
	seen := map[string]bool{}
	for _, raw := range rawIDs {
		id, ok := raw.(string)
		if !ok || id == "" {
			return nil, domain.NewBadRequestError("document_ids 必须是非空文档ID数组")
		}
		if !seen[id] {
			plan.ids = append(plan.ids, id)
			seen[id] = true
		}
	}
	if len(plan.ids) > 100 {
		return nil, domain.NewBadRequestError(typedBatchLimit)
	}
	match := update["match"]
	if match != nil {
		switch match.(type) {
		case string, bool, float64:
		default:
			return nil, domain.NewBadRequestError(typedBatchLimit)
		}
	}
	matchProvided := match != nil && match != ""
	before, err := c.readTypedBatchDocuments(ctx, r.DatasetID, plan.ids)
	if err != nil {
		return nil, err
	}
	for id, metadata := range before {
		current, present := metadata[plan.key]
		if present {
			switch current.(type) {
			case []any, map[string]any:
				return nil, domain.NewBadRequestError(typedBatchLimit)
			}
		}
		// Python str(2.0) is "2.0", while the JSON decoder loses that
		// spelling. Do not write a conditional numeric conversion that we
		// cannot faithfully predict and confirm. Unconditional writes stay
		// supported, as do string/boolean conditional matches.
		_, currentNumber := current.(float64)
		_, matchNumber := match.(float64)
		if matchProvided && (currentNumber || matchNumber) {
			return nil, domain.NewBadRequestError("当前服务不支持涉及数字的批量条件 match；请使用单文档 meta_fields PATCH 并核对当前值")
		}
		expected := typedBatchField{current, present}
		if !matchProvided || (present && pythonBatchScalarString(current) == pythonBatchScalarString(match)) {
			if booleanNumberAlias(current, value) {
				return nil, domain.NewBadRequestError("当前批量服务不支持 false与0、true与1 的等值类型替换；请使用单文档 meta_fields PATCH")
			}
			expected = typedBatchField{value, true}
		}
		plan.expected[id] = expected
	}
	return plan, nil
}

// One page covers the bounded explicit IDs. Require complete, unique results;
// failed permissions, missing metadata or pagination must never broaden scope.
func (c *RemoteMultiragEngine) readTypedBatchDocuments(ctx context.Context, datasetID string, ids []string) (map[string]map[string]any, error) {
	documents := map[string]map[string]any{}
	if len(ids) == 0 {
		return documents, nil
	}
	result, err := c.ListDocuments(ctx, datasetID, domain.DocumentListRequest{IDs: ids, Page: 1, PageSize: 100})
	if err != nil {
		return nil, err
	}
	invalid := func() (map[string]map[string]any, error) {
		return nil, domain.NewUpstreamError("批量类型更新的本库文档/元数据预读不完整，请刷新核对", nil)
	}
	if result.Total != len(ids) || len(result.Documents) != len(ids) {
		return invalid()
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, doc := range result.Documents {
		id, ok := doc["id"].(string)
		if !ok || !selected[id] || documents[id] != nil {
			return invalid()
		}
		if doc["dataset_id"] != datasetID {
			return invalid()
		}
		metadata, ok := doc["meta_fields"].(map[string]any)
		if !ok {
			return invalid()
		}
		documents[id] = metadata
	}
	return documents, nil
}

func metadataBatchCounts(data any) (updated, matched int, err error) {
	result, ok := data.(map[string]any)
	if !ok {
		return 0, 0, domain.DocumentUpdateUnknown("批量元数据更新已受理，但结果计数未确认；请回读核对")
	}
	counts := []*int{&updated, &matched}
	for i, key := range []string{"updated", "matched_docs"} {
		number, ok := result[key].(float64)
		if !ok || number < 0 || number > float64(int(^uint(0)>>1)) || number != float64(int(number)) {
			return 0, 0, domain.DocumentUpdateUnknown("批量元数据更新已受理，但结果计数无效；请回读核对")
		}
		*counts[i] = int(number)
	}
	if updated > matched {
		return 0, 0, domain.DocumentUpdateUnknown("批量元数据更新已受理，但结果计数不一致；请回读核对")
	}
	return updated, matched, nil
}

func (c *RemoteMultiragEngine) confirmTypedBatch(ctx context.Context, r domain.PageRequest, plan *typedBatchPlan, data any) error {
	_, matched, err := metadataBatchCounts(data)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	if matched != len(plan.ids) {
		return domain.DocumentUpdateUnknown("批量元数据更新已受理，但匹配范围未确认；请回读核对")
	}
	after, err := c.readTypedBatchDocuments(ctx, r.DatasetID, plan.ids)
	if err != nil {
		return domain.DocumentUpdateUnknown("批量元数据更新已受理，但独立回读未确认；请回读核对")
	}
	for id, expected := range plan.expected {
		actual, present := after[id][plan.key]
		if present != expected.present || !jsonEqualDocumentValue(actual, expected.value) {
			return domain.DocumentUpdateUnknown("批量元数据更新已受理，但字段类型和值回读不一致；请回读核对")
		}
	}
	return nil
}
