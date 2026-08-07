package runtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamRun_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/test/runs" {
			t.Errorf("path = %q, want /v1/agents/test/runs", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "rt-key" {
			t.Errorf("x-api-key = %q, want rt-key", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "event: done\ndata: {}\n\n")
	}))
	defer server.Close()

	c := NewClient()
	body := []byte(`{"message":"hi"}`)
	rc, err := c.StreamRun(context.Background(), server.URL, "test", "rt-key", body)
	if err != nil {
		t.Fatalf("StreamRun failed: %v", err)
	}
	defer rc.Close()

	buf, _ := io.ReadAll(rc)
	if !strings.Contains(string(buf), "event: done") {
		t.Errorf("response missing event: done, got: %q", string(buf))
	}
}

func TestStreamRun_Non2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"error":"boom"}`)
	}))
	defer server.Close()

	c := NewClient()
	_, err := c.StreamRun(context.Background(), server.URL, "test", "rt-key", []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 error, got: %v", err)
	}
}

func TestStreamRun_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Errorf("x-api-key should be empty, got %q", r.Header.Get("x-api-key"))
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	c := NewClient()
	rc, err := c.StreamRun(context.Background(), server.URL, "test", "", []byte(`{}`))
	if err != nil {
		t.Fatalf("StreamRun failed: %v", err)
	}
	defer rc.Close()
}

func TestGetAgentDetail_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/test" {
			t.Errorf("path = %q, want /v1/agents/test", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "rt-key" {
			t.Errorf("x-api-key = %q, want rt-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"id":"test","name":"test","model":"claude-sonnet-4-6","status":"ready","maxTurns":10,"hasSystemPrompt":false}`)
	}))
	defer server.Close()

	c := NewClient()
	body, err := c.GetAgentDetail(context.Background(), server.URL, "test", "rt-key")
	if err != nil {
		t.Fatalf("GetAgentDetail failed: %v", err)
	}
	if !strings.Contains(string(body), `"id":"test"`) {
		t.Errorf("response body unexpected: %q", string(body))
	}
}

func TestGetAgentDetail_Non2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"error":"Agent not found"}`)
	}))
	defer server.Close()

	c := NewClient()
	_, err := c.GetAgentDetail(context.Background(), server.URL, "test", "rt-key")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 error, got: %v", err)
	}
}

func TestGetAgentDetail_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Errorf("x-api-key should be empty, got %q", r.Header.Get("x-api-key"))
		}
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	c := NewClient()
	_, err := c.GetAgentDetail(context.Background(), server.URL, "test", "")
	if err != nil {
		t.Fatalf("GetAgentDetail failed: %v", err)
	}
}

func TestProxyFiles_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" {
			t.Errorf("path = %q, want /v1/files", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.RawQuery != "path=src&recursive=true" {
			t.Errorf("raw query = %q, want path=src&recursive=true", r.URL.RawQuery)
		}
		if got := r.Header.Get("x-api-key"); got != "rt-key" {
			t.Errorf("x-api-key = %q, want rt-key", got)
		}
		if got := r.Header.Get("Range"); got != "" {
			t.Errorf("Range should be empty for list, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"path":"src","entries":[]}`)
	}))
	defer server.Close()

	c := NewClient()
	resp, err := c.ProxyFiles(context.Background(), http.MethodGet, server.URL, "rt-key", "/v1/files?path=src&recursive=true", "")
	if err != nil {
		t.Fatalf("ProxyFiles failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestProxyFiles_Content_HEAD(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/content" {
			t.Errorf("path = %q, want /v1/files/content", r.URL.Path)
		}
		if r.Method != http.MethodHead {
			t.Errorf("method = %q, want HEAD", r.Method)
		}
		w.Header().Set("Content-Length", "42")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(200)
	}))
	defer server.Close()

	c := NewClient()
	resp, err := c.ProxyFiles(context.Background(), http.MethodHead, server.URL, "rt-key", "/v1/files/content?path=p.json", "")
	if err != nil {
		t.Fatalf("ProxyFiles HEAD failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges missing")
	}
}

func TestProxyFiles_Range(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-31" {
			t.Errorf("Range = %q, want bytes=0-31", got)
		}
		w.Header().Set("Content-Range", "bytes 0-31/844")
		w.WriteHeader(206)
	}))
	defer server.Close()

	c := NewClient()
	resp, err := c.ProxyFiles(context.Background(), http.MethodGet, server.URL, "rt-key", "/v1/files/content?path=p.json", "bytes=0-31")
	if err != nil {
		t.Fatalf("ProxyFiles Range failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 206 {
		t.Errorf("status = %d, want 206", resp.StatusCode)
	}
}

func TestProxyFiles_BusinessError_PassesResponseNotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"error":"File not found"}`)
	}))
	defer server.Close()

	c := NewClient()
	resp, err := c.ProxyFiles(context.Background(), http.MethodGet, server.URL, "rt-key", "/v1/files/content?path=missing", "")
	if err != nil {
		t.Fatalf("404 should return response not error, got err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestProxyFiles_NetworkError_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // 立即关闭模拟连接失败

	c := NewClient()
	_, err := c.ProxyFiles(context.Background(), http.MethodGet, server.URL, "rt-key", "/v1/files", "")
	if err == nil {
		t.Errorf("expected network error, got nil")
	}
}

func TestProxyFiles_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "" {
			t.Errorf("x-api-key should be empty, got %q", r.Header.Get("x-api-key"))
		}
		w.WriteHeader(200)
	}))
	defer server.Close()

	c := NewClient()
	resp, err := c.ProxyFiles(context.Background(), http.MethodGet, server.URL, "", "/v1/files", "")
	if err != nil {
		t.Fatalf("ProxyFiles failed: %v", err)
	}
	defer resp.Body.Close()
}
