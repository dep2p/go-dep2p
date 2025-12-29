// Package addrutil 提供地址解析工具
//
// 本包提供完整地址（含 /p2p/<NodeID>）的解析和构建能力，
// 用于 Bootstrap 配置、用户间地址分享等场景。
package addrutil

import (
	"errors"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidFullAddr 无效的完整地址
	ErrInvalidFullAddr = errors.New("invalid full address: must contain /p2p/<NodeID>")

	// ErrMissingPeerID 缺少 /p2p/<NodeID> 后缀
	ErrMissingPeerID = errors.New("missing /p2p/<NodeID> suffix")

	// ErrInvalidPeerID 无效的 PeerID
	ErrInvalidPeerID = errors.New("invalid peer ID in address")

	// ErrPeerIDNotAtEnd /p2p/<NodeID> 不在地址末尾
	ErrPeerIDNotAtEnd = errors.New("/p2p/<NodeID> must be at the end of address (except for relay circuit)")

	// ErrEmptyAddress 空地址
	ErrEmptyAddress = errors.New("empty address")
)

// ============================================================================
//                              完整地址解析
// ============================================================================

// ParseFullAddr 解析完整地址（含 /p2p/<NodeID>）
//
// 完整地址格式：
//
//	/ip4/<ip>/udp/<port>/quic-v1/p2p/<NodeID>
//	/dns4/<domain>/udp/<port>/quic-v1/p2p/<NodeID>
//
// 对于 Relay 电路地址：
//
//	/ip4/<relay-ip>/udp/<port>/quic-v1/p2p/<relay-id>/p2p-circuit/p2p/<target-id>
//
// 返回：
//   - peerID: 目标节点 ID（完整地址末尾的 /p2p/<NodeID>）
//   - dialAddr: 可拨号地址（去掉最后的 /p2p/<NodeID>）
//   - err: 解析错误
//
// 示例：
//
//	// 普通地址
//	id, addr, _ := ParseFullAddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/5Q2STW...")
//	// id = 5Q2STW..., addr = "/ip4/1.2.3.4/udp/4001/quic-v1"
//
//	// Relay 电路地址
//	id, addr, _ := ParseFullAddr("/ip4/.../p2p/RelayID/p2p-circuit/p2p/TargetID")
//	// id = TargetID, addr = "/ip4/.../p2p/RelayID/p2p-circuit"
func ParseFullAddr(fullAddr string) (peerID types.NodeID, dialAddr string, err error) {
	if fullAddr == "" {
		return types.EmptyNodeID, "", ErrEmptyAddress
	}

	// 查找最后一个 /p2p/
	lastP2P := strings.LastIndex(fullAddr, "/p2p/")
	if lastP2P == -1 {
		return types.EmptyNodeID, "", ErrMissingPeerID
	}

	// 提取 NodeID 字符串
	afterP2P := fullAddr[lastP2P+5:] // 跳过 "/p2p/"

	// 检查 NodeID 后面是否还有路径（除了 Relay 电路的特殊情况）
	// 允许的情况：/p2p/<NodeID> 是最后一个组件
	// 特殊情况：/p2p/<RelayID>/p2p-circuit/p2p/<TargetID>
	if strings.Contains(afterP2P, "/") {
		// 检查是否是 relay 后的目标 ID
		// 如果 /p2p/ 后面还有 /，说明不是末尾
		return types.EmptyNodeID, "", ErrPeerIDNotAtEnd
	}

	// 解析 NodeID
	peerID, err = types.ParseNodeID(afterP2P)
	if err != nil {
		return types.EmptyNodeID, "", ErrInvalidPeerID
	}

	// 提取可拨号地址
	dialAddr = fullAddr[:lastP2P]
	if dialAddr == "" {
		return types.EmptyNodeID, "", ErrInvalidFullAddr
	}

	return peerID, dialAddr, nil
}

// BuildFullAddr 构建完整地址（地址 + /p2p/<NodeID>）
//
// 如果地址已经包含 /p2p/<NodeID>：
//   - 若与提供的 peerID 一致，直接返回原地址
//   - 若不一致，返回错误
//
// 对于 Relay 拨号地址（以 /p2p-circuit 结尾）：
//   - 直接在末尾追加 /p2p/<peerID>
//   - 这将产出可分享的 Relay Full Address
//
// 示例：
//
//	// 普通地址
//	addr := BuildFullAddr("/ip4/1.2.3.4/udp/4001/quic-v1", peerID)
//	// 返回: "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/5Q2STW..."
//
//	// Relay 拨号地址
//	addr := BuildFullAddr("/ip4/.../p2p/RelayID/p2p-circuit", selfID)
//	// 返回: "/ip4/.../p2p/RelayID/p2p-circuit/p2p/5Q2STW..."
func BuildFullAddr(addr string, peerID types.NodeID) (string, error) {
	if addr == "" {
		return "", ErrEmptyAddress
	}
	if peerID.IsEmpty() {
		return "", ErrInvalidPeerID
	}

	// 情况 1: Relay 拨号地址（以 /p2p-circuit 结尾，需要追加目标 /p2p/<peerID>）
	if strings.HasSuffix(addr, "/p2p-circuit") {
		// 直接在末尾追加 /p2p/<peerID>
		return addr + "/p2p/" + peerID.String(), nil
	}

	// 情况 2: 完整的 Relay 电路地址（已含 /p2p-circuit/p2p/<target>）
	if IsRelayAddr(addr) {
		// 解析现有的 targetID
		_, existingTargetID, _, err := ParseRelayAddr(addr)
		if err != nil {
			return "", err
		}
		// 检查一致性
		if !existingTargetID.Equal(peerID) {
			return "", errors.New("relay address already contains different target peer ID")
		}
		return addr, nil
	}

	// 情况 3: 普通地址（含 /p2p/<NodeID>，不含 /p2p-circuit）
	if strings.Contains(addr, "/p2p/") {
		// 解析现有的 NodeID
		existingID, _, err := ParseFullAddr(addr)
		if err != nil {
			return "", err
		}
		// 检查一致性
		if !existingID.Equal(peerID) {
			return "", errors.New("address already contains different peer ID")
		}
		return addr, nil
	}

	// 情况 4: 纯拨号地址（无 /p2p/），直接拼接
	return addr + "/p2p/" + peerID.String(), nil
}

// ExtractPeerID 从地址中提取 PeerID（如果存在）
//
// 如果地址不包含 /p2p/<NodeID>，返回 EmptyNodeID 和 nil 错误。
// 如果地址包含无效的 NodeID，返回 EmptyNodeID 和错误。
func ExtractPeerID(addr string) (types.NodeID, error) {
	if !strings.Contains(addr, "/p2p/") {
		return types.EmptyNodeID, nil
	}

	peerID, _, err := ParseFullAddr(addr)
	return peerID, err
}

// StripPeerID 从地址中移除 /p2p/<NodeID> 后缀
//
// 如果地址不包含 /p2p/<NodeID>，返回原地址。
func StripPeerID(addr string) string {
	lastP2P := strings.LastIndex(addr, "/p2p/")
	if lastP2P == -1 {
		return addr
	}

	// 检查 /p2p/ 后面是否还有路径组件
	afterP2P := addr[lastP2P+5:]
	if strings.Contains(afterP2P, "/") {
		// 不是末尾的 /p2p/，不要移除
		return addr
	}

	return addr[:lastP2P]
}

// HasPeerID 检查地址是否包含 /p2p/<NodeID>
func HasPeerID(addr string) bool {
	return strings.Contains(addr, "/p2p/")
}

// IsRelayAddr 检查是否是 Relay 电路地址
//
// Relay 电路地址格式：
//
//	/ip4/.../p2p/<relay-id>/p2p-circuit/p2p/<target-id>
func IsRelayAddr(addr string) bool {
	return strings.Contains(addr, "/p2p-circuit/")
}

// ParseRelayAddr 解析 Relay 电路地址
//
// 返回：
//   - relayID: 中继节点 ID
//   - targetID: 目标节点 ID
//   - relayAddr: 中继节点地址（不含 /p2p-circuit 及之后部分）
//   - err: 解析错误
func ParseRelayAddr(addr string) (relayID, targetID types.NodeID, relayAddr string, err error) {
	if !IsRelayAddr(addr) {
		return types.EmptyNodeID, types.EmptyNodeID, "", errors.New("not a relay address")
	}

	// 分割 /p2p-circuit/
	parts := strings.SplitN(addr, "/p2p-circuit", 2)
	if len(parts) != 2 {
		return types.EmptyNodeID, types.EmptyNodeID, "", ErrInvalidFullAddr
	}

	relayPart := parts[0]  // /ip4/.../p2p/<relay-id>
	targetPart := parts[1] // /p2p/<target-id>

	// 解析 relay 部分
	relayID, relayAddr, err = ParseFullAddr(relayPart)
	if err != nil {
		return types.EmptyNodeID, types.EmptyNodeID, "", errors.New("invalid relay address: " + err.Error())
	}

	// 解析 target 部分
	if !strings.HasPrefix(targetPart, "/p2p/") {
		return types.EmptyNodeID, types.EmptyNodeID, "", errors.New("missing target /p2p/ in relay address")
	}
	targetIDStr := strings.TrimPrefix(targetPart, "/p2p/")
	targetID, err = types.ParseNodeID(targetIDStr)
	if err != nil {
		return types.EmptyNodeID, types.EmptyNodeID, "", errors.New("invalid target peer ID: " + err.Error())
	}

	return relayID, targetID, relayAddr, nil
}

// ============================================================================
//                              地址类型分类 (REQ-ADDR-001)
// ============================================================================

// AddressType 地址类型枚举
//
// REQ-ADDR-001: Full Address 与 Dial Address 术语边界清晰
type AddressType int

const (
	// AddrTypeUnknown 未知类型
	AddrTypeUnknown AddressType = iota

	// AddrTypeDialAddr 拨号地址（不含 /p2p/<NodeID>）
	//
	// 示例: /ip4/1.2.3.4/udp/4001/quic-v1
	// 用途: 传递给 ConnectWithAddrs
	AddrTypeDialAddr

	// AddrTypeFullAddr 完整地址（含 /p2p/<NodeID>）
	//
	// 示例: /ip4/1.2.3.4/udp/4001/quic-v1/p2p/NodeID
	// 用途: Bootstrap 配置、用户间分享
	AddrTypeFullAddr

	// AddrTypeRelayCircuit Relay 电路地址
	//
	// 示例: /ip4/.../p2p/RelayID/p2p-circuit/p2p/TargetID
	// 用途: 通过中继连接
	AddrTypeRelayCircuit
)

// String 返回地址类型的字符串表示
func (t AddressType) String() string {
	switch t {
	case AddrTypeDialAddr:
		return "dial_addr"
	case AddrTypeFullAddr:
		return "full_addr"
	case AddrTypeRelayCircuit:
		return "relay_circuit"
	default:
		return "unknown"
	}
}

// ClassifyAddress 分类地址类型
//
// REQ-ADDR-001: Full Address 与 Dial Address 术语边界清晰
//
// 返回地址的类型分类，便于上层代码做类型判定。
func ClassifyAddress(addr string) AddressType {
	if addr == "" {
		return AddrTypeUnknown
	}

	// 检查是否是 Relay 电路地址
	if IsRelayAddr(addr) {
		return AddrTypeRelayCircuit
	}

	// 检查是否包含 /p2p/<NodeID>
	if HasPeerID(addr) {
		return AddrTypeFullAddr
	}

	// 检查是否是有效的 multiaddr 格式
	if strings.HasPrefix(addr, "/") {
		return AddrTypeDialAddr
	}

	return AddrTypeUnknown
}

// IsFullAddress 检查是否是完整地址（含 /p2p/<NodeID>，不含 /p2p-circuit）
//
// REQ-ADDR-001: Full Address 必须包含 /p2p/<NodeID>
func IsFullAddress(addr string) bool {
	return ClassifyAddress(addr) == AddrTypeFullAddr
}

// IsDialAddress 检查是否是拨号地址（不含 /p2p/<NodeID>）
//
// REQ-ADDR-001: Dial Address 不应包含 /p2p/<NodeID>
func IsDialAddress(addr string) bool {
	return ClassifyAddress(addr) == AddrTypeDialAddr
}

// ValidateAddressType 验证地址是否符合预期类型
//
// REQ-ADDR-004: 地址解析与校验必须严格
func ValidateAddressType(addr string, expectedType AddressType) error {
	actualType := ClassifyAddress(addr)
	if actualType != expectedType {
		return errors.New("address type mismatch: expected " + expectedType.String() + ", got " + actualType.String())
	}
	return nil
}

// MustParseNodeID 解析 NodeID，失败时 panic
//
// 仅用于测试或初始化已知有效的 NodeID。
func MustParseNodeID(s string) types.NodeID {
	id, err := types.ParseNodeID(s)
	if err != nil {
		panic("invalid NodeID: " + s + ": " + err.Error())
	}
	return id
}

// ============================================================================
//                              地址格式转换（已废弃）
// ============================================================================

// NormalizeToMultiaddr 已废弃
//
// Deprecated: 根据 IMPL-ADDRESS-UNIFICATION.md 规范，地址转换应使用 types.FromHostPort 或 types.ParseMultiaddr。
// 此函数已删除，请使用：
//   - types.ParseMultiaddr(s) 解析 multiaddr 字符串
//   - types.FromHostPort(host, port, transport) 从 host:port 创建 multiaddr
//
// 删除原因：
//   - 默认使用 UDP/QUIC-v1 传输协议会导致歧义
//   - host:port 格式应在 CLI/UI 边界层显式转换，而非自动转换
