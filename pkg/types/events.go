// Package types 定义 DeP2P 公共类型
//
// 本文件定义事件相关类型。
package types

import (
	"time"
)

// ============================================================================
//                              Event - 事件接口
// ============================================================================

// Event 基础事件接口
type Event interface {
	// Type 返回事件类型
	Type() string

	// Timestamp 返回事件时间戳
	Timestamp() time.Time
}

// BaseEvent 基础事件实现
type BaseEvent struct {
	EventType string
	Time      time.Time
}

// Type 返回事件类型
func (e BaseEvent) Type() string {
	return e.EventType
}

// Timestamp 返回事件时间戳
func (e BaseEvent) Timestamp() time.Time {
	return e.Time
}

// NewBaseEvent 创建基础事件
func NewBaseEvent(eventType string) BaseEvent {
	return BaseEvent{
		EventType: eventType,
		Time:      time.Now(),
	}
}

// ============================================================================
//                              连接事件
// ============================================================================

// DisconnectReason 断开原因类型
//
// 用于区分不同的断开场景，支持快速断开检测机制：
// - 优雅断开（Graceful）：正常关闭，延迟 < 100ms
// - 超时断开（Timeout）：QUIC 空闲超时，检测延迟 < 10s
// - 错误断开（Error）：连接错误
// - 本地断开（Local）：本地主动关闭
// 参考：design/03_architecture/L3_behavioral/disconnect_detection.md
type DisconnectReason int

const (
	// DisconnectReasonUnknown 未知原因
	DisconnectReasonUnknown DisconnectReason = iota
	// DisconnectReasonGraceful 优雅断开（对端主动关闭）
	DisconnectReasonGraceful
	// DisconnectReasonTimeout QUIC 空闲超时断开
	DisconnectReasonTimeout
	// DisconnectReasonError 连接错误导致断开
	DisconnectReasonError
	// DisconnectReasonLocal 本地主动关闭连接
	DisconnectReasonLocal
)

// String 返回断开原因的字符串表示
func (r DisconnectReason) String() string {
	switch r {
	case DisconnectReasonGraceful:
		return "graceful"
	case DisconnectReasonTimeout:
		return "timeout"
	case DisconnectReasonError:
		return "error"
	case DisconnectReasonLocal:
		return "local"
	default:
		return "unknown"
	}
}

// EvtPeerConnected 节点连接事件
type EvtPeerConnected struct {
	BaseEvent
	PeerID    PeerID
	Direction Direction
	NumConns  int
}

// EvtPeerDisconnected 节点断开事件
//
// 扩展字段：
// - Reason: 断开原因，用于快速断开检测机制区分断开类型
// - Error: 如果 Reason 为 Error，包含具体错误信息
type EvtPeerDisconnected struct {
	BaseEvent
	PeerID   PeerID
	NumConns int
	Reason   DisconnectReason // 断开原因（快速断开检测使用）
	Error    error            // 错误信息（仅 Reason=Error 时有效）
}

// EvtConnectionClosed 连接关闭事件
type EvtConnectionClosed struct {
	BaseEvent
	PeerID    PeerID
	Direction Direction
	Duration  time.Duration
}

// ============================================================================
//                              发现事件
// ============================================================================

// EvtPeerDiscovered 发现节点事件
type EvtPeerDiscovered struct {
	BaseEvent
	PeerID PeerID
	Addrs  []Multiaddr
	Source DiscoverySource
}

// EvtPeerIdentified 节点识别完成事件
type EvtPeerIdentified struct {
	BaseEvent
	PeerID     PeerID
	Protocols  []ProtocolID
	AgentVer   string
	ListenAddr []Multiaddr
}

// ============================================================================
//                              协议事件
// ============================================================================

// EvtProtocolNegotiated 协议协商完成事件
type EvtProtocolNegotiated struct {
	BaseEvent
	PeerID   PeerID
	Protocol ProtocolID
}

// EvtStreamOpened 流打开事件
type EvtStreamOpened struct {
	BaseEvent
	StreamID  string
	PeerID    PeerID
	Protocol  ProtocolID
	Direction Direction
}

// EvtStreamClosed 流关闭事件
type EvtStreamClosed struct {
	BaseEvent
	StreamID string
	PeerID   PeerID
	Duration time.Duration
}

// ============================================================================
//                              Realm 事件
// ============================================================================

// EvtRealmJoined 加入 Realm 事件
type EvtRealmJoined struct {
	BaseEvent
	RealmID RealmID
	PeerID  PeerID
}

// EvtRealmLeft 离开 Realm 事件
type EvtRealmLeft struct {
	BaseEvent
	RealmID RealmID
	PeerID  PeerID
}

// EvtRealmMemberJoined Realm 成员加入事件
type EvtRealmMemberJoined struct {
	BaseEvent
	RealmID  RealmID
	MemberID PeerID
}

// EvtRealmMemberLeft Realm 成员离开事件
type EvtRealmMemberLeft struct {
	BaseEvent
	RealmID  RealmID
	MemberID PeerID
}

// ============================================================================
//                              NAT 事件
// ============================================================================

// EvtNATTypeDetected NAT 类型检测事件
type EvtNATTypeDetected struct {
	BaseEvent
	NATType      NATType
	ExternalIP   string
	ExternalPort int
	Reachability Reachability
}

// EvtHolePunchAttempt 打洞尝试事件
type EvtHolePunchAttempt struct {
	BaseEvent
	PeerID  PeerID
	Success bool
	RTT     time.Duration
	Error   string
}

// EvtHolePunchComplete 打洞完成事件
type EvtHolePunchComplete struct {
	BaseEvent
	PeerID  PeerID
	Success bool
	Direct  bool // 是否为直连（vs 通过中继）
}

// ============================================================================
//                              存活事件
// ============================================================================

// EvtPeerAlive 节点存活事件
type EvtPeerAlive struct {
	BaseEvent
	PeerID PeerID
	RTT    time.Duration
}

// EvtPeerDead 节点离线事件
type EvtPeerDead struct {
	BaseEvent
	PeerID      PeerID
	LastSeen    time.Time
	FailedPings int
}

// ============================================================================
//                              中继事件
// ============================================================================

// EvtRelayReservation 中继预约事件
type EvtRelayReservation struct {
	BaseEvent
	RelayPeer PeerID
	Success   bool
	Expiry    time.Time
}

// EvtRelayConnection 中继连接事件
type EvtRelayConnection struct {
	BaseEvent
	RelayPeer  PeerID
	RemotePeer PeerID
	Direction  Direction
}

// EvtRelayCircuitStateChanged 中继电路状态变更事件
//
// 当 RelayCircuit 状态发生变化时发射。
// 订阅者可用于监控电路健康状态和可达性变化。
type EvtRelayCircuitStateChanged struct {
	BaseEvent
	RelayPeer  PeerID // 中继节点
	RemotePeer PeerID // 远端节点
	OldState   string // 旧状态（"creating"/"active"/"stale"/"closed"）
	NewState   string // 新状态
	Reason     string // 变更原因（可选）
}

// ============================================================================
//                              地址事件
// ============================================================================

// EvtLocalAddrsUpdated 本地地址更新事件
//
// 当节点的监听地址发生变化时触发（如 Listen 完成、地址变更等）。
// 用于通知 mDNS 等依赖地址的服务及时更新。
type EvtLocalAddrsUpdated struct {
	BaseEvent
	// Current 当前所有地址
	Current []string
	// Added 新增的地址
	Added []string
	// Removed 移除的地址
	Removed []string
}

// ============================================================================
//                              事件类型常量
// ============================================================================

// 事件类型常量
const (
	EventTypePeerConnected            = "peer_connected"
	EventTypePeerDisconnected         = "peer_disconnected"
	EventTypeConnectionClosed         = "connection_closed"
	EventTypePeerDiscovered           = "peer_discovered"
	EventTypePeerIdentified           = "peer_identified"
	EventTypeProtocolNegotiated       = "protocol_negotiated"
	EventTypeStreamOpened             = "stream_opened"
	EventTypeStreamClosed             = "stream_closed"
	EventTypeRealmJoined              = "realm_joined"
	EventTypeRealmLeft                = "realm_left"
	EventTypeRealmMemberJoined        = "realm_member_joined"
	EventTypeRealmMemberLeft          = "realm_member_left"
	EventTypeNATTypeDetected          = "nat_type_detected"
	EventTypeHolePunchAttempt         = "hole_punch_attempt"
	EventTypeHolePunchComplete        = "hole_punch_complete"
	EventTypePeerAlive                = "peer_alive"
	EventTypePeerDead                 = "peer_dead"
	EventTypeRelayReservation         = "relay_reservation"
	EventTypeRelayConnection          = "relay_connection"
	EventTypeLocalAddrsUpdated        = "local_addrs_updated"
	EventTypeRelayCircuitStateChanged = "relay_circuit_state_changed"
)
