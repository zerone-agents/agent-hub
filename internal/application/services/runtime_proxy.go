// internal/application/services/runtime_proxy.go
package services

import (
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"control-panel/internal/domain/agent"
)

// RuntimeProxyAgentRepo is the tenant-scoped agent lookup the runtime proxy
// needs (repo-level, no DTO aggregation — issue #77 design requirement 3).
type RuntimeProxyAgentRepo interface {
	GetByName(tenantID, name string) (*agent.AgentConfig, error)
}

// ProxyError carries stable HTTP semantics for rejected proxy requests.
type ProxyError struct {
	Code   int
	Reason string
}

func (e *ProxyError) Error() string { return fmt.Sprintf("runtime proxy: %d %s", e.Code, e.Reason) }

// ProxyDecision is the validated forwarding plan for one request.
type ProxyDecision struct {
	UpstreamBase  string        // "http://host:port" (JoinHostPort-built)
	CanonicalPath string        // decoded, validated runtime path (prefix stripped)
	Timeout       time.Duration // 0 = no overall deadline (SSE)
	IsSSE         bool
}

type proxyRoute struct {
	methods []string
	pattern string // ":seg" matches exactly one non-empty segment
	timeout time.Duration
	sse     bool
}

// First-phase allowlist, baseline runtime v2.2.0 (issue #77). Anything not
// listed is 404 (path) / 405 (method).
var proxyAllowlist = []proxyRoute{
	{methods: []string{http.MethodGet}, pattern: "/health", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet}, pattern: "/v1/agents", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet}, pattern: "/v1/agents/:agentId", timeout: 120 * time.Second},
	{methods: []string{http.MethodPost}, pattern: "/v1/agents/:agentId/runs", timeout: 0, sse: true},
	{methods: []string{http.MethodPost}, pattern: "/v1/runs/:runId/cancel", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet}, pattern: "/v1/sessions", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet}, pattern: "/v1/sessions/:sessionId", timeout: 120 * time.Second},
	{methods: []string{http.MethodDelete}, pattern: "/v1/sessions/:sessionId", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet}, pattern: "/v1/files", timeout: 120 * time.Second},
	{methods: []string{http.MethodGet, http.MethodHead}, pattern: "/v1/files/content", timeout: 10 * time.Minute},
}

// Conservative org/agent name gate; the authoritative check is the exact-match
// tenant-scoped GetByName below (unknown names → 404 either way).
var proxyNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type RuntimeProxyService struct {
	repo         RuntimeProxyAgentRepo
	upstreamHost string // cfg.Deployer.DeployerURLHost; "" = fail closed
}

func NewRuntimeProxyService(repo RuntimeProxyAgentRepo, upstreamHost string) *RuntimeProxyService {
	return &RuntimeProxyService{repo: repo, upstreamHost: upstreamHost}
}

// Resolve runs the proxy preconditions in spec order: ① path/name validation
// ② tenant-scoped lookup ③ status ④ allowlist ⑤ upstream construction.
// It never inspects credentials (pure passthrough auth model, D4).
func (s *RuntimeProxyService) Resolve(org, agentName, method, escapedRemainder, decodedRemainder string) (*ProxyDecision, *ProxyError) {
	notFound := &ProxyError{Code: 404, Reason: "not found"}
	if !proxyNameRe.MatchString(org) || !proxyNameRe.MatchString(agentName) {
		return nil, notFound
	}
	canon, ok := canonicalizePath(escapedRemainder, decodedRemainder)
	if !ok {
		return nil, notFound
	}
	cfg, err := s.repo.GetByName(org, agentName)
	if err != nil || cfg == nil {
		return nil, notFound
	}
	if cfg.DeploymentStatus != "running" {
		return nil, &ProxyError{Code: 409, Reason: "agent is not running"}
	}
	if cfg.RuntimePort <= 0 {
		return nil, &ProxyError{Code: 502, Reason: "runtime upstream unavailable"}
	}
	route, matched, methodOK := matchAllowlist(method, canon)
	if !matched {
		return nil, notFound
	}
	if !methodOK {
		return nil, &ProxyError{Code: 405, Reason: "method not allowed"}
	}
	// Fail closed BEFORE any URL construction: DB may retain running state
	// from before a config change (spec, round-5 finding).
	if s.upstreamHost == "" {
		return nil, &ProxyError{Code: 502, Reason: "runtime upstream unavailable"}
	}
	base := "http://" + net.JoinHostPort(s.upstreamHost, strconv.Itoa(cfg.RuntimePort))
	return &ProxyDecision{UpstreamBase: base, CanonicalPath: canon, Timeout: route.timeout, IsSSE: route.sse}, nil
}

// canonicalizePath rejects encoded slashes/dots (case-insensitive) in the
// escaped form and dot segments in the decoded form; returns the decoded
// canonical path (semantically-equivalent re-encoding happens at Rewrite).
func canonicalizePath(escaped, decoded string) (string, bool) {
	if !strings.HasPrefix(decoded, "/") || decoded == "/" {
		return "", false
	}
	low := strings.ToLower(escaped)
	if strings.Contains(low, "%2f") || strings.Contains(low, "%2e") {
		return "", false
	}
	for _, seg := range strings.Split(decoded, "/") {
		if seg == "." || seg == ".." {
			return "", false
		}
	}
	return decoded, true
}

func matchAllowlist(method, path string) (route proxyRoute, pathMatched, methodOK bool) {
	req := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, r := range proxyAllowlist {
		pat := strings.Split(strings.TrimPrefix(r.pattern, "/"), "/")
		if len(req) != len(pat) {
			continue
		}
		ok := true
		for i, ps := range pat {
			if strings.HasPrefix(ps, ":") {
				if req[i] == "" {
					ok = false
					break
				}
				continue
			}
			if ps != req[i] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		pathMatched = true
		for _, m := range r.methods {
			if m == method {
				return r, true, true
			}
		}
	}
	return proxyRoute{}, pathMatched, false
}
