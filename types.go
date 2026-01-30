package dep2p

import (
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              节点状态
// ════════════════════════════════════════════════════════════════════════════

// NodeState 节点状态
//
// 表示节点在生命周期中的当前阶段。
type NodeState int

const (
	// StateIdle 空闲状态（已创建，未启动）
	StateIdle NodeState = iota

	// StateInitializing 初始化中（Fx App 启动中）
	StateInitializing

	// StateStarting 启动中（等待组件就绪）
	StateStarting

	// StateRunning 运行中（正常工作状态）
	StateRunning

	// StateStopping 停止中（正在关闭组件）
	StateStopping

	// StateStopped 已停止（可重新启动）
	StateStopped
)

// String 返回状态的字符串表示
func (s NodeState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateInitializing:
		return "initializing"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// ════════════════════════════════════════════════════════════════════════════
//                              带宽统计
// ════════════════════════════════════════════════════════════════════════════

// BandwidthSnapshot 带宽统计快照
//
// 提供节点带宽使用的快照数据。
type BandwidthSnapshot struct {
	// TotalIn 总入站字节数
	TotalIn int64 `json:"total_in"`

	// TotalOut 总出站字节数
	TotalOut int64 `json:"total_out"`

	// RateIn 入站速率 (bytes/sec)
	RateIn float64 `json:"rate_in"`

	// RateOut 出站速率 (bytes/sec)
	RateOut float64 `json:"rate_out"`
}

// TotalBytes 返回总字节数（入+出）
func (s BandwidthSnapshot) TotalBytes() int64 {
	return s.TotalIn + s.TotalOut
}

// TotalRate 返回总速率（入+出）
func (s BandwidthSnapshot) TotalRate() float64 {
	return s.RateIn + s.RateOut
}

// ════════════════════════════════════════════════════════════════════════════
//                              连接信息
// ════════════════════════════════════════════════════════════════════════════

// PeerConnectionInfo 节点连接信息
type PeerConnectionInfo struct {
	PeerID      string    // 节点 ID
	Addrs       []string  // 连接地址
	Direction   string    // 连接方向: "inbound" / "outbound" / "unknown"
	NumStreams  int       // 活跃流数量
	NumConns    int       // 连接数量（一个节点可能有多个连接）
	ConnectedAt time.Time // 连接建立时间（如果可用）
}

// ════════════════════════════════════════════════════════════════════════════
//                              连接事件
// ════════════════════════════════════════════════════════════════════════════

// PeerConnectedEvent 节点连接事件
//
// 当有新节点连接时触发。
type PeerConnectedEvent struct {
	// PeerID 远端节点 ID
	PeerID string

	// Addrs 远端节点地址列表
	Addrs []string

	// Direction 连接方向: "inbound" 或 "outbound"
	Direction string

	// NumConns 与该节点的连接数
	NumConns int

	// Timestamp 事件时间戳
	Timestamp time.Time
}

// PeerDisconnectedEvent 节点断开事件
//
// 当节点断开连接时触发。
type PeerDisconnectedEvent struct {
	// PeerID 远端节点 ID
	PeerID string

	// NumConns 与该节点的剩余连接数（通常为 0）
	NumConns int

	// Reason 断开原因: "graceful", "timeout", "error", "local", "unknown"
	Reason string

	// Error 错误信息（仅 Reason="error" 时有值）
	Error string

	// Timestamp 事件时间戳
	Timestamp time.Time
}

// ════════════════════════════════════════════════════════════════════════════
//                              网络诊断
// ════════════════════════════════════════════════════════════════════════════

// NetworkDiagnosticReport 网络诊断报告（用户友好类型）
type NetworkDiagnosticReport struct {
	// IPv4 相关
	IPv4Available bool   `json:"ipv4_available"`
	IPv4GlobalIP  string `json:"ipv4_global_ip,omitempty"`
	IPv4Port      int    `json:"ipv4_port,omitempty"`

	// IPv6 相关
	IPv6Available bool   `json:"ipv6_available"`
	IPv6GlobalIP  string `json:"ipv6_global_ip,omitempty"`

	// NAT 类型
	NATType string `json:"nat_type"`

	// 端口映射可用性
	UPnPAvailable   bool `json:"upnp_available"`
	NATPMPAvailable bool `json:"natpmp_available"`
	PCPAvailable    bool `json:"pcp_available"`

	// 强制门户
	CaptivePortal bool `json:"captive_portal"`

	// 中继延迟（毫秒）
	RelayLatencies map[string]int64 `json:"relay_latencies,omitempty"`

	// 生成耗时（毫秒）
	Duration int64 `json:"duration_ms"`
}

// ════════════════════════════════════════════════════════════════════════════
//                              种子节点
// ════════════════════════════════════════════════════════════════════════════

// SeedRecord 种子节点记录（用户友好类型）
type SeedRecord struct {
	// ID 节点 ID
	ID string `json:"id"`

	// Addrs 节点地址列表
	Addrs []string `json:"addrs"`

	// LastSeen 最后活跃时间
	LastSeen time.Time `json:"last_seen"`

	// LastPong 最后 Pong 时间
	LastPong time.Time `json:"last_pong"`
}

// ════════════════════════════════════════════════════════════════════════════
//                              自省服务
// ════════════════════════════════════════════════════════════════════════════

// IntrospectInfo 自省信息（用户友好类型）
type IntrospectInfo struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`

	// Addr 监听地址
	Addr string `json:"addr,omitempty"`

	// Endpoints 可用端点列表
	Endpoints []string `json:"endpoints,omitempty"`
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型别名（从 pkg/interfaces 导出）
// ════════════════════════════════════════════════════════════════════════════

// ReadyLevel 就绪级别
type ReadyLevel = pkgif.ReadyLevel

// 就绪级别常量
const (
	ReadyLevelCreated    = pkgif.ReadyLevelCreated
	ReadyLevelNetwork    = pkgif.ReadyLevelNetwork
	ReadyLevelDiscovered = pkgif.ReadyLevelDiscovered
	ReadyLevelReachable  = pkgif.ReadyLevelReachable
	ReadyLevelRealmReady = pkgif.ReadyLevelRealmReady
)

// HealthState 健康状态
type HealthState = pkgif.HealthState

// 健康状态常量
const (
	HealthStateHealthy   = pkgif.HealthStateHealthy
	HealthStateDegraded  = pkgif.HealthStateDegraded
	HealthStateUnhealthy = pkgif.HealthStateUnhealthy
)

// HealthStatus 健康状态详情
type HealthStatus = pkgif.HealthStatus

// NetworkChangeEvent 网络变化事件
type NetworkChangeEvent = pkgif.NetworkChangeEvent

// BootstrapStats Bootstrap 统计信息
type BootstrapStats = pkgif.BootstrapStats

// RelayStats Relay 统计信息
type RelayStats = pkgif.RelayStats

// Direction 连接方向
type Direction = pkgif.Direction

// 连接方向常量
const (
	DirInbound  = pkgif.DirInbound
	DirOutbound = pkgif.DirOutbound
	DirUnknown  = pkgif.DirUnknown
)

// Connectedness 连接状态
type Connectedness = pkgif.Connectedness

// 连接状态常量
const (
	NotConnected  = pkgif.NotConnected
	Connected     = pkgif.Connected
	CanConnect    = pkgif.CanConnect
	CannotConnect = pkgif.CannotConnect
)
