package knowledge

import (
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKnowledgeUpstreamBusinessFailuresHideDatabaseDetails(t *testing.T) {
	const diagnostic = `ProgrammingError psycopg.errors.InsufficientPrivilege: permission denied for table t_ai_document_metadata SQLSTATE42501 private-SQL-value`
	for _, code := range []int{100, 102} {
		for _, legacy := range []bool{false, true} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{"code": code, "message": diagnostic}
				if legacy {
					body = map[string]any{"retcode": code, "retmsg": diagnostic}
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			_, err := client.ListDocuments(context.Background(), "kb", domain.DocumentListRequest{Page: 1, PageSize: 1})
			require.Error(t, err)
			require.Equal(t, 502, domain.StatusCode(err))
			require.Contains(t, err.Error(), fmt.Sprintf("业务码 %d", code))
			for _, secret := range []string{"ProgrammingError", "psycopg", "t_ai_document_metadata", "42501", "private-SQL-value"} {
				require.NotContains(t, err.Error(), secret)
			}
			var internal *domain.Error
			require.True(t, errors.As(err, &internal))
			require.NotNil(t, internal.Cause)
			require.Contains(t, internal.Cause.Error(), diagnostic, "retain original diagnostic only in the internal cause")
			server.Close()
		}
	}
}

func TestKnowledgeUpstreamHTTPAndTransportFailuresAreSafe(t *testing.T) {
	for _, mode := range []string{"HTTP", "transport"} {
		client := NewRemoteMultiragEngine("http://example.invalid", "key", time.Second, time.Hour)
		client.client.Transport = imageRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if mode == "transport" {
				return nil, errors.New("private-secret-URL-and-SQL")
			}
			return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("private-secret-URL-and-SQL"))}, nil
		})
		_, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{})
		require.Error(t, err)
		require.Equal(t, 502, domain.StatusCode(err))
		require.NotContains(t, err.Error(), "private-secret")
	}
}

func TestUpdateChunkPartialFailureRequiresReadbackBeforeFurtherAction(t *testing.T) {
	patches, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			patches++
			_, _ = io.WriteString(w, `{"code":100,"message":"private-SQL-provider-detail","data":{"stage":"image_write","index":"acknowledged","image":"unchanged","outcome":"partial","retry_safe":false}}`)
			return
		}
		reads++
	}))
	defer server.Close()
	chunk, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).UpdateChunk(context.Background(), "kb", "d", "c", domain.ChunkMutationRequest{"content": "updated"})
	require.Error(t, err)
	require.Nil(t, chunk)
	require.Contains(t, err.Error(), "结果未确认")
	require.Contains(t, err.Error(), "业务码 100")
	require.Contains(t, err.Error(), "刷新核对后再操作")
	require.Contains(t, err.Error(), "草稿已保留")
	require.NotContains(t, err.Error(), "稍后重试")
	require.NotContains(t, err.Error(), "private-SQL-provider-detail")
	require.Equal(t, 1, patches)
	require.Zero(t, reads, "a failed PATCH must not be replayed or presented as confirmed by a subsequent read")
}
