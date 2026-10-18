package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCanonicalFilesPreserveJSONEmptyBytesAndSafeDisposition(t *testing.T) {
	for _, tc := range []struct {
		name, action, content, contentType, disposition string
		status                                          int
	}{
		{"json-preview", "preview", `{"success":false,"code":102}`, "application/json", "", 200},
		{"json-download", "download", `{"code":109,"retcode":102}`, "application/json", "", 200},
		{"empty-preview", "preview", "", "application/json", "", 200},
		{"empty-download", "download", "", "application/octet-stream", "", 200},
		{"dangerous-preview", "preview", "<html>file</html>", "text/html", "attachment", 200},
		{"malformed-disposition", "preview", `{}`, "application/json", "invalid; bad", 200},
		{"JSON-denial", "preview", `{"code":109,"message":"private-secret"}`, "application/json", "", 403},
		{"binary-denial", "download", "private-secret", "application/octet-stream", "attachment", 403},
		{"empty-non200", "preview", "", "application/json", "", 204},
		{"partial", "preview", "private-secret", "application/json", "", 206},
		{"redirect", "preview", "private-secret", "application/json", "", 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileBody := &imageTrackingBody{Reader: strings.NewReader(tc.content)}
			requests := 0
			client := NewRemoteMultiragEngine("http://example.invalid", "key", time.Second, time.Hour)
			client.client.Transport = imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.Path == "/api/v1/datasets/kb/documents" {
					body, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"total": 1, "docs": []any{map[string]any{"id": "d", "dataset_id": "kb", "name": "../nested/\r\n报告.json"}}}})
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
				}
				require.Equal(t, "/api/v1/documents/d/"+tc.action, r.URL.Path)
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{tc.contentType}, "Content-Disposition": []string{tc.disposition}, "Location": []string{"/unexpected-envelope"}}, ContentLength: int64(len(tc.content)), Body: fileBody, Request: r}, nil
			})
			stream, err := client.documentFile(context.Background(), "kb", "d", tc.action)
			if tc.status != 200 {
				require.Error(t, err)
				require.Nil(t, stream)
				require.True(t, fileBody.closed)
				require.NotContains(t, err.Error(), "private-secret")
			} else {
				require.NoError(t, err)
				require.False(t, fileBody.closed, "success transfers the body to the consumer")
				body, err := io.ReadAll(stream.Body)
				require.NoError(t, err)
				require.Equal(t, tc.content, string(body))
				require.Equal(t, tc.contentType, stream.ContentType)
				require.Equal(t, int64(len(tc.content)), stream.ContentLength)
				disposition, params, err := mime.ParseMediaType(stream.ContentDisposition)
				require.NoError(t, err)
				if tc.action == "download" || tc.disposition == "attachment" {
					require.Equal(t, "attachment", disposition)
				} else {
					require.Equal(t, "inline", disposition)
				}
				if tc.disposition != "attachment" {
					require.Equal(t, "报告.json", params["filename"])
				}
				require.NotContains(t, stream.ContentDisposition, "\r")
				require.NotContains(t, stream.ContentDisposition, "\n")
				require.NoError(t, stream.Body.Close())
				require.True(t, fileBody.closed)
			}
			require.Equal(t, 2, requests, "redirects must never enter a differently contracted endpoint")
		})
	}
}

func TestCanonicalFileCancellationReachesUpstream(t *testing.T) {
	for _, afterHeaders := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-headers", true: "after-headers"}[afterHeaders], func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/documents") {
					_, _ = io.WriteString(w, `{"code":0,"data":{"total":1,"docs":[{"id":"d","dataset_id":"kb"}]}}`)
					return
				}
				if afterHeaders {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, "{")
					w.(http.Flusher).Flush()
				}
				close(started)
				<-r.Context().Done()
				close(canceled)
			}))
			defer server.Close()
			client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				stream, err := client.PreviewDocument(ctx, "kb", "d")
				if stream != nil {
					_, _ = io.ReadAll(stream.Body)
					_ = stream.Body.Close()
				} else if err == nil {
					t.Error("canceled request returned no stream and no error")
				}
			}()
			<-started
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("file consumer did not observe cancellation")
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("upstream file request was not canceled")
			}
		})
	}
}
