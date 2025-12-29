// Package types 提供 DeP2P 核心类型定义
package types

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ============================================================================
//                              Multiaddr - 统一地址类型
// ============================================================================

// Multiaddr 统一地址类型（值对象）
//
// Multiaddr 是 DeP2P 内部唯一的地址表示形式。
// 所有用于拨号/缓存/签名/地址簿/路由的地址必须是 Multiaddr 类型。
//
// 约束：
//   - String() 必须始终返回 canonical multiaddr（以 "/" 开头）
//   - 不实现 netaddr.Address 接口（保持依赖层级正确）
//   - netaddr.Address 的实现放在 internal/core/address
//
// 格式示例：
//   - /ip4/192.168.1.1/udp/4001/quic-v1
//   - /ip6/::1/udp/4001/quic-v1
//   - /dns4/example.com/udp/4001/quic-v1
//   - /ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNodeID
//   - /ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest
type Multiaddr string

// Multiaddr 错误定义
var (
	// ErrInvalidMultiaddr 无效的 multiaddr 格式
	ErrInvalidMultiaddr = errors.New("invalid multiaddr format")

	// ErrEmptyMultiaddr 空 multiaddr
	ErrEmptyMultiaddr = errors.New("empty multiaddr")

	// ErrNotMultiaddrFormat 不是 multiaddr 格式（不以 / 开头）
	ErrNotMultiaddrFormat = errors.New("not multiaddr format: must start with /")

	// ErrMissingTransport 缺少传输协议
	ErrMissingTransport = errors.New("missing transport protocol")

	// ErrInvalidRelayFormat 无效的中继地址格式
	ErrInvalidRelayFormat = errors.New("invalid relay address format")
)

// ============================================================================
//                              解析/构建
// ============================================================================

// ParseMultiaddr 解析并规范化 multiaddr
//
// 仅接受 multiaddr 格式输入（以 "/" 开头）。
// host:port 格式应在 CLI/UI 边界层使用 FromHostPort 转换后再进入 core。
//
// 示例：
//   - "/ip4/1.2.3.4/udp/4001/quic-v1" → Multiaddr
//   - "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmNode" → Multiaddr
//   - "/p2p/QmRelay/p2p-circuit/p2p/QmDest" → Multiaddr
//   - "1.2.3.4:4001" → error（不是 multiaddr 格式）
func ParseMultiaddr(s string) (Multiaddr, error) {
	if s == "" {
		return "", ErrEmptyMultiaddr
	}

	s = strings.TrimSpace(s)

	// 必须以 / 开头
	if !strings.HasPrefix(s, "/") {
		return "", ErrNotMultiaddrFormat
	}

	// 基本格式校验：检查是否包含有效的协议组件
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return "", ErrInvalidMultiaddr
	}

	// 验证第一个组件是有效的网络类型
	firstComponent := parts[1]
	switch firstComponent {
	case "ip4", "ip6", "dns4", "dns6", "dnsaddr", "p2p":
		// 有效的起始组件
	default:
		return "", fmt.Errorf("%w: unknown protocol %q", ErrInvalidMultiaddr, firstComponent)
	}

	return Multiaddr(s), nil
}

// MustParseMultiaddr 解析 multiaddr，失败时 panic
//
// 仅用于常量初始化或测试代码，生产代码应使用 ParseMultiaddr。
func MustParseMultiaddr(s string) Multiaddr {
	ma, err := ParseMultiaddr(s)
	if err != nil {
		panic(fmt.Sprintf("MustParseMultiaddr(%q): %v", s, err))
	}
	return ma
}

// FromHostPort 从 host:port 创建 multiaddr
//
// 仅供 CLI/UI 边界层使用。需要显式指定传输协议（避免默认值导致歧义）。
//
// 参数：
//   - host: IP 地址或域名
//   - port: 端口号
//   - transport: 传输协议，如 "udp/quic-v1", "tcp"
//
// 示例：
//   - FromHostPort("1.2.3.4", 4001, "udp/quic-v1") → "/ip4/1.2.3.4/udp/4001/quic-v1"
//   - FromHostPort("::1", 4001, "udp/quic-v1") → "/ip6/::1/udp/4001/quic-v1"
//   - FromHostPort("example.com", 4001, "udp/quic-v1") → "/dns4/example.com/udp/4001/quic-v1"
func FromHostPort(host string, port int, transport string) (Multiaddr, error) {
	if host == "" {
		return "", errors.New("empty host")
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid port: %d", port)
	}
	if transport == "" {
		return "", ErrMissingTransport
	}

	// 判断 IP 类型
	var networkType string
	ip := net.ParseIP(host)
	if ip == nil {
		// 可能是域名
		networkType = "dns4"
	} else if ip.To4() != nil {
		networkType = "ip4"
	} else {
		networkType = "ip6"
	}

	// 构建 multiaddr
	// transport 可能是 "udp/quic-v1" 或 "tcp" 等
	var addr string
	if strings.Contains(transport, "/") {
		// 复合传输，如 "udp/quic-v1"
		parts := strings.SplitN(transport, "/", 2)
		addr = fmt.Sprintf("/%s/%s/%s/%d/%s", networkType, host, parts[0], port, parts[1])
	} else {
		// 单一传输，如 "tcp"
		addr = fmt.Sprintf("/%s/%s/%s/%d", networkType, host, transport, port)
	}

	return Multiaddr(addr), nil
}

// ============================================================================
//                              访问方法
// ============================================================================

// String 返回 canonical multiaddr 字符串
func (m Multiaddr) String() string {
	return string(m)
}

// HostPort 返回可展示的 host:port 格式
//
// 仅用于 UI/日志展示，不应用于拨号输入。
// 对于 relay 地址或无法提取 IP:Port 的情况，返回空字符串。
func (m Multiaddr) HostPort() string {
	if m.IsEmpty() || m.IsRelay() {
		return ""
	}

	ip := m.IP()
	port := m.Port()
	if ip == nil || port == 0 {
		return ""
	}

	// IPv6 需要用方括号包裹
	if ip.To4() == nil {
		return fmt.Sprintf("[%s]:%d", ip.String(), port)
	}
	return fmt.Sprintf("%s:%d", ip.String(), port)
}

// IP 返回 IP 地址（如果可用）
func (m Multiaddr) IP() net.IP {
	if m.IsEmpty() {
		return nil
	}

	parts := strings.Split(string(m), "/")
	for i := 1; i < len(parts)-1; i++ {
		switch parts[i] {
		case "ip4", "ip6":
			if i+1 < len(parts) {
				return net.ParseIP(parts[i+1])
			}
		}
	}
	return nil
}

// Port 返回端口号（如果可用）
func (m Multiaddr) Port() int {
	if m.IsEmpty() {
		return 0
	}

	parts := strings.Split(string(m), "/")
	for i := 1; i < len(parts)-1; i++ {
		switch parts[i] {
		case "tcp", "udp":
			if i+1 < len(parts) {
				port, err := strconv.Atoi(parts[i+1])
				if err == nil {
					return port
				}
			}
		}
	}
	return 0
}

// PeerID 返回嵌入的 NodeID（如果有 /p2p/<nodeID> 组件）
//
// 对于 relay 地址，返回第一个 /p2p/ 后的 NodeID（即 relay 节点 ID）。
// 如需获取目标节点 ID，请使用 DestID()。
func (m Multiaddr) PeerID() NodeID {
	if m.IsEmpty() {
		return NodeID{}
	}

	parts := strings.Split(string(m), "/")
	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "p2p" && i+1 < len(parts) {
			nodeID, err := ParseNodeID(parts[i+1])
			if err == nil {
				return nodeID
			}
		}
	}
	return NodeID{}
}

// Transport 返回传输协议
//
// 返回值: "quic-v1", "tcp", "udp", "p2p-circuit", ""
func (m Multiaddr) Transport() string {
	if m.IsEmpty() {
		return ""
	}

	// 优先检查 relay
	if m.IsRelay() {
		return "p2p-circuit"
	}

	parts := strings.Split(string(m), "/")
	for i := len(parts) - 1; i >= 1; i-- {
		switch parts[i] {
		case "quic-v1", "quic", "tcp", "udp", "webtransport", "webrtc":
			return parts[i]
		}
	}
	return ""
}

// Network 返回网络类型（等同于 Transport，用于兼容）
func (m Multiaddr) Network() string {
	return m.Transport()
}

// Bytes 返回地址的字节表示
func (m Multiaddr) Bytes() []byte {
	return []byte(m)
}

// ============================================================================
//                              判断方法
// ============================================================================

// IsRelay 是否是中继地址
func (m Multiaddr) IsRelay() bool {
	return strings.Contains(string(m), "/"+RelayAddrProtocol)
}

// IsPublic 是否是公网地址
func (m Multiaddr) IsPublic() bool {
	ip := m.IP()
	if ip == nil {
		return false
	}
	// 排除私网、回环、链路本地等
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast()
}

// IsPrivate 是否是私网地址
func (m Multiaddr) IsPrivate() bool {
	ip := m.IP()
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

// IsLoopback 是否是回环地址
func (m Multiaddr) IsLoopback() bool {
	ip := m.IP()
	if ip == nil {
		// 检查字符串是否包含 localhost
		return strings.Contains(string(m), "127.0.0.1") || strings.Contains(string(m), "localhost")
	}
	return ip.IsLoopback()
}

// IsEmpty 是否为空
func (m Multiaddr) IsEmpty() bool {
	return m == ""
}

// Equal 比较两个 Multiaddr 是否相等
func (m Multiaddr) Equal(other Multiaddr) bool {
	return m == other
}

// ============================================================================
//                              Relay 地址操作
// ============================================================================

// BuildRelayAddr 构建中继地址（dialable，唯一规范格式）
//
// 参数：
//   - relayAddr: 中继节点地址（必须是包含 /p2p/<relayID> 的完整 Multiaddr）
//   - destID: 目标节点 ID（可为空）
//
// 返回格式（强制）：
//   - <relayAddr>/p2p-circuit/p2p/<destID>
//
// 示例：
//
//	relayAddr = "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay"
//	destID = QmDest
//	返回: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay/p2p-circuit/p2p/QmDest"
func (m Multiaddr) BuildRelayAddr(destID NodeID) (Multiaddr, error) {
	if m.IsEmpty() {
		return "", ErrEmptyMultiaddr
	}

	// 验证 relay 基础地址包含 /p2p/<relayID>
	if m.PeerID().IsEmpty() {
		return "", fmt.Errorf("%w: relay address must contain /p2p/<relayID>", ErrInvalidRelayFormat)
	}

	result := string(m) + "/" + RelayAddrProtocol
	if !destID.IsEmpty() {
		result += "/p2p/" + destID.String()
	}
	return Multiaddr(result), nil
}

// RelayID 返回中继节点 ID（如果是 relay 地址）
func (m Multiaddr) RelayID() NodeID {
	if !m.IsRelay() {
		return NodeID{}
	}

	// 找到 /p2p-circuit 的位置
	idx := strings.Index(string(m), "/"+RelayAddrProtocol)
	if idx == -1 {
		return NodeID{}
	}

	// 在 /p2p-circuit 之前查找 /p2p/<relayID>
	beforeCircuit := string(m)[:idx]
	parts := strings.Split(beforeCircuit, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if parts[i] == "p2p" && i+1 < len(parts) {
			nodeID, err := ParseNodeID(parts[i+1])
			if err == nil {
				return nodeID
			}
		}
	}
	return NodeID{}
}

// DestID 返回目标节点 ID（如果是 relay 地址且包含目标）
func (m Multiaddr) DestID() NodeID {
	if !m.IsRelay() {
		return NodeID{}
	}

	// 找到 /p2p-circuit 的位置
	idx := strings.Index(string(m), "/"+RelayAddrProtocol)
	if idx == -1 {
		return NodeID{}
	}

	// 在 /p2p-circuit 之后查找 /p2p/<destID>
	afterCircuit := string(m)[idx+len("/"+RelayAddrProtocol):]
	parts := strings.Split(afterCircuit, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "p2p" && i+1 < len(parts) {
			nodeID, err := ParseNodeID(parts[i+1])
			if err == nil {
				return nodeID
			}
		}
	}
	return NodeID{}
}

// RelayBaseAddr 返回中继节点的基础地址（去掉 /p2p-circuit 及之后部分）
func (m Multiaddr) RelayBaseAddr() Multiaddr {
	if !m.IsRelay() {
		return m
	}

	idx := strings.Index(string(m), "/"+RelayAddrProtocol)
	if idx == -1 {
		return m
	}
	return Multiaddr(string(m)[:idx])
}

// WithPeerID 附加 /p2p/<nodeID> 组件
//
// 如果已有 /p2p/ 组件，会替换它。
func (m Multiaddr) WithPeerID(nodeID NodeID) Multiaddr {
	if m.IsEmpty() || nodeID.IsEmpty() {
		return m
	}

	// 先移除现有的 /p2p/ 组件（如果有）
	base := m.WithoutPeerID()

	// 添加新的 /p2p/<nodeID>
	return Multiaddr(string(base) + "/p2p/" + nodeID.String())
}

// WithoutPeerID 移除 /p2p/<nodeID> 组件
//
// 对于 relay 地址，只移除最后一个 /p2p/ 组件（即 destID）。
// 如需移除 relayID，请使用其他方法。
func (m Multiaddr) WithoutPeerID() Multiaddr {
	if m.IsEmpty() {
		return m
	}

	s := string(m)

	// 对于 relay 地址，保留 relay 部分
	if m.IsRelay() {
		// 只移除 /p2p-circuit 之后的 /p2p/<destID>
		circuitIdx := strings.Index(s, "/"+RelayAddrProtocol)
		if circuitIdx != -1 {
			afterCircuit := s[circuitIdx+len("/"+RelayAddrProtocol):]
			p2pIdx := strings.Index(afterCircuit, "/p2p/")
			if p2pIdx != -1 {
				return Multiaddr(s[:circuitIdx+len("/"+RelayAddrProtocol)])
			}
		}
		return m
	}

	// 非 relay 地址，移除末尾的 /p2p/<nodeID>
	idx := strings.LastIndex(s, "/p2p/")
	if idx == -1 {
		return m
	}
	return Multiaddr(s[:idx])
}

// ============================================================================
//                              辅助函数
// ============================================================================

// ParseRelayAddrInfo 解析中继地址，返回详细信息
//
// 返回：
//   - relayBaseAddr: 中继节点的基础地址（不含 /p2p-circuit）
//   - relayID: 中继节点 ID
//   - destID: 目标节点 ID（可能为空）
//   - err: 错误信息
func (m Multiaddr) ParseRelayAddrInfo() (relayBaseAddr Multiaddr, relayID NodeID, destID NodeID, err error) {
	if !m.IsRelay() {
		return "", NodeID{}, NodeID{}, ErrNotRelayAddr
	}

	relayBaseAddr = m.RelayBaseAddr()
	relayID = m.RelayID()
	destID = m.DestID()

	if relayID.IsEmpty() {
		return "", NodeID{}, NodeID{}, ErrMissingRelayID
	}

	return relayBaseAddr, relayID, destID, nil
}

// IsDialableRelayAddr 检查是否是可拨号的 relay 地址
//
// 可拨号的 relay 地址必须包含中继节点的底层可达地址。
// 简写格式（如 /p2p/QmRelay/p2p-circuit/p2p/QmDest）不可用于拨号。
func (m Multiaddr) IsDialableRelayAddr() bool {
	if !m.IsRelay() {
		return false
	}

	// 检查是否有底层地址（ip4/ip6/dns4 等）
	parts := strings.Split(string(m), "/")
	if len(parts) < 2 {
		return false
	}

	firstComponent := parts[1]
	switch firstComponent {
	case "ip4", "ip6", "dns4", "dns6", "dnsaddr":
		return true
	case "p2p":
		// 简写格式，不可拨号
		return false
	default:
		return false
	}
}

// ============================================================================
//                              批量转换辅助函数
// ============================================================================

// MultiaddrsToStrings 将 Multiaddr 切片转换为字符串切片
func MultiaddrsToStrings(mas []Multiaddr) []string {
	strs := make([]string, len(mas))
	for i, ma := range mas {
		strs[i] = ma.String()
	}
	return strs
}

// StringsToMultiaddrs 将字符串切片转换为 Multiaddr 切片
//
// 跳过无法解析的地址，不返回错误。
// 如需严格验证，请使用 ParseMultiaddrStrict。
func StringsToMultiaddrs(strs []string) []Multiaddr {
	mas := make([]Multiaddr, 0, len(strs))
	for _, s := range strs {
		ma, err := ParseMultiaddr(s)
		if err == nil {
			mas = append(mas, ma)
		}
	}
	return mas
}

// ParseMultiaddrStrict 严格解析字符串切片为 Multiaddr 切片
//
// 遇到任何无法解析的地址立即返回错误。
func ParseMultiaddrStrict(strs []string) ([]Multiaddr, error) {
	mas := make([]Multiaddr, len(strs))
	for i, s := range strs {
		ma, err := ParseMultiaddr(s)
		if err != nil {
			return nil, fmt.Errorf("invalid address at index %d: %w", i, err)
		}
		mas[i] = ma
	}
	return mas, nil
}

