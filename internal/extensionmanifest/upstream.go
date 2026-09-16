// H7.0 扩展 apiRoutes.upstream 的 SSRF 防护。
//
// 声明时（严格校验）只允许 http(s) URL；若 host 是 IP 字面量则直接拒绝
// 环回 / RFC1918 私网 / 链路本地地址。域名无法解析时不阻断注册（避免
// 误伤合法但暂时不可解析的域名），但代理转发前必须重新解析并校验全部
// A/AAAA 记录，防 DNS rebinding。
package extensionmanifest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// upstreamGuardDisabled 仅供测试使用：临时关闭 upstream 私网地址阻断
// （如 httptest 回环上游）。生产代码不得置位。
//
// 用 atomic.Bool 而非导出布尔量：测试与其它用例并发时会读写它，
// 且成对恢复的 API 形态比裸全局变量更不容易被误用后泄漏状态。
var upstreamGuardDisabled atomic.Bool

// TestOnlyAllowPrivateUpstream 临时关闭 upstream 地址阻断，返回恢复函数。
// 仅供测试使用，调用方必须 defer restore()；生产代码不得调用。
func TestOnlyAllowPrivateUpstream() (restore func()) {
	upstreamGuardDisabled.Store(true)
	return func() { upstreamGuardDisabled.Store(false) }
}

func upstreamGuardEnabled() bool { return !upstreamGuardDisabled.Load() }

// disallowedUpstreamPrefixes 是除标准库类别谓词之外、额外禁止的
// 特殊用途 / 保留地址段。
//
// 判定语义是白名单（见 disallowedIPReason）：先由 netip 的类别谓词排除
// 环回 / 链路本地 / 组播 / 私有 / 未指定，再叠加本表，最后要求剩余地址
// 必须是全局单播。这样"新增保留段时忘了加进黑名单"不会直接变成漏洞——
// 历史上 0.0.0.0、CGNAT(100.64/10)、NAT64/6to4/Teredo 都是因为只做黑名单枚举而漏过的。
var disallowedUpstreamPrefixes = []netip.Prefix{
	// ---- IPv4 特殊用途（RFC 6890 / 5737 / 6598 / 1112）----
	netip.MustParsePrefix("0.0.0.0/8"),       // 本网络；0.0.0.0 拨号等价于本机
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT；含阿里云元数据 100.100.100.200
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF 协议专用
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // 网络设备基准测试
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // 保留；含 255.255.255.255 广播
	// ---- IPv6 特殊用途 ----
	netip.MustParsePrefix("::/128"),        // 未指定
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64；可封装内网 IPv4
	netip.MustParsePrefix("2001::/32"),     // Teredo；可封装内网 IPv4
	netip.MustParsePrefix("2002::/16"),     // 6to4；可封装内网 IPv4
	netip.MustParsePrefix("2001:db8::/32"), // 文档示例
}

// disallowedIPReason 返回 IP 被禁止的原因（空串表示允许）。
// 白名单语义：只有确认为"全局单播且不在保留段内"的地址才放行。
func disallowedIPReason(ip netip.Addr) string {
	if !ip.IsValid() {
		return "无效地址"
	}
	// 带 zone 的地址一律拒绝。netip.Prefix.Contains 对带 zone 的地址恒返回
	// false，且 Unmap() 不剥离 zone——先 Unmap 再比对整表会全部漏过
	// （::1%lo0 / fe80::1%eth0 / fd00::1%eth0 曾因此绕过环回与链路本地拦截）。
	if ip.Zone() != "" {
		return "带 zone 的地址"
	}
	ip = ip.Unmap()
	switch {
	case ip.IsUnspecified():
		return "未指定地址"
	case ip.IsLoopback():
		return "环回地址"
	case ip.IsLinkLocalUnicast():
		return "链路本地地址"
	case ip.IsInterfaceLocalMulticast():
		return "接口本地组播地址"
	case ip.IsLinkLocalMulticast():
		return "链路本地组播地址"
	case ip.IsMulticast():
		return "组播地址"
	case ip.IsPrivate():
		return "私有地址"
	}
	for _, p := range disallowedUpstreamPrefixes {
		if p.Contains(ip) {
			return "保留地址"
		}
	}
	if !ip.IsGlobalUnicast() {
		return "非全局单播地址"
	}
	return ""
}

// ValidateUpstreamIPLiteral 校验 host 为 IP 字面量时是否被禁止；
// 非 IP 字面量（域名）一律放行（解析校验在转发前由
// ResolveAndValidateUpstream 完成）。
func ValidateUpstreamIPLiteral(host string) error {
	if !upstreamGuardEnabled() {
		return nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("缺少主机名")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if reason := disallowedIPReason(ip); reason != "" {
			return fmt.Errorf("upstream 不允许指向%s %s", reason, ip.String())
		}
	}
	return nil
}

// ValidateAPIRouteUpstream 在声明（注册）时校验 upstream：必须是合法
// http(s) URL 且 host 为 IP 字面量时不得指向内网/环回/链路本地地址。
// 域名解析失败不阻断注册，但字面量必须当场拒绝。
func ValidateAPIRouteUpstream(raw string) error {
	up, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || (up.Scheme != "http" && up.Scheme != "https") || up.Host == "" {
		return fmt.Errorf("upstream %q 必须是合法的 http(s) URL", raw)
	}
	return ValidateUpstreamIPLiteral(up.Hostname())
}

// ResolveAndValidateUpstream 在代理转发前解析 upstream 的域名并校验全部
// 解析结果，拒绝任一命中内网/环回/链路本地段的地址（防 DNS rebinding）。
func ResolveAndValidateUpstream(raw string) error {
	_, err := ValidateAndResolveIPs(raw)
	return err
}

// ValidateAndResolveIPs 解析 upstream、解析出全部 IP 并逐个校验，返回
// 校验通过的 IP 列表。调用方应把返回的 IP 固定为实际拨号目标
// （见 SecureHTTPClient），消除"校验后重新解析"的 TOCTOU 窗口。
// UpstreamGuardDisabled（测试）时跳过地址段校验但仍返回解析结果。
func ValidateAndResolveIPs(raw string) ([]netip.Addr, error) {
	up, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || (up.Scheme != "http" && up.Scheme != "https") || up.Host == "" {
		return nil, fmt.Errorf("upstream %q 必须是合法的 http(s) URL", raw)
	}
	host := up.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		if upstreamGuardEnabled() {
			if reason := disallowedIPReason(ip); reason != "" {
				return nil, fmt.Errorf("upstream 不允许指向%s %s", reason, ip.String())
			}
		}
		return []netip.Addr{ip.Unmap()}, nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil, fmt.Errorf("upstream 域名 %q 无法解析，拒绝转发", host)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("upstream 域名 %q 没有可用解析记录，拒绝转发", host)
	}
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ipa := range ips {
		addr, ok := netip.AddrFromSlice(ipa.IP)
		if !ok {
			return nil, fmt.Errorf("upstream 域名 %q 解析出无法识别的地址，拒绝转发", host)
		}
		addr = addr.Unmap()
		if upstreamGuardEnabled() {
			if reason := disallowedIPReason(addr); reason != "" {
				return nil, fmt.Errorf("upstream 域名 %q 解析到%s %s，拒绝转发", host, reason, addr.String())
			}
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// SecureHTTPClient 返回一个把拨号目标固定为 ips 之一的 HTTP 客户端：
//   - Transport.DialContext 忽略 DNS，逐个尝试拨号已校验 IP（TOCTOU 免疫）；
//   - HTTPS 的 TLS ServerName 保持原始域名，证书校验不跳过；
//   - CheckRedirect 拒绝跟随跳转（http.ErrUseLastResponse），3xx 原样透传，
//     由调用方决定是否处理（跟随跳转需重新校验 Location，不能默认开启）。
func SecureHTTPClient(ips []netip.Addr, host string, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil || port == "" {
				port = "80"
			}
			// 忽略 addr 中的主机名/IP：只连接校验过的 IP，DNS 不再参与。
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("没有可用的已校验 IP")
			}
			return nil, lastErr
		},
		TLSClientConfig:     &tls.Config{ServerName: host, InsecureSkipVerify: false},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
