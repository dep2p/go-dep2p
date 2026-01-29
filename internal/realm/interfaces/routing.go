// Package interfaces 定义 realm 模块内部接口
package interfaces

import (
	"context"
	"time"
)

// ============================================================================
//                              路由器接口
// ============================================================================

// Router 域内路由器接口
type Router interface {
	// FindRoute 查找到目标节点的路由
	FindRoute(ctx context.Context, targetPeerID string) (*Route, error)

	// FindRoutes 查找多条路由（用于负载均衡）
	FindRoutes(ctx context.Context, targetPeerID string, count int) ([]*Route, error)

	// SelectBestRoute 选择最佳路由
	SelectBestRoute(ctx context.Context, routes []*Route, policy RoutingPolicy) (*Route, error)

	// InvalidateRoute 使路由失效
	InvalidateRoute(peerID string)

	// GetRouteTable 获取路由表
	GetRouteTable() RouteTable

	// Start 启动路由器
	Start(ctx context.Context) error

	// Stop 停止路由器
	Stop(ctx context.Context) error

	// Close 关闭路由器
	Close() error
}

// ============================================================================
//                              路由表接口
// ============================================================================

// RouteTable 路由表接口
type RouteTable interface {
	// AddNode 添加节点
	AddNode(node *RouteNode) error

	// RemoveNode 移除节点
	RemoveNode(peerID string) error

	// GetNode 获取节点
	GetNode(peerID string) (*RouteNode, error)

	// NearestPeers 返回最近的 K 个节点
	NearestPeers(targetID string, count int) []*RouteNode

	// Size 返回路由表大小
	Size() int

	// Update 更新节点信息
	Update(peerID string, latency time.Duration) error

	// GetAllNodes 获取所有节点
	GetAllNodes() []*RouteNode
}

// ============================================================================
//                              路径查找器接口
// ============================================================================

// PathFinder 路径查找器接口
type PathFinder interface {
	// FindShortestPath 查找最短路径
	FindShortestPath(ctx context.Context, source, target string) (*Path, error)

	// FindMultiplePaths 查找多条路径
	FindMultiplePaths(ctx context.Context, source, target string, count int) ([]*Path, error)

	// ScorePath 评分路径
	ScorePath(path *Path) float64

	// CachePath 缓存路径
	CachePath(path *Path)

	// InvalidatePath 使路径失效
	InvalidatePath(target string)
}

// ============================================================================
//                              负载均衡器接口
// ============================================================================

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	// SelectNode 选择节点（基于负载）
	SelectNode(ctx context.Context, candidates []*RouteNode) (*RouteNode, error)

	// ReportLoad 报告节点负载
	ReportLoad(peerID string, load *NodeLoad) error

	// GetLoad 获取节点负载
	GetLoad(peerID string) (*NodeLoad, error)

	// IsOverloaded 检查节点是否过载
	IsOverloaded(peerID string) bool

	// GetStats 获取负载统计
	GetStats() *LoadBalancerStats
}

// ============================================================================
//                              延迟探测器接口
// ============================================================================

// LatencyProber 延迟探测器接口
type LatencyProber interface {
	// MeasureLatency 测量延迟
	MeasureLatency(ctx context.Context, peerID string) (time.Duration, error)

	// GetLatency 获取缓存的延迟
	GetLatency(peerID string) (time.Duration, bool)

	// RecordLatency 记录延迟（被动测量）
	RecordLatency(peerID string, latency time.Duration)

	// GetStatistics 获取延迟统计
	GetStatistics(peerID string) *LatencyStats

	// Start 启动探测器
	Start(ctx context.Context) error

	// Stop 停止探测器
	Stop(ctx context.Context) error
}

// ============================================================================
//                              Gateway 协作接口
// ============================================================================

// GatewayAdapter Gateway 协作适配器接口
type GatewayAdapter interface {
	// RequestRelay 请求中继
	RequestRelay(ctx context.Context, targetPeerID string, data []byte) error

	// QueryReachable 查询可达节点
	QueryReachable(ctx context.Context) ([]string, error)

	// SyncState 同步状态
	SyncState(ctx context.Context, state *GatewayState) error
}

// ============================================================================
//                              数据结构
// ============================================================================

// RouteNode 路由节点信息
type RouteNode struct {
	PeerID      string
	Addrs       []string
	Latency     time.Duration
	LastSeen    time.Time
	Load        *NodeLoad
	IsReachable bool
}

// Route 路由信息
type Route struct {
	TargetPeerID string
	NextHop      string
	Path         []string
	Latency      time.Duration
	Hops         int
	Score        float64
	CreatedAt    time.Time
}

// Path 路径信息
type Path struct {
	Nodes     []string
	TotalLatency time.Duration
	Hops      int
	Valid     bool
}

// NodeLoad 节点负载信息
type NodeLoad struct {
	ConnectionCount int
	BandwidthUsage  int64
	CPUUsage        float64
	LastUpdated     time.Time
}

// RoutingPolicy 路由策略
type RoutingPolicy int

const (
	// PolicyLowestLatency 最低延迟策略
	PolicyLowestLatency RoutingPolicy = iota
	// PolicyLeastHops 最少跳数策略
	PolicyLeastHops
	// PolicyLoadBalance 负载均衡策略
	PolicyLoadBalance
	// PolicyMixed 混合策略
	PolicyMixed
)

// String 返回策略名称
func (p RoutingPolicy) String() string {
	switch p {
	case PolicyLowestLatency:
		return "LowestLatency"
	case PolicyLeastHops:
		return "LeastHops"
	case PolicyLoadBalance:
		return "LoadBalance"
	case PolicyMixed:
		return "Mixed"
	default:
		return "Unknown"
	}
}

// LatencyStats 延迟统计
type LatencyStats struct {
	Mean time.Duration
	P50  time.Duration
	P95  time.Duration
	P99  time.Duration
	Min  time.Duration
	Max  time.Duration
}

// LoadBalancerStats 负载均衡统计
type LoadBalancerStats struct {
	TotalNodes      int
	OverloadedNodes int
	AverageLoad     float64
	LoadVariance    float64
}

// GatewayState Gateway 状态
type GatewayState struct {
	ReachableNodes []string
	RelayNodes     []string
	LastUpdated    time.Time
}

// RoutingMetrics 路由指标
type RoutingMetrics struct {
	TotalQueries   int64
	SuccessQueries int64
	FailedQueries  int64
	CacheHits      int64
	CacheMisses    int64
	AvgLatency     time.Duration
}
