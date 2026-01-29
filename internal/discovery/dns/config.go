package dns

import (
	"errors"
	"time"
)

// ============================================================================
//                              配置定义
// ============================================================================

// Config DNS 发现器配置
type Config struct {
	// Domains 要查询的域名列表
	Domains []string

	// Timeout DNS 查询超时
	Timeout time.Duration

	// MaxDepth dnsaddr 最大递归深度
	MaxDepth int

	// CacheTTL DNS 结果缓存 TTL
	CacheTTL time.Duration

	// CustomResolver 自定义 DNS 解析器地址（格式: "ip:port"）
	CustomResolver string

	// RefreshInterval 后台刷新间隔
	RefreshInterval time.Duration
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		Domains:         nil,
		Timeout:         10 * time.Second,
		MaxDepth:        3,
		CacheTTL:        5 * time.Minute,
		CustomResolver:  "",
		RefreshInterval: 5 * time.Minute,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.MaxDepth < 0 {
		return errors.New("max depth must be non-negative")
	}
	if c.MaxDepth > 10 {
		return errors.New("max depth too large (max 10)")
	}
	if c.CacheTTL < 0 {
		return errors.New("cache TTL must be non-negative")
	}
	if c.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive")
	}
	return nil
}
