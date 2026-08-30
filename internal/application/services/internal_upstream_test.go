package services

import (
	"testing"
	"time"
)

// newKongEnabledStub returns a KongGatewayService whose enabled() is true
// (non-nil client), reusing the in-memory fake from kong_gateway_test.go.
func newKongEnabledStub() *KongGatewayService {
	return &KongGatewayService{client: newFakeKong()}
}

// D1：Kong 模式探针有意走公网（Kong 部署公网可达性验证信号）；
// 无 Kong 探针走 deployer 内网 hostname；空 host 直接失败且不调 healthProbe。
func TestProbeHostBranching(t *testing.T) {
	// 无 Kong + 内网 host
	s := &AgentDeployerService{publicHost: "203.0.113.10", upstreamHost: "agent-deployer"}
	if got, err := s.probeHost(); err != nil || got != "agent-deployer" {
		t.Fatalf("no-kong probeHost = %q, %v", got, err)
	}
	// 无 Kong + 空 host → 直接失败（不调 healthProbe：probeHost 在 WaitForHealthy
	// 入口调用，错误即短路）
	s2 := &AgentDeployerService{publicHost: "203.0.113.10", upstreamHost: ""}
	if _, err := s2.probeHost(); err == nil {
		t.Fatal("empty upstream host must fail closed")
	}
	// Kong 模式（kongSvc.enabled() 为 true 的最小桩）：保持 publicHost
	s3 := &AgentDeployerService{publicHost: "203.0.113.10", upstreamHost: ""}
	s3.kongSvc = newKongEnabledStub()
	if got, err := s3.probeHost(); err != nil || got != "203.0.113.10" {
		t.Fatalf("kong probeHost = %q, %v (must stay publicHost)", got, err)
	}
}

func TestWaitForHealthyFailsFastOnEmptyHost(t *testing.T) {
	s := &AgentDeployerService{publicHost: "203.0.113.10", upstreamHost: ""} // kongSvc == nil
	start := time.Now()
	if _, err := s.WaitForHealthy(nil, "test", 5*time.Second); err == nil {
		t.Fatal("expected immediate error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("must fail fast (before any polling/dial), took %v", elapsed)
	}
}

// ResolveRuntime 的无 Kong fallback：JoinHostPort（IPv6 安全）+ 空 host 错误。
func TestNoKongUpstream(t *testing.T) {
	s := &AgentChatService{publicHost: "203.0.113.10", upstreamHost: "agent-deployer"}
	if got, err := s.noKongUpstream(32100); err != nil || got != "http://agent-deployer:32100" {
		t.Fatalf("noKongUpstream = %q, %v", got, err)
	}
	s6 := &AgentChatService{publicHost: "203.0.113.10", upstreamHost: "2001:db8::1"}
	if got, _ := s6.noKongUpstream(32100); got != "http://[2001:db8::1]:32100" {
		t.Fatalf("IPv6 = %q", got)
	}
	sEmpty := &AgentChatService{publicHost: "203.0.113.10", upstreamHost: ""}
	if _, err := sEmpty.noKongUpstream(32100); err == nil {
		t.Fatal("empty upstream host must return error")
	}
}

// healthProbeURL（默认无 Kong 探针）必须用 JoinHostPort：IPv6 publicHost 产出
// 合法带方括号 URL，hostname 形式不变。
func TestHealthProbeURL(t *testing.T) {
	if got := healthProbeURL("2001:db8::1", 8080); got != "http://[2001:db8::1]:8080/health" {
		t.Fatalf("IPv6 = %q, want http://[2001:db8::1]:8080/health", got)
	}
	if got := healthProbeURL("hub.example.com", 80); got != "http://hub.example.com:80/health" {
		t.Fatalf("hostname = %q, want http://hub.example.com:80/health", got)
	}
}

func TestKongEnabledForChat(t *testing.T) {
	nilDeployer := &AgentChatService{}
	if nilDeployer.kongEnabledForChat() {
		t.Fatal("nil deployerSvc must be treated as no-Kong")
	}
	withKong := &AgentChatService{deployerSvc: &AgentDeployerService{kongSvc: &KongGatewayService{client: newFakeKong()}}}
	if !withKong.kongEnabledForChat() {
		t.Fatal("kong-enabled deployerSvc must report true")
	}
}

// resolveBaseURL 严格按模式分支（issue #77 验收 #10：Kong 链路零变化）。
// Kong+空 URL（预注册边缘）在真实 GetStatus→toDTO 链路中不可达（toDTO 在
// Kong 模式恒填充 RuntimeURL），经 resolveBaseURL 缝隙直测：必须恢复 PR 前
// 的 publicHost:port 原有 fallback，绝不落 deployer 内网回源。
func TestResolveBaseURLStrictModeBranching(t *testing.T) {
	s := &AgentChatService{publicHost: "203.0.113.10", upstreamHost: "agent-deployer"}
	// Kong + 空 URL → PR 前 main 的 publicHost:port 形式（非 deployer upstream）
	got, err := s.resolveBaseURL(true, "", 32100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "http://203.0.113.10:32100" {
		t.Fatalf("kong+empty = %q, want http://203.0.113.10:32100 (publicHost form, not deployer upstream)", got)
	}
	// IPv6 publicHost（专家二轮）：仅构造方式换 JoinHostPort，上面 203.0.113.10
	// 断言保持不变；方括号形式为新增回归。
	s6 := &AgentChatService{publicHost: "2001:db8::1", upstreamHost: "agent-deployer"}
	if got, err := s6.resolveBaseURL(true, "", 32100); err != nil || got != "http://[2001:db8::1]:32100" {
		t.Fatalf("kong+empty IPv6 = %q, %v; want http://[2001:db8::1]:32100", got, err)
	}
	// Kong + 网关 URL → 原样透传
	if got, _ := s.resolveBaseURL(true, "https://kong.example.com/zerone/agent", 32100); got != "https://kong.example.com/zerone/agent" {
		t.Fatalf("kong+url = %q, want verbatim gateway URL", got)
	}
	// 无 Kong → 永远内网回源，无视传入公开地址（相对路径或 hairpin 绝对 URL）
	if got, err := s.resolveBaseURL(false, "/runtime/default/test", 32100); err != nil || got != "http://agent-deployer:32100" {
		t.Fatalf("no-kong relative = %q, %v; want deployer upstream", got, err)
	}
	if got, err := s.resolveBaseURL(false, "http://203.0.113.10:32100", 32100); err != nil || got != "http://agent-deployer:32100" {
		t.Fatalf("no-kong absolute = %q, %v; want deployer upstream", got, err)
	}
	// 无 Kong + 空 upstream host → fail closed
	if _, err := (&AgentChatService{publicHost: "203.0.113.10", upstreamHost: ""}).resolveBaseURL(false, "", 32100); err == nil {
		t.Fatal("no-kong empty upstream host must fail closed")
	}
}
