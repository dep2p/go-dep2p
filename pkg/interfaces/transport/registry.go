// Package transport 定义传输层接口
package transport

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
)

// ============================================================================
//                              TransportRegistry 接口
// ============================================================================

// TransportRegistry 传输注册表接口
//
// TransportRegistry 管理多个传输实现，提供统一的传输选择能力。
// 这是 Relay 集成到传输层的核心抽象：Endpoint 通过 Registry 选择合适的传输，
// 而不是直接依赖单一传输实例。
type TransportRegistry interface {
	// AddTransport 添加传输到注册表
	//
	// 如果已存在同协议的传输，返回错误。
	AddTransport(t Transport) error

	// RemoveTransport 移除指定协议的传输
	//
	// protocol: 协议标识，如 "quic-v1", "tcp", "p2p-circuit"
	RemoveTransport(protocol string) error

	// TransportForDialing 获取可拨号到指定地址的传输
	//
	// 根据地址协议选择合适的传输：
	// - /ip4/.../quic-v1 → QUIC Transport
	// - /ip4/.../tcp → TCP Transport
	// - /.../p2p-circuit/... → Relay Transport
	//
	// 如果没有合适的传输，返回 nil
	TransportForDialing(addr netaddr.Address) Transport

	// TransportForProtocol 获取指定协议的传输
	//
	// protocol: 协议标识，如 "quic-v1", "tcp", "p2p-circuit"
	TransportForProtocol(protocol string) Transport

	// Transports 返回所有注册的传输
	Transports() []Transport

	// Protocols 返回所有支持的协议
	Protocols() []string

	// Close 关闭所有传输
	Close() error
}

// ============================================================================
//                              地址类型常量
// ============================================================================

// AddressType 地址类型
type AddressType int

const (
	// AddressTypeDirect 直连地址（可直接拨号）
	AddressTypeDirect AddressType = iota

	// AddressTypeRelay 中继地址（通过 Relay 节点转发）
	AddressTypeRelay
)

// String 返回地址类型的字符串表示
func (t AddressType) String() string {
	switch t {
	case AddressTypeDirect:
		return "direct"
	case AddressTypeRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              地址排序接口
// ============================================================================

// AddressRanker 地址排序接口
//
// 用于对多个地址进行优先级排序，以优化连接建立的效率。
type AddressRanker interface {
	// RankAddresses 对地址进行排序
	//
	// 返回按优先级排序的地址列表（优先级高的在前）
	// 默认策略：直连地址优先于中继地址
	RankAddresses(addrs []netaddr.Address) []netaddr.Address
}

// ============================================================================
//                              错误定义
// ============================================================================

// 注册表错误
var (
	// ErrTransportExists 传输已存在
	ErrTransportExists = transportError("transport already exists for protocol")

	// ErrTransportNotFound 传输不存在
	ErrTransportNotFound = transportError("transport not found for protocol")

	// ErrNoSuitableTransport 没有合适的传输
	ErrNoSuitableTransport = transportError("no suitable transport for address")
)

// transportError 传输错误类型
type transportError string

func (e transportError) Error() string {
	return string(e)
}

