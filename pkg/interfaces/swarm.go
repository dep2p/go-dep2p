// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Swarm 组件接口，对应 internal/core/swarm/ 实现。
// 包括：Swarm（连接群管理）、BandwidthCounter（带宽统计）、PathHealthManager（路径健康）
package interfaces

import (
	"context"
	"time"
)

// InboundStreamHandler 入站流处理函数类型
//
// 当有新的入站流时被调用，负责协议协商和路由。
type InboundStreamHandler func(stream Stream)

// Swarm 定义连接群管理接口
//
// Swarm 管理所有出站和入站连接，提供多路复用的连接池。
type Swarm interface {
	// LocalPeer 返回本地节点 ID
	LocalPeer() string

	// Listen 监听指定地址
	//
	// addrs 是 multiaddr 格式的地址列表，例如：
	//   - "/ip4/0.0.0.0/udp/4001/quic-v1"
	//   - "/ip4/0.0.0.0/tcp/4001"
	Listen(addrs ...string) error

	// ListenAddrs 返回所有监听地址
	ListenAddrs() []string

	// Peers 返回所有已连接的节点 ID
	Peers() []string

	// Conns 返回所有活跃连接
	Conns() []Connection

	// ConnsToPeer 返回到指定节点的所有连接
	ConnsToPeer(peerID string) []Connection

	// Connectedness 返回与指定节点的连接状态
	Connectedness(peerID string) Connectedness

	// DialPeer 拨号连接到指定节点
	DialPeer(ctx context.Context, peerID string) (Connection, error)

	// ClosePeer 关闭与指定节点的所有连接
	ClosePeer(peerID string) error

	// NewStream 创建到指定节点的新流（默认优先级）
	NewStream(ctx context.Context, peerID string) (Stream, error)

	// NewStreamWithPriority 创建到指定节点的新流（指定优先级）(v1.2 新增)
	//
	// 允许指定流优先级。在 QUIC 连接上，优先级会传递给底层传输层。
	// 在 TCP 连接上，优先级会被忽略（优雅降级）。
	//
	// 参数:
	//   - ctx: 上下文
	//   - peerID: 目标节点 ID
	//   - priority: 流优先级 (0=Critical, 1=High, 2=Normal, 3=Low)
	NewStreamWithPriority(ctx context.Context, peerID string, priority int) (Stream, error)

	// SetInboundStreamHandler 设置入站流处理器
	//
	// 由 Host 在启动时调用，设置入站流的协议协商和路由处理逻辑。
	// 当 Swarm 接受新连接后，会为每个入站流调用此处理器。
	SetInboundStreamHandler(handler InboundStreamHandler)

	// AddInboundConnection 添加入站连接到连接池
	//
	// 用于添加外部创建的连接（如中继电路）到 Swarm 的连接池。
	// 这允许其他代码通过 ConnsToPeer/NewStream 使用这些连接。
	//
	// v2.0 新增：支持 RelayCircuit 集成
	AddInboundConnection(conn Connection)

	// Notify 注册连接事件通知
	Notify(notifier SwarmNotifier)

	// Close 关闭 Swarm
	Close() error
}

// Connectedness 表示与节点的连接状态
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

// SwarmNotifier 定义 Swarm 事件通知接口
type SwarmNotifier interface {
	// Connected 当建立新连接时调用
	Connected(conn Connection)

	// Disconnected 当连接断开时调用
	Disconnected(conn Connection)
}

// ════════════════════════════════════════════════════════════════════════════
// BandwidthCounter 接口（Swarm 子能力）
// 实现位置：internal/core/swarm/bandwidth/
// ════════════════════════════════════════════════════════════════════════════

// BandwidthStats 带宽统计快照
type BandwidthStats struct {
	TotalIn  int64   // 总入站字节数
	TotalOut int64   // 总出站字节数
	RateIn   float64 // 入站速率 (bytes/sec)
	RateOut  float64 // 出站速率 (bytes/sec)
}

// TotalBytes 返回总字节数（入+出）
func (s BandwidthStats) TotalBytes() int64 { return s.TotalIn + s.TotalOut }

// TotalRate 返回总速率（入+出）
func (s BandwidthStats) TotalRate() float64 { return s.RateIn + s.RateOut }

// BandwidthCounter 带宽计数器接口
type BandwidthCounter interface {
	LogSentMessage(size int64)
	LogRecvMessage(size int64)
	LogSentStream(size int64, proto string, peer string)
	LogRecvStream(size int64, proto string, peer string)
	GetTotals() BandwidthStats
	GetForPeer(peer string) BandwidthStats
	GetForProtocol(proto string) BandwidthStats
	GetByPeer() map[string]BandwidthStats
	GetByProtocol() map[string]BandwidthStats
	Reset()
	TrimIdle(since time.Time)
}

// BandwidthConfig 带宽统计配置
type BandwidthConfig struct {
	Enabled         bool
	TrackByPeer     bool
	TrackByProtocol bool
	IdleTimeout     time.Duration
	TrimInterval    time.Duration
}

// DefaultBandwidthConfig 返回默认配置
func DefaultBandwidthConfig() BandwidthConfig {
	return BandwidthConfig{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
		IdleTimeout:     time.Hour,
		TrimInterval:    10 * time.Minute,
	}
}

// ════════════════════════════════════════════════════════════════════════════
// PathHealthManager 接口（Swarm 子能力）
// 实现位置：internal/core/swarm/pathhealth/
// ════════════════════════════════════════════════════════════════════════════

// PathID 路径标识
type PathID string

// PathType 路径类型
type PathType int

const (
	PathTypeDirect PathType = iota
	PathTypeRelay
)

func (t PathType) String() string {
	if t == PathTypeDirect {
		return "direct"
	}
	return "relay"
}

// PathState 路径状态
type PathState int

const (
	PathStateUnknown PathState = iota
	PathStateHealthy
	PathStateSuspect
	PathStateDead
)

func (s PathState) String() string {
	names := []string{"unknown", "healthy", "suspect", "dead"}
	if int(s) < len(names) {
		return names[s]
	}
	return "unknown"
}

func (s PathState) IsUsable() bool { return s == PathStateHealthy || s == PathStateSuspect }

// PathStats 路径统计信息
type PathStats struct {
	PathID              PathID
	PathType            PathType
	State               PathState
	EWMARTT             time.Duration
	LastRTT             time.Duration
	MinRTT              time.Duration
	MaxRTT              time.Duration
	SuccessCount        int64
	FailureCount        int64
	ConsecutiveFailures int
	LastSeen            time.Time
	FirstSeen           time.Time
	Score               float64
}

func (s *PathStats) SuccessRate() float64 {
	total := s.SuccessCount + s.FailureCount
	if total == 0 {
		return 0
	}
	return float64(s.SuccessCount) / float64(total)
}

// SwitchReason 切换原因
type SwitchReason int

const (
	SwitchReasonNone SwitchReason = iota
	SwitchReasonCurrentDead
	SwitchReasonBetterPath
	SwitchReasonManual
	SwitchReasonNetworkChange
)

func (r SwitchReason) String() string {
	names := []string{"none", "current_dead", "better_path", "manual", "network_change"}
	if int(r) < len(names) {
		return names[r]
	}
	return "unknown"
}

// SwitchDecision 路径切换决策
type SwitchDecision struct {
	ShouldSwitch bool
	Reason       SwitchReason
	TargetPath   PathID
	CurrentScore float64
	TargetScore  float64
}

// PathHealthManager 路径健康管理接口
type PathHealthManager interface {
	Start(ctx context.Context) error
	Stop() error
	ObservePeerAddrs(peerID string, addrs []string)
	ReportProbe(peerID string, addr string, rtt time.Duration, err error)
	ReportHandshake(peerID string, addr string, rtt time.Duration, err error)
	GetPathStats(peerID string, addr string) *PathStats
	GetPeerPaths(peerID string) []*PathStats
	GetBestPath(peerID string) *PathStats
	RankAddrs(peerID string, addrs []string) []string
	ShouldSwitch(peerID string, currentPath PathID) SwitchDecision
	OnNetworkChange(ctx context.Context, reason string)
	RemovePeer(peerID string)
	Reset()
}

// PathHealthConfig 路径健康管理配置
type PathHealthConfig struct {
	EWMAAlpha            float64
	HealthyRTTThreshold  time.Duration
	SuspectRTTThreshold  time.Duration
	DeadFailureThreshold int
	ProbeInterval        time.Duration
	SuspectProbeInterval time.Duration
	SwitchHysteresis     float64
	StabilityWindow      time.Duration
	DirectPathBonus      float64
	MaxPathsPerPeer      int
	PathExpiry           time.Duration
}

// DefaultPathHealthConfig 返回默认配置
func DefaultPathHealthConfig() PathHealthConfig {
	return PathHealthConfig{
		EWMAAlpha:            0.2,
		HealthyRTTThreshold:  200 * time.Millisecond,
		SuspectRTTThreshold:  500 * time.Millisecond,
		DeadFailureThreshold: 3,
		ProbeInterval:        30 * time.Second,
		SuspectProbeInterval: 5 * time.Second,
		SwitchHysteresis:     0.2,
		StabilityWindow:      5 * time.Second,
		DirectPathBonus:      0.8,
		MaxPathsPerPeer:      10,
		PathExpiry:           10 * time.Minute,
	}
}
