// Package types 定义 DeP2P 公共类型
//
// 本文件重导出 multiaddr 包的类型和函数。
package types

import (
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
)

// ============================================================================
//                              Multiaddr - 多地址
// ============================================================================

// Multiaddr 表示多地址
//
// Multiaddr 是一种自描述的网络地址格式。
// 例如：/ip4/127.0.0.1/tcp/4001/p2p/12D3KooW...
type Multiaddr = multiaddr.Multiaddr

// ============================================================================
//                              构造函数
// ============================================================================

// NewMultiaddr 从字符串创建多地址
var NewMultiaddr = multiaddr.NewMultiaddr

// ParseMultiaddr 从字符串解析多地址（别名）
var ParseMultiaddr = multiaddr.NewMultiaddr

// NewMultiaddrBytes 从字节创建多地址
var NewMultiaddrBytes = multiaddr.NewMultiaddrBytes

// ============================================================================
//                              转换函数
// ============================================================================

// FromTCPAddr 从 TCP 地址创建多地址
var FromTCPAddr = multiaddr.FromTCPAddr

// FromUDPAddr 从 UDP 地址创建多地址
var FromUDPAddr = multiaddr.FromUDPAddr

// FromNetAddr 从 net.Addr 创建多地址
var FromNetAddr = multiaddr.FromNetAddr

// ============================================================================
//                              工具函数
// ============================================================================

// SplitMultiaddr 分离传输地址和 P2P 组件
//
// 输入：/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
// 输出：/ip4/1.2.3.4/tcp/4001, 12D3KooW...
func SplitMultiaddr(m Multiaddr) (transport Multiaddr, peerID PeerID) {
	t, id := multiaddr.Split(m)
	return t, PeerID(id)
}

// JoinMultiaddr 合并传输地址和 P2P 组件
func JoinMultiaddr(transport Multiaddr, peerID PeerID) Multiaddr {
	return multiaddr.Join(transport, string(peerID))
}

// FilterMultiaddrs 过滤多地址
var FilterMultiaddrs = multiaddr.FilterAddrs

// UniqueMultiaddrs 去重多地址
var UniqueMultiaddrs = multiaddr.UniqueAddrs

// HasProtocol 检查多地址是否包含指定协议
var HasProtocol = multiaddr.HasProtocol

// GetPeerID 从多地址中提取 PeerID
func GetPeerID(m Multiaddr) (PeerID, error) {
	id, err := multiaddr.GetPeerID(m)
	return PeerID(id), err
}

// WithPeerID 为多地址添加或替换 PeerID
func WithPeerID(m Multiaddr, peerID PeerID) (Multiaddr, error) {
	return multiaddr.WithPeerID(m, string(peerID))
}

// WithoutPeerID 移除多地址中的 PeerID
var WithoutPeerID = multiaddr.WithoutPeerID

// ============================================================================
//                              辅助函数（兼容性）
// ============================================================================

// P2PMultiaddr 创建 /p2p/<peerID> 多地址
func P2PMultiaddr(peerID PeerID) Multiaddr {
	ma, _ := NewMultiaddr("/p2p/" + string(peerID))
	return ma
}

// IsEmpty 检查多地址是否为空
func IsEmpty(m Multiaddr) bool {
	return m == nil || len(m.Bytes()) == 0
}

// ValueForProtocolName 获取指定协议名的值
func ValueForProtocolName(m Multiaddr, name string) (string, error) {
	proto := multiaddr.ProtocolWithName(name)
	if proto.Code == 0 {
		return "", multiaddr.ErrInvalidProtocol
	}
	return m.ValueForProtocol(proto.Code)
}

// ============================================================================
//                              协议常量
// ============================================================================

// 协议代码常量（重导出）
const (
	ProtocolIP4       = multiaddr.P_IP4
	ProtocolIP6       = multiaddr.P_IP6
	ProtocolTCP       = multiaddr.P_TCP
	ProtocolUDP       = multiaddr.P_UDP
	ProtocolQUIC      = multiaddr.P_QUIC
	ProtocolQUIC_V1   = multiaddr.P_QUIC_V1
	ProtocolP2P       = multiaddr.P_P2P
	ProtocolWS        = multiaddr.P_WS
	ProtocolWSS       = multiaddr.P_WSS
	ProtocolDNS       = multiaddr.P_DNS
	ProtocolDNS4      = multiaddr.P_DNS4
	ProtocolDNS6      = multiaddr.P_DNS6
	ProtocolDNSADDR   = multiaddr.P_DNSADDR
)

// 简短别名（用于兼容性）
const (
	P_IP4             = multiaddr.P_IP4
	P_IP6             = multiaddr.P_IP6
	P_TCP             = multiaddr.P_TCP
	P_UDP             = multiaddr.P_UDP
	P_QUIC            = multiaddr.P_QUIC
	P_QUIC_V1         = multiaddr.P_QUIC_V1
	P_P2P             = multiaddr.P_P2P
	P_WS              = multiaddr.P_WS
	P_WSS             = multiaddr.P_WSS
	P_DNS             = multiaddr.P_DNS
	P_DNS4            = multiaddr.P_DNS4
	P_DNS6            = multiaddr.P_DNS6
	P_DNSADDR         = multiaddr.P_DNSADDR
	P_CIRCUIT         = multiaddr.P_CIRCUIT
	P_WEBTRANSPORT    = multiaddr.P_WEBTRANSPORT
	P_WEBRTC          = multiaddr.P_WEBRTC
	P_WEBRTC_DIRECT   = multiaddr.P_WEBRTC_DIRECT
	P_P2P_WEBRTC_DIRECT = multiaddr.P_P2P_WEBRTC_DIRECT
)

// Component 表示多地址组件
type Component = multiaddr.Component

// Protocol 返回协议信息
type Protocol = multiaddr.Protocol

// ForEach 遍历多地址中的每个组件
func ForEach(m Multiaddr, fn func(Component) bool) {
	multiaddr.ForEach(m, fn)
}

// SplitFirst 分离多地址的第一个组件
func SplitFirst(m Multiaddr) (Component, Multiaddr) {
	return multiaddr.SplitFirst(m)
}
