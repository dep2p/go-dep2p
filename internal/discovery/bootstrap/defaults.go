// Package bootstrap 提供引导节点发现服务
//
// 本文件定义 Bootstrap 能力的内置默认值。
// 这些值是经过调优的，用户无法通过配置修改。
package bootstrap

import "time"

// ════════════════════════════════════════════════════════════════════════════
// 存储参数（内置默认值，用户不可配置）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultMaxNodes 最大存储节点数
	// 覆盖中小型网络，内存占用约 50MB
	DefaultMaxNodes = 50000

	// DefaultPersistPath 持久化路径（相对于 DataDir）
	DefaultPersistPath = "bootstrap.db"

	// DefaultCacheSize LRU 缓存大小
	// 加速频繁查询，减少磁盘 I/O
	DefaultCacheSize = 1000
)

// ════════════════════════════════════════════════════════════════════════════
// 探测参数（内置默认值，用户不可配置）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultProbeInterval 存活探测间隔
	// 平衡探测频率与网络负载
	DefaultProbeInterval = 5 * time.Minute

	// DefaultProbeBatchSize 每批探测数量
	// 避免瞬时网络负载过高
	DefaultProbeBatchSize = 100

	// DefaultProbeTimeout 单次探测超时
	// 应足够短以快速检测离线节点
	DefaultProbeTimeout = 10 * time.Second

	// DefaultProbeMaxConcurrent 最大并发探测数
	DefaultProbeMaxConcurrent = 20
)

// ════════════════════════════════════════════════════════════════════════════
// 发现参数（内置默认值，用户不可配置）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultDiscoveryInterval 主动发现间隔
	// 避免 DHT 压力过大
	DefaultDiscoveryInterval = 10 * time.Minute

	// DefaultDiscoveryWalkLen Random Walk 步数
	// 每次发现时执行的 FIND_NODE 次数
	DefaultDiscoveryWalkLen = 20
)

// ════════════════════════════════════════════════════════════════════════════
// 过期参数（内置默认值，用户不可配置）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultNodeExpireTime 节点过期时间
	// 允许节点短暂离线后恢复
	DefaultNodeExpireTime = 24 * time.Hour

	// DefaultOfflineThreshold 连续失败阈值
	// 超过此次数标记为离线
	DefaultOfflineThreshold = 3

	// DefaultCleanupInterval 清理间隔
	// 定期清理过期节点
	DefaultCleanupInterval = 1 * time.Hour
)

// ════════════════════════════════════════════════════════════════════════════
// 响应参数（内置默认值，用户不可配置）
// ════════════════════════════════════════════════════════════════════════════

const (
	// DefaultResponseK FIND_NODE 响应返回的节点数
	// Kademlia 标准值，足够触发 DHT 迭代
	DefaultResponseK = 20
)

// ════════════════════════════════════════════════════════════════════════════
// 节点状态
// ════════════════════════════════════════════════════════════════════════════

// NodeStatus 节点状态枚举
type NodeStatus int32

const (
	// NodeStatusUnknown 未知状态（新加入节点）
	NodeStatusUnknown NodeStatus = iota

	// NodeStatusOnline 在线
	NodeStatusOnline

	// NodeStatusOffline 离线
	NodeStatusOffline
)

// String 返回状态字符串表示
func (s NodeStatus) String() string {
	switch s {
	case NodeStatusUnknown:
		return "Unknown"
	case NodeStatusOnline:
		return "Online"
	case NodeStatusOffline:
		return "Offline"
	default:
		return "Invalid"
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 默认配置构造
// ════════════════════════════════════════════════════════════════════════════

// BootstrapDefaults 返回所有 Bootstrap 内置默认值
// 这些值用于 EnableBootstrap() 初始化
type BootstrapDefaults struct {
	// 存储参数
	MaxNodes    int
	PersistPath string
	CacheSize   int

	// 探测参数
	ProbeInterval     time.Duration
	ProbeBatchSize    int
	ProbeTimeout      time.Duration
	ProbeMaxConcurrent int

	// 发现参数
	DiscoveryInterval time.Duration
	DiscoveryWalkLen  int

	// 过期参数
	NodeExpireTime   time.Duration
	OfflineThreshold int
	CleanupInterval  time.Duration

	// 响应参数
	ResponseK int
}

// GetDefaults 返回内置默认配置
func GetDefaults() BootstrapDefaults {
	return BootstrapDefaults{
		// 存储参数
		MaxNodes:    DefaultMaxNodes,
		PersistPath: DefaultPersistPath,
		CacheSize:   DefaultCacheSize,

		// 探测参数
		ProbeInterval:      DefaultProbeInterval,
		ProbeBatchSize:     DefaultProbeBatchSize,
		ProbeTimeout:       DefaultProbeTimeout,
		ProbeMaxConcurrent: DefaultProbeMaxConcurrent,

		// 发现参数
		DiscoveryInterval: DefaultDiscoveryInterval,
		DiscoveryWalkLen:  DefaultDiscoveryWalkLen,

		// 过期参数
		NodeExpireTime:   DefaultNodeExpireTime,
		OfflineThreshold: DefaultOfflineThreshold,
		CleanupInterval:  DefaultCleanupInterval,

		// 响应参数
		ResponseK: DefaultResponseK,
	}
}
