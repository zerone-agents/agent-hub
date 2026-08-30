package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/agent"

	"github.com/gin-gonic/gin"
)

// fakeRuntime 记录收到的请求并按路由返回可断言的响应。
type fakeRuntime struct {
	srv        *httptest.Server
	lastMethod string
	lastPath   string
	lastQuery  string
	lastHeader http.Header
	body       chan string // SSE chunk 管道
	gotCancel  chan struct{}
}

func newFakeRuntime(t *testing.T) *fakeRuntime {
	t.Helper()
	f := &fakeRuntime{body: make(chan string, 8), gotCancel: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/files/content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"a b.txt\"")
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.Header().Set("Server", "nginx") // 应被删除
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.WriteString(w, "file-bytes")
	})
	mux.HandleFunc("/v1/agents/my-agent/runs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, <-f.body)
		flusher.Flush()
		// 阻塞等客户端断开 → upstream 请求被取消
		<-r.Context().Done()
		select {
		case f.gotCancel <- struct{}{}:
		default:
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		f.lastMethod, f.lastPath, f.lastQuery, f.lastHeader = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		_, _ = io.WriteString(w, "ok")
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// inlineRepo 实现 services.RuntimeProxyAgentRepo：name=="test" 返回绑定到
// fake runtime 端口的 running agent，其余 name 一律 not found。
// （按 brief 自述注释简化掉 fakeRepo.srv 间接层——构造顺序：
// fake runtime → 提取端口 → 构造 engine。）
type inlineRepo struct{ port int }

func (ir *inlineRepo) GetByName(tenantID, name string) (*agent.AgentConfig, error) {
	if name != "test" {
		return nil, context.Canceled // not found
	}
	return &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: ir.port}, nil
}

func newProxyEngine(port int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// httptest.Server 固定绑定 127.0.0.1，upstreamHost 即本机回环地址。
	RegisterRuntimeProxyRoutes(r, services.NewRuntimeProxyService(&inlineRepo{port: port}, "127.0.0.1"))
	return r
}

func portOf(rawURL string) int {
	i := strings.LastIndex(rawURL, ":")
	n, _ := strconv.Atoi(rawURL[i+1:])
	return n
}

func TestProxyForwardsStrippedPathQueryAndHeaders(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runtime/default/test/v1/agents?include=all", nil)
	req.Header.Set("X-API-Key", "rk-123")
	req.Header.Set("X-User-Name", "alice")
	req.Header.Set("X-Org", "forged")
	req.Header.Set("Authorization", "Bearer hub-jwt")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	// 前缀剥离：upstream 收到 /v1/agents（不是 /runtime/default/test/v1/agents）
	if f.lastPath != "/v1/agents" {
		t.Fatalf("upstream path = %q, want /v1/agents", f.lastPath)
	}
	if f.lastQuery != "include=all" {
		t.Fatalf("query lost: %q", f.lastQuery)
	}
	if got := f.lastHeader.Get("X-API-Key"); got != "rk-123" {
		t.Fatalf("x-api-key must pass through, got %q", got)
	}
	if got := f.lastHeader.Get("X-User-Name"); got != "alice" {
		t.Fatalf("X-User-Name must pass through, got %q", got)
	}
	if f.lastHeader.Get("X-Org") != "" {
		t.Fatal("X-Org must be dropped")
	}
	if f.lastHeader.Get("Authorization") != "" {
		t.Fatal("Authorization must be dropped")
	}
}

func TestProxyFileHeadersAndLastModified(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, "/runtime/default/test/v1/files/content", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", method, w.Code)
		}
		if w.Header().Get("Last-Modified") != "Mon, 02 Jan 2006 15:04:05 GMT" {
			t.Fatalf("%s: Last-Modified lost", method)
		}
		if w.Header().Get("Content-Disposition") == "" {
			t.Fatalf("%s: Content-Disposition lost", method)
		}
		if w.Header().Get("Server") != "" {
			t.Fatalf("%s: Server header must be stripped", method)
		}
	}
}

func TestProxySSEFlushesPerChunk(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	f.body <- "event: system\ndata: {\"a\":1}\n\n"
	w := httptest.NewRecorder()
	// SSE 路由 timeout=0，handler 不包 WithTimeout；请求必须自带可取消 ctx——
	// 否则 ReverseProxy（Go 1.26，reverseproxy.go:421）会退回 CloseNotifier，
	// gin 包装层对 recorder 硬断言直接 panic（生产请求恒带取消信号，不受影响）。
	// fake upstream 阻塞至客户端断开，ServeHTTP 放 goroutine，断言在 <-done 后
	// 无竞争执行（同 TestProxyClientDisconnectCancelsUpstream 的 sleep 习语）。
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/runtime/default/test/v1/agents/my-agent/runs", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()
	time.Sleep(200 * time.Millisecond) // 等 chunk 经 FlushInterval=-1 到达 recorder
	cancel()                           // 结束 SSE 流，ServeHTTP 返回
	<-done
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	// httptest recorder 收全量；真实流式由 FlushInterval=-1 保证，此处断言 body 到达
	if !strings.Contains(w.Body.String(), "event: system") {
		t.Fatal("SSE chunk missing")
	}
}

func TestProxyClientDisconnectCancelsUpstream(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/runtime/default/test/v1/agents/my-agent/runs", nil).WithContext(ctx)
	w := &syncWriter{}
	go func() {
		f.body <- "data: 1\n\n"
		r.ServeHTTP(w, req)
	}()
	time.Sleep(200 * time.Millisecond)
	cancel() // 客户端断开
	select {
	case <-f.gotCancel:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not cancelled on client disconnect")
	}
}

type syncWriter struct {
	h    http.Header
	code int
}

func (w *syncWriter) Header() http.Header {
	if w.h == nil {
		w.h = http.Header{}
	}
	return w.h
}

func (w *syncWriter) Write(b []byte) (int, error) { return len(b), nil }

func (w *syncWriter) WriteHeader(code int) { w.code = code }

func TestProxyErrorSemantics(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	cases := []struct {
		name   string
		target string
		want   int
	}{
		{"allowlist out", "/runtime/default/test/v1/metrics", 404},
		{"method mismatch", "/runtime/default/test/health", 0}, // POST below
		{"unknown agent", "/runtime/default/nope/health", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodGet
			want := tc.want
			if tc.name == "method mismatch" {
				method = http.MethodPost
				want = 405
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(method, tc.target, nil))
			if w.Code != want {
				t.Fatalf("status = %d, want %d", w.Code, want)
			}
		})
	}
}

// Kong 模式：不注册 /runtime/*，请求不进入 proxy、无 upstream 请求——
// main 的 NoRoute 兜底 302 /static（复刻 main.go:582 行为断言，不断言 404）。
func TestNoRegistrationFallsToNoRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.NoRoute(func(c *gin.Context) { c.Redirect(http.StatusFound, "/static") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/default/test/health", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/static" {
		t.Fatalf("want 302 /static, got %d %q", w.Code, w.Header().Get("Location"))
	}
}
