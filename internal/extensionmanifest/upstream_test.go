// H7 upstream SSRF 防护回归测试：声明时拒绝内网/环回/链路本地字面量，
// 公网域名放行；转发前解析校验同样拒绝。
package extensionmanifest

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
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
		// 曾经漏拦的类别（见 disallowedIPReason 的白名单语义）
		"http://0.0.0.0:8080/api",
		"http://[::]:8080/api",
		"http://100.100.100.200/latest/meta-data",
		"http://[::1%25lo0]:8080/api",
		"http://[fe80::1%25eth0]/api",
		"http://[64:ff9b::a9fe:a9fe]/api",
		"http://[2002:a9fe:a9fe::1]/api",
		"http://224.0.0.1/api",
		"http://255.255.255.255/api",
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
	if err := ValidateAPIRouteUpstream("https://93.184.216.34:8443/v1/metrics"); err != nil {
		t.Errorf("公网 IP 字面量应通过：%v", err)
	}
	// 公网域名（注册时不强制解析，解析失败不阻断）
	if err := ValidateAPIRouteUpstream("https://stats.io.zerone.example/v1/metrics"); err != nil {
		t.Errorf("公网域名应通过：%v", err)
	}
}

// TestDisallowedIPReasonCoversPreviouslyMissedRanges 锁定曾经漏掉的地址类别。
// 这些地址此前全部 blocked=false（0.0.0.0 更是实测能打通本机监听）。
func TestDisallowedIPReasonCoversPreviouslyMissedRanges(t *testing.T) {
	// 0.0.0.0 就是其中之一：拨号到 0.0.0.0:port 等价于本机回环。
	cases := []struct{ addr, want string }{
		{"0.0.0.0", "未指定地址"},
		{"::", "未指定地址"},
		{"100.100.100.200", "保留地址"}, // 阿里云元数据（CGNAT 段）
		{"100.64.0.1", "保留地址"},      // CGNAT
		{"224.0.0.1", "链路本地组播地址"},   // 224.0.0.0/24 属链路本地组播
		{"239.1.1.1", "组播地址"},
		{"255.255.255.255", "保留地址"},
		{"240.0.0.1", "保留地址"},
		{"192.0.0.1", "保留地址"},
		{"198.18.0.1", "保留地址"},
		{"192.0.2.1", "保留地址"},
		{"198.51.100.1", "保留地址"},
		{"203.0.113.1", "保留地址"},
		{"64:ff9b::a9fe:a9fe", "保留地址"}, // NAT64 封装的 169.254.169.254
		{"2002:a9fe:a9fe::1", "保留地址"},  // 6to4 封装的 169.254.169.254
		{"2001::1", "保留地址"},            // Teredo
		{"2001:db8::1", "保留地址"},        // 文档示例段
		{"ff02::1", "链路本地组播地址"},
		{"ff05::1", "组播地址"},
	}
	for _, c := range cases {
		ip := netip.MustParseAddr(c.addr)
		if got := disallowedIPReason(ip); got != c.want {
			t.Errorf("%s 应被拒绝（%s），实得 %q", c.addr, c.want, got)
		}
	}
}

// TestDisallowedIPReasonRejectsIPv6Zone 锁定 zone 绕过：
// netip.Prefix.Contains 对带 zone 的地址恒返回 false，且 Unmap() 不剥离
// zone，因此必须先按 zone 显式拒绝，否则环回/链路本地/ULA 全部漏拦。
func TestDisallowedIPReasonRejectsIPv6Zone(t *testing.T) {
	for _, raw := range []string{"::1%lo0", "fe80::1%eth0", "fd00::1%eth0", "fc00::1%eth0"} {
		ip := netip.MustParseAddr(raw)
		if reason := disallowedIPReason(ip); reason != "带 zone 的地址" {
			t.Errorf("%s 必须因带 zone 被拒绝，实得 %q", raw, reason)
		}
	}
}

// TestDisallowedIPReasonAllowsPublic 确认白名单没有误伤正常公网地址。
func TestDisallowedIPReasonAllowsPublic(t *testing.T) {
	for _, raw := range []string{"93.184.216.34", "8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		ip := netip.MustParseAddr(raw)
		if reason := disallowedIPReason(ip); reason != "" {
			t.Errorf("%s 是公网地址，应放行，实得 %q", raw, reason)
		}
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
	ips, err := ValidateAndResolveIPs("https://93.184.216.34:8443/v1/metrics")
	if err != nil {
		t.Fatalf("公网 IP 字面量应通过：%v", err)
	}
	if len(ips) != 1 || ips[0].String() != "93.184.216.34" {
		t.Fatalf("应返回字面量 IP 93.184.216.34，得到 %v", ips)
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
	t.Cleanup(TestOnlyAllowPrivateUpstream())
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

// httptestPort 提取 httptest 服务监听的端口。请求 URL 的 host 会被换成
// 受信域名（DialContext 只认已校验 IP，host 仅用于 TLS ServerName），
// 所以这里只需要端口。
func httptestPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("无法解析 %q 的监听端口：%v", srv.URL, err)
	}
	return port
}

// TestSecureHTTPClientPinsIPs 验证拨号被固定到校验过的 IP（DNS 不参与）：
// 用 example.invalid 这类不可解析域名，请求仍成功——说明根本没走 DNS。
func TestSecureHTTPClientPinsIPs(t *testing.T) {
	// 用 httptest 承担 HTTP 分帧，而不是手写裸 TCP 响应：手写版在对端提前
	// 关闭连接时可能让 transport 静默重试，把第二个响应写到已经空闲的连接
	// 上，触发 "Unsolicited response received on idle HTTP channel" 偶发失败。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

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

	resp, err := client.Get("http://example.invalid:" + httptestPort(t, srv) + "/x")
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
// 跳转目标是一个真实存在的监听端口：如果客户端跟随了跳转，redirectHit
// 必然被置为 true，断言才能真正证伪（此前该变量未被赋值，是死断言）。
func TestSecureHTTPClientRejectsRedirects(t *testing.T) {
	var redirectHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectHit.Store(true)
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()

	// 同上：跳转目标用真实存在的 httptest 服务，跟随了就会被观测到。
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/internal")
		w.WriteHeader(http.StatusFound)
	}))
	defer source.Close()

	pinned := netip.MustParseAddr("127.0.0.1")
	client := SecureHTTPClient([]netip.Addr{pinned}, "example.invalid", 3*time.Second)
	resp, err := client.Get("http://example.invalid:" + httptestPort(t, source) + "/x")
	if err != nil {
		t.Fatalf("请求失败：%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("3xx 必须原样透传（不跟随），得到 %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != target.URL+"/internal" {
		t.Fatalf("Location 头应保留，得到 %q", loc)
	}
	if redirectHit.Load() {
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
		"http://0.0.0.0:8080/api",
		"http://100.100.100.200/latest/meta-data",
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
