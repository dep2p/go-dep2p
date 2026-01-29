// Package interfaces 定义 realm 模块内部接口
package interfaces

import (
	"context"
	"io"
	"time"
)

// ============================================================================
//                              Gateway 接口
// ============================================================================

// Gateway 域网关接口
type Gateway interface {
	// Relay 中继转发（被 routing 调用）
	Relay(ctx context.Context, req *RelayRequest) error

	// ServeRelay 启动中继服务监听
	ServeRelay(ctx context.Context) error

	// GetReachableNodes 查询当前可服务的节点列表
	GetReachableNodes() []string

	// ReportState 报告网关状态给 routing
	ReportState(ctx context.Context) (*GatewayState, error)

	// Start 启动网关
	Start(ctx context.Context) error

	// Stop 停止网关
	Stop(ctx context.Context) error

	// Close 关闭网关
	Close() error
}

// ============================================================================
//                              RelayService 接口
// ============================================================================

// RelayService 中继服务接口
type RelayService interface {
	// HandleRelayRequest 处理中继请求
	HandleRelayRequest(ctx context.Context, stream io.ReadWriteCloser) error

	// ForwardStream 双向流转发
	ForwardStream(src, dst io.ReadWriteCloser) error

	// NewSession 创建中继会话
	NewSession(req *RelayRequest) RelaySession

	// GetActiveSessions 获取活跃会话数
	GetActiveSessions() int
}

// ============================================================================
//                              RelaySession 接口
// ============================================================================

// RelaySession 中继会话接口
type RelaySession interface {
	// Transfer 执行双向转发
	Transfer(ctx context.Context, conn io.ReadWriteCloser) error

	// Close 关闭会话
	Close() error

	// GetStats 会话统计
	GetStats() *SessionStats

	// ID 会话ID
	ID() string
}

// ============================================================================
//                              ConnectionPool 接口
// ============================================================================

// ConnectionPool 连接池接口
type ConnectionPool interface {
	// Acquire 获取连接
	Acquire(ctx context.Context, peerID string) (io.ReadWriteCloser, error)

	// Release 释放连接
	Release(peerID string, conn io.ReadWriteCloser)

	// Remove 移除连接
	Remove(peerID string)

	// CleanupIdle 清理空闲连接
	CleanupIdle()

	// GetStats 连接池统计
	GetStats() *PoolStats

	// Close 关闭连接池
	Close() error
}

// ============================================================================
//                              BandwidthLimiter 接口
// ============================================================================

// BandwidthLimiter 带宽限流器接口
type BandwidthLimiter interface {
	// Acquire 获取流量配额
	Acquire(ctx context.Context, bytes int64) (*BandwidthToken, error)

	// Release 释放配额
	Release(token *BandwidthToken)

	// UpdateRate 动态调整速率
	UpdateRate(bytesPerSec int64)

	// GetStats 限流统计
	GetStats() *LimiterStats

	// Close 关闭限流器
	Close() error
}

// ============================================================================
//                              ProtocolValidator 接口
// ============================================================================

// ProtocolValidator 协议验证器接口
type ProtocolValidator interface {
	// ValidateProtocol 验证协议
	ValidateProtocol(protocol, realmID string) error

	// ExtractRealmID 提取 RealmID
	ExtractRealmID(protocol string) (string, error)

	// IsRealmProtocol 判断是否 Realm 协议
	IsRealmProtocol(protocol string) bool
}

// ============================================================================
//                              RouterAdapter 接口
// ============================================================================

// RouterAdapter Routing 协作适配器接口
type RouterAdapter interface {
	// RegisterWithRouter 注册到 Router
	RegisterWithRouter(gateway Gateway) error

	// ReportCapacity 定期报告容量
	ReportCapacity(ctx context.Context) error

	// UpdateReachable 更新可达节点
	UpdateReachable(peers []string)

	// OnRelayRequest 处理来自 Router 的请求
	OnRelayRequest(ctx context.Context, req *RelayRequest) error
}

// ============================================================================
//                              数据结构
// ============================================================================

// RelayRequest 中继请求
type RelayRequest struct {
	SourcePeerID string
	TargetPeerID string
	Protocol     string
	RealmID      string
	AuthProof    []byte
	Data         []byte
}

// RelayResponse 中继响应
type RelayResponse struct {
	Success     bool
	Error       string
	RelayNodeID string
}

// SessionStats 会话统计
type SessionStats struct {
	ID          string
	Source      string
	Target      string
	Protocol    string
	StartTime   time.Time
	BytesSent   int64
	BytesRecv   int64
	Duration    time.Duration
}

// PoolStats 连接池统计
type PoolStats struct {
	TotalConnections int
	ActiveConnections int
	IdleConnections  int
	AcquireCount     int64
	ReleaseCount     int64
	HitRate          float64
}

// LimiterStats 限流统计
type LimiterStats struct {
	Rate            int64
	Capacity        int64
	CurrentTokens   int64
	TotalAcquired   int64
	TotalReleased   int64
	ThrottledCount  int64
	AverageWaitTime time.Duration
}

// BandwidthToken 带宽令牌
type BandwidthToken struct {
	Bytes     int64
	Timestamp time.Time
}

// GatewayCapacity 网关容量
type GatewayCapacity struct {
	ActiveConnections  int
	AvailableBandwidth int64
	CPUUsage           float64
	ReachablePeers     []string
}

// GatewayMetrics 网关指标
type GatewayMetrics struct {
	RelayCount        int64
	RelaySuccess      int64
	RelayFailed       int64
	BytesTransferred  int64
	ActiveConnections int
	AvgLatency        time.Duration
	BandwidthUsage    int64
}
