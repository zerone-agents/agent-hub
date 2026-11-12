package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	runtimeclient "control-panel/internal/infrastructure/runtime"

	"github.com/gin-gonic/gin"
)

// RuntimeProxyHandler forwards allowlisted runtime API calls to the agent's
// runtime container. Pure passthrough credentials (D4): runtime is the only
// auth point, exactly like the Kong path.
type RuntimeProxyHandler struct {
	svc       *services.RuntimeProxyService
	transport *http.Transport
	// memory 非空时对首轮 run body 合并注入记忆概要（spec 2026-11-11 §4）；
	// 变参注入保持既有构造调用兼容。
	memory *services.MemoryInjector
}

func NewRuntimeProxyHandler(svc *services.RuntimeProxyService, injectors ...*services.MemoryInjector) *RuntimeProxyHandler {
	// One shared transport: ResponseHeaderTimeout gives every endpoint fast
	// first-byte failure; overall deadlines are per-request contexts
	// (ReverseProxy.Transport is a RoundTripper — no Client.Timeout exists).
	// MaxIdleConnsPerHost: 32 — 单一共享 transport 服务于 chat/SSH/文件/
	// 清单全部代理链路，Go 默认 2 在并发负载下连接 churn（issue #91 批次三）。
	h := &RuntimeProxyHandler{
		svc:       svc,
		transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second, MaxIdleConnsPerHost: 32},
	}
	if len(injectors) > 0 {
		h.memory = injectors[0]
	}
	return h
}

// RegisterRuntimeProxyRoutes mounts the proxy. Called only when Kong is NOT
// configured (cmd/server/main.go); Kong mode keeps its own route chain.
// ModeBuiltin (issue #114) mounts the single-segment public form
// "/runtime/<agent>" — the implicit default tenant never appears in the URL.
// The two shapes can't be mounted together: gin forbids conflicting wildcard
// names at the same prefix position.
func RegisterRuntimeProxyRoutes(r *gin.Engine, svc *services.RuntimeProxyService, authMode services.AuthMode, injectors ...*services.MemoryInjector) {
	h := NewRuntimeProxyHandler(svc, injectors...)
	if authMode == services.ModeBuiltin {
		rg := r.Group("/runtime/:agent")
		rg.Any("/*path", h.ProxyBuiltin)
		return
	}
	rg := r.Group("/runtime/:org/:agent")
	rg.Any("/*path", h.Proxy)
}

func (h *RuntimeProxyHandler) Proxy(c *gin.Context) {
	org := c.Param("org")
	agentName := c.Param("agent")
	decoded := c.Param("path") // leading "/" included by gin
	h.proxyResolved(c, org, agentName, escapedRemainder(c.Request.URL.EscapedPath()), decoded)
}

// ProxyBuiltin serves the single-segment builtin form "/runtime/<agent>"
// (issue #114): the tenant resolves internally to the implicit default
// tenant and never surfaces in the public URL (it does appear in the audit
// log, which is internal-only).
func (h *RuntimeProxyHandler) ProxyBuiltin(c *gin.Context) {
	agentName := c.Param("agent")
	decoded := c.Param("path")
	h.proxyResolved(c, tenant.DefaultID, agentName, escapedRemainderBuiltin(c.Request.URL.EscapedPath()), decoded)
}

// proxyResolved is the shared proxy tail for both public shapes: Resolve
// preconditions, the stripped reverse proxy, and the audit line. start is
// taken here so both shapes measure the full request.
func (h *RuntimeProxyHandler) proxyResolved(c *gin.Context, org, agentName, escaped, decoded string) {
	start := time.Now()
	decision, perr := h.svc.Resolve(org, agentName, c.Request.Method, escaped, decoded)
	if perr != nil {
		// 405 携带第一条 path 命中路由的 Allow 头（issue #91 批次三，
		// ProxyError.AllowHeader 由 service 层填充）；其他错误分支零变化。
		if perr.Code == http.StatusMethodNotAllowed && perr.AllowHeader != "" {
			c.Header("Allow", perr.AllowHeader)
		}
		respondError(c, perr.Code, ErrCodeRuntimeProxyError, perr.Reason)
		auditRuntimeProxy(c, org, agentName, perr.Code, start, "")
		return
	}
	if c.Request.Method == http.MethodPost && strings.HasPrefix(decision.CanonicalPath, "/v1/agents/") && strings.HasSuffix(decision.CanonicalPath, "/runs") {
		usesContext, cleanup, err := inspectProxyContextWithDeadline(c.Request, c.Writer, proxyBodyReadTimeout)
		defer cleanup()
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errProxyContextLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			if errors.Is(err, errProxyBodyTimeout) {
				status = http.StatusRequestTimeout
			}
			respondError(c, status, "invalid_runtime_request", "运行请求正文无效、超时或超过限制")
			auditRuntimeProxy(c, org, agentName, status, start, "")
			return
		}
		if usesContext {
			probeCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			caps, err := runtimeclient.NewClient().ProbeMemoryCapabilities(probeCtx, decision.UpstreamBase, c.GetHeader("x-api-key"))
			cancel()
			if err != nil || !caps.TurnContext {
				respondError(c, http.StatusServiceUnavailable, "turn_context_unsupported", "当前 Runtime 未声明临时上下文能力")
				auditRuntimeProxy(c, org, agentName, http.StatusServiceUnavailable, start, "")
				return
			}
		}
		// 记忆首轮注入（spec 2026-11-11 §4）：Agent 开启记忆 + 注入器就绪。
		// 首轮信号 = body 无 sessionId（runtime 将新建会话）；与 turnContext
		// 互斥（上方上下文检查先行且两者不允许并存）。读改失败一律降级
		// 透传原文——injectMemorySummary 内部恢复 body。
		if decision.AgentMemoryEnabled && h.memory != nil {
			h.injectMemorySummary(c, org, agentName)
		}
	}
	upstream, err := url.Parse(decision.UpstreamBase)
	if err != nil { // unreachable: base is JoinHostPort-built from validated parts
		respondError(c, http.StatusBadGateway, ErrCodeRuntimeUnreachable, "runtime upstream unavailable")
		auditRuntimeProxy(c, org, agentName, http.StatusBadGateway, start, "")
		return
	}

	ctx := c.Request.Context()
	cancel := func() {}
	if decision.Timeout > 0 {
		var c2 context.CancelFunc
		ctx, c2 = context.WithTimeout(ctx, decision.Timeout)
		cancel = c2
	}
	defer cancel()
	outReq := c.Request.Clone(ctx)

	var upstreamErr string
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream) // scheme/host/port only (upstream path is "/")
			// SetURL joins the inbound FULL path behind the upstream base —
			// must be overwritten with the stripped, validated runtime path
			// (spec 3.4, round-1 P1). RawPath="" = canonical re-encoding;
			// contract is decoded-path equivalence, not byte-identical escapes.
			pr.Out.URL.Path = decision.CanonicalPath
			pr.Out.URL.RawPath = ""
			// RawQuery survives the clone; SetURL does not touch it.
			pr.Out.Host = upstream.Host
			// Header policy (spec 3.4): drop tenant forgery vector and hub
			// credentials; pass x-api-key / X-User-Name through untouched.
			pr.Out.Header.Del("X-Org")
			pr.Out.Header.Del("Authorization")
			// No forwarding-chain headers: drop client's, inject none.
			for _, hdr := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port", "X-Forwarded-Server", "Forwarded", "Via"} {
				pr.Out.Header.Del(hdr)
			}
		},
		FlushInterval: -1, // flush after every write: SSE must not buffer
		Transport:     h.transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) {
			upstreamErr = classifyUpstreamErr(e)
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway) // stable 502; never leak upstream address
			// Same envelope as respondError — one endpoint, one error schema.
			// No gin.Context here, so write the identical fields by hand.
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "runtime upstream unavailable"})
		},
		ModifyResponse: func(resp *http.Response) error {
			for _, hdr := range []string{"Set-Cookie2", "Server", "X-Powered-By"} {
				resp.Header.Del(hdr)
			}
			// Egress redaction (issue #111: detail must not leak the
			// capability): the agent list/detail routes echo the resolved
			// agent config, whose MCP headers the hub armed at deploy time.
			// Strip credentials before the bytes cross the hub boundary —
			// the runtime's own redaction is not trusted to keep up.
			if isAgentConfigEgress(c.Request.Method, decision.CanonicalPath) {
				return redactProxyAgentJSON(resp)
			}
			return nil
		},
	}
	proxy.ServeHTTP(c.Writer, outReq)
	auditRuntimeProxy(c, org, agentName, c.Writer.Status(), start, upstreamErr)
}

// injectMemorySummary merges the first-turn memory summary into a proxied run
// body's appendSystemPrompt（记忆段前置，业务自带值保留——spec §4）。
// Degrade-only：任一跳过条件（ContentLength 未知/超限、JSON 不可解析、
// 含 turnContext（互斥）、含非空 sessionId（非首轮）、message 为空、
// summary 为空、序列化失败）都原样恢复 body 透传，绝不阻断代理。
// 仅在 ContentLength 已知且 ≤ proxyContextBodyLimit 时读改——上方的
// inspectProxyContext 已重写 body 并校正 ContentLength，此时读取是完整
// 的；更大或未知长度的 run 不做注入（聊天首轮正文量级远低于该限）。
func (h *RuntimeProxyHandler) injectMemorySummary(c *gin.Context, org, agentName string) {
	if c.Request.ContentLength <= 0 || c.Request.ContentLength > proxyContextBodyLimit {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, c.Request.ContentLength))
	c.Request.Body.Close()
	restore := func() {
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		c.Request.ContentLength = int64(len(raw))
	}
	if err != nil {
		restore()
		return
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil {
		restore()
		return
	}
	if _, has := body["turnContext"]; has {
		restore()
		return
	}
	if sid, has := body["sessionId"]; has {
		var s string
		if json.Unmarshal(sid, &s) == nil && s != "" {
			restore()
			return
		}
	}
	var msg string
	if m, has := body["message"]; has {
		_ = json.Unmarshal(m, &msg)
	}
	if strings.TrimSpace(msg) == "" {
		restore()
		return
	}
	summary, _ := h.memory.BuildSummary(c.Request.Context(), org, agentName, msg)
	if summary == "" {
		restore()
		return
	}
	merged := summary
	if exist, has := body["appendSystemPrompt"]; has {
		var s string
		if json.Unmarshal(exist, &s) == nil && strings.TrimSpace(s) != "" {
			merged = summary + "\n" + s
		}
	}
	mergedRaw, err := json.Marshal(merged)
	if err != nil {
		restore()
		return
	}
	body["appendSystemPrompt"] = mergedRaw
	out, err := json.Marshal(body)
	if err != nil {
		restore()
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(out))
	c.Request.ContentLength = int64(len(out))
}

// escapedRemainder extracts the escaped wildcard remainder INCLUDING the
// agent segment ("/{agent}/{path}") from the full escaped path
// ("/runtime/{org}/{agent}/{path}") — SplitN(path, "/", 4) stops after the
// third separator, so parts[3] still carries the agent name. It is a
// fail-closed SUPERSET consumed by the %2f/%2e containment scan; exact
// allowlist matching uses only the decoded path.
func escapedRemainder(escapedPath string) string {
	parts := strings.SplitN(escapedPath, "/", 4)
	if len(parts) < 4 || parts[1] != "runtime" {
		return ""
	}
	return "/" + parts[3]
}

// escapedRemainderBuiltin is the single-segment variant for the builtin route
// shape "/runtime/{agent}/{path}": SplitN depth 3 keeps the agent segment
// inside parts[2] so the %2f/%2e containment superset scan sees the same
// agent+path span as the two-segment form.
func escapedRemainderBuiltin(escapedPath string) string {
	parts := strings.SplitN(escapedPath, "/", 3)
	if len(parts) < 3 || parts[1] != "runtime" {
		return ""
	}
	return "/" + parts[2]
}

// maxRedactBodyBytes caps how much of an upstream agent-config response the
// proxy buffers for redaction — same cap as the admin detail client
// (runtime.Client.GetAgentDetail). A larger body fails JSON decoding and
// fails closed (502) rather than truncating silently.
const maxRedactBodyBytes = 1 << 20

// isAgentConfigEgress reports whether the allowlisted route returns agent
// configuration JSON (agent list/detail) — the responses that echo MCP
// connection headers hub armed at deploy time (X-Agent-Capability "v1.*"
// and Authorization Bearer <runtime token> on knowledge MCPs, issue #111).
// Only GETs: the runs route is POST/SSE and carries stream content, not
// agent config.
func isAgentConfigEgress(method, canonicalPath string) bool {
	if method != http.MethodGet {
		return false
	}
	return canonicalPath == "/v1/agents" || strings.HasPrefix(canonicalPath, "/v1/agents/")
}

// redactProxyAgentJSON buffers the upstream agent-config JSON, strips
// credential fields via redactAgentDetail (shared with the admin detail
// egress), and swaps the response body in place. Any error — read failure or
// malformed JSON — fails closed: ReverseProxy routes it to ErrorHandler's
// stable 502, so uninspectable bytes never egress unredacted.
func redactProxyAgentJSON(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRedactBodyBytes))
	resp.Body.Close()
	if err != nil {
		return err
	}
	redacted, err := redactAgentDetail(body)
	if err != nil {
		return err
	}
	resp.Body = io.NopCloser(bytes.NewReader(redacted))
	resp.ContentLength = int64(len(redacted))
	resp.Header.Set("Content-Length", strconv.Itoa(len(redacted)))
	return nil
}

func classifyUpstreamErr(e error) string {
	if e == nil {
		return ""
	}
	s := e.Error()
	switch {
	case strings.Contains(s, "context deadline exceeded") || strings.Contains(s, "timeout"):
		return "timeout"
	case strings.Contains(s, "dial") || strings.Contains(s, "connection refused"):
		return "dial"
	case strings.Contains(s, "reset by peer"):
		return "reset"
	case strings.Contains(s, "EOF"):
		return "eof"
	}
	return "upstream"
}

// auditRuntimeProxy logs one structured line per proxied request. Never logs
// credentials, URL query, bodies, or the upstream address (spec 3.4 audit).
func auditRuntimeProxy(c *gin.Context, org, agentName string, status int, start time.Time, upstreamErr string) {
	log.Printf(`[RuntimeProxy] {"tenant":%q,"agent":%q,"method":%q,"path":%q,"status":%d,"duration_ms":%d,"upstream_err":%q}`,
		org, agentName, c.Request.Method, c.Request.URL.Path, status, time.Since(start).Milliseconds(), upstreamErr)
}
