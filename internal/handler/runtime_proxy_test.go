package handler

import (
	"bufio"
	"context"
	"encoding/json"
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
	srv           *httptest.Server
	lastMethod    string
	lastPath      string
	lastQuery     string
	lastHeader    http.Header
	lastFileRange string      // /v1/files/content 收到的 Range 头
	body          chan string // SSE chunk 管道
	gotCancel     chan struct{}
	agentListBody string // /v1/agents* 代理出口的响应体（R3 egress redaction 测试可注入）
}

func newFakeRuntime(t *testing.T) *fakeRuntime {
	t.Helper()
	f := &fakeRuntime{body: make(chan string, 8), gotCancel: make(chan struct{}, 1)}
	f.agentListBody = "{}" // 默认合法 JSON：/v1/agents* 是 agent-config 出口，R3 redact fail-closed
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/files/content", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\"a b.txt\"")
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
		w.Header().Set("Server", "nginx") // 应被删除
		// Range 往返（专家二轮 3a）：记录请求 Range，按请求区间回 206 并回显
		// Content-Range（total = len("file-bytes") = 10）+ Accept-Ranges。
		if rng := r.Header.Get("Range"); rng != "" {
			f.lastFileRange = rng
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", "bytes "+strings.TrimPrefix(rng, "bytes=")+"/10")
			w.WriteHeader(http.StatusPartialContent)
			if r.Method != http.MethodHead {
				_, _ = io.WriteString(w, "file")
			}
			return
		}
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
		for {
			select {
			case chunk := <-f.body:
				_, _ = io.WriteString(w, chunk)
				flusher.Flush()
			case <-r.Context().Done(): // 客户端断开 → upstream 请求被取消
				select {
				case f.gotCancel <- struct{}{}:
				default:
				}
				return
			}
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	// /v1/agents 与 /v1/agents/<id>（agent-config 出口）：返回可注入的
	// JSON 体。R3 egress redaction 对非 JSON 响应 fail-closed(502)，转发
	// 语义类的旧测试默认收到合法 JSON 走完整链路；redact 专项测试注入
	// 含敏感 headers 的 JSON 断言脱敏输出。
	mux.HandleFunc("/v1/agents/", func(w http.ResponseWriter, r *http.Request) {
		f.lastMethod, f.lastPath, f.lastQuery, f.lastHeader = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, f.agentListBody)
	})
	mux.HandleFunc("/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		f.lastMethod, f.lastPath, f.lastQuery, f.lastHeader = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, f.agentListBody)
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
	RegisterRuntimeProxyRoutes(r, services.NewRuntimeProxyService(&inlineRepo{port: port}, "127.0.0.1"), services.ModeCasdoor)
	return r
}

// newBuiltinProxyEngine 挂 builtin 模式的单段路由（issue #114）：公开 URL
// 形态 /runtime/<agent>，隐式 default 租户不出现在路径里。
func newBuiltinProxyEngine(port int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRuntimeProxyRoutes(r, services.NewRuntimeProxyService(&inlineRepo{port: port}, "127.0.0.1"), services.ModeBuiltin)
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

// Range 往返（专家二轮 3a）：请求 Range 原样到达 upstream，206 / Content-Range /
// Accept-Ranges 原样回到客户端——断点续传经 proxy 不降级。
func TestProxyRangeRoundTrip(t *testing.T) {
	f := newFakeRuntime(t)
	r := newProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runtime/default/test/v1/files/content", nil)
	req.Header.Set("Range", "bytes=0-3")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", w.Code)
	}
	if f.lastFileRange != "bytes=0-3" {
		t.Fatalf("upstream Range = %q, want bytes=0-3", f.lastFileRange)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 0-3/10" {
		t.Fatalf("Content-Range = %q, want bytes 0-3/10", got)
	}
	if got := w.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
}

// SSE 实时逐 chunk flush（专家二轮 3b）：gin engine 挂真实 httptest.Server，
// 客户端带可取消 ctx 流式读 body。chunk 2 在 chunk 1 被客户端完整读到之后才
// 进管道，且 upstream 在客户端断开前永不结束——若 proxy 缓冲整个响应而非每个
// write 后 flush（FlushInterval=-1），限时读必在 deadline 超时；因此任何一次
// 成功的限时读出都只可能来自 mid-stream flush。
func TestProxySSEFlushesPerChunk(t *testing.T) {
	f := newFakeRuntime(t)
	proxySrv := httptest.NewServer(newProxyEngine(portOf(f.srv.URL)))
	t.Cleanup(proxySrv.Close)

	const chunk1 = "event: system\ndata: {\"a\":1}\n\n"
	const chunk2 = "data: {\"b\":2}\n\n"

	// chunk 1 先入管道：upstream WriteHeader 后阻塞在 channel 读，客户端要等
	// 首个 write+flush 才能拿到响应头，Do 须限时防护（防回归时整个测试挂死）。
	f.body <- chunk1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		proxySrv.URL+"/runtime/default/test/v1/agents/my-agent/runs", nil)
	if err != nil {
		t.Fatal(err)
	}
	type doResult struct {
		resp *http.Response
		err  error
	}
	doDone := make(chan doResult, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		doDone <- doResult{resp, err}
	}()
	var resp *http.Response
	select {
	case dr := <-doDone:
		if dr.err != nil {
			t.Fatal(dr.err)
		}
		resp = dr.resp
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for response headers — headers only reach the client with the first flushed chunk")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}

	// 限时读单个 SSE 事件（空行结束）。deadline 到点即失败：此刻后续 chunk 尚
	// 未生产、upstream 也永不返回，读到只能是实时 flush 的结果。
	br := bufio.NewReader(resp.Body)
	readEvent := func(want string) {
		t.Helper()
		type evResult struct {
			event string
			err   error
		}
		done := make(chan evResult, 1)
		go func() {
			var sb strings.Builder
			for {
				line, err := br.ReadString('\n')
				if err != nil {
					done <- evResult{err: err}
					return
				}
				sb.WriteString(line)
				if line == "\n" { // SSE 事件以空行结束
					done <- evResult{event: sb.String()}
					return
				}
			}
		}()
		select {
		case r := <-done:
			if r.err != nil {
				t.Fatalf("read SSE event: %v", r.err)
			}
			if r.event != want {
				t.Fatalf("event = %q, want %q", r.event, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for SSE event %q — response not flushed mid-stream", want)
		}
	}

	readEvent(chunk1) // 此刻 chunk 2 尚不存在：读到即 mid-stream flush 的直接证据
	f.body <- chunk2
	readEvent(chunk2)
	cancel() // 结束流；断开→upstream 取消传播由 TestProxyClientDisconnectCancelsUpstream 覆盖
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

// 502 路径（终审 T3-3 部分）：upstream 端口无人监听时，ErrorHandler 必须回
// respondError 同形状的 JSON 信封 {"success":false,"error":"..."}，中性文案
// 不泄 upstream 地址。端口获取：先起 httptest.Server 再 Close，复用其端口。
func TestProxyUpstreamUnavailableReturnsEnvelope(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	port := portOf(dead.URL)
	dead.Close() // 端口回归空闲：dial 被拒 → ReverseProxy ErrorHandler

	r := newProxyEngine(port)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/default/test/health", nil))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the respondError envelope: %q (err %v)", w.Body.String(), err)
	}
	// 恰好两个字段：success=false + 中性 error 文案，无多余字段。
	if len(body) != 2 || body["success"] != false || body["error"] != "runtime upstream unavailable" {
		t.Fatalf("body = %v, want exactly {success:false, error:%q}", body, "runtime upstream unavailable")
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

// builtin 单段路由（issue #114）：/runtime/<agent>/<path>，租户在 handler
// 内部解析为隐式 default，公开 URL 不暴露租户段。
func TestProxyBuiltinSingleSegmentResolvesImplicitTenant(t *testing.T) {
	f := newFakeRuntime(t)
	r := newBuiltinProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/test/v1/agents?include=all", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	// 前缀剥离语义不变：upstream 收到 /v1/agents。
	if f.lastPath != "/v1/agents" {
		t.Fatalf("upstream path = %q, want /v1/agents", f.lastPath)
	}
	if f.lastQuery != "include=all" {
		t.Fatalf("query lost: %q", f.lastQuery)
	}
}

func TestProxyBuiltinUnknownAgent404(t *testing.T) {
	f := newFakeRuntime(t)
	r := newBuiltinProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/nope/health", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestProxyBuiltinEscapedTraversalRejected(t *testing.T) {
	// containment 超集扫描在单段变体下仍生效：%2f/%2e 编码逃逸 404。
	f := newFakeRuntime(t)
	r := newBuiltinProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/test/v1/%2e%2e/etc", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestProxyRedactsAgentConfigEgress（PR #118 复审 P1-2）锁定代理出口的
// agent-config 脱敏：runtime 返回的列表/detail 若含 hub 部署时武装的
// MCP 凭据（X-Agent-Capability / Authorization），必须在越过 hub 边界前
// 被剥离——即使上游 runtime 自身的脱敏版本滞后或漏配。
func TestProxyRedactsAgentConfigEgress(t *testing.T) {
	f := newFakeRuntime(t)
	f.agentListBody = `{"agents":[{"id":"a","mcpServers":{"knowledge":{"url":"http://hub/mcp","headers":{"Authorization":"Bearer sekrit-token","X-Agent-Capability":"v1.cGF5bG9hZA.s2ln","X-Custom":"plain"}}}}]}`
	r := newBuiltinProxyEngine(portOf(f.srv.URL))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/test/v1/agents", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, leak := range []string{"sekrit-token", "v1.cGF5bG9hZA", "s2ln", "Bearer"} {
		if strings.Contains(body, leak) {
			t.Errorf("credential leaked through proxy egress: %q in %s", leak, body)
		}
	}
	if !strings.Contains(body, "X-Custom") || !strings.Contains(body, `"***"`) {
		t.Errorf("non-sensitive header key must survive with masked value, got: %s", body)
	}
	if !strings.Contains(body, "http://hub/mcp") {
		t.Errorf("non-credential detail content must pass through, got: %s", body)
	}
}
