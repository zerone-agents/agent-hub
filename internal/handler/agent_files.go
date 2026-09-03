package handler

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

	"github.com/gin-gonic/gin"
)

// AgentFilesHandler proxies runtime /v1/files* endpoints. It reuses the
// existing AgentDetailService (ResolveRuntime + RuntimeClient) so no new
// service / interface is introduced.
//
// Three endpoints share one private proxy() helper:
//
//	GET  /api/v1/admin/agents/:name/files           → runtime GET  /v1/files
//	GET  /api/v1/admin/agents/:name/files/content   → runtime GET  /v1/files/content
//	HEAD /api/v1/admin/agents/:name/files/content   → runtime HEAD /v1/files/content
type AgentFilesHandler struct {
	svc AgentDetailService
}

func NewAgentFilesHandler(svc AgentDetailService) *AgentFilesHandler {
	return &AgentFilesHandler{svc: svc}
}

func (h *AgentFilesHandler) ListFiles(c *gin.Context) {
	h.proxy(c, http.MethodGet, "/v1/files")
}

func (h *AgentFilesHandler) GetContent(c *gin.Context) {
	h.proxy(c, http.MethodGet, "/v1/files/content")
}

func (h *AgentFilesHandler) HeadContent(c *gin.Context) {
	h.proxy(c, http.MethodHead, "/v1/files/content")
}

// proxy resolves the agent's runtime, builds a proxy request, and streams
// the runtime response back to the client verbatim. Response headers are
// forwarded via a whitelist (we don't want to leak runtime internals like
// Server or X-Powered-By). Status codes pass through unchanged so 4xx
// business errors (400/404/416) reach the client with the same meaning.
func (h *AgentFilesHandler) proxy(c *gin.Context, method, runtimePath string) {
	agentName := services.NormalizeAgentName(c.Param("name"))

	baseURL, apiKey, _, err := h.svc.ResolveRuntime(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}

	// Pass the original query string (path/recursive/depth/...) to runtime
	// unchanged. URL-encoding is preserved by RawQuery.
	pathAndQuery := runtimePath
	if c.Request.URL.RawQuery != "" {
		pathAndQuery += "?" + c.Request.URL.RawQuery
	}

	// 10 min timeout covers large file downloads; chat-style endpoints finish
	// in milliseconds. The context cancels automatically when the client
	// disconnects (gin request context).
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	resp, err := h.svc.RuntimeClient().ProxyFiles(ctx, method, baseURL, apiKey, pathAndQuery, c.GetHeader("Range"))
	if err != nil {
		respondError(c, http.StatusBadGateway, "runtime unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	// spec §5.4: runtime 5xx → control-panel 502 ("runtime unreachable: ...").
	// ProxyFiles returns 5xx as a normal *http.Response, so we explicitly wrap
	// it here. Read+close the body ourselves and return early; the deferred
	// close above is idempotent on an already-closed body.
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		respondError(c, http.StatusBadGateway, "runtime unreachable: HTTP "+strconv.Itoa(resp.StatusCode)+": "+string(body))
		return
	}

	// Whitelist of response headers to forward. Anything outside this list
	// (e.g. Server, X-Powered-By, Set-Cookie) is dropped.
	forwardHeaders := []string{
		"Content-Type",
		"Content-Disposition",
		"Content-Length",
		"Accept-Ranges",
		"Content-Range",
		"Last-Modified",
	}
	for _, hdr := range forwardHeaders {
		if v := resp.Header.Get(hdr); v != "" {
			c.Header(hdr, v)
		}
	}

	// Status code passes through verbatim: 200/206/400/404/416 all retain
	// their runtime meaning. 5xx was already wrapped to 502 above.
	c.Status(resp.StatusCode)

	// HEAD never writes a body. For GET we stream bytes via io.Copy, which
	// reads from resp.Body in 32KB chunks and writes them to c.Writer
	// immediately — Go's bufio default keeps memory bounded without manual
	// flushing. (Per-chunk flushing matters for SSE streams, not file bytes.)
	if method != http.MethodHead {
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}
