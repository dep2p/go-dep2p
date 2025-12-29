package types

import "time"

// ============================================================================
//                              节点状态
// ============================================================================

// PeerStatus 节点状态
type PeerStatus int

const (
	// PeerStatusUnknown 未知状态 - 从未连接过
	PeerStatusUnknown PeerStatus = iota
	// PeerStatusOnline 在线 - 可连接，响应正常
	PeerStatusOnline
	// PeerStatusDegraded 降级 - 可连接但不稳定，响应慢
	PeerStatusDegraded
	// PeerStatusOffline 离线 - 主动离线或心跳超时
	PeerStatusOffline
)

// String 返回节点状态的字符串表示
func (s PeerStatus) String() string {
	switch s {
	case PeerStatusUnknown:
		return "unknown"
	case PeerStatusOnline:
		return "online"
	case PeerStatusDegraded:
		return "degraded"
	case PeerStatusOffline:
		return "offline"
	default:
		return "invalid"
	}
}

// IsAvailable 节点是否可用（在线或降级）
func (s PeerStatus) IsAvailable() bool {
	return s == PeerStatusOnline || s == PeerStatusDegraded
}

// ============================================================================
//                              Goodbye 原因
// ============================================================================

// GoodbyeReason Goodbye 消息的离线原因
type GoodbyeReason string

const (
	// GoodbyeReasonShutdown 正常关闭
	GoodbyeReasonShutdown GoodbyeReason = "shutdown"
	// GoodbyeReasonMaintenance 维护
	GoodbyeReasonMaintenance GoodbyeReason = "maintenance"
	// GoodbyeReasonMigration 迁移到新地址
	GoodbyeReasonMigration GoodbyeReason = "migration"
	// GoodbyeReasonOverload 过载，暂时下线
	GoodbyeReasonOverload GoodbyeReason = "overload"
)

// String 返回 Goodbye 原因的字符串表示
func (r GoodbyeReason) String() string {
	return string(r)
}

// ============================================================================
//                              Goodbye 消息
// ============================================================================

// GoodbyeMessage Goodbye 协议消息
type GoodbyeMessage struct {
	// NodeID 要离线的节点
	NodeID NodeID

	// Reason 离线原因
	Reason GoodbyeReason

	// Timestamp 时间戳
	Timestamp time.Time

	// Signature 签名（防止伪造）
	Signature []byte

	// NewAddresses 新地址（仅当 Reason 为 migration 时使用）
	NewAddresses []string
}

// ============================================================================
//                              节点健康信息
// ============================================================================

// PeerHealth 节点健康信息
type PeerHealth struct {
	// NodeID 节点 ID
	NodeID NodeID

	// Status 当前状态
	Status PeerStatus

	// LastSeen 最后一次看到的时间
	LastSeen time.Time

	// LastPing 最后一次 Ping 的时间
	LastPing time.Time

	// LastPingRTT 最后一次 Ping 的 RTT
	LastPingRTT time.Duration

	// AvgRTT 平均 RTT
	AvgRTT time.Duration

	// FailedPings 连续失败的 Ping 次数
	FailedPings int

	// HealthScore 健康评分（0-100）
	HealthScore int
}

// ============================================================================
//                              状态变更事件
// ============================================================================

// PeerStatusChangeEvent 节点状态变更事件
type PeerStatusChangeEvent struct {
	// NodeID 节点 ID
	NodeID NodeID

	// OldStatus 旧状态
	OldStatus PeerStatus

	// NewStatus 新状态
	NewStatus PeerStatus

	// Reason 变更原因
	Reason string

	// Timestamp 变更时间
	Timestamp time.Time
}

// ============================================================================
//                              心跳配置阈值
// ============================================================================

// LivenessThresholds 存活检测阈值
type LivenessThresholds struct {
	// DegradedRTT RTT 超过此值判定为降级（默认 500ms）
	DegradedRTT time.Duration

	// HeartbeatInterval 心跳间隔（默认 15s）
	HeartbeatInterval time.Duration

	// HeartbeatTimeout 心跳超时（默认 45s，3次心跳）
	HeartbeatTimeout time.Duration

	// StatusExpiry 状态过期时间（默认 5min）
	StatusExpiry time.Duration
}

// DefaultLivenessThresholds 默认存活检测阈值
func DefaultLivenessThresholds() LivenessThresholds {
	return LivenessThresholds{
		DegradedRTT:       500 * time.Millisecond,
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  45 * time.Second,
		StatusExpiry:      5 * time.Minute,
	}
}


