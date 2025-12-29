package types

// ============================================================================
//                              KeyType - 密钥类型
// ============================================================================

// KeyType 密钥类型
type KeyType int

const (
	// KeyTypeUnknown 未知密钥类型
	KeyTypeUnknown KeyType = iota
	// KeyTypeEd25519 Ed25519 密钥
	KeyTypeEd25519
	// KeyTypeECDSA ECDSA 密钥（通用）
	KeyTypeECDSA
	// KeyTypeECDSAP256 ECDSA P-256 密钥
	KeyTypeECDSAP256
	// KeyTypeECDSAP384 ECDSA P-384 密钥
	KeyTypeECDSAP384
	// KeyTypeRSA RSA 密钥
	KeyTypeRSA
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
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              Direction - 连接方向
// ============================================================================

// Direction 连接方向
type Direction int

const (
	// DirUnknown 未知方向
	DirUnknown Direction = iota
	// DirInbound 入站连接
	DirInbound
	// DirOutbound 出站连接
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
//                              NATType - NAT 类型
// ============================================================================

// NATType NAT 类型
type NATType int

const (
	// NATTypeUnknown 未知类型
	NATTypeUnknown NATType = iota
	// NATTypeNone 无 NAT（公网）
	NATTypeNone
	// NATTypeFull 完全锥形 NAT
	NATTypeFull
	// NATTypeRestricted 受限锥形 NAT
	NATTypeRestricted
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
	case NATTypeFull:
		return "full_cone"
	case NATTypeRestricted:
		return "restricted"
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
//                              Connectedness - 连接状态
// ============================================================================

// Connectedness 节点连接状态
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

