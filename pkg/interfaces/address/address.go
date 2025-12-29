// Package address 定义地址管理相关接口
//
// 地址模块负责网络地址的解析、验证和管理，包括：
// - 地址解析
// - 地址验证
// - 地址簿管理
package address

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              AddressManager 接口
// ============================================================================

// AddressManager 地址管理器接口
//
// 管理节点的监听地址和通告地址。
type AddressManager interface {
	// ListenAddrs 返回监听地址列表
	//
	// 这些是节点实际绑定的本地地址。
	ListenAddrs() []endpoint.Address

	// AdvertisedAddrs 返回通告地址列表
	//
	// 这些是向网络通告的公网可达地址。
	AdvertisedAddrs() []endpoint.Address

	// AddListenAddr 添加监听地址
	AddListenAddr(addr endpoint.Address)

	// RemoveListenAddr 移除监听地址
	RemoveListenAddr(addr endpoint.Address)

	// AddAdvertisedAddr 添加通告地址
	AddAdvertisedAddr(addr endpoint.Address)

	// RemoveAdvertisedAddr 移除通告地址
	RemoveAdvertisedAddr(addr endpoint.Address)

	// SetAdvertisedAddrs 设置通告地址（替换现有）
	SetAdvertisedAddrs(addrs []endpoint.Address)

	// BestAddr 返回最佳地址
	//
	// 根据地址类型和优先级返回最优的通告地址。
	BestAddr() endpoint.Address
}

// ============================================================================
//                              AddressBook 接口
// ============================================================================

// AddressBook 地址簿接口
//
// 存储和管理已知节点的地址。
type AddressBook interface {
	// Add 添加地址
	Add(nodeID types.NodeID, addrs ...endpoint.Address)

	// AddWithTTL 添加带 TTL 的地址
	AddWithTTL(nodeID types.NodeID, ttl time.Duration, addrs ...endpoint.Address)

	// Get 获取地址
	Get(nodeID types.NodeID) []endpoint.Address

	// Remove 移除节点的所有地址
	Remove(nodeID types.NodeID)

	// RemoveAddr 移除节点的特定地址
	RemoveAddr(nodeID types.NodeID, addr endpoint.Address)

	// Clear 清空地址簿
	Clear()

	// Peers 返回所有已知节点
	Peers() []types.NodeID

	// PeersWithAddrs 返回有地址的节点
	PeersWithAddrs() []types.NodeID

	// 注意：SetTTL/UpdateAddrs 方法已删除（v1.1 清理）- 未实现。
	// 使用 AddWithTTL 覆盖地址来设置 TTL。

	// CertifiedAddrs 返回经过验证的地址
	CertifiedAddrs(nodeID types.NodeID) []endpoint.Address

	// SetCertifiedAddrs 设置经过验证的地址
	SetCertifiedAddrs(nodeID types.NodeID, addrs []endpoint.Address)
}

// ============================================================================
//                              AddressParser 接口
// ============================================================================

// AddressParser 地址解析器接口
type AddressParser interface {
	// Parse 解析地址字符串
	//
	// 支持多种格式：
	// - "192.168.1.1:8000"
	// - "/ip4/192.168.1.1/tcp/8000"
	// - "example.com:8000"
	Parse(s string) (endpoint.Address, error)

	// ParseMultiple 解析多个地址
	ParseMultiple(ss []string) ([]endpoint.Address, error)
}

// ============================================================================
//                              AddressValidator / AddressSorter（v1.1 已删除）
// ============================================================================

// 注意：AddressValidator、AddressSorter 接口已删除（v1.1 清理）。
// 原因：无外部实现/使用，作为"扩展点"不承诺支持。
// 地址验证和排序功能由 internal/core/address 内部实现。

// ============================================================================
//                              地址类型
// ============================================================================

// AddressType 地址类型
type AddressType int

const (
	// AddressTypeUnknown 未知类型
	AddressTypeUnknown AddressType = iota
	// AddressTypeIPv4 IPv4 地址
	AddressTypeIPv4
	// AddressTypeIPv6 IPv6 地址
	AddressTypeIPv6
	// AddressTypeDNS DNS 地址
	AddressTypeDNS
	// AddressTypeRelay 中继地址
	AddressTypeRelay
)

// String 返回地址类型的字符串表示
func (t AddressType) String() string {
	switch t {
	case AddressTypeIPv4:
		return "ip4"
	case AddressTypeIPv6:
		return "ip6"
	case AddressTypeDNS:
		return "dns"
	case AddressTypeRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              地址优先级（可达性优先策略）
// ============================================================================

// AddressPriority 地址发布优先级
//
// 用于"可达性优先"策略的地址排序：
// - 已验证的直连地址优先级最高（可直接通信，延迟低）
// - Relay 地址始终保留（保证任何网络环境下都能连通）
// - 未验证的候选地址不发布（避免误导）
type AddressPriority int

const (
	// PriorityVerifiedDirect 已验证直连地址（优先级最高）
	// 来源：UPnP/NAT-PMP 端口映射成功 + 可达性验证通过
	PriorityVerifiedDirect AddressPriority = 100

	// PrioritySTUNReflexive 已验证 STUN 反射地址
	// 来源：STUN 查询成功 + 可达性验证通过
	PrioritySTUNReflexive AddressPriority = 80

	// PriorityRelayGuarantee Relay 中继地址（始终保留，保证可达）
	// 来源：AutoRelay 预留成功
	PriorityRelayGuarantee AddressPriority = 50

	// PriorityLocalListen 本地监听地址（回退）
	// 来源：本地绑定的地址，仅在无其他地址时使用
	PriorityLocalListen AddressPriority = 10

	// PriorityUnverified 未验证候选地址（不发布）
	// 来源：推断的地址，未经可达性验证
	PriorityUnverified AddressPriority = 0
)

// String 返回优先级的字符串表示
func (p AddressPriority) String() string {
	switch p {
	case PriorityVerifiedDirect:
		return "verified-direct"
	case PrioritySTUNReflexive:
		return "stun-reflexive"
	case PriorityRelayGuarantee:
		return "relay-guarantee"
	case PriorityLocalListen:
		return "local-listen"
	case PriorityUnverified:
		return "unverified"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              地址实现
// ============================================================================

// IPAddress IP 地址接口
type IPAddress interface {
	endpoint.Address

	// IP 返回 IP 地址
	IP() string

	// Port 返回端口
	Port() int

	// IsIPv4 是否是 IPv4
	IsIPv4() bool

	// IsIPv6 是否是 IPv6
	IsIPv6() bool
}

// DNSAddress DNS 地址接口
type DNSAddress interface {
	endpoint.Address

	// Host 返回主机名
	Host() string

	// Port 返回端口
	Port() int

	// Resolve 解析为 IP 地址
	Resolve() ([]endpoint.Address, error)
}

// RelayAddress 中继地址接口
type RelayAddress interface {
	endpoint.Address

	// RelayID 返回中继节点 ID
	RelayID() types.NodeID

	// DestID 返回目标节点 ID
	DestID() types.NodeID
}

// ============================================================================
//                              配置
// ============================================================================

// Config 地址模块配置
type Config struct {
	// DefaultTTL 默认地址 TTL
	DefaultTTL time.Duration

	// MaxAddrsPerPeer 每节点最大地址数
	MaxAddrsPerPeer int

	// EnableAddressSorting 启用地址排序
	EnableAddressSorting bool

	// PreferIPv6 优先使用 IPv6
	PreferIPv6 bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		DefaultTTL:           time.Hour,
		MaxAddrsPerPeer:      100,
		EnableAddressSorting: true,
		PreferIPv6:           false,
	}
}
