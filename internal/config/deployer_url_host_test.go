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

func TestLegacyUpstreamHostConfigFieldRemainsSupported(t *testing.T) {
	v := viper.New()
	v.Set("deployer.upstream_host", "docker-host.internal")
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := cfg.Deployer.UpstreamHost; got != "docker-host.internal" {
		t.Fatalf("legacy deployer.upstream_host = %q, want docker-host.internal", got)
	}
}

// DeployerURLHost remains a derived compatibility value. Runtime callers use
// UpstreamHost so a sibling hub container can dial host-published ports.
func TestResolveBaseURLHostSemantics(t *testing.T) {
	if got := deriveDeployerURLHost("http://agent-deployer:8080"); got != "agent-deployer" {
		t.Fatalf("unexpected host %q", got)
	}
}

func TestRuntimeUpstreamHostEnvOverridesDeployerURLHost(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("AGENT_DEPLOYER_URL", "http://deployer:8080")
	t.Setenv("AGENT_DEPLOYER_PUBLIC_HOST", "runtime.example.com")
	t.Setenv("AGENT_RUNTIME_UPSTREAM_HOST", "host.docker.internal")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Deployer.UpstreamHost; got != "host.docker.internal" {
		t.Fatalf("UpstreamHost = %q, want host.docker.internal", got)
	}
	if got := cfg.Deployer.DeployerURLHost; got != "deployer" {
		t.Fatalf("compatibility DeployerURLHost = %q, want deployer", got)
	}
}

func TestRuntimeUpstreamHostLegacyFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		deployerURL string
		publicHost  string
		want        string
	}{
		{name: "deployer URL host", deployerURL: "http://deployer:8080", publicHost: "runtime.example.com", want: "deployer"},
		{name: "public host without URL", publicHost: "runtime.example.com", want: "runtime.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("AGENT_DEPLOYER_URL", tc.deployerURL)
			t.Setenv("AGENT_DEPLOYER_PUBLIC_HOST", tc.publicHost)
			t.Setenv("AGENT_RUNTIME_UPSTREAM_HOST", "")
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if got := cfg.Deployer.UpstreamHost; got != tc.want {
				t.Fatalf("UpstreamHost = %q, want %q", got, tc.want)
			}
		})
	}
}
