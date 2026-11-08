package knowledge

import (
	"context"
	"net/http"
	"net/url"

	domain "control-panel/internal/domain/knowledge"
)

// CreateEmptyDocument is the standard virtual-document API. It intentionally
// does not start parsing or submit an ingest job.
func (c *RemoteMultiragEngine) CreateEmptyDocument(ctx context.Context, datasetID, name string) (*domain.Document, error) {
	var raw map[string]any
	_, err := c.doJSON(ctx, http.MethodPost, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents", url.Values{"type": {"empty"}}, map[string]string{"name": name}, &raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, domain.NewUpstreamError("空白文档创建结果未确认", nil)
	}
	doc := domain.NormalizeDocument(raw)
	return &doc, nil
}

// MemoryListDocuments keeps the same wire contract as ListDocuments, while
// requiring an explicit total and array. Missing data must never be interpreted
// as confirmed absence during unknown-write recovery or deletion readback.
func (c *RemoteMultiragEngine) MemoryListDocuments(ctx context.Context, datasetID string, page, pageSize int, keywords string, ids []string) (*domain.DocumentListResult, error) {
	query := url.Values{}
	addInt(query, "page", page)
	addInt(query, "page_size", pageSize)
	addString(query, "keywords", keywords)
	for _, id := range ids {
		query.Add("ids", id)
	}
	var raw struct {
		Total *int             `json:"total"`
		Docs  *[]domain.Object `json:"docs"`
	}
	_, err := c.doJSON(ctx, http.MethodGet, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/documents", query, nil, &raw)
	if err != nil {
		return nil, err
	}
	if raw.Total == nil || *raw.Total < 0 || raw.Docs == nil || *raw.Total < len(*raw.Docs) || len(*raw.Docs) > pageSize {
		return nil, domain.NewUpstreamError("记忆文档回读格式未确认", nil)
	}
	docs := make([]domain.Document, 0, len(*raw.Docs))
	for _, doc := range *raw.Docs {
		docs = append(docs, domain.NormalizeDocument(doc))
	}
	return &domain.DocumentListResult{Total: *raw.Total, Documents: docs}, nil
}

// MemoryListChunks is used without keywords/highlighting, preserving exact
// content for deduplication and write recovery.
func (c *RemoteMultiragEngine) MemoryListChunks(ctx context.Context, datasetID, documentID string, page, pageSize int) (*domain.ChunkListResult, error) {
	query := url.Values{}
	addInt(query, "page", page)
	addInt(query, "page_size", pageSize)
	var raw struct {
		Total  *int             `json:"total"`
		Chunks *[]domain.Object `json:"chunks"`
		Doc    domain.Object    `json:"doc"`
	}
	path := "/api/v1/datasets/" + url.PathEscape(datasetID) + "/documents/" + url.PathEscape(documentID) + "/chunks"
	_, err := c.doJSON(ctx, http.MethodGet, path, query, nil, &raw)
	if err != nil {
		return nil, err
	}
	if raw.Total == nil || *raw.Total < 0 || raw.Chunks == nil || raw.Doc == nil || *raw.Total < len(*raw.Chunks) || len(*raw.Chunks) > pageSize {
		return nil, domain.NewUpstreamError("记忆分块回读格式未确认", nil)
	}
	chunks := make([]domain.Chunk, 0, len(*raw.Chunks))
	for _, chunk := range *raw.Chunks {
		// Validate before normalization can overwrite a conflicting alias.
		for _, aliases := range [][]string{{"id", "chunk_id"}, {"doc_id", "document_id"}, {"kb_id", "dataset_id"}} {
			if _, valid := retrievalIdentity(chunk, aliases...); !valid {
				return nil, domain.NewUpstreamError("记忆分块回读字段不一致", nil)
			}
		}
		if content, present := chunk["content"]; present {
			text, ok := content.(string)
			if !ok {
				return nil, domain.NewUpstreamError("记忆分块正文格式无效", nil)
			}
			if alias, exists := chunk["content_with_weight"]; exists {
				other, ok := alias.(string)
				if !ok || text != other {
					return nil, domain.NewUpstreamError("记忆分块正文不一致", nil)
				}
			}
		}
		chunks = append(chunks, domain.NormalizeChunk(chunk))
	}
	return &domain.ChunkListResult{Total: *raw.Total, Chunks: chunks, Document: domain.NormalizeDocument(raw.Doc)}, nil
}
