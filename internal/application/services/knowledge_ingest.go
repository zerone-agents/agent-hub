package services

import (
	"context"
	"control-panel/internal/domain/knowledge"
)

func (s *KnowledgeService) IngestDocuments(ctx context.Context, req knowledge.DocumentIngestRequest) error {
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
