package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamResumeUsesAuthenticatedExistingSession(t *testing.T) {
	var got ResumeRunRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/agents/agent-b/runs", r.URL.Path)
		require.Equal(t, "runtime-secret", r.Header.Get("x-api-key"))
		require.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: result\ndata: {\"type\":\"result\",\"subtype\":\"success\"}\n\n"))
	}))
	defer srv.Close()

	rc, err := NewClient().StreamResume(context.Background(), srv.URL, "agent-b", "runtime-secret", "session-1", "controlled callback")
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, ResumeRunRequest{Message: "controlled callback", SessionID: "session-1"}, got)
}

func TestStreamResumeRejectsMissingSessionWithoutCallingRuntime(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	_, err := NewClient().StreamResume(context.Background(), srv.URL, "agent-b", "secret", "", "result")
	require.ErrorContains(t, err, "requires session id")
	require.False(t, called)
}
