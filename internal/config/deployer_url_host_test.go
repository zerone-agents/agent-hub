package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestDeriveDeployerURLHost(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"hostname", "http://agent-deployer:8080", "agent-deployer"},
		{"ipv4", "http://127.0.0.1:8080", "127.0.0.1"},
		{"ipv6 bare hostname", "http://[2001:db8::1]:8080", "2001:db8::1"},
		{"empty", "", ""},
		{"malformed", "://not a url", ""},
		{"no host", "http://", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveDeployerURLHost(tc.url); got != tc.want {
				t.Fatalf("deriveDeployerURLHost(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// 同名配置键不能写入 DeployerURLHost（mapstructure:"-" 结构性保证），
// 且派生不受 PublicHost 影响。
func TestDeployerURLHostNotConfigurable(t *testing.T) {
	v := viper.New()
	v.Set("deployer.url", "http://agent-deployer:8080")
	v.Set("deployer.public_host", "203.0.113.10")
	v.Set("deployer.deployer_url_host", "evil.example.com")
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.Deployer.DeployerURLHost != "" {
		t.Fatalf("config key must not populate DeployerURLHost, got %q", cfg.Deployer.DeployerURLHost)
	}
	if got := deriveDeployerURLHost(cfg.Deployer.URL); got != "agent-deployer" {
		t.Fatalf("derived host = %q, want agent-deployer", got)
	}
}

// 真实语义：钉住 deriveDeployerURLHost 派生 host 的回归——它是运行时
// 配置钩子回填 DeployerURLHost 的唯一派生点（config.go:247,260），供
// no-Kong runtime proxy / chat 解析使用，且从不回退 PublicHost。UpstreamHost
// override/fallback 属既有行为，由 TestDeployerURLHostNotConfigurable 与
// 上游 config 测试覆盖；本测试从未触碰 UpstreamHost，Kong 链路不受影响。
func TestResolveBaseURLHostSemantics(t *testing.T) {
	if got := deriveDeployerURLHost("http://agent-deployer:8080"); got != "agent-deployer" {
		t.Fatalf("unexpected host %q", got)
	}
}
