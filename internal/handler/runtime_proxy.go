package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// RuntimeProxyHandler forwards allowlisted runtime API calls to the agent's
// runtime container. Pure passthrough credentials (D4): runtime is the only
// auth point, exactly like the Kong path.
type RuntimeProxyHandler struct {
	svc       *services.RuntimeProxyService
	transport *http.Transport
}

func NewRuntimeProxyHandler(svc *services.RuntimeProxyService) *RuntimeProxyHandler {
	// One shared transport: ResponseHeaderTimeout gives every endpoint fast
	// first-byte failure; overall deadlines are per-request contexts
	// (ReverseProxy.Transport is a RoundTripper — no Client.Timeout exists).
	return &RuntimeProxyHandler{
		svc:       svc,
		transport: &http.Transport{ResponseHeaderTimeout: 30 * time.Second},
	}
}

// RegisterRuntimeProxyRoutes mounts the proxy. Called only when Kong is NOT
// configured (cmd/server/main.go); Kong mode keeps its own route chain.
// builtinAuth (issue #114) mounts the single-segment public form
// "/runtime/<agent>" — the implicit default tenant never appears in the URL.
// The two shapes can't be mounted together: gin forbids conflicting wildcard
// names at the same prefix position.
func RegisterRuntimeProxyRoutes(r *gin.Engine, svc *services.RuntimeProxyService, builtinAuth bool) {
	h := NewRuntimeProxyHandler(svc)
	if builtinAuth {
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
		respondError(c, perr.Code, perr.Reason)
		auditRuntimeProxy(c, org, agentName, perr.Code, start, "")
		return
	}
	upstream, err := url.Parse(decision.UpstreamBase)
	if err != nil { // unreachable: base is JoinHostPort-built from validated parts
		respondError(c, http.StatusBadGateway, "runtime upstream unavailable")
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
			return nil
		},
	}
	proxy.ServeHTTP(c.Writer, outReq)
	auditRuntimeProxy(c, org, agentName, c.Writer.Status(), start, upstreamErr)
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
