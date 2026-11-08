package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domain "control-panel/internal/domain/knowledge"
	"github.com/stretchr/testify/require"
)

func TestPartialUploadProjectsConfirmedScopedDocuments(t *testing.T) {
	var posts, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		if r.Method == http.MethodPost {
			posts.Add(1)
			_, _ = io.WriteString(w, `{"code":500,"message":"private SQL and credentials","data":{"uploaded":[{"id":"d1","name":"one.txt","dataset_id":"kb","private":"secret"}]}}`)
			return
		}
		reads.Add(1)
		require.Equal(t, []string{"d1"}, r.URL.Query()["ids"])
		require.Equal(t, "100", r.URL.Query().Get("page_size"))
		_, _ = io.WriteString(w, `{"code":0,"data":{"total":1,"docs":[{"id":"d1","dataset_id":"kb"}]}}`)
	}))
	defer server.Close()
	client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	docs, err := client.UploadDocuments(context.Background(), "kb", domain.UploadRequest{Body: strings.NewReader("stream"), ContentType: "multipart/form-data; boundary=x"})
	require.Nil(t, docs)
	var partial *domain.UploadPartialError
	require.ErrorAs(t, err, &partial)
	require.Equal(t, []domain.Document{{"id": "d1", "name": "one.txt", "dataset_id": "kb", "run": "0"}}, partial.Uploaded)
	require.Equal(t, http.StatusBadGateway, domain.StatusCode(err))
	require.NotContains(t, err.Error(), "private")
	require.EqualValues(t, 1, posts.Load())
	require.EqualValues(t, 1, reads.Load())
}

func TestPartialUploadRejectsInvalidOrUnconfirmedReceipts(t *testing.T) {
	valid := `{"uploaded":[{"id":"d1","name":"one.txt","dataset_id":"kb"}]}`
	cases := []struct {
		name, data string
		http, code int
		readback   string
	}{
		{"cross-dataset", `{"uploaded":[{"id":"d1","name":"one.txt","dataset_id":"foreign"}]}`, 200, 500, ""},
		{"duplicate", `{"uploaded":[{"id":"d1","name":"one.txt","dataset_id":"kb"},{"id":"d1","name":"two.txt","dataset_id":"kb"}]}`, 200, 500, ""},
		{"blank-id", `{"uploaded":[{"id":" ","name":"one.txt","dataset_id":"kb"}]}`, 200, 500, ""},
		{"null-id", `{"uploaded":[{"id":null,"name":"one.txt","dataset_id":"kb"}]}`, 200, 500, ""},
		{"empty", `{"uploaded":[]}`, 200, 500, ""},
		{"wrong-business", valid, 200, 102, ""},
		{"server-failure", valid, 500, 500, ""},
		{"missing-document", valid, 200, 500, `{"code":0,"data":{"total":0,"docs":[]}}`},
		{"foreign-readback", valid, 200, 500, `{"code":0,"data":{"total":1,"docs":[{"id":"d1","dataset_id":"foreign"}]}}`},
		{"wrong-readback-id", valid, 200, 500, `{"code":0,"data":{"total":1,"docs":[{"id":"other","dataset_id":"kb"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
					w.WriteHeader(tc.http)
					_ = json.NewEncoder(w).Encode(map[string]any{"code": tc.code, "message": "private", "data": json.RawMessage(tc.data)})
					return
				}
				require.NotEmpty(t, tc.readback, "an invalid receipt must not read documents")
				_, _ = io.WriteString(w, tc.readback)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			docs, err := client.UploadDocuments(context.Background(), "kb", domain.UploadRequest{Body: strings.NewReader("stream"), ContentType: "multipart/form-data; boundary=x"})
			require.Error(t, err)
			require.Nil(t, docs)
			var partial *domain.UploadPartialError
			require.False(t, errors.As(err, &partial))
			require.NotContains(t, err.Error(), "private")
			require.EqualValues(t, 1, posts.Load())
		})
	}
}

func TestChunkImageRecoveryRemainsFailureWithoutReplayingMutation(t *testing.T) {
	const chunkID = "0123456789abcdef"
	for _, action := range []string{"create", "update"} {
		t.Run(action, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.NotEqual(t, http.MethodGet, r.Method)
				_, _ = io.WriteString(w, `{"code":100,"message":"private SQL","data":{"outcome":"partial","stage":"image_write","index":"acknowledged","image":"unchanged","image_id":"kb-0123456789abcdef","retry_safe":false,"private":"secret"}}`)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			var chunk *domain.Chunk
			var err error
			if action == "create" {
				chunk, err = client.CreateChunk(context.Background(), "kb", "d1", domain.ChunkMutationRequest{"content": "draft", "image_base64": "image"})
			} else {
				chunk, err = client.UpdateChunk(context.Background(), "kb", "d1", chunkID, domain.ChunkMutationRequest{"image_base64": "image", "image_update_mode": "replace"})
			}
			var recovery *domain.ChunkImageWriteError
			require.ErrorAs(t, err, &recovery)
			require.Nil(t, chunk)
			require.Equal(t, "kb-"+chunkID, recovery.Recovery.ImageID)
			require.False(t, recovery.Recovery.RetrySafe)
			require.NotContains(t, err.Error(), "private")
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestChunkImageRecoveryRejectsUnboundAndAmbiguousData(t *testing.T) {
	base := map[string]any{"outcome": "partial", "stage": "image_write", "index": "acknowledged", "image": "unchanged", "image_id": "kb-0123456789abcdef", "retry_safe": false}
	for field, value := range map[string]any{"image_id": "foreign-0123456789abcdef", "stage": "private-details", "outcome": "success", "index": "success", "image": "success", "retry_safe": true} {
		t.Run(field, func(t *testing.T) {
			data := domain.CloneObject(base)
			data[field] = value
			payload, err := json.Marshal(data)
			require.NoError(t, err)
			failure := &knowledgeUpstreamFailure{httpStatus: 200, businessCode: 100, data: payload, cause: domain.NewUpstreamError("safe", nil)}
			require.Nil(t, chunkImageFailure(failure, "kb", "0123456789abcdef"))
		})
	}
	payload, err := json.Marshal(base)
	require.NoError(t, err)
	failure := &knowledgeUpstreamFailure{httpStatus: 200, businessCode: 100, data: payload, cause: domain.NewUpstreamError("safe", nil)}
	require.Nil(t, chunkImageFailure(failure, "kb", "other"))
	require.Nil(t, chunkImageFailure(context.DeadlineExceeded, "kb", ""))
	delete(base, "retry_safe")
	payload, err = json.Marshal(base)
	require.NoError(t, err)
	failure.data = payload
	require.Nil(t, chunkImageFailure(failure, "kb", ""))
}
