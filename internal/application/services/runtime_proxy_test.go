// internal/application/services/runtime_proxy_test.go
package services

import (
	"fmt"
	"testing"

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
		timeout string // "plain" | "file" | "sse" | ""
	}{
		{"health", "GET", "/health", true, "plain"},
		{"agents list", "GET", "/v1/agents", true, "plain"},
		{"agent detail", "GET", "/v1/agents/my-agent", true, "plain"},
		{"sse run", "POST", "/v1/agents/my-agent/runs", true, "sse"},
		{"cancel", "POST", "/v1/runs/run-123/cancel", true, "plain"},
		{"sessions", "GET", "/v1/sessions", true, "plain"},
		{"session detail", "GET", "/v1/sessions/s-1", true, "plain"},
		{"session delete", "DELETE", "/v1/sessions/s-1", true, "plain"},
		{"files list", "GET", "/v1/files", true, "plain"},
		{"file get", "GET", "/v1/files/content", true, "file"},
		{"file head", "HEAD", "/v1/files/content", true, "file"},
		{"metrics excluded", "GET", "/v1/metrics", false, ""},
		{"cron excluded", "POST", "/v1/cron/jobs", false, ""},
		{"bare root excluded", "GET", "/", false, ""},
		{"unknown path", "GET", "/v1/unknown", false, ""},
		{"method mismatch 405", "POST", "/health", false, ""},
		{"method mismatch on files", "PUT", "/v1/files/content", false, ""},
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
				} else if pe.Code != 404 {
					t.Fatalf("want 404, got %d (%s)", pe.Code, pe.Reason)
				}
				return
			}
			if pe != nil {
				t.Fatalf("unexpected error: %v", pe)
			}
			if d.Timeout == 0 && !d.IsSSE {
				t.Fatalf("plain/file endpoints must carry a timeout, got 0")
			}
			if tc.timeout == "sse" != d.IsSSE {
				t.Fatalf("IsSSE = %v, want %v", d.IsSSE, tc.timeout == "sse")
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
