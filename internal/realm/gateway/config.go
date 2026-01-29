package gateway

import (
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              配置定义
// ============================================================================

// Config Gateway 配置
type Config struct {
	// 带宽限制
	MaxBandwidth     int64         // 最大带宽（字节/秒）
	MaxConnPerPeer   int           // 每节点最大连接数
	MaxRelayDuration time.Duration // 最大中继时长

	// 连接池
	MaxConcurrent int           // 最大并发连接数
	IdleTimeout   time.Duration // 空闲超时

	// 流量控制
	EnableRateLimit bool  // 启用速率限制
	BurstSize       int64 // 突发容量（字节）

	// 协议验证
	StrictProtocolCheck bool     // 严格协议检查
	AllowedProtocols    []string // 允许的协议前缀

	// Routing 协作
	EnableRouterSync       bool          // 启用与 Router 同步
	CapacityReportInterval time.Duration // 容量报告间隔
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		// 带宽限制
		MaxBandwidth:     100 * 1024 * 1024, // 100 MB/s
		MaxConnPerPeer:   10,
		MaxRelayDuration: 0, // 无限制

		// 连接池
		MaxConcurrent: 1000,
		IdleTimeout:   5 * time.Minute,

		// 流量控制
		EnableRateLimit: true,
		BurstSize:       10 * 1024 * 1024, // 10 MB

		// 协议验证
		StrictProtocolCheck: true,
		AllowedProtocols: []string{
			protocol.PrefixRealm + "/",
			protocol.PrefixApp + "/",
		},

		// Routing 协作
		EnableRouterSync:       true,
		CapacityReportInterval: 10 * time.Second,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.MaxBandwidth <= 0 {
		return fmt.Errorf("%w: MaxBandwidth must be positive", ErrInvalidConfig)
	}

	if c.MaxConnPerPeer <= 0 {
		return fmt.Errorf("%w: MaxConnPerPeer must be positive", ErrInvalidConfig)
	}

	if c.MaxConcurrent <= 0 {
		return fmt.Errorf("%w: MaxConcurrent must be positive", ErrInvalidConfig)
	}

	if c.IdleTimeout <= 0 {
		return fmt.Errorf("%w: IdleTimeout must be positive", ErrInvalidConfig)
	}

	if c.EnableRateLimit && c.BurstSize <= 0 {
		return fmt.Errorf("%w: BurstSize must be positive when rate limit is enabled", ErrInvalidConfig)
	}

	return nil
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	return &Config{
		MaxBandwidth:           c.MaxBandwidth,
		MaxConnPerPeer:         c.MaxConnPerPeer,
		MaxRelayDuration:       c.MaxRelayDuration,
		MaxConcurrent:          c.MaxConcurrent,
		IdleTimeout:            c.IdleTimeout,
		EnableRateLimit:        c.EnableRateLimit,
		BurstSize:              c.BurstSize,
		StrictProtocolCheck:    c.StrictProtocolCheck,
		AllowedProtocols:       append([]string{}, c.AllowedProtocols...),
		EnableRouterSync:       c.EnableRouterSync,
		CapacityReportInterval: c.CapacityReportInterval,
	}
}
