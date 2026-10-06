package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"control-panel/internal/domain/knowledge"

	"github.com/gin-gonic/gin"
)

const mcpSearchTimeout = 45 * time.Second

type mcpSearchPage struct {
	datasetID string
	result    *knowledge.RetrievalResult
}

func (h *KnowledgeMcpHandler) searchKnowledge(ctx context.Context, c *gin.Context, id interface{}, params json.RawMessage) (jsonRPCResponse, error) {
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(params, &supplied); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	allowed, deny, err := h.resolveAgentContext(c, id)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	if len(allowed) == 0 {
		return mcpCodedErrorResult(id, mcpErrNoDatasetBound, "当前 Agent 未绑定任何知识库数据集"), nil
	}
	for _, field := range []string{"dataset_ids", "doc_ids", "meta_data_filter", "reference_metadata_fields"} {
		if raw, exists := supplied[field]; exists && strings.TrimSpace(string(raw)) == "null" {
			return mcpCodedErrorResult(id, mcpErrInvalidSearchArguments, field+" 不能为 null；不限制范围时请省略该字段"), nil
		}
	}
	var args knowledgeSearchArgs
	if err := json.Unmarshal(params, &args); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("参数解析失败: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		args.Query = strings.TrimSpace(args.Question)
	}
	if args.Query == "" {
		return jsonRPCResponse{}, fmt.Errorf("query 不能为空")
	}
	if err := validateMcpSearchArgs(&args); err != nil {
		return mcpCodedErrorResult(id, mcpErrInvalidSearchArguments, err.Error()), nil
	}

	datasetIDs := allowed
	if args.DatasetIDs != nil {
		datasetIDs = *args.DatasetIDs
		if len(datasetIDs) == 0 {
			return mcpCodedErrorResult(id, mcpErrInvalidSearchArguments, "dataset_ids 为空；请选择至少一个已绑定知识库"), nil
		}
	}
	datasetIDs, err = mcpDistinctIDs(datasetIDs)
	if err != nil {
		return mcpCodedErrorResult(id, mcpErrInvalidSearchArguments, err.Error()), nil
	}
	if !isStringSubset(datasetIDs, allowed) {
		return mcpCodedErrorResult(id, mcpErrDatasetNotAuthorized, knowledgeCapabilityDeniedMessage), nil
	}
	if len(datasetIDs) > mcpSearchMaxDatasets {
		return mcpCodedErrorResult(id, mcpErrInvalidSearchArguments, "本次最多检索 8 个知识库；请用 knowledge_datasets 选择较小范围"), nil
	}
	if args.RerankID != "" {
		if err := h.validateMcpRerankModel(ctx, args.RerankID); err != nil {
			log.Printf("knowledge-mcp: rerank model unavailable: %v", err)
			return mcpCodedErrorResult(id, mcpErrRerankUnavailable, "重排模型未启用或模型列表不可用；请调用 knowledge_rerank_models 重新选择"), nil
		}
	}
	if args.DocIDs != nil && len(*args.DocIDs) == 0 {
		return mcpTextResult(id, "显式指定的 doc_ids 为空，检索范围为空；未查询任何知识库。"), nil
	}

	searchCtx, cancel := context.WithTimeout(ctx, mcpSearchTimeout)
	defer cancel()
	pages := make([]mcpSearchPage, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		req := mcpSearchRequest(args, datasetID)
		result, err := h.knowledgeService.Retrieval(searchCtx, req)
		if err != nil {
			log.Printf("knowledge-mcp: retrieval failed (dataset=%s): %v", datasetID, err)
			h.probeDatasetsForDiagnosis(ctx, datasetIDs)
			message := "知识库检索失败，请稍后重试"
			if len(datasetIDs) > 1 {
				message = "按库检索未全部成功，未返回不完整结果；请指定一个知识库重试"
			}
			return mcpCodedErrorResult(id, mcpErrRetrievalFailed, message), nil
		}
		if err := validateMcpSearchResult(result, datasetID, args.DocIDs); err != nil {
			log.Printf("knowledge-mcp: invalid retrieval scope (dataset=%s): %v", datasetID, err)
			return mcpCodedErrorResult(id, mcpErrRetrievalScopeMismatch, "检索来源无法确认所属知识库或文档；结果已隐藏"), nil
		}
		pages = append(pages, mcpSearchPage{datasetID: datasetID, result: result})
	}
	return mcpTextResult(id, formatMcpSearchPages(pages, args)), nil
}

func validateMcpSearchArgs(args *knowledgeSearchArgs) error {
	if args.TopK == nil {
		defaultTopK := 64
		args.TopK = &defaultTopK
	}
	if *args.TopK < 1 || *args.TopK > mcpSearchMaxTopK {
		return fmt.Errorf("top_k 必须在 1 到 %d 之间", mcpSearchMaxTopK)
	}
	if args.Page == nil {
		page := 1
		args.Page = &page
	}
	if *args.Page < 1 || *args.Page > mcpSearchMaxPage {
		return fmt.Errorf("page 必须在 1 到 %d 之间", mcpSearchMaxPage)
	}
	if args.PageSize == nil {
		pageSize := min(8, *args.TopK)
		args.PageSize = &pageSize
	}
	if *args.PageSize < 1 || *args.PageSize > mcpSearchMaxPageSize {
		return fmt.Errorf("page_size 必须在 1 到 %d 之间", mcpSearchMaxPageSize)
	}
	if *args.Page > *args.TopK / *args.PageSize {
		return fmt.Errorf("top_k 候选数必须覆盖 page × page_size；请增大 top_k 或缩小页码")
	}
	if args.SimilarityThreshold == nil {
		value := 0.2
		args.SimilarityThreshold = &value
	}
	if !mcpUnitFloat(*args.SimilarityThreshold) {
		return fmt.Errorf("similarity_threshold 必须在 0 到 1 之间")
	}
	if args.VectorSimilarityWeight == nil {
		value := 0.3
		args.VectorSimilarityWeight = &value
	}
	if !mcpUnitFloat(*args.VectorSimilarityWeight) {
		return fmt.Errorf("vector_similarity_weight 必须在 0 到 1 之间")
	}
	if args.Highlight == nil {
		value := false
		args.Highlight = &value
	}
	if args.SearchMode == "" {
		args.SearchMode = "dense"
	}
	switch args.SearchMode {
	case "dense", "sparse", "hybrid", "fusion":
	default:
		return fmt.Errorf("search_mode 必须是 dense、sparse、hybrid 或 fusion")
	}
	if args.HybridDenseWeight != nil && (args.SearchMode != "hybrid" || !mcpUnitFloat(*args.HybridDenseWeight)) {
		return fmt.Errorf("hybrid_dense_weight 仅可用于 hybrid 模式且必须在 0 到 1 之间")
	}
	if args.DocIDs != nil {
		ids, err := mcpDistinctIDs(*args.DocIDs)
		if err != nil {
			return err
		}
		if len(ids) > 100 {
			return fmt.Errorf("doc_ids 最多包含 100 个文档")
		}
		args.DocIDs = &ids
	}
	if args.ReferenceMetadataFields != nil {
		fields, err := mcpDistinctIDs(*args.ReferenceMetadataFields)
		if err != nil || len(fields) == 0 || len(fields) > 10 {
			return fmt.Errorf("reference_metadata_fields 必须包含 1 到 10 个非空字段")
		}
		args.ReferenceMetadataFields = &fields
	}
	if err := validateMcpMetadataFilter(args.MetadataFilter); err != nil {
		return err
	}
	args.RerankID = strings.TrimSpace(args.RerankID)
	return nil
}

func mcpUnitFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func mcpDistinctIDs(ids []string) ([]string, error) {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, value := range ids {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, fmt.Errorf("ID 不能为空")
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

func validateMcpMetadataFilter(filter map[string]any) error {
	if filter == nil {
		return nil
	}
	if filter["method"] != "manual" {
		return fmt.Errorf("MCP 元数据过滤只支持 method=manual")
	}
	if logic := filter["logic"]; logic != nil && logic != "and" && logic != "or" {
		return fmt.Errorf("元数据过滤 logic 必须是 and 或 or")
	}
	for key := range filter {
		if key != "method" && key != "logic" && key != "manual" {
			return fmt.Errorf("MCP 元数据过滤包含未支持的字段")
		}
	}
	manual, ok := filter["manual"].([]any)
	if !ok || len(manual) == 0 || len(manual) > 10 {
		return fmt.Errorf("manual 必须包含 1 到 10 个正向文本条件")
	}
	for _, raw := range manual {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(mcpString(item["key"])) == "" {
			return fmt.Errorf("元数据条件 key 必须是非空字符串")
		}
		for key := range item {
			if key != "key" && key != "op" && key != "value" {
				return fmt.Errorf("元数据条件包含未支持的字段")
			}
		}
		switch item["op"] {
		case "in":
			switch value := item["value"].(type) {
			case string:
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("in 条件值不能为空")
				}
			case []any:
				if len(value) == 0 || len(value) > 20 {
					return fmt.Errorf("in 条件需包含 1 到 20 个标量值")
				}
				for _, entry := range value {
					switch scalar := entry.(type) {
					case string:
						if strings.TrimSpace(scalar) == "" {
							return fmt.Errorf("in 条件文本值不能为空")
						}
					case bool:
					case float64:
						if math.IsNaN(scalar) || math.IsInf(scalar, 0) {
							return fmt.Errorf("in 条件数字必须有限")
						}
					default:
						return fmt.Errorf("in 条件只支持文本、数字、布尔值")
					}
				}
			default:
				return fmt.Errorf("in 条件只支持文本或文本、数字、布尔值列表")
			}
		case "contains", "start with", "end with":
			if strings.TrimSpace(mcpString(item["value"])) == "" {
				return fmt.Errorf("元数据文本条件值不能为空")
			}
		case "empty":
			item["value"] = ""
		default:
			return fmt.Errorf("MCP 当前只支持已验证的正向元数据条件")
		}
	}
	return nil
}

func mcpString(value any) string {
	text, _ := value.(string)
	return text
}

func mcpSearchRequest(args knowledgeSearchArgs, datasetID string) knowledge.RetrievalRequest {
	req := knowledge.RetrievalRequest{
		"question": args.Query, "dataset_ids": []string{datasetID},
		"page": *args.Page, "size": *args.PageSize, "top_k": *args.TopK,
		"similarity_threshold":     *args.SimilarityThreshold,
		"vector_similarity_weight": *args.VectorSimilarityWeight,
		"highlight":                *args.Highlight,
		"search_mode":              map[string]any{"type": args.SearchMode},
	}
	if args.SearchMode == "hybrid" && args.HybridDenseWeight != nil {
		req["search_mode"] = map[string]any{"type": "hybrid", "weight_dense": *args.HybridDenseWeight, "weight_sparse": 1 - *args.HybridDenseWeight}
	}
	if args.DocIDs != nil {
		req["doc_ids"] = *args.DocIDs
	}
	if args.MetadataFilter != nil {
		req["meta_data_filter"] = args.MetadataFilter
	}
	if args.ReferenceMetadataFields != nil {
		req["reference_metadata"] = map[string]any{"include": true, "fields": *args.ReferenceMetadataFields}
	}
	if args.RerankID != "" {
		req["rerank_id"] = args.RerankID
	}
	return req
}

func validateMcpSearchResult(result *knowledge.RetrievalResult, datasetID string, docIDs *[]string) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	raw := map[string]any(*result)
	chunks, ok := raw["chunks"].([]any)
	if !ok {
		return fmt.Errorf("missing chunks array")
	}
	allowedDocs := map[string]bool{}
	if docIDs != nil {
		for _, docID := range *docIDs {
			allowedDocs[docID] = true
		}
	}
	for _, item := range chunks {
		chunk, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid chunk")
		}
		kbID := mcpString(chunk["kb_id"])
		if kbID == "" {
			kbID = mcpString(chunk["dataset_id"])
		}
		chunkID := mcpString(chunk["id"])
		if chunkID == "" {
			chunkID = mcpString(chunk["chunk_id"])
		}
		docID := mcpString(chunk["document_id"])
		if docID == "" {
			docID = mcpString(chunk["doc_id"])
		}
		if kbID != datasetID || chunkID == "" || docID == "" || (docIDs != nil && !allowedDocs[docID]) {
			return fmt.Errorf("source identity outside request scope")
		}
	}
	return nil
}

func formatMcpSearchPages(pages []mcpSearchPage, args knowledgeSearchArgs) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("知识库检索完成：%d 个知识库分别检索；第 %d 页，每库最多 %d 条。不同知识库的分数不作跨库合并排序。\n", len(pages), *args.Page, *args.PageSize))
	if args.DocIDs != nil || args.MetadataFilter != nil {
		sb.WriteString("注意：显式文档或元数据范围下，MultiRAG 可能绕过最终相似度阈值；请核对每条来源。\n")
	}
	for _, page := range pages {
		sb.WriteString("\n知识库 ID：")
		sb.WriteString(page.datasetID)
		if total, ok := mcpPageTotal(page.result); ok {
			sb.WriteString(fmt.Sprintf("；该库命中总数：%d", total))
		}
		sb.WriteString("\n")
		sb.WriteString(formatRetrievalResult(page.result))
	}
	return sb.String()
}

func mcpPageTotal(result *knowledge.RetrievalResult) (int, bool) {
	if result == nil {
		return 0, false
	}
	switch value := map[string]any(*result)["total"].(type) {
	case int:
		return value, value >= 0
	case float64:
		if value >= 0 && value <= 1<<53 && math.Trunc(value) == value {
			return int(value), true
		}
	}
	return 0, false
}

func mcpTextResult(id interface{}, value string) jsonRPCResponse {
	return jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: map[string]interface{}{
		"content": []map[string]interface{}{{"type": "text", "text": value}}, "isError": false,
	}}
}

func (h *KnowledgeMcpHandler) availableMcpRerankModels(ctx context.Context) ([]string, error) {
	if h.modelsSource == nil {
		return nil, fmt.Errorf("MultiRAG model source unavailable")
	}
	raw, err := h.modelsSource.ListMyLLMs(ctx)
	if err != nil {
		return nil, err
	}
	var groups map[string]struct {
		LLM []struct {
			Type   string `json:"type"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"llm"`
	}
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	models := make([]string, 0)
	for factory, group := range groups {
		for _, model := range group.LLM {
			if strings.EqualFold(model.Type, "rerank") && model.Status == "1" && model.Name != "" {
				models = append(models, model.Name+"@"+factory)
			}
		}
	}
	sort.Strings(models)
	return models, nil
}

func (h *KnowledgeMcpHandler) validateMcpRerankModel(ctx context.Context, modelID string) error {
	models, err := h.availableMcpRerankModels(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range models {
		if candidate == modelID {
			return nil
		}
	}
	return fmt.Errorf("model not enabled")
}

func (h *KnowledgeMcpHandler) handleKnowledgeRerankModels(ctx context.Context, c *gin.Context, id interface{}) (jsonRPCResponse, error) {
	_, deny, err := h.resolveAgentContext(c, id)
	if err != nil {
		return jsonRPCResponse{}, err
	}
	if deny != nil {
		return *deny, nil
	}
	models, err := h.availableMcpRerankModels(ctx)
	if err != nil {
		log.Printf("knowledge-mcp: list rerank models failed: %v", err)
		return mcpCodedErrorResult(id, mcpErrRerankUnavailable, "重排模型列表暂不可用"), nil
	}
	return mcpJSONResult(id, map[string]any{"rerank_models": models})
}
