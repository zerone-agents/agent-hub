package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTurnContextRunMapsOnlyTemporaryField(t *testing.T) {
	for _, text := range []string{strings.Repeat("鹿", 6000), strings.Repeat("🦌", 6000)} {
		body, err := MarshalTurnContextRun(TurnContextRunRequest{Message: "actual user message", TurnContext: text, SessionID: "s1"})
		if err != nil {
			t.Fatal(err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req["message"] != "actual user message" || req["appendSystemPrompt"] != text || req["turnContext"] != nil {
			t.Fatal("business context must map only to per-run appendSystemPrompt")
		}
	}
	for _, text := range []string{strings.Repeat("鹿", 6001), string([]byte{0xff})} {
		if _, err := MarshalTurnContextRun(TurnContextRunRequest{TurnContext: text}); !errors.Is(err, ErrInvalidTurnContext) {
			t.Fatalf("want context rejection, got %v", err)
		}
	}
}

func TestMemoryRuntimeCapabilitiesFailClosedOnLegacyAndMalformed(t *testing.T) {
	for _, body := range []string{`{"version":"2.12.0"}`, `{"version":"99.0.0","capabilities":{"turnContext":{"field":"appendSystemPrompt","maxCodePoints":9999,"persistence":"session"}}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		capabilities, err := NewClient().ProbeMemoryCapabilities(context.Background(), srv.URL, "")
		srv.Close()
		if err != nil {
			t.Fatal(err)
		}
		if capabilities.TurnContext || capabilities.SessionAppend || capabilities.SessionRollback || capabilities.ExplicitZeroTools {
			t.Fatal("undeclared contracts must remain unavailable")
		}
	}
}

func TestTurnContextRunRequiresDeclarationAndPreservesSSE(t *testing.T) {
	var runs int
	declared := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if !declared {
				_, _ = io.WriteString(w, `{"version":"2.12.0"}`)
				return
			}
			_, _ = io.WriteString(w, `{"version":"2.12.0","capabilities":{"turnContext":{"field":"appendSystemPrompt","maxCodePoints":6000,"persistence":"run-only"},"sessionHistoryMutations":{"version":1,"append":true,"rollback":"keep-anchor","revisionCAS":true,"idempotency":true},"explicitZeroTools":true}}`)
			return
		}
		runs++
		if r.Header.Get("x-api-key") != "runtime-key" {
			t.Error("runtime authentication not preserved")
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload["appendSystemPrompt"] != "temporary fact" || payload["message"] != "actual message" {
			t.Error("temporary context mapping is incorrect")
		}
		_, _ = io.WriteString(w, "event: result\ndata: {}\n\n")
	}))
	defer srv.Close()
	client := NewClient()
	req := TurnContextRunRequest{Message: "actual message", TurnContext: "temporary fact"}
	if _, err := client.StreamTurnContextRun(context.Background(), srv.URL, "deer", "runtime-key", req); !errors.Is(err, ErrTurnContextUnsupported) {
		t.Fatalf("legacy runtime must be blocked: %v", err)
	}
	if runs != 0 {
		t.Fatal("capability rejection must not generate")
	}
	declared = true
	capabilities, err := client.ProbeMemoryCapabilities(context.Background(), srv.URL, "runtime-key")
	if err != nil || !capabilities.SessionAppend || !capabilities.SessionRollback || !capabilities.ExplicitZeroTools {
		t.Fatalf("declared capabilities missing: %+v, %v", capabilities, err)
	}
	stream, err := client.StreamTurnContextRun(context.Background(), srv.URL, "deer", "runtime-key", req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	data, _ := io.ReadAll(stream)
	if string(data) != "event: result\ndata: {}\n\n" || runs != 1 {
		t.Fatal("SSE transport changed")
	}
}
