// H7.0 扩展 apiRoutes.upstream 的 SSRF 防护。
//
// 声明时（严格校验）只允许 http(s) URL；若 host 是 IP 字面量则直接拒绝
// 环回 / RFC1918 私网 / 链路本地地址。域名无法解析时不阻断注册（避免
// 误伤合法但暂时不可解析的域名），但代理转发前必须重新解析并校验全部
// A/AAAA 记录，防 DNS rebinding。
package extensionmanifest

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// UpstreamGuardDisabled 仅供测试使用：临时关闭 upstream 私网地址阻断
// （如 httptest 回环上游）。生产代码不得置位。
var UpstreamGuardDisabled = false

// disallowedUpstreamPrefixes 是 upstream 禁止指向的地址段：
// 127.0.0.0/8、10.0.0.0/8、172.16.0.0/12、192.168.0.0/16、169.254.0.0/16、
// ::1/128、fc00::/7（ULA）、fe80::/10（链路本地）。
var disallowedUpstreamPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// disallowedIPReason 返回 IP 被禁止的原因（空串表示允许）。
func disallowedIPReason(ip netip.Addr) string {
	ip = ip.Unmap()
	for _, p := range disallowedUpstreamPrefixes {
		if p.Contains(ip) {
			switch {
			case ip.IsLoopback():
				return "环回地址"
			case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
				return "链路本地地址"
			case ip.IsPrivate():
				return "私有地址"
			default:
				return "保留地址"
			}
		}
	}
	return ""
}

// ValidateUpstreamIPLiteral 校验 host 为 IP 字面量时是否被禁止；
// 非 IP 字面量（域名）一律放行（解析校验在转发前由
// ResolveAndValidateUpstream 完成）。
func ValidateUpstreamIPLiteral(host string) error {
	if UpstreamGuardDisabled {
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
	if UpstreamGuardDisabled {
		return nil
	}
	up, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || (up.Scheme != "http" && up.Scheme != "https") || up.Host == "" {
		return fmt.Errorf("upstream %q 必须是合法的 http(s) URL", raw)
	}
	host := up.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		if reason := disallowedIPReason(ip); reason != "" {
			return fmt.Errorf("upstream 不允许指向%s %s", reason, ip.String())
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return fmt.Errorf("upstream 域名 %q 无法解析，拒绝转发", host)
	}
	if len(ips) == 0 {
		return fmt.Errorf("upstream 域名 %q 没有可用解析记录，拒绝转发", host)
	}
	for _, ipa := range ips {
		addr, ok := netip.AddrFromSlice(ipa.IP)
		if !ok {
			return fmt.Errorf("upstream 域名 %q 解析出无法识别的地址，拒绝转发", host)
		}
		if reason := disallowedIPReason(addr); reason != "" {
			return fmt.Errorf("upstream 域名 %q 解析到%s %s，拒绝转发", host, reason, addr.String())
		}
	}
	return nil
}
