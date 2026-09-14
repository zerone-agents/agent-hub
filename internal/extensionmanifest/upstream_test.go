// H7 upstream SSRF 防护回归测试：声明时拒绝内网/环回/链路本地字面量，
// 公网域名放行；转发前解析校验同样拒绝。
package extensionmanifest

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
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

// TestValidateAndResolveIPs 返回校验通过的 IP 列表，供 SecureHTTPClient
// 固定拨号目标（TOCTOU 免疫）。
func TestValidateAndResolveIPs(t *testing.T) {
	// 公网 IP 字面量：原样返回
	ips, err := ValidateAndResolveIPs("https://203.0.113.10:8443/v1/metrics")
	if err != nil {
		t.Fatalf("公网 IP 字面量应通过：%v", err)
	}
	if len(ips) != 1 || ips[0].String() != "203.0.113.10" {
		t.Fatalf("应返回字面量 IP 203.0.113.10，得到 %v", ips)
	}

	// 内网字面量 / 环回域名：拒绝且不返回 IP
	if _, err := ValidateAndResolveIPs("http://10.0.0.8/api"); err == nil {
		t.Fatal("内网字面量必须被拒绝")
	}
	if _, err := ValidateAndResolveIPs("http://localhost:8080/api"); err == nil {
		t.Fatal("localhost 解析到环回地址，必须被拒绝")
	}
	if _, err := ValidateAndResolveIPs("ftp://example.com/x"); err == nil {
		t.Fatal("非 http(s) scheme 必须被拒绝")
	}

	// 测试开关下：域名解析结果（环回）原样返回，不阻断
	UpstreamGuardDisabled = true
	t.Cleanup(func() { UpstreamGuardDisabled = false })
	ips, err = ValidateAndResolveIPs("http://localhost:1/v1")
	if err != nil {
		t.Fatalf("测试开关下 localhost 应解析成功：%v", err)
	}
	if len(ips) == 0 {
		t.Fatal("应返回非空 IP 列表")
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			t.Fatalf("localhost 应解析到环回地址，得到 %v", ip)
		}
	}
}

// TestSecureHTTPClientPinsIPs 验证拨号被固定到校验过的 IP（DNS 不参与）：
// 用 example.invalid 这类不可解析域名，请求仍成功——说明根本没走 DNS。
func TestSecureHTTPClientPinsIPs(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听回环端口失败：%v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"))
			_ = conn.Close()
		}
	}()
	pinned := netip.MustParseAddr("127.0.0.1")
	client := SecureHTTPClient([]netip.Addr{pinned}, "example.invalid", 3*time.Second)

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 应为 *http.Transport，得到 %T", client.Transport)
	}
	if tr.DialContext == nil {
		t.Fatal("Transport 必须自定义 DialContext 以固定拨号 IP")
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("TLS 必须校验证书（InsecureSkipVerify=false）")
	}
	if tr.TLSClientConfig.ServerName != "example.invalid" {
		t.Fatalf("TLS ServerName 必须保持原始域名，得到 %q", tr.TLSClientConfig.ServerName)
	}

	resp, err := client.Get("http://example.invalid:" + fmt.Sprint(ln.Addr().(*net.TCPAddr).Port) + "/x")
	if err != nil {
		t.Fatalf("拨号应固定到已校验 IP 且不依赖 DNS：%v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("响应体应为 ok，得到 %q", body)
	}
}

// TestSecureHTTPClientRejectsRedirects 验证 3xx 原样返回、不跟随跳转。
func TestSecureHTTPClientRejectsRedirects(t *testing.T) {
	redirectHit := false
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听回环端口失败：%v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:9/internal\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			_ = conn.Close()
		}
	}()
	pinned := netip.MustParseAddr("127.0.0.1")
	client := SecureHTTPClient([]netip.Addr{pinned}, "example.invalid", 3*time.Second)
	resp, err := client.Get("http://example.invalid:" + fmt.Sprint(ln.Addr().(*net.TCPAddr).Port) + "/x")
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("3xx 必须原样透传（不跟随），得到 %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "http://127.0.0.1:9/internal" {
		t.Fatalf("Location 头应保留，得到 %q", loc)
	}
	if redirectHit {
		t.Fatal("绝不能访问跳转目标")
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
