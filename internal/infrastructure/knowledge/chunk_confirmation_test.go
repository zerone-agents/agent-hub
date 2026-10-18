package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpdateChunkWaitsForAllRequestedFieldsAfterStaleSuccess(t *testing.T) {
	var patches, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			patches.Add(1)
			// Even an object acknowledgement is not an independent durable read.
			_, _ = io.WriteString(w, `{"code":0,"data":{"content":"updated","tag_kwd":["new"]}}`)
			return
		}
		content, tag := "old-sensitive-content", "old-sensitive-tag"
		switch reads.Add(1) {
		case 1:
		case 2:
			content = "updated"
		default:
			content, tag = "updated", "new"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"id": "c", "doc_id": "d", "kb_id": "kb", "content_with_weight": content, "tag_kwd": []string{tag}, "available_int": 1, "unmodified": "unrelated"}})
	}))
	defer server.Close()
	chunk, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated", "tag_kwd": []string{"new"}})
	require.NoError(t, err)
	require.Equal(t, "updated", (*chunk)["content"])
	require.Equal(t, []any{"new"}, (*chunk)["tag_kwd"])
	require.Equal(t, int32(1), patches.Load())
	require.Equal(t, int32(3), reads.Load())
}

func TestUpdateChunkPersistentStaleOrCancelledNeverReplaysPatch(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-budget", true: "cancelled-stale"}[cancelRead], func(t *testing.T) {
			var patches, reads atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PATCH" {
					patches.Add(1)
					_, _ = io.WriteString(w, `{"code":0,"data":null}`)
					return
				}
				reads.Add(1)
				_, _ = io.WriteString(w, `{"code":0,"data":{"id":"c","doc_id":"d","content_with_weight":"old-sensitive-content"}}`)
				if cancelRead {
					time.AfterFunc(25*time.Millisecond, cancel)
				}
			}))
			defer server.Close()
			started := time.Now()
			chunk, err := NewRemoteMultiragEngine(server.URL, "key", 5*time.Second, time.Hour).UpdateChunk(ctx, "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
			require.Error(t, err)
			require.Nil(t, chunk)
			require.Equal(t, 502, domain.StatusCode(err))
			require.Contains(t, err.Error(), "回读未确认")
			require.NotContains(t, err.Error(), "old-sensitive-content")
			require.Equal(t, int32(1), patches.Load())
			require.LessOrEqual(t, reads.Load(), int32(11))
			if cancelRead {
				require.True(t, errors.Is(err, context.Canceled))
				require.Equal(t, int32(1), reads.Load())
				require.Less(t, time.Since(started), 500*time.Millisecond)
			} else {
				require.GreaterOrEqual(t, reads.Load(), int32(2))
				require.Less(t, time.Since(started), 1500*time.Millisecond)
			}
		})
	}
}

func TestChunkReadbackOnlyComparesModifiedFieldsWithAliases(t *testing.T) {
	for _, tc := range []struct {
		expected, actual map[string]any
		matches          bool
	}{
		{map[string]any{"tag_kwd": []string{}}, map[string]any{"tag_kwd": []any{}, "content_with_weight": "unmodified"}, true},
		{map[string]any{"tag_feas": map[string]any{}}, map[string]any{"tag_feas": map[string]any{}}, true},
		{map[string]any{"available": false}, map[string]any{"available_int": float64(0)}, true},
		{map[string]any{"available": false}, map[string]any{"available": false, "available_int": float64(1)}, false},
		{map[string]any{"content": "new"}, map[string]any{"content": "new", "content_with_weight": "old"}, false},
		{map[string]any{"important_keywords": []string{"k"}, "questions": []string{"q"}}, map[string]any{"important_kwd": []any{"k"}, "question_kwd": []any{"q"}}, true},
		{map[string]any{"positions": [][]int{{1, 2, 3, 4, 5}}}, map[string]any{"position_int": []any{[]any{float64(1), float64(2), float64(3), float64(4), float64(5)}}}, true},
		{map[string]any{"tag_kwd": []string{"new"}}, map[string]any{"content": "new"}, false},
		{map[string]any{"image_base64": "write-only", "image_update_mode": "replace"}, map[string]any{"content": "unchanged"}, true},
	} {
		require.Equal(t, tc.matches, chunkReadbackMatches(tc.actual, tc.expected), "%v %v", tc.actual, tc.expected)
	}
}

func TestUpdateChunkRejectsConflictingReadbackIdentityWithoutRetry(t *testing.T) {
	for _, raw := range []string{
		`{"id":"c","chunk_id":"other","doc_id":"d","content":"updated"}`,
		`{"id":"c","doc_id":"d","document_id":"other","content":"updated"}`,
		`{"id":"c","doc_id":"d","kb_id":"kb","dataset_id":"other","content":"updated"}`,
	} {
		var patches, reads atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PATCH" {
				patches.Add(1)
				_, _ = io.WriteString(w, `{"code":0,"data":null}`)
				return
			}
			reads.Add(1)
			_, _ = io.WriteString(w, `{"code":0,"data":`+raw+`}`)
		}))
		client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
		chunk, err := client.UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
		require.Error(t, err)
		require.Nil(t, chunk)
		require.Equal(t, int32(1), patches.Load())
		require.Equal(t, int32(1), reads.Load())
		server.Close()
	}
}
