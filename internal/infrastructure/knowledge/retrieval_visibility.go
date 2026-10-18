package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	retrievalParentBatchSize = 100
	retrievalParentMaxDocs   = 1000
	retrievalParentMaxReads  = 32
)

var retrievalParentID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

type retrievalSource struct{ datasetID, documentID string }

// Check before renameInbound can overwrite a conflicting source alias. The
// index can lag SQL status, so neither a chunk's available_int nor its model
// supplied source IDs prove the parent document is still readable/enabled.
func (c *RemoteMultiragEngine) verifyRetrievalParents(ctx context.Context, req domain.RetrievalRequest, raw map[string]any) error {
	unconfirmed := func() error {
		return domain.NewUpstreamError("检索来源的权限或启用状态未确认，请刷新后重试；本次未返回结果", nil)
	}
	selected := []string{}
	allowedKBs := map[string]bool{}
	for _, id := range req["dataset_ids"].([]string) {
		if !retrievalParentID.MatchString(id) {
			return unconfirmed()
		}
		if !allowedKBs[id] {
			selected = append(selected, id)
			allowedKBs[id] = true
		}
	}
	allowedDocs := map[string]bool{}
	scope, scoped := req["doc_ids"].([]string)
	for _, id := range scope {
		allowedDocs[id] = true
	}
	chunks, ok := raw["chunks"].([]any)
	if !ok || len(chunks) > 4096 {
		return unconfirmed()
	}
	sources := []retrievalSource{}
	docs := map[string]bool{}
	for _, field := range []string{"chunks", "doc_aggs"} {
		items := chunks
		if field == "doc_aggs" {
			if _, present := raw[field]; !present {
				continue
			}
			items, ok = raw[field].([]any)
			if !ok || len(items) > 4096 {
				return unconfirmed()
			}
		}
		for _, value := range items {
			item, ok := value.(map[string]any)
			if !ok {
				return unconfirmed()
			}
			kbID, kbOK := retrievalIdentity(item, "kb_id", "dataset_id")
			docID, docOK := retrievalIdentity(item, "doc_id", "document_id")
			if !kbOK || !docOK || !retrievalParentID.MatchString(docID) || (kbID != "" && !allowedKBs[kbID]) || (scoped && !allowedDocs[docID]) {
				return unconfirmed()
			}
			if field == "chunks" {
				chunkID, valid := retrievalIdentity(item, "id", "chunk_id")
				if kbID == "" || !valid || chunkID == "" || !retrievalSourceEnabled(item) {
					return unconfirmed()
				}
			}
			sources = append(sources, retrievalSource{kbID, docID})
			docs[docID] = true
			if len(docs) > retrievalParentMaxDocs {
				return unconfirmed()
			}
		}
	}
	if len(docs) == 0 {
		if total, ok := raw["total"].(float64); ok && total > 0 {
			return unconfirmed()
		}
		return nil
	}
	// Plan the entire read cost before issuing any parent requests. No retries,
	// unbounded pagination, or per-chunk request fanout are permitted.
	if len(selected)*((len(docs)+retrievalParentBatchSize-1)/retrievalParentBatchSize) > retrievalParentMaxReads {
		return unconfirmed()
	}
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	owners := map[string]string{}
	for _, kbID := range selected {
		for start := 0; start < len(ids); start += retrievalParentBatchSize {
			end := min(start+retrievalParentBatchSize, len(ids))
			batch := ids[start:end]
			rows, err := c.readRetrievalParents(ctx, kbID, batch)
			if err != nil {
				return unconfirmed()
			}
			requested := map[string]bool{}
			for _, id := range batch {
				requested[id] = true
			}
			for _, row := range rows {
				docID, docOK := retrievalIdentity(map[string]any(row), "id", "doc_id", "document_id")
				owner, kbOK := retrievalIdentity(map[string]any(row), "kb_id", "dataset_id")
				if !docOK || !kbOK || !requested[docID] || owner != kbID || owners[docID] != "" || row["status"] != "1" || !retrievalSourceEnabled(map[string]any(row)) {
					return unconfirmed()
				}
				owners[docID] = owner
			}
		}
	}
	for _, source := range sources {
		owner := owners[source.documentID]
		if owner == "" || (source.datasetID != "" && source.datasetID != owner) {
			return unconfirmed()
		}
	}
	return nil
}

func (c *RemoteMultiragEngine) readRetrievalParents(ctx context.Context, kbID string, ids []string) ([]domain.Object, error) {
	query := url.Values{"page": []string{"1"}, "page_size": []string{"100"}, "ids": ids}
	var raw struct {
		Total *int            `json:"total"`
		Docs  []domain.Object `json:"docs"`
	}
	_, err := c.doJSON(ctx, http.MethodGet, "/api/v1/datasets/"+url.PathEscape(kbID)+"/documents", query, nil, &raw)
	if err != nil || raw.Total == nil || raw.Docs == nil || *raw.Total != len(raw.Docs) || len(raw.Docs) > len(ids) {
		return nil, domain.NewUpstreamError("检索父文档回读未确认", nil)
	}
	return raw.Docs, nil
}

// Missing aliases remain missing. Present aliases must be complete and agree;
// never trim, pick a preferred value, or manufacture a missing source ID.
func retrievalIdentity(item map[string]any, aliases ...string) (string, bool) {
	id := ""
	for _, alias := range aliases {
		value, exists := item[alias]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || text == "" || len(text) > 128 || strings.TrimSpace(text) != text || (id != "" && id != text) {
			return "", false
		}
		id = text
	}
	return id, true
}

func retrievalSourceEnabled(item map[string]any) bool {
	for _, flag := range []string{"available", "enabled"} {
		if value, present := item[flag]; present && value != true {
			return false
		}
	}
	if value, present := item["available_int"]; present && value != float64(1) && value != "1" {
		return false
	}
	return true
}
