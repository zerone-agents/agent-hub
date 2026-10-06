package services

import (
	"context"
	"control-panel/internal/domain/knowledge"
)

func (s *KnowledgeService) KnowledgePage(ctx context.Context, req knowledge.PageRequest) (any, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	if req.Operation == "tags-aggregation" {
		if req.Query, err = knowledge.TagsAggregationDatasetsQuery(req.Query); err != nil {
			return nil, err
		}
	}
	if req.Operation != "metadata-keys" && req.Operation != "metadata-flattened" && req.Operation != "tags-aggregation" {
		if req.DatasetID, err = requireID("datasetId", req.DatasetID); err != nil {
			return nil, err
		}
	}
	page, ok := engine.(knowledge.PageEngine)
	if !ok {
		return nil, knowledge.NewUnavailableError("知识库扩展能力未配置")
	}
	return page.KnowledgePage(ctx, req)
}
