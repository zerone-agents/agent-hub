// internal/application/services/runtime_proxy_test.go
package services

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"control-panel/internal/domain/agent"
)

type fakeProxyRepo struct {
	agents map[string]*agent.AgentConfig // key: org + "/" + name
	gotOrg string                        // 记录查询用的 tenant scope
}

func (f *fakeProxyRepo) GetByName(tenantID, name string) (*agent.AgentConfig, error) {
	f.gotOrg = tenantID
	cfg, ok := f.agents[tenantID+"/"+name]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return cfg, nil
}

func newTestRepo() *fakeProxyRepo {
	return &fakeProxyRepo{agents: map[string]*agent.AgentConfig{
		"default/test": {DeploymentStatus: "running", RuntimePort: 32100},
		"acme/test":    {DeploymentStatus: "running", RuntimePort: 32101},
	}}
}

func TestResolveAllowlistMatrix(t *testing.T) {
	svc := NewRuntimeProxyService(newTestRepo(), "agent-deployer")
	cases := []struct {
		name    string
		method  string
		decoded string
		wantOK  bool
		timeout time.Duration // 期望超时：plain=120s、file=10min、sse=0
	}{
		{"health", "GET", "/health", true, 120 * time.Second},
		{"agents list", "GET", "/v1/agents", true, 120 * time.Second},
		{"agent detail", "GET", "/v1/agents/my-agent", true, 120 * time.Second},
		{"sse run", "POST", "/v1/agents/my-agent/runs", true, 0},
		{"cancel", "POST", "/v1/runs/run-123/cancel", true, 120 * time.Second},
		{"sessions", "GET", "/v1/sessions", true, 120 * time.Second},
		{"session detail", "GET", "/v1/sessions/s-1", true, 120 * time.Second},
		{"session delete", "DELETE", "/v1/sessions/s-1", true, 120 * time.Second},
		{"files list", "GET", "/v1/files", true, 120 * time.Second},
		{"file get", "GET", "/v1/files/content", true, 10 * time.Minute},
		{"file head", "HEAD", "/v1/files/content", true, 10 * time.Minute},
		{"metrics excluded", "GET", "/v1/metrics", false, 0},
		{"cron excluded", "POST", "/v1/cron/jobs", false, 0},
		{"bare root excluded", "GET", "/", false, 0},
		{"unknown path", "GET", "/v1/unknown", false, 0},
		{"method mismatch 405", "POST", "/health", false, 0},
		{"method mismatch on files", "PUT", "/v1/files/content", false, 0},
		// 尾斜杠/空段：不得被 :param 模式吞掉（issue #91，T3 锁定现状）。
		{"trailing slash 404", "GET", "/v1/agents/", false, 0},
		{"empty segment 404", "GET", "/v1//agents", false, 0},
		{"double slash 404", "GET", "/v1/agents//my-agent", false, 0},
		{"file trailing slash 404", "GET", "/v1/files/content/", false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, pe := svc.Resolve("default", "test", tc.method, tc.decoded, tc.decoded)
			if !tc.wantOK {
				if pe == nil {
					t.Fatalf("expected rejection, got decision %+v", d)
				}
				if tc.name == "method mismatch 405" || tc.name == "method mismatch on files" {
					if pe.Code != 405 {
						t.Fatalf("want 405, got %d", pe.Code)
					}
					if pe.AllowHeader == "" {
						t.Fatalf("405 must carry Allow header, got %q", pe.AllowHeader)
					}
				} else if pe.Code != 404 {
					t.Fatalf("want 404, got %d (%s)", pe.Code, pe.Reason)
				}
				return
			}
			if pe != nil {
				t.Fatalf("unexpected error: %v", pe)
			}
			if d.Timeout != tc.timeout {
				t.Fatalf("%s: timeout = %v, want %v", tc.name, d.Timeout, tc.timeout)
			}
			if d.IsSSE != (tc.timeout == 0) {
				t.Fatalf("%s: IsSSE = %v, want %v", tc.name, d.IsSSE, tc.timeout == 0)
			}
		})
	}
}

func TestResolvePathTraversal(t *testing.T) {
	svc := NewRuntimeProxyService(newTestRepo(), "agent-deployer")
	for _, tc := range []struct{ name, decoded, escaped string }{
		{"dot segment", "/v1/../v1/agents", "/v1/../v1/agents"},
		{"dot dir", "/v1/./agents", "/v1/./agents"},
		{"encoded slash", "/v1/agents", "/v1%2fagents"},
		{"encoded slash upper", "/v1/agents", "/v1%2Fagents"},
		{"encoded dot", "/v1/agents", "/v1%2e%2e/agents"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, pe := svc.Resolve("default", "test", "GET", tc.escaped, tc.decoded)
			if pe == nil || pe.Code != 404 {
				t.Fatalf("want 404, got %+v", pe)
			}
		})
	}
}

func TestResolveTenantIsolation(t *testing.T) {
	repo := newTestRepo()
	svc := NewRuntimeProxyService(repo, "agent-deployer")
	// default 租户的 agent 存在；acme 同名 agent 也存在，端口不同
	d, pe := svc.Resolve("default", "test", "GET", "/health", "/health")
	if pe != nil {
		t.Fatalf("unexpected: %v", pe)
	}
	if d.UpstreamBase != "http://agent-deployer:32100" {
		t.Fatalf("default/test upstream = %q", d.UpstreamBase)
	}
	if repo.gotOrg != "default" {
		t.Fatalf("repo must be scoped by path org, got %q", repo.gotOrg)
	}
	d2, _ := svc.Resolve("acme", "test", "GET", "/health", "/health")
	if d2.UpstreamBase != "http://agent-deployer:32101" {
		t.Fatalf("acme/test upstream = %q", d2.UpstreamBase)
	}
	// 不存在的租户 → 404，无跨租户回退
	if _, pe := svc.Resolve("other", "test", "GET", "/health", "/health"); pe == nil || pe.Code != 404 {
		t.Fatalf("want 404 for unknown org, got %+v", pe)
	}
}

func TestResolveStatusSemantics(t *testing.T) {
	repo := newTestRepo()
	repo.agents["default/stopped"] = &agent.AgentConfig{DeploymentStatus: "stopped", RuntimePort: 32100}
	repo.agents["default/noport"] = &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: 0}
	svc := NewRuntimeProxyService(repo, "agent-deployer")
	if _, pe := svc.Resolve("default", "stopped", "GET", "/health", "/health"); pe == nil || pe.Code != 409 {
		t.Fatalf("stopped want 409, got %+v", pe)
	}
	if _, pe := svc.Resolve("default", "noport", "GET", "/health", "/health"); pe == nil || pe.Code != 502 {
		t.Fatalf("noport want 502, got %+v", pe)
	}
}

// 危险组合：URL 空（upstreamHost 空）+ PublicHost 无关 + DB 残留 running deployment
// → 稳定错误、零拨号（Resolve 在构造 URL 前即失败，无 transport 参与）。
func TestResolveEmptyHostFailClosed(t *testing.T) {
	svc := NewRuntimeProxyService(newTestRepo(), "")
	d, pe := svc.Resolve("default", "test", "GET", "/health", "/health")
	if pe == nil || pe.Code != 502 {
		t.Fatalf("want 502 fail-closed, got decision=%+v err=%+v", d, pe)
	}
	if d != nil {
		t.Fatalf("no decision may be produced on empty host")
	}
}

// upstream 构造使用 JoinHostPort（IPv6 安全）。
func TestResolveUpstreamIPv6(t *testing.T) {
	svc := NewRuntimeProxyService(newTestRepo(), "2001:db8::1")
	d, pe := svc.Resolve("default", "test", "GET", "/health", "/health")
	if pe != nil {
		t.Fatalf("unexpected: %v", pe)
	}
	if d.UpstreamBase != "http://[2001:db8::1]:32100" {
		t.Fatalf("IPv6 upstream = %q, want http://[2001:db8::1]:32100", d.UpstreamBase)
	}
}

// legacy 不合规命名（大写/下划线/前导数字/尾连字符）必须在触碰 DB 之前 404
// （issue #77 验收 #2）——即使 DB 中存在形近存量行（大小写不敏感 collation
// 或 legacy 行可能命中），命名门也先行拒绝，GetByName 不被调用。
func TestResolveLegacyInvalidNames(t *testing.T) {
	repo := newTestRepo()
	// 预置 legacy 形式的存量行：若命名门失效，这些行可能被命中。
	repo.agents["Acme/test"] = &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: 32102}
	repo.agents["default/under_score"] = &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: 32103}
	repo.agents["0rg/test"] = &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: 32104}
	repo.agents["default/test-"] = &agent.AgentConfig{DeploymentStatus: "running", RuntimePort: 32105}
	svc := NewRuntimeProxyService(repo, "agent-deployer")
	cases := []struct{ name, org, agentName string }{
		{"uppercase org", "Acme", "test"},
		{"underscore agent", "default", "under_score"},
		{"leading-digit org", "0rg", "test"},
		{"trailing-hyphen agent", "default", "test-"},
		{"hyphenated org", "ac-me", "test"},
		{"uppercase agent", "default", "Test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.gotOrg = "" // 每例重置，验证不触 DB
			d, pe := svc.Resolve(tc.org, tc.agentName, "GET", "/health", "/health")
			if pe == nil || pe.Code != 404 {
				t.Fatalf("want 404, got decision=%+v err=%+v", d, pe)
			}
			if d != nil {
				t.Fatalf("no decision may be produced for legacy-invalid names")
			}
			if repo.gotOrg != "" {
				t.Fatalf("repo must not be queried for legacy-invalid %q/%q (got query org=%q)", tc.org, tc.agentName, repo.gotOrg)
			}
		})
	}
	// 对照：合规命名仍放行（default/test 在 newTestRepo 中为 running）。
	if _, pe := svc.Resolve("default", "test", "GET", "/health", "/health"); pe != nil {
		t.Fatalf("conforming name must keep passing, got %v", pe)
	}
}

// TestMatchAllowlistFirstMatchWins 锁定 matchAllowlist 的重叠路径语义（issue #91
// 审查 P3）：切片序中第一个 method+path 均命中的路由胜出——注释与实现一致，
// 防未来「取最后匹配行」的误解回归。
func TestMatchAllowlistFirstMatchWins(t *testing.T) {
	// 模拟未来重叠模式：/v1/agents/:id 与 /v1/agents/special 同时命中
	// /v1/agents/special（param 通配 + 字面量）。切片序靠前者胜出。
	saved := proxyAllowlist
	defer func() { proxyAllowlist = saved }()
	proxyAllowlist = []proxyRoute{
		{methods: []string{http.MethodGet}, pattern: "/v1/agents/:id", timeout: 120 * time.Second},
		{methods: []string{http.MethodGet}, pattern: "/v1/agents/special", timeout: 9 * time.Second},
	}

	route, pathMatched, methodOK := matchAllowlist(http.MethodGet, "/v1/agents/special")
	if !pathMatched || !methodOK {
		t.Fatalf("overlap must match, got pathMatched=%v methodOK=%v", pathMatched, methodOK)
	}
	if route.pattern != "/v1/agents/:id" {
		t.Fatalf("first match wins: pattern = %q, want /v1/agents/:id", route.pattern)
	}
	if route.timeout != 120*time.Second {
		t.Fatalf("first match wins: timeout = %v, want 120s", route.timeout)
	}

	// method 不符时不返回路由，但 pathMatched 保留（潜语义注释所述）。
	// 批次三（#91）：route 返回第一条 path 命中的路由（供 405 Allow 头），
	// 不再是零值。
	route, pathMatched, methodOK = matchAllowlist(http.MethodPost, "/v1/agents/special")
	if !pathMatched {
		t.Fatal("path matched flag must persist on method mismatch")
	}
	if methodOK {
		t.Fatal("POST must not match GET-only route")
	}
	if route.pattern != "/v1/agents/:id" {
		t.Fatalf("method mismatch must carry first path-matched route, got %q", route.pattern)
	}
	if len(route.methods) != 1 || route.methods[0] != http.MethodGet {
		t.Fatalf("first path-matched route methods = %v, want [GET]", route.methods)
	}
}
