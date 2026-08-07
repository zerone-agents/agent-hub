package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupAgentFilesRouter(svc AgentDetailService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAgentFilesHandler(svc)
	r := gin.New()
	r.GET("/api/v1/admin/agents/:name/files", h.ListFiles)
	r.GET("/api/v1/admin/agents/:name/files/content", h.GetContent)
	r.HEAD("/api/v1/admin/agents/:name/files/content", h.HeadContent)
	return r
}

func TestListFiles_Success_QueryPassthrough(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files" {
			t.Errorf("runtime path = %q, want /v1/files", r.URL.Path)
		}
		if r.URL.RawQuery != "path=src&recursive=true" {
			t.Errorf("runtime raw query = %q, want path=src&recursive=true", r.URL.RawQuery)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("x-api-key"); got != "rt-secret" {
			t.Errorf("x-api-key = %q, want rt-secret", got)
		}
		// 验证客户端 Authorization 头不被转发到 runtime
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization should NOT be forwarded to runtime, got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"path":"src","entries":[{"name":"index.ts","type":"file"}]}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "rt-secret"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files?path=src&recursive=true", nil)
	req.Header.Set("Authorization", "Bearer user-jwt") // 用户 token 不应转发
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"index.ts"`) {
		t.Errorf("body should pass through runtime JSON, got: %q", body)
	}
}

func TestGetContent_Success_HeadersForwarded(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/content" {
			t.Errorf("path = %q, want /v1/files/content", r.URL.Path)
		}
		if r.URL.RawQuery != "path=package.json" {
			t.Errorf("raw query = %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''package.json")
		w.Header().Set("Content-Length", "844")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Last-Modified", "Tue, 07 Jul 2026 09:17:37 GMT")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"name":"pkg"}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files/content?path=package.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// 白名单响应头必须转发
	for _, h := range []string{"Content-Type", "Content-Disposition", "Content-Length", "Accept-Ranges", "Last-Modified"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("response header %q should be forwarded", h)
		}
	}
}

func TestGetContent_Range(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-31" {
			t.Errorf("Range = %q, want bytes=0-31", got)
		}
		w.Header().Set("Content-Range", "bytes 0-31/844")
		w.Header().Set("Content-Length", "32")
		w.WriteHeader(206)
		_, _ = w.Write([]byte(`{"name":"@zerone-agent/open-"`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files/content?path=package.json", nil)
	req.Header.Set("Range", "bytes=0-31")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Errorf("status = %d, want 206", rec.Code)
	}
	if cr := rec.Header().Get("Content-Range"); cr != "bytes 0-31/844" {
		t.Errorf("Content-Range = %q", cr)
	}
}

func TestHeadContent_NoBody(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %q, want HEAD", r.Method)
		}
		w.Header().Set("Content-Length", "844")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodHead, "/api/v1/admin/agents/x/files/content?path=p.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD should not return body, got %d bytes", rec.Body.Len())
	}
	if rec.Header().Get("Content-Length") != "844" {
		t.Errorf("Content-Length should be forwarded, got %q", rec.Header().Get("Content-Length"))
	}
}

func TestAgentFiles_RuntimeNotFound_Forwarded(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"File not found"}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files/content?path=missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestAgentFiles_RuntimeInvalidPath_Forwarded(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"Not a file"}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files/content?path=src", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestAgentFiles_AgentNotResolved(t *testing.T) {
	router := setupAgentFilesRouter(&fakeAgentDetailSvc{err: fmt.Errorf("agent not deployed")})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/ghost/files", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
}

func TestAgentFiles_Runtime5xx_MappedTo502(t *testing.T) {
	runtimeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer runtimeSrv.Close()

	router := setupAgentFilesRouter(&fakeAgentDetailSvc{baseURL: runtimeSrv.URL, apiKey: "k"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/agents/x/files", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 (5xx should be wrapped)", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "HTTP 500") {
		t.Errorf("502 body should mention wrapped HTTP 500 status, got: %q", body)
	}
	if !strings.Contains(body, "internal") {
		t.Errorf("502 body should include runtime error context, got: %q", body)
	}
}
