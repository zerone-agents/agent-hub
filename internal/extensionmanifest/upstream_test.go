// H7 upstream SSRF 防护回归测试：声明时拒绝内网/环回/链路本地字面量，
// 公网域名放行；转发前解析校验同样拒绝。
package extensionmanifest

import (
	"strings"
	"testing"
)

func TestValidateAPIRouteUpstreamRejectsDisallowedIPs(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:8080/v1/metrics",
		"http://127.1.2.3/v1/metrics",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.8/api",
		"https://10.255.255.1/api",
		"http://192.168.1.1/router",
		"http://[::1]:8080/api",
		"http://[fe80::1]/api",
		"http://[fd00::1234]/api",
	}
	for _, raw := range cases {
		if err := ValidateAPIRouteUpstream(raw); err == nil {
			t.Errorf("upstream %q 必须被拒绝（内网/环回/链路本地）", raw)
		} else if !strings.Contains(err.Error(), "不允许") {
			t.Errorf("upstream %q 错误信息应为中文拒绝说明，得到 %v", raw, err)
		}
	}
}

func TestValidateAPIRouteUpstreamAllowsPublic(t *testing.T) {
	// 公网 IP 字面量
	if err := ValidateAPIRouteUpstream("https://203.0.113.10:8443/v1/metrics"); err != nil {
		t.Errorf("公网 IP 字面量应通过：%v", err)
	}
	// 公网域名（注册时不强制解析，解析失败不阻断）
	if err := ValidateAPIRouteUpstream("https://stats.io.zerone.example/v1/metrics"); err != nil {
		t.Errorf("公网域名应通过：%v", err)
	}
}

func TestResolveAndValidateUpstreamRejectsLiteralIPs(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080/v1",
		"http://169.254.169.254/latest/meta-data",
		"http://10.20.30.40/v1",
		"http://192.168.3.4/v1",
		"http://[::1]/v1",
	} {
		if err := ResolveAndValidateUpstream(raw); err == nil {
			t.Errorf("转发前校验必须拒绝 %q", raw)
		}
	}
}

func TestValidateExtensionManifestRejectsDisallowedUpstream(t *testing.T) {
	// 严格校验（注册）层：upstream 指向 127.0.0.1 必须被拒绝
	for _, upstream := range []string{
		"http://127.0.0.1:8080/v1/metrics",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1/api",
		"http://192.168.0.1/api",
		"http://[::1]/api",
	} {
		raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.evil","version":"1.0.0",` +
			`"displayName":"x","description":"y",` +
			`"apiRoutes":[{"method":"GET","path":"/api/v1/extensions/io.zerone.evil/metrics","upstream":"` + upstream + `"}]}`
		_, errs := ValidateExtensionManifest([]byte(raw))
		if len(errs) == 0 {
			t.Errorf("注册必须拒绝内网 upstream %q", upstream)
			continue
		}
		joined := strings.Join(errs, "；")
		if !strings.Contains(joined, "upstream") {
			t.Errorf("期望 upstream 中文错误，得到 %v", errs)
		}
	}
}

func TestValidateExtensionManifestAllowsPublicUpstream(t *testing.T) {
	raw := `{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.ok","version":"1.0.0",` +
		`"displayName":"x","description":"y",` +
		`"apiRoutes":[{"method":"GET","path":"/api/v1/extensions/io.zerone.ok/metrics","upstream":"https://api.example.com/v1/metrics"}]}`
	if _, errs := ValidateExtensionManifest([]byte(raw)); len(errs) > 0 {
		t.Fatalf("公网域名 upstream 应通过注册：%v", errs)
	}
}
