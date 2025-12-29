// Package addrutil 提供地址解析工具
package addrutil

import (
	"net"
	"strings"
)

// ============================================================================
//                              IP 类型判断工具
// ============================================================================

// IsLoopbackAddr 判断 multiaddr 字符串是否是回环地址
//
// 支持格式：
//   - /ip4/127.0.0.1/...
//   - /ip6/::1/...
//   - /ip4/127.x.x.x/...
func IsLoopbackAddr(addr string) bool {
	ip := extractIPFromMultiaddr(addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// IsPrivateAddr 判断 multiaddr 字符串是否是私网地址
//
// 私网地址范围：
//   - 10.0.0.0/8
//   - 172.16.0.0/12
//   - 192.168.0.0/16
//   - fc00::/7 (IPv6 ULA)
//   - fe80::/10 (IPv6 链路本地)
func IsPrivateAddr(addr string) bool {
	ip := extractIPFromMultiaddr(addr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// IsPublicAddr 判断 multiaddr 字符串是否是公网地址
//
// 公网地址：非回环、非私网、非链路本地的有效单播地址
func IsPublicAddr(addr string) bool {
	ip := extractIPFromMultiaddr(addr)
	if ip == nil {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback()
}

// extractIPFromMultiaddr 从地址字符串中提取 IP 地址
//
// 支持格式：
//   - /ip4/<ip>/... (multiaddr)
//   - /ip6/<ip>/... (multiaddr)
//   - /dns4/<domain>/... (返回 nil，无法判断)
//   - host:port (传统格式，如 192.168.1.1:4001)
//   - [ipv6]:port (传统 IPv6 格式)
func extractIPFromMultiaddr(addr string) net.IP {
	if addr == "" {
		return nil
	}

	// 检查是否是 multiaddr 格式（以 / 开头）
	if strings.HasPrefix(addr, "/") {
		parts := strings.Split(addr, "/")
		for i, part := range parts {
			switch part {
			case "ip4", "ip6":
				if i+1 < len(parts) {
					ip := net.ParseIP(parts[i+1])
					return ip
				}
			case "dns4", "dns6", "dnsaddr":
				// DNS 地址无法直接判断 IP 类型
				return nil
			}
		}
		return nil
	}

	// 尝试解析传统 host:port 格式
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// 可能不是 host:port 格式，尝试直接解析为 IP
		return net.ParseIP(addr)
	}
	return net.ParseIP(host)
}

// ExtractIP 从地址字符串中提取 IP 地址。
//
// 支持格式：
//   - multiaddr: /ip4/<ip>/..., /ip6/<ip>/...
//   - host:port: 1.2.3.4:4001
//   - [ipv6]:port: [::1]:4001
//   - 纯 IP: 1.2.3.4 / ::1
//
// 注意：
// - 对于 /dns4/ /dns6/ /dnsaddr/ 这类无法直接得到 IP 的地址，会返回 nil。
func ExtractIP(addr string) net.IP {
	return extractIPFromMultiaddr(addr)
}

// IsRelayCircuitAddr 判断是否是中继电路地址
//
// 中继地址格式：/ip4/.../p2p/<relay-id>/p2p-circuit/...
func IsRelayCircuitAddr(addr string) bool {
	return strings.Contains(addr, "/p2p-circuit")
}

// AddrType 返回地址类型描述
//
// 返回值：
//   - "loopback" - 回环地址
//   - "private" - 私网地址
//   - "public" - 公网地址
//   - "relay" - 中继地址
//   - "dns" - DNS 地址（无法判断 IP 类型）
//   - "unknown" - 未知类型
func AddrType(addr string) string {
	if addr == "" {
		return "unknown"
	}

	// 优先检查中继地址
	if IsRelayCircuitAddr(addr) {
		return "relay"
	}

	// 检查是否是 DNS 地址
	if strings.Contains(addr, "/dns4/") || strings.Contains(addr, "/dns6/") || strings.Contains(addr, "/dnsaddr/") {
		return "dns"
	}

	// 检查 IP 类型
	if IsLoopbackAddr(addr) {
		return "loopback"
	}
	if IsPrivateAddr(addr) {
		return "private"
	}
	if IsPublicAddr(addr) {
		return "public"
	}

	return "unknown"
}

// ============================================================================
//                              地址格式转换（已废弃）
// ============================================================================

// ToMultiaddr 已废弃
//
// Deprecated: 根据 IMPL-ADDRESS-UNIFICATION.md 规范，地址转换应使用 types.FromHostPort 或 types.ParseMultiaddr。
// 此函数已删除，请使用：
//   - types.ParseMultiaddr(s) 解析 multiaddr 字符串
//   - types.FromHostPort(host, port, transport) 从 host:port 创建 multiaddr
//
// 删除原因：
//   - 默认使用 UDP/QUIC-v1 传输协议会导致歧义
//   - host:port 格式应在 CLI/UI 边界层显式转换，而非自动转换
