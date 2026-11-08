package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProxyTurnContextCapabilityGateAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, health                       string
		healthStatus, want, wantRuns, wantHealth int
	}{
		{"old runtime", `{"message":"hi","turnContext":"fact"}`, `{"version":"2.12.0"}`, 200, 503, 0, 1},
		{"present runtime", `{"message":"hi","turnContext":"fact"}`, `{"capabilities":{"turnContext":{"field":"appendSystemPrompt","maxCodePoints":6000,"persistence":"run-only"}}}`, 200, 200, 1, 1},
		{"append field", `{"message":"hi","appendSystemPrompt":"fact"}`, `{"capabilities":{"turnContext":{"field":"appendSystemPrompt","maxCodePoints":6000,"persistence":"run-only"}}}`, 200, 200, 1, 1},
		{"wrong type", `{"message":"hi","turnContext":123}`, `{}`, 200, 400, 0, 0},
		{"null", `{"message":"hi","turnContext":null}`, `{}`, 200, 400, 0, 0},
		{"dual fields", `{"message":"hi","turnContext":"x","appendSystemPrompt":"x"}`, `{}`, 200, 400, 0, 0},
		{"over budget", `{"message":"hi","turnContext":"` + strings.Repeat("🦌", 6001) + `"}`, `{}`, 200, 400, 0, 0},
		{"health auth denial", `{"message":"hi","turnContext":"fact"}`, `{"error":"secret"}`, 401, 503, 0, 1},
		{"ordinary compatibility", `{"message":"hi"}`, `{}`, 200, 200, 1, 0},
		{"escaped key", `{"message":"hi","\u0074urnContext":"fact"}`, `{}`, 200, 503, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs, probes := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					probes++
					if r.Header.Get("x-api-key") != "private-runtime-key" {
						t.Error("probe lost runtime authentication")
					}
					w.WriteHeader(tc.healthStatus)
					_, _ = io.WriteString(w, tc.health)
					return
				}
				runs++
				body, _ := io.ReadAll(r.Body)
				if string(body) != tc.body {
					t.Error("run body changed")
				}
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			defer upstream.Close()
			router := newProxyEngine(portOf(upstream.URL))
			req := httptest.NewRequest("POST", "/runtime/default/test/v1/agents/deer/runs", strings.NewReader(tc.body))
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			req.Header.Set("x-api-key", "private-runtime-key")
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != tc.want || runs != tc.wantRuns || probes != tc.wantHealth {
				t.Fatalf("status/runs/probes=%d/%d/%d", out.Code, runs, probes)
			}
			if strings.Contains(out.Body.String(), "private-runtime-key") || strings.Contains(out.Body.String(), "secret") {
				t.Fatal("error leaked sensitive upstream information")
			}
		})
	}
}

func TestProxyContextScannerKeepsLargeOrdinaryMessagesButRejectsLateContext(t *testing.T) {
	large := strings.Repeat("x", proxyContextBodyLimit+100)
	for _, body := range []string{
		`{"message":"` + large + `"}`,
		`{"message":"` + large + `","turnContext":"fact"}`,
	} {
		req := httptest.NewRequest("POST", "/", strings.NewReader(body))
		_, cleanup, err := inspectProxyContext(req)
		if strings.Contains(body, "turnContext") {
			if err != errProxyContextLarge {
				t.Fatalf("late context must be bounded, got %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(req.Body)
			if !bytes.Equal(got, []byte(body)) {
				t.Fatal("ordinary large body changed")
			}
		}
		cleanup()
	}
}

func TestProxyContextScannerIgnoresNestedAndStringMentions(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"message": `say "turnContext": please`, "other": map[string]any{"turnContext": "not a top-level runtime field"}})
	req := httptest.NewRequest("POST", "/", bytes.NewReader(body))
	uses, cleanup, err := inspectProxyContext(req)
	defer cleanup()
	if uses || err != nil {
		t.Fatal("nested or quoted text was mistaken for a run context field")
	}
}

type proxyCountingBody struct {
	io.Reader
	read   int64
	closed bool
}

func (r *proxyCountingBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += int64(n)
	return n, err
}
func (r *proxyCountingBody) Close() error { r.closed = true; return nil }

func TestProxyContextBodyLimitsBoundKnownAndChunkedLegacyBodies(t *testing.T) {
	for _, knownLength := range []bool{true, false} {
		t.Run(map[bool]string{true: "content_length", false: "chunked"}[knownLength], func(t *testing.T) {
			body := &proxyCountingBody{Reader: io.MultiReader(strings.NewReader(`{"message":"`), io.LimitReader(zeroByteReader{}, proxyLegacyBodyLimit+1024))}
			req := httptest.NewRequest("POST", "/", nil)
			req.Body = body
			if knownLength {
				req.ContentLength = proxyLegacyBodyLimit + 1024
			} else {
				req.ContentLength = -1
				req.TransferEncoding = []string{"chunked"}
			}
			before, _ := os.ReadDir("/dev/fd")
			_, cleanup, err := inspectProxyContext(req)
			cleanup()
			if err != errProxyContextLarge {
				t.Fatalf("expected 413 body limit, got %v", err)
			}
			if !body.closed {
				t.Fatal("original body was not closed")
			}
			if knownLength && body.read != 0 {
				t.Fatal("known oversized body must be rejected before reading")
			}
			if body.read > proxyLegacyBodyLimit+1 {
				t.Fatalf("chunked body over-read: %d", body.read)
			}
			after, _ := os.ReadDir("/dev/fd")
			if len(before) > 0 && len(after) > len(before) {
				t.Fatal("spill descriptor leaked after oversized-body rejection")
			}
		})
	}
	// Exactly 16 MiB without temporary fields remains a compatible run body.
	prefix, suffix := `{"message":"`, `"}`
	body := prefix + strings.Repeat("x", proxyLegacyBodyLimit-len(prefix)-len(suffix)) + suffix
	req := httptest.NewRequest("POST", "/", strings.NewReader(body))
	uses, cleanup, err := inspectProxyContext(req)
	defer cleanup()
	if err != nil || uses {
		t.Fatalf("exact-limit ordinary body rejected: %v", err)
	}
	got, _ := io.ReadAll(req.Body)
	if !bytes.Equal(got, []byte(body)) {
		t.Fatal("exact-limit ordinary body changed")
	}
}

type zeroByteReader struct{}

func (zeroByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestProxyContextBodyReadDeadlineStopsSlowChunkedIngress(t *testing.T) {
	// Real net/http writer: ResponseController must cancel a blocked body read,
	// independently of the SSE/generation request lifetime.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, cleanup, err := inspectProxyContextWithDeadline(r, w, 40*time.Millisecond)
		defer cleanup()
		if err != errProxyBodyTimeout {
			t.Errorf("expected body timeout, got %v", err)
		}
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan *http.Response, 1)
	go func() {
		req, _ := http.NewRequest("POST", server.URL, reader)
		resp, err := (&http.Client{Timeout: time.Second}).Do(req)
		if err != nil {
			done <- nil
			return
		}
		done <- resp
	}()
	_, _ = io.WriteString(writer, `{"message":"`)
	select {
	case resp := <-done:
		if resp == nil {
			t.Fatal("slow ingress did not receive bounded rejection")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusRequestTimeout {
			t.Fatalf("status=%d", resp.StatusCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow body read was not bounded")
	}
}

func TestProxyRunBodyLimitRejectsUnauthenticatedOversizeBeforeUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("oversized request reached upstream")
		w.WriteHeader(500)
	}))
	defer upstream.Close()
	router := newProxyEngine(portOf(upstream.URL))
	for _, known := range []bool{true, false} {
		body := &proxyCountingBody{Reader: io.MultiReader(strings.NewReader(`{"message":"`), io.LimitReader(zeroByteReader{}, proxyLegacyBodyLimit+1024))}
		req := httptest.NewRequest("POST", "/runtime/default/test/v1/agents/deer/runs", nil)
		req.Body = body
		if known {
			req.ContentLength = proxyLegacyBodyLimit + 1024
		} else {
			req.ContentLength = -1
			req.TransferEncoding = []string{"chunked"}
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if out.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status=%d, want413", out.Code)
		}
		if body.read > proxyLegacyBodyLimit+1 {
			t.Fatal("oversized request was drained beyond its bound")
		}
	}
}
