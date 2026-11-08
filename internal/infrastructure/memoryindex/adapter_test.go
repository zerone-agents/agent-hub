package memoryindex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	knowledge "control-panel/internal/infrastructure/knowledge"
	"control-panel/internal/memory"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All test resources live inside this private HTTP fixture; no deployment,
// provider, persistent dataset, or real user data is touched.
type remoteFixture struct {
	mu                                                  sync.Mutex
	t                                                   *testing.T
	document                                            map[string]any
	chunks                                              []map[string]any
	requests, creates, puts, removes                    int
	lostCreate, lostPut, lostDelete                     bool
	holdDelete, holdChunksOnDrop                        bool
	failedPut, malformedList, conflictingAlias          bool
	crossScopeSearch                                    bool
	searchBody                                          map[string]any
	omitChunkDataset, foreignParent, conflictingDataset bool
}

func newRemote(t *testing.T) (*Adapter, *remoteFixture) {
	f := &remoteFixture{t: t, chunks: []map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	engine := knowledge.NewRemoteMultiragEngine(server.URL, "synthetic-test-key", time.Second, time.Second)
	return New(engine), f
}

func (f *remoteFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	assert.Equal(f.t, "Bearer synthetic-test-key", r.Header.Get("Authorization"))
	w.Header().Set("Content-Type", "application/json")
	respond := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data}) }
	decode := func() map[string]any {
		var body map[string]any
		assert.NoError(f.t, json.NewDecoder(r.Body).Decode(&body))
		return body
	}
	switch {
	case r.URL.Path == "/api/v1/datasets/kb/documents" && r.Method == http.MethodGet:
		if f.malformedList {
			respond(nil)
			return
		}
		docs := []any{}
		if f.document != nil {
			ids := r.URL.Query()["ids"]
			if len(ids) == 0 || slices.Contains(ids, "doc") {
				docs = append(docs, f.document)
			}
		}
		respond(map[string]any{"docs": docs, "total": len(docs)})
	case r.URL.Path == "/api/v1/datasets/kb/documents" && r.Method == http.MethodPost:
		assert.Equal(f.t, "empty", r.URL.Query().Get("type"))
		body := decode()
		assert.Len(f.t, body, 1)
		f.creates++
		f.document = map[string]any{"id": "doc", "dataset_id": "kb", "name": body["name"], "status": "1"}
		if f.lostCreate {
			_, _ = w.Write([]byte("{"))
			return
		}
		respond(f.document)
	case r.URL.Path == "/api/v1/datasets/kb/documents" && r.Method == http.MethodDelete:
		body := decode()
		assert.Equal(f.t, []any{"doc"}, body["ids"])
		assert.NotContains(f.t, body, "delete_all")
		f.document = nil
		if !f.holdChunksOnDrop {
			f.chunks = []map[string]any{}
		}
		respond(true)
	case r.URL.Path == "/api/v1/datasets/kb/documents/doc/chunks" && r.Method == http.MethodGet:
		if f.malformedList {
			respond(map[string]any{"total": 0})
			return
		}
		chunks := make([]map[string]any, 0, len(f.chunks))
		for _, chunk := range f.chunks {
			copy := map[string]any{}
			for k, v := range chunk {
				copy[k] = v
			}
			if f.conflictingAlias {
				copy["doc_id"] = "other"
			}
			if f.omitChunkDataset {
				delete(copy, "dataset_id")
			}
			if f.conflictingDataset {
				copy["kb_id"] = "other-kb"
			}
			chunks = append(chunks, copy)
		}
		parent := map[string]any{"id": "doc", "dataset_id": "kb"}
		if f.foreignParent {
			parent["dataset_id"] = "other-kb"
		}
		respond(map[string]any{"total": len(chunks), "chunks": chunks, "doc": parent})
	case r.URL.Path == "/api/v1/datasets/kb/documents/doc/chunks" && r.Method == http.MethodPost:
		body := decode()
		assert.Equal(f.t, map[string]any{"content": body["content"]}, body)
		f.puts++
		if f.failedPut {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 102, "message": "secret user text must not escape"})
			return
		}
		chunk := map[string]any{"id": "returned_hash", "document_id": "doc", "dataset_id": "kb", "content": body["content"], "available": true}
		f.chunks = append(f.chunks, chunk)
		if f.lostPut {
			_, _ = w.Write([]byte("{"))
			return
		}
		created := map[string]any{}
		for k, v := range chunk {
			created[k] = v
		}
		if f.omitChunkDataset {
			delete(created, "dataset_id")
		}
		respond(map[string]any{"chunk": created})
	case r.URL.Path == "/api/v1/datasets/kb/documents/doc/chunks" && r.Method == http.MethodPatch:
		body := decode()
		assert.Equal(f.t, map[string]any{"chunk_ids": []any{"returned_hash"}, "available": false}, body)
		for _, chunk := range f.chunks {
			chunk["available"] = false
		}
		respond(true)
	case r.URL.Path == "/api/v1/datasets/kb/documents/doc/chunks" && r.Method == http.MethodDelete:
		body := decode()
		assert.Equal(f.t, map[string]any{"chunk_ids": []any{"returned_hash"}}, body)
		f.removes++
		if !f.holdDelete {
			f.chunks = []map[string]any{}
		}
		if f.lostDelete {
			_, _ = w.Write([]byte("{"))
			return
		}
		respond(true)
	case r.URL.Path == "/api/v1/datasets/search":
		body := decode()
		f.searchBody = body
		assert.Equal(f.t, []any{"kb"}, body["dataset_ids"])
		assert.Equal(f.t, []any{"doc"}, body["doc_ids"])
		assert.Equal(f.t, false, body["use_kg"])
		assert.NotContains(f.t, body, "search_id")
		chunks := []any{}
		for _, chunk := range f.chunks {
			copy := map[string]any{}
			for k, v := range chunk {
				copy[k] = v
			}
			if f.crossScopeSearch {
				copy["document_id"] = "other"
			}
			chunks = append(chunks, copy)
		}
		respond(map[string]any{"total": len(chunks), "chunks": chunks})
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		http.Error(w, "unexpected", http.StatusNotFound)
	}
}

func (f *remoteFixture) update(fn func()) { f.mu.Lock(); defer f.mu.Unlock(); fn() }

func prepare(t *testing.T, a *Adapter) {
	t.Helper()
	id, err := a.EnsureDocument(context.Background(), "kb", "memory_scope_123", true)
	require.NoError(t, err)
	require.Equal(t, "doc", id)
}

func TestEnsureDocumentRecoversByNameAndNeverBlindlyCreates(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint("lost=", lost), func(t *testing.T) {
			a, f := newRemote(t)
			f.update(func() { f.lostCreate = lost })
			prepare(t, a)
			id, err := a.EnsureDocument(context.Background(), "kb", "memory_scope_123", false)
			require.NoError(t, err)
			require.Equal(t, "doc", id)
			f.update(func() { require.Equal(t, 1, f.creates) })
		})
	}
	a, f := newRemote(t)
	_, err := a.EnsureDocument(context.Background(), "kb", "memory_scope_123", false)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() { require.Zero(t, f.creates) })
}

func TestPutExactContentDedupAndLostResponseRecovery(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint("lost=", lost), func(t *testing.T) {
			a, f := newRemote(t)
			prepare(t, a)
			f.update(func() { f.lostPut = lost })
			for range 2 {
				id, err := a.Put(context.Background(), "kb", "doc", "合成测试：我喜欢蓝色。")
				require.NoError(t, err)
				require.Equal(t, "returned_hash", id)
			}
			f.update(func() { require.Equal(t, 1, f.puts) })
		})
	}
}

func TestPutBusinessFailureStaysUnknownWithoutRepeating(t *testing.T) {
	a, f := newRemote(t)
	prepare(t, a)
	f.update(func() { f.failedPut = true })
	_, err := a.Put(context.Background(), "kb", "doc", "合成测试事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	require.NotContains(t, err.Error(), "secret")
	f.update(func() { require.Equal(t, 1, f.puts) })
	id, err := a.Find(context.Background(), "kb", "doc", "合成测试事实")
	require.NoError(t, err)
	require.Empty(t, id)
	f.update(func() { require.Equal(t, 1, f.puts) })
}

func TestDisabledExactContentCannotBeReportedAsSynced(t *testing.T) {
	a, f := newRemote(t)
	prepare(t, a)
	_, err := a.Put(context.Background(), "kb", "doc", "合成事实")
	require.NoError(t, err)
	f.update(func() { f.chunks[0]["available"] = false })
	_, err = a.Put(context.Background(), "kb", "doc", "合成事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() { require.Equal(t, 1, f.puts) })
}

func TestChunkDatasetOmissionRequiresConfirmedParentScope(t *testing.T) {
	a, f := newRemote(t)
	prepare(t, a)
	f.update(func() { f.omitChunkDataset = true })
	id, err := a.Put(context.Background(), "kb", "doc", "合成旧版DTO事实")
	require.NoError(t, err)
	require.Equal(t, "returned_hash", id)
	read, err := a.Find(context.Background(), "kb", "doc", "合成旧版DTO事实")
	require.NoError(t, err)
	require.Equal(t, id, read)
	f.update(func() { f.foreignParent = true })
	_, err = a.Find(context.Background(), "kb", "doc", "合成旧版DTO事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() { f.foreignParent = false; f.conflictingDataset = true })
	_, err = a.Find(context.Background(), "kb", "doc", "合成旧版DTO事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
}

func TestDeleteRequiresIndependentAbsence(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprint("held=", held), func(t *testing.T) {
			a, f := newRemote(t)
			prepare(t, a)
			_, err := a.Put(context.Background(), "kb", "doc", "合成事实")
			require.NoError(t, err)
			f.update(func() { f.holdDelete, f.lostDelete = held, true })
			err = a.Remove(context.Background(), "kb", "doc", "returned_hash")
			if held {
				require.ErrorIs(t, err, memory.ErrIndexUnknown)
			} else {
				require.NoError(t, err)
			}
			f.update(func() { require.Equal(t, 1, f.removes) })
		})
	}
}

func TestDropDocumentChecksSearchCopyAfterMetadataDisappears(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprint("held=", held), func(t *testing.T) {
			a, f := newRemote(t)
			prepare(t, a)
			_, err := a.Put(context.Background(), "kb", "doc", "合成事实")
			require.NoError(t, err)
			f.update(func() { f.holdChunksOnDrop = held })
			err = a.DropDocument(context.Background(), "kb", "doc")
			if held {
				require.ErrorIs(t, err, memory.ErrIndexUnknown)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSearchWhitelistAndScopeRejection(t *testing.T) {
	a, f := newRemote(t)
	prepare(t, a)
	_, err := a.Put(context.Background(), "kb", "doc", "合成事实")
	require.NoError(t, err)
	hits, err := a.Search(context.Background(), "kb", "doc", "测试", 5)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Equal(t, "doc", hits[0].DocumentID)
	f.update(func() { f.crossScopeSearch = true })
	_, err = a.Search(context.Background(), "kb", "doc", "测试", 5)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() {
		require.NotContains(t, f.searchBody, "search_id")
		require.NotContains(t, f.searchBody, "document_ids")
	})
}

func TestEmptyScopeMakesNoRequests(t *testing.T) {
	a, f := newRemote(t)
	ctx := context.Background()
	_, err := a.Search(ctx, "kb", "", "test", 5)
	require.ErrorIs(t, err, memory.ErrInvalid)
	_, err = a.Put(ctx, "", "doc", "test")
	require.ErrorIs(t, err, memory.ErrInvalid)
	require.ErrorIs(t, a.Remove(ctx, "kb", "doc", ""), memory.ErrInvalid)
	require.ErrorIs(t, a.DropDocument(ctx, "kb", ""), memory.ErrInvalid)
	_, err = a.EnsureDocument(ctx, "", "memory_name", true)
	require.ErrorIs(t, err, memory.ErrInvalid)
	f.update(func() { require.Zero(t, f.requests) })
}

func TestMalformedReadbackAndConflictingAliasesFailClosed(t *testing.T) {
	a, f := newRemote(t)
	prepare(t, a)
	_, err := a.Put(context.Background(), "kb", "doc", strings.Repeat("合成", 100))
	require.NoError(t, err)
	f.update(func() { f.conflictingAlias = true })
	_, err = a.Find(context.Background(), "kb", "doc", "合成事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() { f.conflictingAlias, f.malformedList = false, true })
	_, err = a.Find(context.Background(), "kb", "doc", "合成事实")
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	_, err = a.EnsureDocument(context.Background(), "kb", "memory_scope_123", true)
	require.ErrorIs(t, err, memory.ErrIndexUnknown)
	f.update(func() { require.Equal(t, 1, f.creates) })
}
