// Package memoryindex adapts a dedicated MultiRAG dataset to the memory ledger.
// It never accepts a generic retrieval payload or retries a mutation internally.
package memoryindex

import (
	"context"
	"regexp"
	"strings"

	domain "control-panel/internal/domain/knowledge"
	knowledge "control-panel/internal/infrastructure/knowledge"
	"control-panel/internal/memory"
)

const pageSize, maxPages = 100, 100

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
var internalName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Adapter struct {
	engine *knowledge.RemoteMultiragEngine
}

func New(engine *knowledge.RemoteMultiragEngine) *Adapter { return &Adapter{engine: engine} }

var _ memory.Index = (*Adapter)(nil)

func unknown() error { return memory.ErrIndexUnknown }

func (a *Adapter) validScope(datasetID, documentID string) bool {
	return a != nil && a.engine != nil && identifier.MatchString(datasetID) && identifier.MatchString(documentID)
}

// EnsureDocument only sends a create when explicitly allowed by the durable
// outbox. A retry after an ambiguous create must pass allowCreate=false.
func (a *Adapter) EnsureDocument(ctx context.Context, datasetID, name string, allowCreate bool) (string, error) {
	if a == nil || a.engine == nil || !identifier.MatchString(datasetID) || !internalName.MatchString(name) {
		return "", memory.ErrInvalid
	}
	id, err := a.findDocument(ctx, datasetID, name)
	if err != nil || id != "" {
		return id, err
	}
	if !allowCreate {
		return "", unknown()
	}
	created, createErr := a.engine.CreateEmptyDocument(ctx, datasetID, name)
	// An HTTP failure can happen after commit. Recover by name, never by
	// issuing a second create or manufacturing an upstream content hash.
	id, readErr := a.findDocument(ctx, datasetID, name)
	if readErr != nil || id == "" {
		return "", unknown()
	}
	if createErr == nil {
		if created == nil || !sameDocument(map[string]any(*created), datasetID, id) || (*created)["name"] != name {
			return "", unknown()
		}
	}
	return id, nil
}

func (a *Adapter) findDocument(ctx context.Context, datasetID, name string) (string, error) {
	found, seen, total := "", map[string]bool{}, -1
	for page := 1; page <= maxPages; page++ {
		result, err := a.engine.MemoryListDocuments(ctx, datasetID, page, pageSize, name, nil)
		if err != nil || result == nil || result.Total > pageSize*maxPages || (total >= 0 && total != result.Total) {
			return "", unknown()
		}
		total = result.Total
		for _, doc := range result.Documents {
			id, ok := identity(map[string]any(doc), "id", "document_id", "doc_id")
			if !ok || !identifier.MatchString(id) || seen[id] || !sameDocument(map[string]any(doc), datasetID, id) {
				return "", unknown()
			}
			seen[id] = true
			if doc["name"] == name {
				if found != "" {
					return "", unknown()
				}
				found = id
			}
		}
		if len(seen) == total {
			return found, nil
		}
		if len(result.Documents) != pageSize || len(seen) > total {
			return "", unknown()
		}
	}
	return "", unknown()
}

// Find performs a read-only exact-content scan. Empty ID with nil error means
// the complete bounded snapshot contains no match; it is not proof that an
// earlier remote request cannot finish later.
func (a *Adapter) Find(ctx context.Context, datasetID, documentID, content string) (string, error) {
	if !a.validScope(datasetID, documentID) || strings.TrimSpace(content) == "" {
		return "", memory.ErrInvalid
	}
	chunks, err := a.chunks(ctx, datasetID, documentID)
	if err != nil {
		return "", err
	}
	found := ""
	for _, chunk := range chunks {
		if chunk["content"] == content {
			if chunk["available"] != true {
				// A prior delete may have disabled the content-addressed chunk.
				// Mapping a new fact to it would falsely report search readiness.
				return "", unknown()
			}
			id, _ := identity(map[string]any(chunk), "id", "chunk_id")
			if found != "" && found != id {
				return "", unknown()
			}
			found = id
		}
	}
	return found, nil
}

func (a *Adapter) Put(ctx context.Context, datasetID, documentID, content string) (string, error) {
	id, err := a.Find(ctx, datasetID, documentID, content)
	if err != nil || id != "" {
		return id, err
	}
	created, createErr := a.engine.CreateChunk(ctx, datasetID, documentID, domain.ChunkMutationRequest{"content": content})
	id, err = a.Find(ctx, datasetID, documentID, content)
	if err != nil || id == "" {
		return "", unknown()
	}
	if createErr == nil {
		// Find independently bound this chunk to a parent document whose
		// dataset identity was checked. Older standard chunk DTOs omit kb_id.
		if created == nil || !sameListedChunk(map[string]any(*created), datasetID, documentID, id) || (*created)["content"] != content {
			return "", unknown()
		}
	}
	return id, nil
}

func (a *Adapter) Remove(ctx context.Context, datasetID, documentID, chunkID string) error {
	if !a.validScope(datasetID, documentID) || !identifier.MatchString(chunkID) {
		return memory.ErrInvalid
	}
	chunks, err := a.chunks(ctx, datasetID, documentID)
	if err != nil {
		return err
	}
	present := false
	for _, chunk := range chunks {
		if chunk["id"] == chunkID {
			present = true
		}
	}
	if !present {
		return nil
	}
	// Disable before physical deletion. Even an ambiguous acknowledgement is
	// followed by only scoped readback; a write is sent at most once per call.
	if err := a.engine.SwitchChunks(ctx, datasetID, documentID, []string{chunkID}, false); err != nil {
		return unknown()
	}
	deleteErr := a.engine.DeleteChunks(ctx, datasetID, documentID, domain.DeleteChunksRequest{ChunkIDs: []string{chunkID}})
	chunks, err = a.chunks(ctx, datasetID, documentID)
	if err != nil {
		return unknown()
	}
	for _, chunk := range chunks {
		if chunk["id"] == chunkID {
			return unknown()
		}
	}
	// Absence is independently observed, even if the delete response was lost.
	_ = deleteErr
	return nil
}

func (a *Adapter) DropDocument(ctx context.Context, datasetID, documentID string) error {
	if !a.validScope(datasetID, documentID) {
		return memory.ErrInvalid
	}
	result, err := a.engine.MemoryListDocuments(ctx, datasetID, 1, pageSize, "", []string{documentID})
	if err != nil || result == nil || result.Total != len(result.Documents) {
		return unknown()
	}
	for _, doc := range result.Documents {
		if !sameDocument(map[string]any(doc), datasetID, documentID) {
			return unknown()
		}
	}
	if len(result.Documents) > 0 {
		_ = a.engine.DeleteDocuments(ctx, datasetID, domain.DeleteRequest{IDs: []string{documentID}})
	}
	result, err = a.engine.MemoryListDocuments(ctx, datasetID, 1, pageSize, "", []string{documentID})
	if err != nil || result == nil || result.Total != 0 || len(result.Documents) != 0 {
		return unknown()
	}
	// Deleted SQL metadata alone does not prove the search copy is gone. A
	// scoped search must independently observe zero remaining results.
	hits, err := a.Search(ctx, datasetID, documentID, "memory cleanup verification", 1)
	if err != nil || len(hits) != 0 {
		return unknown()
	}
	return nil
}

func (a *Adapter) Search(ctx context.Context, datasetID, documentID, query string, limit int) ([]memory.Hit, error) {
	if !a.validScope(datasetID, documentID) || strings.TrimSpace(query) == "" || limit < 1 || limit > 100 {
		return nil, memory.ErrInvalid
	}
	result, err := a.engine.Retrieval(ctx, domain.RetrievalRequest{
		"dataset_ids": []string{datasetID}, "doc_ids": []string{documentID},
		"question": query, "size": limit, "top_k": limit,
		"similarity_threshold": 0, "use_kg": false, "highlight": false,
	})
	if err != nil || result == nil {
		return nil, unknown()
	}
	items, ok := (*result)["chunks"].([]any)
	if !ok || len(items) > limit {
		return nil, unknown()
	}
	hits := make([]memory.Hit, 0, len(items))
	seen := map[string]bool{}
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, unknown()
		}
		id, valid := identity(item, "id", "chunk_id")
		content, contentOK := item["content"].(string)
		if !valid || !identifier.MatchString(id) || seen[id] || !sameChunk(item, datasetID, documentID, id) || !contentOK || strings.TrimSpace(content) == "" {
			return nil, unknown()
		}
		seen[id] = true
		score, _ := item["similarity"].(float64)
		hits = append(hits, memory.Hit{ChunkID: id, DocumentID: documentID, Content: content, Score: score})
	}
	if len(items) == 0 {
		if total, ok := (*result)["total"].(float64); !ok || total != 0 {
			return nil, unknown()
		}
	}
	return hits, nil
}

func (a *Adapter) chunks(ctx context.Context, datasetID, documentID string) ([]domain.Chunk, error) {
	all, seen, total := []domain.Chunk{}, map[string]bool{}, -1
	for page := 1; page <= maxPages; page++ {
		result, err := a.engine.MemoryListChunks(ctx, datasetID, documentID, page, pageSize)
		if err != nil || result == nil || !sameDocument(map[string]any(result.Document), datasetID, documentID) || result.Total > pageSize*maxPages || (total >= 0 && total != result.Total) {
			return nil, unknown()
		}
		total = result.Total
		for _, chunk := range result.Chunks {
			id, valid := identity(map[string]any(chunk), "id", "chunk_id")
			if !valid || !identifier.MatchString(id) || seen[id] || !sameListedChunk(map[string]any(chunk), datasetID, documentID, id) {
				return nil, unknown()
			}
			seen[id] = true
			all = append(all, chunk)
		}
		if len(all) == total {
			return all, nil
		}
		if len(result.Chunks) != pageSize || len(all) > total {
			return nil, unknown()
		}
	}
	return nil, unknown()
}

func sameDocument(item map[string]any, datasetID, documentID string) bool {
	id, ok := identity(item, "id", "document_id", "doc_id")
	owner, ownerOK := identity(item, "dataset_id", "kb_id")
	return ok && ownerOK && id == documentID && owner == datasetID
}

func sameChunk(item map[string]any, datasetID, documentID, chunkID string) bool {
	id, ok := identity(item, "id", "chunk_id")
	doc, docOK := identity(item, "document_id", "doc_id")
	owner, ownerOK := identity(item, "dataset_id", "kb_id")
	return ok && docOK && ownerOK && id == chunkID && doc == documentID && owner == datasetID
}

// Only use this after a scoped list read confirms both parent identities (or
// after Find established that readback). The standard list DTO on deployed
// versions includes document_id but may omit dataset_id. Ownership aliases,
// when supplied, must still be valid and match the confirmed parent.
func sameListedChunk(item map[string]any, datasetID, documentID, chunkID string) bool {
	id, ok := identity(item, "id", "chunk_id")
	doc, docOK := identity(item, "document_id", "doc_id")
	if !ok || !docOK || id != chunkID || doc != documentID {
		return false
	}
	_, hasDataset := item["dataset_id"]
	_, hasKB := item["kb_id"]
	if !hasDataset && !hasKB {
		return true
	}
	owner, valid := identity(item, "dataset_id", "kb_id")
	return valid && owner == datasetID
}

// All aliases must agree; no type coercion or preferred alias can hide a
// conflicting identity supplied by a stale or mis-scoped upstream response.
func identity(item map[string]any, aliases ...string) (string, bool) {
	value := ""
	for _, alias := range aliases {
		if raw, present := item[alias]; present {
			text, ok := raw.(string)
			if !ok || text == "" || strings.TrimSpace(text) != text || (value != "" && text != value) {
				return "", false
			}
			value = text
		}
	}
	return value, value != ""
}
