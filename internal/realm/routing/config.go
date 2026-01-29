package routing

import (
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              配置定义
// ============================================================================

// Config 路由器配置
type Config struct {
	// 路由策略
	DefaultPolicy interfaces.RoutingPolicy

	// 缓存配置
	CacheSize int           // 路由缓存大小
	CacheTTL  time.Duration // 缓存 TTL

	// 路径查找配置
	MaxHops         int           // 最大跳数
	MaxPaths        int           // 最大路径数
	PathTimeout     time.Duration // 路径查找超时
	PathScoreWeight PathScoreWeights

	// 负载均衡配置
	LoadBalanceStrategy string        // 策略（weighted, least_connection）
	LoadUpdateInterval  time.Duration // 负载更新间隔
	OverloadThreshold   float64       // 过载阈值（0-1）

	// 延迟探测配置
	LatencyProbeInterval time.Duration // 探测间隔
	LatencyWindowSize    int           // 统计窗口大小
	LatencyTimeout       time.Duration // 探测超时

	// 路由表配置
	TableRefreshInterval time.Duration // 路由表刷新间隔
	NodeExpireTime       time.Duration // 节点过期时间

	// Gateway 配置
	EnableGatewayRouting bool          // 启用 Gateway 路由
	GatewaySyncInterval  time.Duration // Gateway 同步间隔
}

// PathScoreWeights 路径评分权重
type PathScoreWeights struct {
	Latency float64 // 延迟权重
	Hops    float64 // 跳数权重
	Load    float64 // 负载权重
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		// 策略
		DefaultPolicy: interfaces.PolicyMixed,

		// 缓存
		CacheSize: 1000,
		CacheTTL:  5 * time.Minute,

		// 路径
		MaxHops:     10,
		MaxPaths:    3,
		PathTimeout: 5 * time.Second,
		PathScoreWeight: PathScoreWeights{
			Latency: 0.5,
			Hops:    0.3,
			Load:    0.2,
		},

		// 负载均衡
		LoadBalanceStrategy: "weighted",
		LoadUpdateInterval:  10 * time.Second,
		OverloadThreshold:   0.8,

		// 延迟探测
		LatencyProbeInterval: 30 * time.Second,
		LatencyWindowSize:    10,
		LatencyTimeout:       3 * time.Second,

		// 路由表
		TableRefreshInterval: 5 * time.Minute,
		NodeExpireTime:       30 * time.Minute,

		// Gateway
		EnableGatewayRouting: true,
		GatewaySyncInterval:  1 * time.Minute,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.CacheSize <= 0 {
		return fmt.Errorf("%w: CacheSize must be positive", ErrInvalidConfig)
	}

	if c.CacheTTL <= 0 {
		return fmt.Errorf("%w: CacheTTL must be positive", ErrInvalidConfig)
	}

	if c.MaxHops <= 0 {
		return fmt.Errorf("%w: MaxHops must be positive", ErrInvalidConfig)
	}

	if c.MaxPaths <= 0 {
		return fmt.Errorf("%w: MaxPaths must be positive", ErrInvalidConfig)
	}

	if c.OverloadThreshold < 0 || c.OverloadThreshold > 1 {
		return fmt.Errorf("%w: OverloadThreshold must be between 0 and 1", ErrInvalidConfig)
	}

	// 验证评分权重和为 1.0
	totalWeight := c.PathScoreWeight.Latency + c.PathScoreWeight.Hops + c.PathScoreWeight.Load
	if totalWeight < 0.99 || totalWeight > 1.01 {
		return fmt.Errorf("%w: PathScoreWeight sum must be 1.0", ErrInvalidConfig)
	}

	return nil
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	return &Config{
		DefaultPolicy:        c.DefaultPolicy,
		CacheSize:            c.CacheSize,
		CacheTTL:             c.CacheTTL,
		MaxHops:              c.MaxHops,
		MaxPaths:             c.MaxPaths,
		PathTimeout:          c.PathTimeout,
		PathScoreWeight:      c.PathScoreWeight,
		LoadBalanceStrategy:  c.LoadBalanceStrategy,
		LoadUpdateInterval:   c.LoadUpdateInterval,
		OverloadThreshold:    c.OverloadThreshold,
		LatencyProbeInterval: c.LatencyProbeInterval,
		LatencyWindowSize:    c.LatencyWindowSize,
		LatencyTimeout:       c.LatencyTimeout,
		TableRefreshInterval: c.TableRefreshInterval,
		NodeExpireTime:       c.NodeExpireTime,
		EnableGatewayRouting: c.EnableGatewayRouting,
		GatewaySyncInterval:  c.GatewaySyncInterval,
	}
}
