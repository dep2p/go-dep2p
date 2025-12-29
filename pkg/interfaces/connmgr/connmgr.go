// Package connmgr 定义连接管理相关接口
//
// 连接管理模块负责控制节点连接数量、保护重要连接、防止资源耗尽，包括：
// - 连接数量控制（水位线机制）
// - 重要连接保护
// - 智能裁剪策略
// - 连接过滤
package connmgr

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              ConnectionManager 接口
// ============================================================================

// ConnectionManager 连接管理器接口
//
// 负责管理节点的连接生命周期，确保系统资源在可控范围内，同时保持网络连通性。
// 使用水位线机制控制连接数量，支持保护重要连接不被裁剪。
type ConnectionManager interface {
	// ==================== 连接通知 ====================

	// NotifyConnected 通知新连接建立
	//
	// 当新连接建立时调用，连接管理器会检查是否需要触发裁剪。
	NotifyConnected(conn endpoint.Connection)

	// NotifyDisconnected 通知连接断开
	//
	// 当连接断开时调用，更新连接计数。
	NotifyDisconnected(nodeID types.NodeID)

	// ==================== 保护机制 ====================

	// Protect 保护连接不被裁剪
	//
	// 使用 tag 标记保护原因，同一连接可以有多个保护标签。
	// 只有所有标签都被移除后，连接才会失去保护。
	//
	// 典型的 tag 包括：
	// - "bootstrap": Bootstrap 节点
	// - "validator": 验证者节点
	// - "relay": 中继节点
	// - "dht": DHT 邻居
	// - "realm:xxx": 特定 Realm 的核心邻居
	Protect(nodeID types.NodeID, tag string)

	// Unprotect 移除保护标签
	//
	// 移除指定的保护标签。如果节点没有剩余的保护标签，
	// 则在下次裁剪时可能被关闭。
	Unprotect(nodeID types.NodeID, tag string)

	// IsProtected 检查节点是否受保护
	IsProtected(nodeID types.NodeID) bool

	// TagsForPeer 返回节点的所有保护标签
	TagsForPeer(nodeID types.NodeID) []string

	// ==================== 查询 ====================

	// ConnCount 返回当前连接数
	ConnCount() int

	// ConnCountByTag 返回指定标签的连接数
	ConnCountByTag(tag string) int

	// GetConnInfo 获取连接信息
	GetConnInfo(nodeID types.NodeID) (ConnectionInfo, bool)

	// AllConnInfo 返回所有连接信息
	AllConnInfo() []ConnectionInfo

	// ==================== 过滤 ====================

	// AllowConnection 检查是否允许新连接
	//
	// 在连接建立早期调用，根据当前连接数和配置决定是否接受连接。
	// direction 指定连接方向（入站/出站）。
	AllowConnection(nodeID types.NodeID, direction types.Direction) bool

	// SetConnectionFilter 设置连接过滤器
	//
	// 过滤器在连接建立前被调用，可以拒绝特定节点的连接。
	SetConnectionFilter(filter ConnectionFilter)

	// ==================== 配置 ====================

	// SetLimits 设置水位线限制
	SetLimits(low, high int)

	// GetLimits 获取水位线限制
	GetLimits() (low, high int)

	// ==================== 生命周期 ====================

	// Start 启动连接管理器
	//
	// 启动后台任务（定期检查、裁剪等）。
	Start(ctx context.Context) error

	// Close 关闭连接管理器
	Close() error

	// TriggerTrim 手动触发裁剪
	//
	// 强制执行一次裁剪，用于资源紧张时主动释放连接。
	TriggerTrim()
}

// ============================================================================
//                              ConnectionInfo 类型别名
// ============================================================================

// ConnectionInfo 连接信息（类型别名，实际定义在 types 包）
type ConnectionInfo = types.ConnectionInfo

// ============================================================================
//                              ConnectionFilter 接口
// ============================================================================

// ConnectionFilter 连接过滤器接口
//
// 在连接建立早期决定是否接受连接。
type ConnectionFilter interface {
	// Allow 检查是否允许连接
	//
	// nodeID: 对端节点 ID
	// direction: 连接方向
	// 返回 true 表示允许连接，false 表示拒绝
	Allow(nodeID types.NodeID, direction types.Direction) bool
}

// ConnectionFilterFunc 函数类型的连接过滤器
type ConnectionFilterFunc func(nodeID types.NodeID, direction types.Direction) bool

// Allow 实现 ConnectionFilter 接口
func (f ConnectionFilterFunc) Allow(nodeID types.NodeID, direction types.Direction) bool {
	return f(nodeID, direction)
}

// ============================================================================
//                              TrimStrategy（v1.1 已删除）
// ============================================================================

// 注意：TrimStrategy 接口已删除（v1.1 清理）。
// 原因：无外部使用，自定义裁剪策略作为"扩展点"不承诺支持。
// 裁剪逻辑由 internal/core/connmgr 内部实现（multi-factor scoring）。

// ============================================================================
//                              预定义保护标签
// ============================================================================

// 保护标签常量
const (
	// TagBootstrap Bootstrap 节点
	TagBootstrap = "bootstrap"

	// TagValidator 验证者节点
	TagValidator = "validator"

	// TagRelay 中继节点
	TagRelay = "relay"

	// TagDHT DHT 邻居
	TagDHT = "dht"

	// TagMDNS mDNS 发现的本地节点
	TagMDNS = "mdns"

	// TagActive 活跃通信节点
	TagActive = "active"

	// TagPersistent 持久连接（长期合作节点）
	TagPersistent = "persistent"
)

// ============================================================================
//                              事件
// ============================================================================

// 连接管理事件类型
const (
	// EventConnManagerTrimStarted 裁剪开始
	EventConnManagerTrimStarted = "connmgr.trim_started"

	// EventConnManagerTrimCompleted 裁剪完成
	EventConnManagerTrimCompleted = "connmgr.trim_completed"

	// EventConnManagerLimitReached 达到连接上限
	EventConnManagerLimitReached = "connmgr.limit_reached"
)

// TrimStartedEvent 裁剪开始事件
type TrimStartedEvent struct {
	CurrentCount int
	TargetCount  int
}

// Type 返回事件类型
func (e TrimStartedEvent) Type() string {
	return EventConnManagerTrimStarted
}

// TrimCompletedEvent 裁剪完成事件
type TrimCompletedEvent struct {
	ClosedCount  int
	CurrentCount int
	Duration     time.Duration
}

// Type 返回事件类型
func (e TrimCompletedEvent) Type() string {
	return EventConnManagerTrimCompleted
}

// LimitReachedEvent 连接限制达到事件
type LimitReachedEvent struct {
	CurrentCount int
	Limit        int
	RejectedPeer types.NodeID
}

// Type 返回事件类型
func (e LimitReachedEvent) Type() string {
	return EventConnManagerLimitReached
}

// ============================================================================
//                              配置
// ============================================================================

// Config 连接管理器配置
type Config struct {
	// LowWater 低水位线
	//
	// 当连接数低于此值时停止裁剪。
	// 默认值: 50
	LowWater int

	// HighWater 高水位线
	//
	// 当连接数超过此值时触发裁剪。
	// 默认值: 100
	HighWater int

	// EmergencyWater 紧急水位线
	//
	// 当连接数超过此值时拒绝所有新连接。
	// 默认值: 150
	EmergencyWater int

	// GracePeriod 新连接保护期
	//
	// 新建立的连接在此期间内不会被裁剪。
	// 默认值: 1 分钟
	GracePeriod time.Duration

	// IdleTimeout 空闲超时
	//
	// 超过此时间没有活动的连接优先被裁剪。
	// 默认值: 5 分钟
	IdleTimeout time.Duration

	// TrimInterval 裁剪检查间隔
	//
	// 后台定期检查是否需要裁剪的间隔。
	// 默认值: 1 分钟
	TrimInterval time.Duration

	// EnableMetrics 启用指标收集
	EnableMetrics bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		LowWater:       50,
		HighWater:      100,
		EmergencyWater: 150,
		GracePeriod:    time.Minute,
		IdleTimeout:    5 * time.Minute,
		TrimInterval:   time.Minute,
		EnableMetrics:  false,
	}
}

// MobileConfig 返回移动端配置
func MobileConfig() Config {
	return Config{
		LowWater:       20,
		HighWater:      50,
		EmergencyWater: 70,
		GracePeriod:    30 * time.Second,
		IdleTimeout:    2 * time.Minute,
		TrimInterval:   30 * time.Second,
		EnableMetrics:  false,
	}
}

// ServerConfig 返回服务器配置
func ServerConfig() Config {
	return Config{
		LowWater:       200,
		HighWater:      500,
		EmergencyWater: 700,
		GracePeriod:    2 * time.Minute,
		IdleTimeout:    10 * time.Minute,
		TrimInterval:   2 * time.Minute,
		EnableMetrics:  true,
	}
}
