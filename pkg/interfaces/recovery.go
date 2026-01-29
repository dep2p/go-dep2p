// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Recovery 组件接口，对应 internal/core/recovery/ 实现。
// 包括：RecoveryManager（恢复管理）、ConnectionHealthMonitor（健康监控）、NetworkMonitor（网络变化监控）
package interfaces

import (
	"context"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// NetworkMonitor 接口（系统网络变化监控）
// ════════════════════════════════════════════════════════════════════════════

// NetworkMonitor 提供系统网络变化监控能力
//
// 监控系统级网络变化事件（如网卡变化、IP 变化），由 Recovery 模块使用。
type NetworkMonitor interface {
	// Start 启动网络监控
	Start(ctx context.Context) error

	// Stop 停止网络监控
	Stop() error

	// Subscribe 订阅网络变化事件
	Subscribe() <-chan NetworkChangeEvent

	// NotifyChange 外部通知网络变化（用于 Android 等平台）
	NotifyChange()

	// CurrentState 获取当前网络状态
	CurrentState() NetworkState
}

// NetworkChangeEvent 网络变化事件
type NetworkChangeEvent struct {
	// Type 变化类型
	Type NetworkChangeType

	// OldAddrs 旧地址列表
	OldAddrs []string

	// NewAddrs 新地址列表（可能为空，需要重新探测）
	NewAddrs []string

	// Timestamp 事件时间
	Timestamp time.Time
}

// NetworkChangeType 网络变化类型
type NetworkChangeType int

const (
	// NetworkChangeMinor IP 地址变化但接口不变
	NetworkChangeMinor NetworkChangeType = iota

	// NetworkChangeMajor 网络接口变化（如 4G→WiFi）
	NetworkChangeMajor
)

// String 返回类型字符串
func (t NetworkChangeType) String() string {
	switch t {
	case NetworkChangeMinor:
		return "minor"
	case NetworkChangeMajor:
		return "major"
	default:
		return "unknown"
	}
}

// NetworkState 网络状态
type NetworkState struct {
	// Interfaces 活跃的网络接口
	Interfaces []NetworkInterface

	// PreferredInterface 首选接口
	PreferredInterface string

	// IsOnline 是否在线
	IsOnline bool
}

// NetworkInterface 网络接口信息
type NetworkInterface struct {
	// Name 接口名称
	Name string

	// Addrs 接口地址列表
	Addrs []string

	// IsUp 接口是否激活
	IsUp bool

	// IsLoopback 是否为回环接口
	IsLoopback bool
}

// ════════════════════════════════════════════════════════════════════════════
// NetworkChangeResponder 接口（网络变化响应者）
// 实现位置：各组件可选实现这些接口以响应网络变化
// ════════════════════════════════════════════════════════════════════════════

// NATRefresher NAT 刷新接口
type NATRefresher interface {
	ForceSTUN(ctx context.Context) error
}

// RelayConnectionManager 中继连接管理接口
type RelayConnectionManager interface {
	CloseStaleConnections(ctx context.Context) error
}

// DiscoveryAddressUpdater 发现地址更新接口
type DiscoveryAddressUpdater interface {
	UpdateAddrs(ctx context.Context) error
}

// TransportRebinder 传输重绑定接口
type TransportRebinder interface {
	Rebind(ctx context.Context) error
}

// DNSResetter DNS 重置接口
type DNSResetter interface {
	Reset(ctx context.Context) error
}

// EndpointStateResetter 端点状态重置接口
type EndpointStateResetter interface {
	ResetStates(ctx context.Context) error
}

// RealmNetworkNotifier Realm 网络通知接口
type RealmNetworkNotifier interface {
	NotifyNetworkChange(ctx context.Context, event NetworkChangeEvent) error
}

// ════════════════════════════════════════════════════════════════════════════
// ConnectionHealthMonitor 接口（连接健康监控）
// ════════════════════════════════════════════════════════════════════════════

// ConnectionHealth 连接健康状态
type ConnectionHealth int

const (
	// ConnectionHealthy 健康状态：所有连接正常
	ConnectionHealthy ConnectionHealth = iota

	// ConnectionDegraded 降级状态：部分连接失败
	ConnectionDegraded

	// ConnectionDown 断开状态：全部连接失败或检测到关键错误
	ConnectionDown

	// ConnectionRecovering 恢复中：正在尝试恢复连接
	ConnectionRecovering
)

// String 返回状态的字符串表示
func (s ConnectionHealth) String() string {
	switch s {
	case ConnectionHealthy:
		return "healthy"
	case ConnectionDegraded:
		return "degraded"
	case ConnectionDown:
		return "down"
	case ConnectionRecovering:
		return "recovering"
	default:
		return "unknown"
	}
}

// IsHealthy 检查是否处于健康状态
func (s ConnectionHealth) IsHealthy() bool {
	return s == ConnectionHealthy
}

// NeedsRecovery 检查是否需要恢复
func (s ConnectionHealth) NeedsRecovery() bool {
	return s == ConnectionDown || s == ConnectionDegraded
}

// StateChangeReason 状态变更原因
type StateChangeReason int

const (
	ReasonUnknown StateChangeReason = iota
	ReasonErrorThreshold
	ReasonCriticalError
	ReasonAllConnectionsLost
	ReasonRecoveryStarted
	ReasonRecoverySucceeded
	ReasonRecoveryFailed
	ReasonManualTrigger
	ReasonConnectionRestored
	ReasonNetworkChanged
	ReasonProbeSuccess
	ReasonProbeFailed
	ReasonProbePartialFailure
)

// String 返回原因的字符串表示
func (r StateChangeReason) String() string {
	names := []string{
		"unknown", "error_threshold", "critical_error", "all_connections_lost",
		"recovery_started", "recovery_succeeded", "recovery_failed", "manual_trigger",
		"connection_restored", "network_changed", "probe_success", "probe_failed",
		"probe_partial_failure",
	}
	if int(r) < len(names) {
		return names[r]
	}
	return "unknown"
}

// ConnectionHealthChange 连接健康状态变更事件
type ConnectionHealthChange struct {
	PreviousState ConnectionHealth
	CurrentState  ConnectionHealth
	Reason        StateChangeReason
	Timestamp     time.Time
	TriggerPeer   string
	TriggerError  error
}

// ConnectionHealthSnapshot 连接健康状态快照
type ConnectionHealthSnapshot struct {
	State            ConnectionHealth
	Timestamp        time.Time
	TotalPeers       int
	HealthyPeers     int
	FailingPeers     int
	LastError        error
	LastErrorTime    time.Time
	RecoveryAttempts int
	LastRecoveryTime time.Time
}

// ConnectionHealthMonitor 网络连接健康状态监控接口
//
// 实现位置：internal/core/recovery/netmon/
type ConnectionHealthMonitor interface {
	Start(ctx context.Context) error
	Stop() error
	OnSendError(peer string, err error)
	OnSendSuccess(peer string)
	GetState() ConnectionHealth
	GetSnapshot() ConnectionHealthSnapshot
	Subscribe() <-chan ConnectionHealthChange
	Unsubscribe(ch <-chan ConnectionHealthChange)
	TriggerRecoveryState()
	NotifyRecoverySuccess()
	NotifyRecoveryFailed(err error)
	Reset()
}

// ConnectionHealthMonitorConfig 连接健康监控配置
type ConnectionHealthMonitorConfig struct {
	ErrorThreshold        int
	ProbeInterval         time.Duration
	RecoveryProbeInterval time.Duration
	ErrorWindow           time.Duration
	CriticalErrors        []string
	MaxRecoveryAttempts   int
	StateChangeDebounce   time.Duration
	EnableAutoRecovery    bool
}

// DefaultConnectionHealthMonitorConfig 返回默认配置
func DefaultConnectionHealthMonitorConfig() ConnectionHealthMonitorConfig {
	return ConnectionHealthMonitorConfig{
		ErrorThreshold:        3,
		ProbeInterval:         30 * time.Second,
		RecoveryProbeInterval: 1 * time.Second,
		ErrorWindow:           1 * time.Minute,
		CriticalErrors:        []string{"network is unreachable", "no route to host", "connection refused", "host is down"},
		MaxRecoveryAttempts:   5,
		StateChangeDebounce:   500 * time.Millisecond,
		EnableAutoRecovery:    true,
	}
}

// ════════════════════════════════════════════════════════════════════════════
// RecoveryManager 接口
// ════════════════════════════════════════════════════════════════════════════

// ============================================================================
//                              恢复原因
// ============================================================================

// RecoveryReason 恢复原因
type RecoveryReason int

const (
	// RecoveryReasonUnknown 未知原因
	RecoveryReasonUnknown RecoveryReason = iota

	// RecoveryReasonNetworkUnreachable 网络不可达
	RecoveryReasonNetworkUnreachable

	// RecoveryReasonNoRoute 无路由
	RecoveryReasonNoRoute

	// RecoveryReasonConnectionRefused 连接被拒绝
	RecoveryReasonConnectionRefused

	// RecoveryReasonAllConnectionsLost 所有连接丢失
	RecoveryReasonAllConnectionsLost

	// RecoveryReasonErrorThreshold 错误达到阈值
	RecoveryReasonErrorThreshold

	// RecoveryReasonManualTrigger 手动触发
	RecoveryReasonManualTrigger

	// RecoveryReasonNetworkChange 网络变更
	RecoveryReasonNetworkChange
)

// String 返回原因的字符串表示
func (r RecoveryReason) String() string {
	switch r {
	case RecoveryReasonUnknown:
		return "unknown"
	case RecoveryReasonNetworkUnreachable:
		return "network_unreachable"
	case RecoveryReasonNoRoute:
		return "no_route"
	case RecoveryReasonConnectionRefused:
		return "connection_refused"
	case RecoveryReasonAllConnectionsLost:
		return "all_connections_lost"
	case RecoveryReasonErrorThreshold:
		return "error_threshold"
	case RecoveryReasonManualTrigger:
		return "manual_trigger"
	case RecoveryReasonNetworkChange:
		return "network_change"
	default:
		return "unknown"
	}
}

// NeedsRebind 检查此原因是否需要 rebind
func (r RecoveryReason) NeedsRebind() bool {
	switch r {
	case RecoveryReasonNetworkUnreachable, RecoveryReasonNoRoute, RecoveryReasonNetworkChange:
		return true
	default:
		return false
	}
}

// ============================================================================
//                              恢复结果
// ============================================================================

// RecoveryResult 恢复结果
type RecoveryResult struct {
	// Success 是否成功
	Success bool

	// Reason 恢复原因
	Reason RecoveryReason

	// Attempts 尝试次数
	Attempts int

	// RebindPerformed 是否执行了 rebind
	RebindPerformed bool

	// AddressesDiscovered 发现的新地址数
	AddressesDiscovered int

	// ConnectionsRestored 恢复的连接数
	ConnectionsRestored int

	// Duration 恢复耗时
	Duration time.Duration

	// Error 错误信息（如果失败）
	Error error
}

// IsSuccess 检查是否成功
func (r *RecoveryResult) IsSuccess() bool {
	return r.Success
}

// ============================================================================
//                              恢复管理接口
// ============================================================================

// RecoveryManager 网络恢复管理接口
//
// RecoveryManager 负责在网络故障时执行恢复操作：
// 1. Rebind 底层传输（重建 UDP socket）
// 2. 重新发现地址（STUN）
// 3. 重建关键连接
//
// 使用示例:
//
//	manager := recovery.NewManager(config)
//	manager.Start(ctx)
//
//	// 触发恢复
//	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkUnreachable)
//	if result.Success {
//	    log.Info("恢复成功")
//	}
type RecoveryManager interface {
	// ==================== 生命周期 ====================

	// Start 启动恢复管理器
	Start(ctx context.Context) error

	// Stop 停止恢复管理器
	Stop() error

	// ==================== 恢复触发 ====================

	// TriggerRecovery 触发恢复流程
	//
	// 恢复流程：
	// 1. 如果需要，执行 rebind
	// 2. 重新发现地址（STUN）
	// 3. 重建关键节点连接
	//
	// 返回恢复结果
	TriggerRecovery(ctx context.Context, reason RecoveryReason) *RecoveryResult

	// ==================== 状态查询 ====================

	// IsRecovering 检查是否正在恢复
	IsRecovering() bool

	// GetAttemptCount 获取当前尝试次数
	GetAttemptCount() int

	// ResetAttempts 重置尝试次数
	ResetAttempts()

	// ==================== 回调注册 ====================

	// OnRecoveryComplete 注册恢复完成回调
	OnRecoveryComplete(callback func(RecoveryResult))

	// ==================== 关键节点管理 ====================

	// SetCriticalPeers 设置关键节点列表
	//
	// 关键节点在恢复时会被优先重连。
	SetCriticalPeers(peers []string)

	// SetCriticalPeersWithAddrs 设置关键节点列表（带地址）
	//
	// 提供地址可以在恢复时使用直连，提高恢复成功率。
	SetCriticalPeersWithAddrs(peers []string, addrs []string)
}

// ============================================================================
//                              依赖接口
// ============================================================================

// Rebinder 可重绑定的传输接口
//
// 实现此接口的传输层可以在网络故障时重新绑定 socket。
type Rebinder interface {
	// Rebind 重新绑定传输层
	// 关闭现有的 socket 并重新创建
	Rebind(ctx context.Context) error

	// IsRebindNeeded 检查是否需要 rebind
	IsRebindNeeded() bool
}

// AddressDiscoverer 地址发现接口
//
// 实现此接口可以在恢复时重新发现外部地址。
type AddressDiscoverer interface {
	// DiscoverAddresses 发现外部地址
	DiscoverAddresses(ctx context.Context) error
}

// RecoveryConnector 恢复连接器接口
//
// 用于在恢复过程中重建连接。
type RecoveryConnector interface {
	// Connect 连接到节点
	Connect(ctx context.Context, peerID string) error

	// ConnectWithAddrs 使用地址连接到节点
	ConnectWithAddrs(ctx context.Context, peerID string, addrs []string) error

	// ConnectionCount 获取连接数
	ConnectionCount() int
}

// ============================================================================
//                              配置
// ============================================================================

// RecoveryConfig 恢复管理器配置
type RecoveryConfig struct {
	// MaxAttempts 最大恢复尝试次数
	// 默认值: 5
	MaxAttempts int

	// InitialBackoff 初始退避时间
	// 默认值: 1s
	InitialBackoff time.Duration

	// MaxBackoff 最大退避时间
	// 默认值: 30s
	MaxBackoff time.Duration

	// BackoffFactor 退避因子
	// 默认值: 1.5
	BackoffFactor float64

	// RecoveryTimeout 单次恢复超时
	// 默认值: 30s
	RecoveryTimeout time.Duration

	// RebindOnCriticalError 关键错误时是否 rebind
	// 默认值: true
	RebindOnCriticalError bool

	// RediscoverAddresses 恢复时是否重新发现地址
	// 默认值: true
	RediscoverAddresses bool
}

// DefaultRecoveryConfig 返回默认配置
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		MaxAttempts:           5,
		InitialBackoff:        1 * time.Second,
		MaxBackoff:            30 * time.Second,
		BackoffFactor:         1.5,
		RecoveryTimeout:       30 * time.Second,
		RebindOnCriticalError: true,
		RediscoverAddresses:   true,
	}
}
