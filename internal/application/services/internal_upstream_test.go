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
