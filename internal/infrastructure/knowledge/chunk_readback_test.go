package knowledge

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	domain "control-panel/internal/domain/knowledge"
	"github.com/stretchr/testify/require"
)

func TestUpdateChunkReadsBackNullAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, patch, get string
		reads            int
		wantError        bool
	}{
		{"null", `{"code":0,"data":null}`, `{"code":0,"data":{"content_with_weight":"updated","doc_id":"d","tag_kwd":["v2"]}}`, 1, false},
		{"object", `{"code":0,"data":{"content":"updated","id":"c","document_id":"d"}}`, "", 0, false},
		{"patch-failure", `{"code":100,"data":null}`, "", 0, true},
		{"read-failure", `{"code":0,"data":null}`, `{"code":100,"data":null}`, 1, true},
		{"read-null", `{"code":0,"data":null}`, `{"code":0,"data":null}`, 1, true},
		{"read-empty", `{"code":0,"data":null}`, `{"code":0,"data":{}}`, 1, true},
		{"read-false", `{"code":0,"data":null}`, `{"code":0,"data":false}`, 1, true},
		{"wrong-chunk", `{"code":0,"data":null}`, `{"code":0,"data":{"id":"other","doc_id":"d"}}`, 1, true},
		{"wrong-document", `{"code":0,"data":null}`, `{"code":0,"data":{"doc_id":"other","content":"updated"}}`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/datasets/kb/documents/d/chunks/c", r.URL.Path)
				require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
				if r.Method == http.MethodPatch {
					_, _ = io.WriteString(w, tc.patch)
				} else {
					require.Equal(t, http.MethodGet, r.Method)
					reads++
					_, _ = io.WriteString(w, tc.get)
				}
			}))
			defer upstream.Close()
			client := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour)
			chunk, err := client.UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, chunk)
			} else {
				require.NoError(t, err)
				require.Equal(t, "updated", (*chunk)["content"])
				require.Equal(t, "c", (*chunk)["id"])
				require.Equal(t, "d", (*chunk)["document_id"])
			}
			require.Equal(t, tc.reads, reads)
		})
	}
}

func TestUpdateChunkRetriesOnlyVisibilityGetAfterOnePatch(t *testing.T) {
	var patches, reads atomic.Int32
	var visibleAt atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/datasets/kb/documents/d/chunks/c", r.URL.Path)
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		if r.Method == http.MethodPatch {
			patches.Add(1)
			visibleAt.Store(time.Now().Add(350 * time.Millisecond).UnixNano())
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
			return
		}
		require.Equal(t, http.MethodGet, r.Method)
		reads.Add(1)
		if time.Now().UnixNano() < visibleAt.Load() {
			_, _ = io.WriteString(w, `{"code":102,"message":"Chunk not found!","data":false}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":"c","doc_id":"d","content_with_weight":"updated"}}`)
	}))
	defer upstream.Close()
	started := time.Now()
	chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", 5*time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
	require.NoError(t, err)
	require.Equal(t, "updated", (*chunk)["content"])
	require.Equal(t, "c", (*chunk)["id"])
	require.Equal(t, "d", (*chunk)["document_id"])
	require.Equal(t, int32(1), patches.Load())
	require.GreaterOrEqual(t, reads.Load(), int32(2))
	require.LessOrEqual(t, reads.Load(), int32(11))
	require.Less(t, time.Since(started), 1500*time.Millisecond)
}

func TestUpdateChunkPersistentMissingHasOneSecondReadBudget(t *testing.T) {
	var patches, reads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patches.Add(1)
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
			return
		}
		reads.Add(1)
		_, _ = io.WriteString(w, `{"retcode":102,"retmsg":"Chunk not found!","data":false}`)
	}))
	defer upstream.Close()
	started := time.Now()
	chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", 5*time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
	require.Error(t, err)
	require.Nil(t, chunk)
	require.Equal(t, http.StatusBadGateway, domain.StatusCode(err))
	require.Contains(t, err.Error(), "已受理，但回读未确认")
	require.Equal(t, int32(1), patches.Load())
	require.GreaterOrEqual(t, reads.Load(), int32(2))
	require.LessOrEqual(t, reads.Load(), int32(11))
	require.GreaterOrEqual(t, time.Since(started), 800*time.Millisecond)
	require.Less(t, time.Since(started), 1500*time.Millisecond)
}

func TestUpdateChunkNeverRetriesPermissionsOtherErrorsOrEmptySuccess(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{200, `{"code":109,"message":"Chunk not found!","data":false}`},
		{200, `{"code":102,"message":"You don't own the dataset or document","data":false}`},
		{200, `{"code":102,"message":"Invalid request","data":false}`},
		{200, `{"code":100,"message":"Chunk not found!","data":false}`},
		{200, `{"code":109,"retcode":102,"retmsg":"Chunk not found!","data":false}`},
		{200, `{"code":102,"retmsg":"Chunk not found!","message":"No authorization","data":false}`},
		{200, `{"code":102,"message":"Chunk not found!","error":"No authorization","data":false}`},
		{200, `{"data":false,"message":"Chunk not found!"}`},
		{403, `{"code":102,"message":"Chunk not found!","data":false}`},
		{404, `{"code":102,"message":"Chunk not found!","data":false}`},
		{500, `{"code":102,"message":"Chunk not found!","data":false}`},
		{200, `{"code":0,"data":null}`}, {200, `{"code":0,"data":{}}`}, {200, `{"code":0,"data":false}`},
		{200, `{"code":0,"data":{"id":"other","doc_id":"d"}}`},
		{200, `{"code":0,"data":{"id":"c","doc_id":"other"}}`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			var patches, reads atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					patches.Add(1)
					_, _ = io.WriteString(w, `{"code":0,"data":null}`)
					return
				}
				reads.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
			require.Error(t, err)
			require.Nil(t, chunk)
			require.Equal(t, int32(1), patches.Load())
			require.Equal(t, int32(1), reads.Load())
		})
	}
}

func TestUpdateChunkCancellationInterruptsVisibilityWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var patches, reads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patches.Add(1)
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
			return
		}
		reads.Add(1)
		_, _ = io.WriteString(w, `{"code":102,"message":"Chunk not found!","data":false}`)
		time.AfterFunc(25*time.Millisecond, cancel)
	}))
	defer upstream.Close()
	started := time.Now()
	chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", 5*time.Second, time.Hour).UpdateChunk(ctx, "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
	require.Error(t, err)
	require.Nil(t, chunk)
	require.Equal(t, int32(1), patches.Load())
	require.Equal(t, int32(1), reads.Load())
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestUpdateChunkShorterContextAlsoBoundsInFlightRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var patches, reads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			patches.Add(1)
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
			return
		}
		reads.Add(1)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	started := time.Now()
	chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", 5*time.Second, time.Hour).UpdateChunk(ctx, "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
	require.Error(t, err)
	require.Nil(t, chunk)
	require.Equal(t, int32(1), patches.Load())
	require.Equal(t, int32(1), reads.Load())
	require.Less(t, time.Since(started), 500*time.Millisecond)
}

func TestUpdateChunkStopsAfterVisibilityRetryOnUnsafeReadback(t *testing.T) {
	for _, body := range []string{
		`{"code":109,"message":"No authorization","data":false}`,
		`{"code":102,"message":"You don't own the dataset or document","data":false}`,
		`{"code":0,"data":{"id":"other","doc_id":"d"}}`,
		`{"code":0,"data":{"id":"c","doc_id":"other"}}`,
		`{"code":0,"data":null}`, `{"code":0,"data":{}}`,
	} {
		t.Run(body, func(t *testing.T) {
			var patches, reads atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPatch {
					patches.Add(1)
					_, _ = io.WriteString(w, `{"code":0,"data":null}`)
					return
				}
				if reads.Add(1) == 1 {
					_, _ = io.WriteString(w, `{"code":102,"message":"Chunk not found","data":false}`)
				} else {
					_, _ = io.WriteString(w, body)
				}
			}))
			defer upstream.Close()
			chunk, err := NewRemoteMultiragEngine(upstream.URL, "key", 5*time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
			require.Error(t, err)
			require.Nil(t, chunk)
			require.Equal(t, int32(1), patches.Load())
			require.Equal(t, int32(2), reads.Load())
		})
	}
}
