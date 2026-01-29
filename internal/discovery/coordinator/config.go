package coordinator

import (
	"fmt"
	"time"
)

// Config 协调器配置
type Config struct {
	// FindTimeout 发现超时时间
	FindTimeout time.Duration

	// AdvertiseTimeout 广播超时时间
	AdvertiseTimeout time.Duration

	// EnableCache 是否启用节点缓存
	EnableCache bool

	// CacheTTL 缓存过期时间
	CacheTTL time.Duration

	// MaxCacheSize 最大缓存条目数
	MaxCacheSize int

	// EnableParallel 是否并行查询所有发现器
	EnableParallel bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		FindTimeout:      30 * time.Second,
		AdvertiseTimeout: 10 * time.Second,
		EnableCache:      true,
		CacheTTL:         5 * time.Minute,
		MaxCacheSize:     1000,
		EnableParallel:   true,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.FindTimeout <= 0 {
		return fmt.Errorf("%w: FindTimeout must be positive", ErrInvalidConfig)
	}

	if c.AdvertiseTimeout <= 0 {
		return fmt.Errorf("%w: AdvertiseTimeout must be positive", ErrInvalidConfig)
	}

	if c.EnableCache {
		if c.CacheTTL <= 0 {
			return fmt.Errorf("%w: CacheTTL must be positive when cache is enabled", ErrInvalidConfig)
		}

		if c.MaxCacheSize <= 0 {
			return fmt.Errorf("%w: MaxCacheSize must be positive when cache is enabled", ErrInvalidConfig)
		}
	}

	return nil
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	return &Config{
		FindTimeout:      c.FindTimeout,
		AdvertiseTimeout: c.AdvertiseTimeout,
		EnableCache:      c.EnableCache,
		CacheTTL:         c.CacheTTL,
		MaxCacheSize:     c.MaxCacheSize,
		EnableParallel:   c.EnableParallel,
	}
}
