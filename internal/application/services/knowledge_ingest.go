package services

import (
	"context"
	"control-panel/internal/domain/knowledge"
)

func (s *KnowledgeService) IngestDocuments(ctx context.Context, req knowledge.DocumentIngestRequest) error {
	if s.datasetGuard != nil {
		reserved, err := s.datasetGuard.IDs(ctx)
		if err != nil {
			return err
		}
		if len(reserved) > 0 {
			return knowledge.NewBadRequestError("记忆库隔离期间请通过指定知识库的解析入口操作文档")
		}
	}
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	if err := req.Validate(); err != nil {
		return err
	}
	ingest, ok := engine.(knowledge.DocumentIngestEngine)
	if !ok {
		return knowledge.NewUnavailableError("文档摄取能力未配置")
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(req.DocIDs))
	for _, id := range req.DocIDs {
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	req.DocIDs = ids
	return ingest.IngestDocuments(ctx, req)
}
