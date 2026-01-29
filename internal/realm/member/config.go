package member

import (
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              配置定义
// ============================================================================

// Config 成员管理器配置
type Config struct {
	// 缓存配置
	CacheSize int           // 缓存最大容量
	CacheTTL  time.Duration // 缓存 TTL

	// 存储配置
	StorePath       string        // 存储路径
	StoreType       string        // 存储类型（file, memory）
	CompactOnStart  bool          // 启动时压缩
	CompactInterval time.Duration // 压缩间隔

	// 同步配置
	SyncInterval    time.Duration // 同步间隔
	SyncFullOnStart bool          // 启动时全量同步

	// 心跳配置
	HeartbeatInterval time.Duration // 心跳间隔
	HeartbeatTimeout  time.Duration // 心跳超时
	HeartbeatRetries  int           // 最大重试次数

	// 发现配置
	DiscoveryNamespace string        // Rendezvous 命名空间
	DiscoveryTTL       time.Duration // 注册 TTL
	DiscoveryRefresh   time.Duration // 刷新间隔

	// 清理配置
	CleanupInterval    time.Duration // 清理间隔
	MaxOfflineDuration time.Duration // 最大离线时长

	// 断开保护配置（
	DisconnectProtection time.Duration // 断开保护期：成员断开后，在此期间内拒绝重新添加
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		// 缓存
		CacheSize: 1000,
		CacheTTL:  10 * time.Minute,

		// 存储
		StorePath:       "",
		StoreType:       "file",
		CompactOnStart:  false,
		CompactInterval: 1 * time.Hour,

		// 同步
		SyncInterval:    30 * time.Second,
		SyncFullOnStart: true,

		// 心跳
		HeartbeatInterval: 15 * time.Second,
		HeartbeatTimeout:  45 * time.Second,
		HeartbeatRetries:  3,

		// 发现
		DiscoveryNamespace: protocol.PrefixRealm,
		DiscoveryTTL:       1 * time.Hour,
		DiscoveryRefresh:   30 * time.Minute,

		// 清理
		CleanupInterval:    5 * time.Minute,
		MaxOfflineDuration: 24 * time.Hour,

		// 断开保护（
		DisconnectProtection: 30 * time.Second,
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

	if c.StoreType != "file" && c.StoreType != "memory" {
		return fmt.Errorf("%w: StoreType must be 'file' or 'memory'", ErrInvalidConfig)
	}

	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("%w: HeartbeatInterval must be positive", ErrInvalidConfig)
	}

	if c.HeartbeatRetries < 0 {
		return fmt.Errorf("%w: HeartbeatRetries must be non-negative", ErrInvalidConfig)
	}

	return nil
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	return &Config{
		CacheSize:            c.CacheSize,
		CacheTTL:             c.CacheTTL,
		StorePath:            c.StorePath,
		StoreType:            c.StoreType,
		CompactOnStart:       c.CompactOnStart,
		CompactInterval:      c.CompactInterval,
		SyncInterval:         c.SyncInterval,
		SyncFullOnStart:      c.SyncFullOnStart,
		HeartbeatInterval:    c.HeartbeatInterval,
		HeartbeatTimeout:     c.HeartbeatTimeout,
		HeartbeatRetries:     c.HeartbeatRetries,
		DiscoveryNamespace:   c.DiscoveryNamespace,
		DiscoveryTTL:         c.DiscoveryTTL,
		DiscoveryRefresh:     c.DiscoveryRefresh,
		CleanupInterval:      c.CleanupInterval,
		MaxOfflineDuration:   c.MaxOfflineDuration,
		DisconnectProtection: c.DisconnectProtection,
	}
}
