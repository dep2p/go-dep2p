// Package endpoint 定义 DeP2P 的核心接口
//
// 这是 API 层的接口包，定义 Endpoint、Connection、Stream 等核心接口。
// 本包提供向后兼容的类型别名，以及 Address 接口定义。
package endpoint

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                        类型别名（向后兼容）
// ============================================================================

// 基础类型别名 - 从 pkg/types 导入
type (
	// NodeID 节点唯一标识符
	NodeID = types.NodeID

	// StreamID 流唯一标识符
	StreamID = types.StreamID

	// ProtocolID 协议标识符
	ProtocolID = types.ProtocolID

	// RealmID 业务域标识符
	RealmID = types.RealmID
)

// 枚举类型别名 - 从 pkg/types 导入
type (
	// KeyType 密钥类型
	KeyType = types.KeyType

	// Direction 连接方向
	Direction = types.Direction

	// NATType NAT 类型
	NATType = types.NATType

	// Priority 优先级类型
	Priority = types.Priority

	// Connectedness 连接状态
	Connectedness = types.Connectedness

	// Reachability 可达性状态
	Reachability = types.Reachability
)

// 统计类型别名 - 从 pkg/types 导入
type (
	// ConnectionStats 连接统计
	ConnectionStats = types.ConnectionStats

	// StreamStats 流统计
	StreamStats = types.StreamStats

	// NetworkStats 网络统计
	NetworkStats = types.NetworkStats
)

// 地址类型别名 - 从 pkg/types 导入
type (
	// AddressInfo 带节点信息的地址
	AddressInfo = types.AddressInfo

	// AddressRecord 签名的地址记录
	AddressRecord = types.AddressRecord

	// AddressState 地址状态
	AddressState = types.AddressState

	// AddressEntry 地址簿条目
	AddressEntry = types.AddressEntry

	// AddressSource 地址来源
	AddressSource = types.AddressSource
)

// 密钥接口别名 - 从 pkg/interfaces/identity 导入
type (
	// PublicKey 公钥接口
	PublicKey = identity.PublicKey

	// PrivateKey 私钥接口
	PrivateKey = identity.PrivateKey

	// 注意：KeyGenerator 类型别名已删除（v1.1 清理）- 仅测试使用
)

// ============================================================================
//                        常量别名（向后兼容）
// ============================================================================

// EmptyNodeID 空节点 ID
var EmptyNodeID = types.EmptyNodeID

// KeyType 常量
const (
	KeyTypeEd25519 = types.KeyTypeEd25519
	KeyTypeECDSA   = types.KeyTypeECDSA
	KeyTypeRSA     = types.KeyTypeRSA
)

// Direction 常量
const (
	DirUnknown  = types.DirUnknown
	DirInbound  = types.DirInbound
	DirOutbound = types.DirOutbound
)

// NATType 常量
const (
	NATTypeUnknown        = types.NATTypeUnknown
	NATTypeNone           = types.NATTypeNone
	NATTypeFull           = types.NATTypeFull
	NATTypeRestricted     = types.NATTypeRestricted
	NATTypePortRestricted = types.NATTypePortRestricted
	NATTypeSymmetric      = types.NATTypeSymmetric
)

// Priority 常量
const (
	PriorityLow      = types.PriorityLow
	PriorityNormal   = types.PriorityNormal
	PriorityHigh     = types.PriorityHigh
	PriorityCritical = types.PriorityCritical
)

// Connectedness 常量
const (
	NotConnected  = types.NotConnected
	Connected     = types.Connected
	CanConnect    = types.CanConnect
	CannotConnect = types.CannotConnect
)

// Reachability 常量
const (
	ReachabilityUnknown = types.ReachabilityUnknown
	ReachabilityPublic  = types.ReachabilityPublic
	ReachabilityPrivate = types.ReachabilityPrivate
)

// AddressState 常量
const (
	AddressStateUnknown     = types.AddressStateUnknown
	AddressStatePending     = types.AddressStatePending
	AddressStateAvailable   = types.AddressStateAvailable
	AddressStateInvalid     = types.AddressStateInvalid
	AddressStateUnreachable = types.AddressStateUnreachable
)

// AddressSource 常量
const (
	AddressSourceUnknown   = types.AddressSourceUnknown
	AddressSourceDHT       = types.AddressSourceDHT
	AddressSourceMDNS      = types.AddressSourceMDNS
	AddressSourceManual    = types.AddressSourceManual
	AddressSourceBootstrap = types.AddressSourceBootstrap
	AddressSourceRelay     = types.AddressSourceRelay
	AddressSourceIncoming  = types.AddressSourceIncoming
)

// DefaultRealmID 默认 Realm ID
const DefaultRealmID = types.DefaultRealmID

// ============================================================================
//                        函数别名（向后兼容）
// ============================================================================

// NodeIDFromBytes 从字节切片创建 NodeID
var NodeIDFromBytes = types.NodeIDFromBytes

// ============================================================================
//                        Address 接口（类型别名）
// ============================================================================

// Address 网络地址接口
//
// 类型别名，指向 netaddr.Address。
// 为保持向后兼容，继续暴露为 endpoint.Address。
//
// Address 接口定义了网络地址的基本操作：
//   - Network() string: 返回网络类型 ("ip4", "ip6", "dns", "relay")
//   - String() string: 返回地址字符串表示
//   - Bytes() []byte: 返回地址的字节表示
//   - Equal(other Address) bool: 比较两个地址
//   - IsPublic/IsPrivate/IsLoopback() bool: 地址类型判断
type Address = netaddr.Address
