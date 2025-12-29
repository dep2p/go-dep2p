// Package address 提供地址管理模块的实现
package address

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidAddress 无效地址
	ErrInvalidAddress = errors.New("invalid address")

	// ErrUnsupportedProtocol 不支持的协议
	ErrUnsupportedProtocol = errors.New("unsupported protocol")

	// ErrMalformedMultiaddr 格式错误的多地址
	ErrMalformedMultiaddr = errors.New("malformed multiaddr")

	// ErrNotMultiaddrFormat 不是 multiaddr 格式
	// 根据 IMPL-ADDRESS-UNIFICATION.md：拨号层禁止 host:port，
	// host:port 格式应在 CLI/UI 边界层使用 types.FromHostPort 转换后再进入 core。
	ErrNotMultiaddrFormat = errors.New("not multiaddr format: address must start with '/', use types.FromHostPort for host:port conversion")
)

// ============================================================================
//                              AddressParser 实现
// ============================================================================

// Parser 地址解析器
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，仅支持解析 multiaddr 格式：
//   - /ip4/192.168.1.1/tcp/8000
//   - /ip4/192.168.1.1/udp/8000/quic-v1
//   - /ip6/::1/udp/8000/quic-v1
//   - /dns4/example.com/udp/8000/quic-v1
//   - /p2p/QmPeer/p2p-circuit/p2p/QmDest
//
// host:port 格式（如 "192.168.1.1:8000"）不再支持。
// 如需从 host:port 创建地址，请在 CLI/UI 边界层使用 types.FromHostPort 转换。
type Parser struct{}

// NewParser 创建地址解析器
func NewParser() *Parser {
	return &Parser{}
}

// Parse 解析地址字符串
//
// 仅接受 multiaddr 格式（以 "/" 开头）。
// host:port 格式应在 CLI/UI 边界层使用 types.FromHostPort 转换后再进入 core。
//
// 示例：
//   - "/ip4/1.2.3.4/udp/4001/quic-v1" → 成功
//   - "/dns4/example.com/tcp/8000" → 成功
//   - "192.168.1.1:8000" → 错误（ErrNotMultiaddrFormat）
func (p *Parser) Parse(s string) (endpoint.Address, error) {
	if s == "" {
		return nil, ErrInvalidAddress
	}

	s = strings.TrimSpace(s)

	// 根据 IMPL-ADDRESS-UNIFICATION.md：拨号层禁止 host:port
	// 必须是 multiaddr 格式（以 "/" 开头）
	if !strings.HasPrefix(s, "/") {
		return nil, fmt.Errorf("%w: got %q", ErrNotMultiaddrFormat, s)
	}

		return p.parseMultiaddr(s)
}

// ParseMultiple 解析多个地址
func (p *Parser) ParseMultiple(ss []string) ([]endpoint.Address, error) {
	result := make([]endpoint.Address, 0, len(ss))
	for _, s := range ss {
		addr, err := p.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address %q: %w", s, err)
		}
		result = append(result, addr)
	}
	return result, nil
}

// ============================================================================
//                              Multiaddr 解析
// ============================================================================

// parseMultiaddr 解析 Multiaddr 格式地址
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，返回统一的 *Addr 类型（而非旧的 *ParsedAddress）。
//
// 支持格式：
// - /ip4/1.2.3.4/tcp/8000
// - /ip4/1.2.3.4/udp/8000/quic-v1
// - /ip6/::1/udp/8000/quic-v1
// - /dns4/example.com/udp/8000/quic-v1
// - /p2p/QmPeer/p2p-circuit/p2p/QmDest
func (p *Parser) parseMultiaddr(s string) (endpoint.Address, error) {
	// 使用统一的 Addr 类型（基于 types.Multiaddr）
	// 这里委托给 Parse 函数，它已经实现了正确的验证逻辑
	return Parse(s)
}

// ============================================================================
//                              Host:Port 解析（已废弃）
// ============================================================================

// parseHostPort 解析 Host:Port 格式地址
//
// Deprecated: 根据 IMPL-ADDRESS-UNIFICATION.md，Parser.Parse() 不再支持 host:port 格式。
// 此方法仅保留用于内部迁移测试，未来版本将删除。
// 如需从 host:port 创建地址，请使用 types.FromHostPort。
func (p *Parser) parseHostPort(s string) (endpoint.Address, error) {
	addr := &ParsedAddress{
		raw:        s,
		components: make(map[string]string),
	}

	// 尝试分离 host 和 port
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// 可能没有端口
		host = s
		portStr = ""
	}

	if host != "" {
		// 尝试解析为 IP
		ip := net.ParseIP(host)
		if ip != nil {
			addr.ip = ip
			if ip.To4() != nil {
				addr.network = "ip4"
			} else {
				addr.network = "ip6"
			}
		} else {
			// 可能是域名
			addr.host = host
			addr.network = "dns"
		}
	}

	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 0 || port > 65535 {
			return nil, fmt.Errorf("%w: invalid port %q", ErrInvalidAddress, portStr)
		}
		addr.port = port
	}

	return addr, nil
}

// ============================================================================
//                              ParsedAddress 实现（已废弃）
// ============================================================================

// ParsedAddress 解析后的地址
//
// Deprecated: 根据 IMPL-ADDRESS-UNIFICATION.md 规范，此类型已被 Addr 替代。
// 新代码应使用 internal/core/address.Addr（基于 types.Multiaddr）。
// 此类型仅保留用于向后兼容，未来版本将删除。
type ParsedAddress struct {
	raw        string
	ip         net.IP
	host       string
	port       int
	network    string
	transport  string
	peerID     string
	isRelay    bool
	components map[string]string
}

// Network 返回网络类型
func (a *ParsedAddress) Network() string {
	if a.network != "" {
		return a.network
	}
	return "unknown"
}

// String 返回地址字符串
func (a *ParsedAddress) String() string {
	return a.raw
}

// Bytes 返回地址字节
func (a *ParsedAddress) Bytes() []byte {
	return []byte(a.raw)
}

// Equal 比较地址是否相等
func (a *ParsedAddress) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.raw == other.String()
}

// IsPublic 是否是公网地址
func (a *ParsedAddress) IsPublic() bool {
	if a.ip == nil {
		return false
	}
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !a.ip.IsLoopback() &&
		!a.ip.IsPrivate() &&
		!a.ip.IsUnspecified() &&
		!a.ip.IsLinkLocalUnicast() &&
		!a.ip.IsLinkLocalMulticast()
}

// IsPrivate 是否是私有地址
func (a *ParsedAddress) IsPrivate() bool {
	if a.ip == nil {
		return false
	}
	return a.ip.IsPrivate()
}

// IsLoopback 是否是回环地址
func (a *ParsedAddress) IsLoopback() bool {
	if a.ip == nil {
		return strings.Contains(a.raw, "127.0.0.1") || strings.Contains(a.raw, "localhost")
	}
	return a.ip.IsLoopback()
}

// Multiaddr 返回 multiaddr 格式
func (a *ParsedAddress) Multiaddr() string {
	// 如果原始地址已是 multiaddr 格式
	if strings.HasPrefix(a.raw, "/") {
		return a.raw
	}
	// 构建 multiaddr
	ipType := "ip4"
	if a.ip != nil && a.ip.To4() == nil {
		ipType = "ip6"
	}
	if a.port > 0 {
		transport := a.transport
		if transport == "" {
			transport = "udp"
		}
		return fmt.Sprintf("/%s/%s/%s/%d", ipType, a.host, transport, a.port)
	}
	return fmt.Sprintf("/%s/%s", ipType, a.host)
}

// IP 返回 IP 地址
func (a *ParsedAddress) IP() net.IP {
	return a.ip
}

// Host 返回主机名
func (a *ParsedAddress) Host() string {
	return a.host
}

// Port 返回端口
func (a *ParsedAddress) Port() int {
	return a.port
}

// Transport 返回传输协议
func (a *ParsedAddress) Transport() string {
	return a.transport
}

// PeerID 返回节点 ID
func (a *ParsedAddress) PeerID() string {
	return a.peerID
}

// IsRelay 是否是中继地址
func (a *ParsedAddress) IsRelay() bool {
	return a.isRelay
}

// ToMultiaddr 转换为 Multiaddr 格式
func (a *ParsedAddress) ToMultiaddr() string {
	if strings.HasPrefix(a.raw, "/") {
		return a.raw
	}

	var parts []string

	// IP 或 DNS
	if a.ip != nil {
		if a.ip.To4() != nil {
			parts = append(parts, "ip4", a.ip.String())
		} else {
			parts = append(parts, "ip6", a.ip.String())
		}
	} else if a.host != "" {
		parts = append(parts, "dns4", a.host)
	}

	// 传输协议和端口
	if a.port > 0 {
		transport := a.transport
		if transport == "" {
			transport = "tcp"
		}
		parts = append(parts, transport, strconv.Itoa(a.port))
	}

	// QUIC
	if a.transport == "quic" {
		parts = append(parts, "quic-v1")
	}

	// PeerID
	if a.peerID != "" {
		parts = append(parts, "p2p", a.peerID)
	}

	return "/" + strings.Join(parts, "/")
}

// Resolve 解析域名为 IP 地址
func (a *ParsedAddress) Resolve() ([]endpoint.Address, error) {
	if a.ip != nil {
		return []endpoint.Address{a}, nil
	}

	if a.host == "" {
		return nil, errors.New("no host to resolve")
	}

	ips, err := net.LookupIP(a.host)
	if err != nil {
		return nil, err
	}

	result := make([]endpoint.Address, 0, len(ips))
	for _, ip := range ips {
		resolved := &ParsedAddress{
			ip:        ip,
			port:      a.port,
			transport: a.transport,
			peerID:    a.peerID,
			isRelay:   a.isRelay,
		}
		if ip.To4() != nil {
			resolved.network = "ip4"
		} else {
			resolved.network = "ip6"
		}
		resolved.raw = resolved.ToMultiaddr()
		result = append(result, resolved)
	}

	return result, nil
}

// ============================================================================
//                              RelayAddress 实现
// ============================================================================

// RelayAddr 中继地址
type RelayAddr struct {
	relayID types.NodeID
	destID  types.NodeID
	raw     string
}

// NewRelayAddr 创建中继地址
func NewRelayAddr(relayID, destID types.NodeID) *RelayAddr {
	return &RelayAddr{
		relayID: relayID,
		destID:  destID,
		raw:     fmt.Sprintf("/p2p/%s/p2p-circuit/p2p/%s", relayID.String(), destID.String()),
	}
}

// Network 返回网络类型
func (a *RelayAddr) Network() string {
	return "p2p-circuit"
}

func (a *RelayAddr) String() string {
	return a.raw
}

// Bytes 返回地址的字节表示
func (a *RelayAddr) Bytes() []byte {
	return []byte(a.raw)
}

// Equal 检查两个地址是否相等
func (a *RelayAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.raw == other.String()
}

// IsPublic 返回是否为公网地址
func (a *RelayAddr) IsPublic() bool {
	return false
}

// IsPrivate 返回是否为私有地址
func (a *RelayAddr) IsPrivate() bool {
	return false
}

// IsLoopback 返回是否为回环地址
func (a *RelayAddr) IsLoopback() bool {
	return false
}

// RelayID 返回中继节点 ID
func (a *RelayAddr) RelayID() types.NodeID {
	return a.relayID
}

// DestID 返回目标节点 ID
func (a *RelayAddr) DestID() types.NodeID {
	return a.destID
}
