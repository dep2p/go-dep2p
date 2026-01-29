// Package types 定义 DeP2P 的基础类型
//
// 本文件定义所有枚举类型，用于表示各种状态和模式。
package types

// ============================================================================
//                              KeyType - 密钥类型
// ============================================================================

// KeyType 密钥类型
type KeyType int

const (
	// KeyTypeUnknown 未知密钥类型
	KeyTypeUnknown KeyType = iota
	// KeyTypeEd25519 Ed25519 密钥（默认推荐）
	KeyTypeEd25519
	// KeyTypeECDSA ECDSA 密钥（通用）
	KeyTypeECDSA
	// KeyTypeECDSAP256 ECDSA P-256 密钥
	KeyTypeECDSAP256
	// KeyTypeECDSAP384 ECDSA P-384 密钥
	KeyTypeECDSAP384
	// KeyTypeRSA RSA 密钥
	KeyTypeRSA
	// KeyTypeSecp256k1 Secp256k1 密钥（区块链兼容）
	KeyTypeSecp256k1
)

// String 返回密钥类型的字符串表示
func (kt KeyType) String() string {
	switch kt {
	case KeyTypeEd25519:
		return "Ed25519"
	case KeyTypeECDSA:
		return "ECDSA"
	case KeyTypeECDSAP256:
		return "ECDSA-P256"
	case KeyTypeECDSAP384:
		return "ECDSA-P384"
	case KeyTypeRSA:
		return "RSA"
	case KeyTypeSecp256k1:
		return "Secp256k1"
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              Direction - 方向
// ============================================================================

// Direction 连接/流方向
type Direction int

const (
	// DirUnknown 未知方向
	DirUnknown Direction = iota
	// DirInbound 入站
	DirInbound
	// DirOutbound 出站
	DirOutbound
)

// String 返回方向的字符串表示
func (d Direction) String() string {
	switch d {
	case DirInbound:
		return "inbound"
	case DirOutbound:
		return "outbound"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              Connectedness - 连接状态
// ============================================================================

// Connectedness 与节点的连接状态
type Connectedness int

const (
	// NotConnected 未连接
	NotConnected Connectedness = iota
	// Connected 已连接
	Connected
	// CanConnect 可连接（有地址但未连接）
	CanConnect
	// CannotConnect 无法连接
	CannotConnect
)

// String 返回连接状态的字符串表示
func (c Connectedness) String() string {
	switch c {
	case NotConnected:
		return "not_connected"
	case Connected:
		return "connected"
	case CanConnect:
		return "can_connect"
	case CannotConnect:
		return "cannot_connect"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              Reachability - 可达性
// ============================================================================

// Reachability 网络可达性状态
type Reachability int

const (
	// ReachabilityUnknown 未知
	ReachabilityUnknown Reachability = iota
	// ReachabilityPublic 公网可达
	ReachabilityPublic
	// ReachabilityPrivate 仅私网可达
	ReachabilityPrivate
)

// String 返回可达性状态的字符串表示
func (r Reachability) String() string {
	switch r {
	case ReachabilityPublic:
		return "public"
	case ReachabilityPrivate:
		return "private"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              NATType - NAT 类型
// ============================================================================

// NATType NAT 类型
type NATType int

const (
	// NATTypeUnknown 未知类型
	NATTypeUnknown NATType = iota
	// NATTypeNone 无 NAT（公网）
	NATTypeNone
	// NATTypeFullCone 完全锥形 NAT
	NATTypeFullCone
	// NATTypeRestrictedCone 受限锥形 NAT
	NATTypeRestrictedCone
	// NATTypePortRestricted 端口受限锥形 NAT
	NATTypePortRestricted
	// NATTypeSymmetric 对称型 NAT
	NATTypeSymmetric
)

// String 返回 NAT 类型的字符串表示
func (n NATType) String() string {
	switch n {
	case NATTypeNone:
		return "none"
	case NATTypeFullCone:
		return "full_cone"
	case NATTypeRestrictedCone:
		return "restricted_cone"
	case NATTypePortRestricted:
		return "port_restricted"
	case NATTypeSymmetric:
		return "symmetric"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              Priority - 优先级
// ============================================================================

// Priority 优先级类型
type Priority int

const (
	// PriorityLow 低优先级（后台传输）
	PriorityLow Priority = iota
	// PriorityNormal 普通优先级（默认）
	PriorityNormal
	// PriorityHigh 高优先级（交互式）
	PriorityHigh
	// PriorityCritical 关键优先级（控制消息）
	PriorityCritical
)

// String 返回优先级的字符串表示
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              DHTMode - DHT 模式
// ============================================================================

// DHTMode DHT 运行模式
type DHTMode int

const (
	// DHTModeAuto 自动模式（根据网络情况决定）
	DHTModeAuto DHTMode = iota
	// DHTModeClient 客户端模式（只查询，不存储）
	DHTModeClient
	// DHTModeServer 服务端模式（存储和响应查询）
	DHTModeServer
)

// String 返回 DHT 模式的字符串表示
func (m DHTMode) String() string {
	switch m {
	case DHTModeAuto:
		return "auto"
	case DHTModeClient:
		return "client"
	case DHTModeServer:
		return "server"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              RealmAuthMode - Realm 认证模式
// ============================================================================

// RealmAuthMode Realm 认证模式
type RealmAuthMode int

const (
	// AuthModePSK 预共享密钥认证
	AuthModePSK RealmAuthMode = iota
	// AuthModeCert 证书认证
	AuthModeCert
	// AuthModeCustom 自定义认证
	AuthModeCustom
)

// String 返回认证模式的字符串表示
func (m RealmAuthMode) String() string {
	switch m {
	case AuthModePSK:
		return "psk"
	case AuthModeCert:
		return "cert"
	case AuthModeCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              RealmRole - Realm 角色
// ============================================================================

// RealmRole Realm 成员角色
type RealmRole int

const (
	// RoleMember 普通成员
	RoleMember RealmRole = iota
	// RoleAdmin 管理员
	RoleAdmin
	// RoleRelay 中继节点
	RoleRelay
)

// String 返回角色的字符串表示
func (r RealmRole) String() string {
	switch r {
	case RoleMember:
		return "member"
	case RoleAdmin:
		return "admin"
	case RoleRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              DiscoverySource - 发现来源
// ============================================================================

// DiscoverySource 节点发现来源
type DiscoverySource string

const (
	// SourceDHT DHT 发现
	SourceDHT DiscoverySource = "dht"
	// SourceMDNS mDNS 局域网发现
	SourceMDNS DiscoverySource = "mdns"
	// SourceBootstrap 引导节点
	SourceBootstrap DiscoverySource = "bootstrap"
	// SourceRendezvous Rendezvous 发现
	SourceRendezvous DiscoverySource = "rendezvous"
	// SourceDNS DNS 发现
	SourceDNS DiscoverySource = "dns"
	// SourceManual 手动添加
	SourceManual DiscoverySource = "manual"
)

// String 返回发现来源的字符串表示
func (s DiscoverySource) String() string {
	return string(s)
}
