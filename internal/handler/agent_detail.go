package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/runtime"

	"github.com/gin-gonic/gin"
)

// AgentDetailService is the subset of *services.AgentChatService that
// AgentDetailHandler depends on. Declared as an interface (following the
// knowledge_mcp.go pattern) so tests can substitute a fake without
// constructing the full AgentChatService.
//
// *services.AgentChatService implicitly satisfies this interface.
type AgentDetailService interface {
	ResolveRuntime(tenantID, agentName string) (string, string, string, error)
	RuntimeClient() *runtime.Client
}

// AgentDetailHandler proxies GET /v1/agents/:agentId from runtime.
// It deliberately bypasses respondSuccess and writes the runtime JSON
// unwrapped — the runtime's AgentDetail is already a stable API shape —
// but NOT verbatim: egress redaction (redactAgentDetail) strips
// credential headers before the bytes reach the client (issue #111).
type AgentDetailHandler struct {
	svc AgentDetailService
}

// NewAgentDetailHandler accepts a service that satisfies AgentDetailService.
// In production, pass *services.AgentChatService; it implicitly satisfies the
// interface. Tests pass a fake satisfying the same interface.
func NewAgentDetailHandler(svc AgentDetailService) *AgentDetailHandler {
	return &AgentDetailHandler{svc: svc}
}

// redactedHeaderValue is the fixed mask written over every value inside a
// map nested under a "headers" key. The runtime contract already pre-redacts
// header values to exactly this string (the frontend renders "***" as
// redacted), so masking is a byte-level no-op for a well-behaved runtime and
// a safety net for one whose redaction misses a header.
const redactedHeaderValue = "***"

// sensitiveDetailKeys are detail field names whose values must never cross
// the hub boundary, wherever they appear (deep traversal — not only under
// mcpServers.<name>.headers). Both are reusable credentials the hub itself
// arms at deploy time on every knowledge MCP:
//   - x-agent-capability: per-agent capability, wire format
//     "v1.<b64url(payload)>.<b64url(hmac)>" (issue #111) — a bearer-grade
//     credential for knowledge retrieval;
//   - authorization: "Bearer <runtime token>", substituted from the
//     $agent_runtime_token placeholder before the deployer write.
//
// Matching is case-insensitive: HTTP header names are case-insensitive and
// the runtime may echo any casing in its JSON.
var sensitiveDetailKeys = map[string]struct{}{
	"x-agent-capability": {},
	"authorization":      {},
}

// redactAgentDetail decodes the runtime AgentDetail JSON, strips credential
// fields (redactDetailValue), and re-encodes it. Numbers are decoded with
// UseNumber so integer literals re-encode byte-identically instead of
// round-tripping through float64. Malformed input returns an error; the
// caller fails closed rather than passing through uninspectable bytes.
func redactAgentDetail(body []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	redactDetailValue(root)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// redactDetailValue walks the decoded detail in place:
//   - map entries whose key names a sensitive credential are deleted, at any
//     depth — whatever fields the runtime adds in the future, keys in the
//     set never egress;
//   - every value inside a map nested under a "headers" key is masked:
//     user-configured MCP headers are credentials under arbitrary key names
//     a fixed set cannot enumerate, and header values carry no display
//     value the runtime contract doesn't already mask.
func redactDetailValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if _, sensitive := sensitiveDetailKeys[strings.ToLower(k)]; sensitive {
				delete(t, k)
				continue
			}
			if headers, ok := child.(map[string]any); ok && strings.EqualFold(k, "headers") {
				maskHeaderValues(headers)
				continue
			}
			redactDetailValue(child)
		}
	case []any:
		for _, item := range t {
			redactDetailValue(item)
		}
	}
}

// maskHeaderValues deletes sensitive keys and masks every remaining value
// in a headers map.
func maskHeaderValues(headers map[string]any) {
	for k := range headers {
		if _, sensitive := sensitiveDetailKeys[strings.ToLower(k)]; sensitive {
			delete(headers, k)
			continue
		}
		headers[k] = redactedHeaderValue
	}
}

// GetAgentDetail handles GET /api/v1/admin/agents/:name/detail.
// It resolves the agent's runtime container, fetches AgentDetail from
// runtime, redacts credential fields, and returns the JSON unwrapped.
func (h *AgentDetailHandler) GetAgentDetail(c *gin.Context) {
	agentName := services.NormalizeAgentName(c.Param("name"))

	baseURL, apiKey, _, err := h.svc.ResolveRuntime(tenant.GetTenantID(c), agentName)
	if err != nil {
		respondError(c, http.StatusConflict, "agent not available: "+err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// runtime 注册名为裸 Agent ID（issue #114）；scoped deployment key 仅是
	// deployer 资源标识，不参与 runtime 寻址。
	body, err := h.svc.RuntimeClient().GetAgentDetail(ctx, baseURL, services.NormalizeAgentName(agentName), apiKey)
	if err != nil {
		// Non-2xx upstream bodies are intentionally discarded by the
		// runtime client (they can echo hub-armed MCP credentials, issue
		// #111); only the status code survives. Map 404 specifically;
		// other errors are 502 with a neutral Chinese message — the
		// English detail goes to server-side logs only (CONTRIBUTING).
		var httpErr *runtime.RuntimeHTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			respondError(c, http.StatusNotFound, "Agent 在运行时不存在")
			return
		}
		log.Printf("agent detail for %q failed: %v", agentName, err)
		respondError(c, http.StatusBadGateway, "Agent 运行时不可用")
		return
	}

	// Egress redaction (issue #111: 日志与 detail 不泄漏 capability). The
	// runtime detail echoes the agent's resolved config, whose MCP connection
	// headers the hub itself armed at deploy time (X-Agent-Capability "v1.*"
	// and Authorization Bearer <runtime token> on every knowledge MCP). The
	// runtime redacts header values, but the hub must not depend on that
	// redaction keeping up with new headers — strip the credentials here,
	// before the bytes reach the client. Malformed runtime JSON fails closed
	// (502, neutral message, no body echo).
	redacted, err := redactAgentDetail(body)
	if err != nil {
		log.Printf("agent detail for %q: redaction failed (malformed runtime JSON): %v", agentName, err)
		respondError(c, http.StatusBadGateway, "Agent 运行时返回了无法解析的详情数据")
		return
	}

	// Unwrapped passthrough shape: do NOT wrap in {success, data}. The
	// runtime JSON shape is the stable contract; re-wrapping would just
	// bloat responses.
	c.Data(http.StatusOK, "application/json", redacted)
}
