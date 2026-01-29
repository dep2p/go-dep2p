// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 ConnMgr 组件接口，对应 internal/core/connmgr/ 实现。
// 包括：ConnManager（连接管理）、ConnGater（连接门控）、JitterTolerance（抖动容忍）
package interfaces

import (
	"context"
	"time"
)

// ConnManager 定义连接管理器接口
//
// ConnManager 负责连接的生命周期管理、优先级和垃圾回收。
type ConnManager interface {
	// TagPeer 为节点添加标签（影响优先级）
	TagPeer(peerID string, tag string, weight int)

	// UntagPeer 移除节点标签
	UntagPeer(peerID string, tag string)

	// UpsertTag 更新或插入节点标签
	UpsertTag(peerID string, tag string, upsert func(int) int)

	// GetTagInfo 获取节点的标签信息
	GetTagInfo(peerID string) *TagInfo

	// TrimOpenConns 裁剪连接到目标数量
	TrimOpenConns(ctx context.Context)

	// Notifee 返回连接通知接口
	Notifee() SwarmNotifier

	// Protect 保护节点连接不被裁剪
	Protect(peerID string, tag string)

	// Unprotect 取消节点保护
	Unprotect(peerID string, tag string) bool

	// IsProtected 检查节点是否受保护
	IsProtected(peerID string, tag string) bool

	// ==================== 查询（新增）====================

	// ConnCount 返回当前连接数
	ConnCount() int

	// DialedConnCount 返回当前出站连接数
	DialedConnCount() int

	// InboundConnCount 返回当前入站连接数
	InboundConnCount() int

	// ==================== 水位线（新增）====================

	// SetLimits 设置水位线限制
	//
	// low: 低水位线（目标连接数）
	// high: 高水位线（触发裁剪）
	SetLimits(low, high int)

	// GetLimits 获取水位线限制
	//
	// 返回 (low, high)
	GetLimits() (low, high int)

	// ==================== 裁剪触发（新增）====================

	// TriggerTrim 手动触发裁剪
	//
	// 立即执行一次连接裁剪，不等待定时器触发
	TriggerTrim()

	// ==================== 消息速率追踪（msgrate 集成）====================

	// UpdatePeerRate 更新节点消息速率测量结果
	//
	// 参数:
	//   - peerID: 节点 ID
	//   - kind: 消息类型（自定义，如 1=Messaging, 2=PubSub 等）
	//   - elapsed: 请求耗时
	//   - items: 处理的消息数量（0 表示失败）
	UpdatePeerRate(peerID string, kind uint64, elapsed time.Duration, items int)

	// GetPeerCapacity 获取节点在目标 RTT 内可处理的消息数量
	//
	// 参数:
	//   - peerID: 节点 ID
	//   - kind: 消息类型
	//   - targetRTT: 目标往返时间
	//
	// 返回值:
	//   - 节点可处理的消息数量估计
	GetPeerCapacity(peerID string, kind uint64, targetRTT time.Duration) int

	// GetTargetRTT 获取当前目标 RTT
	GetTargetRTT() time.Duration

	// Close 关闭连接管理器
	Close() error
}

// TagInfo 节点标签信息
type TagInfo struct {
	// FirstSeen 首次发现时间
	FirstSeen time.Time

	// Value 标签权重总和
	Value int

	// Tags 标签映射
	Tags map[string]int

	// Conns 连接数量
	Conns int
}

// ConnGater 定义连接门控接口
//
// ConnGater 实现黑白名单和速率限制等连接控制策略。
type ConnGater interface {
	// InterceptPeerDial 在拨号前检查是否允许连接到目标节点
	InterceptPeerDial(peerID string) bool

	// InterceptAddrDial 在拨号前检查是否允许连接到目标地址
	InterceptAddrDial(peerID string, addr string) bool

	// InterceptAccept 在接受连接前检查是否允许
	InterceptAccept(conn Connection) bool

	// InterceptSecured 在安全握手后检查是否允许
	InterceptSecured(dir Direction, peerID string, conn Connection) bool

	// InterceptUpgraded 在连接升级后检查是否允许
	InterceptUpgraded(conn Connection) (bool, error)
}

// ════════════════════════════════════════════════════════════════════════════
// JitterTolerance 接口（ConnMgr 子能力）
// 实现位置：internal/core/connmgr/jitter.go
// ════════════════════════════════════════════════════════════════════════════

// JitterState 抖动状态
type JitterState int

const (
	StateConnected    JitterState = iota // 正常连接状态
	StateDisconnected                    // 断开状态（等待重连）
	StateReconnecting                    // 重连中
	StateHeld                            // 保持状态（重连失败后等待）
	StateRemoved                         // 已移除（超过状态保持时间）
)

func (s JitterState) String() string {
	names := []string{"connected", "disconnected", "reconnecting", "held", "removed"}
	if int(s) < len(names) {
		return names[s]
	}
	return "unknown"
}

// JitterStats 抖动统计
type JitterStats struct {
	TotalDisconnected      int // 当前断开连接的节点数
	TotalReconnectAttempts int // 总重连尝试次数
	Reconnecting           int // 重连中的节点数
	Held                   int // 处于保持状态的节点数
}

// ReconnectCallback 重连回调函数
type ReconnectCallback func(ctx context.Context, peerID string) error

// StateChangeCallback 状态变更回调函数
type StateChangeCallback func(peerID string, state JitterState)

// JitterConfig 抖动容忍配置
type JitterConfig struct {
	Enabled               bool          // 是否启用
	ToleranceWindow       time.Duration // 容忍窗口时间
	StateHoldTime         time.Duration // 状态保持时间
	ReconnectEnabled      bool          // 是否启用自动重连
	InitialReconnectDelay time.Duration // 初始重连延迟
	MaxReconnectDelay     time.Duration // 最大重连延迟
	MaxReconnectAttempts  int           // 最大重连次数
	BackoffMultiplier     float64       // 退避乘数
}

// DefaultJitterConfig 返回默认配置
func DefaultJitterConfig() JitterConfig {
	return JitterConfig{
		Enabled:               true,
		ToleranceWindow:       5 * time.Second,
		StateHoldTime:         30 * time.Second,
		ReconnectEnabled:      true,
		InitialReconnectDelay: 1 * time.Second,
		MaxReconnectDelay:     60 * time.Second,
		MaxReconnectAttempts:  5,
		BackoffMultiplier:     2.0,
	}
}
