package mdns

import (
	"errors"
	"time"
)

const (
	// DefaultServiceTag mDNS 服务标签
	DefaultServiceTag = "_dep2p._udp"

	// DefaultInterval 广播间隔
	DefaultInterval = 10 * time.Second

	// MDNSDomain mDNS 域名
	MDNSDomain = "local"

	// DNSAddrPrefix TXT 记录前缀
	DNSAddrPrefix = "dnsaddr="
)

// Config MDNS 配置
type Config struct {
	// ServiceTag mDNS 服务标签，默认 "_dep2p._udp"
	ServiceTag string

	// Interval 广播间隔，默认 10s
	Interval time.Duration

	// Enabled 是否启用，默认 true
	Enabled bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ServiceTag: DefaultServiceTag,
		Interval:   DefaultInterval,
		Enabled:    true,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}

	if c.ServiceTag == "" {
		return errors.New("service tag is empty")
	}

	if c.Interval <= 0 {
		return errors.New("interval must be positive")
	}

	return nil
}

// ApplyOptions 应用配置选项
func (c *Config) ApplyOptions(opts ...ConfigOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// ConfigOption 配置选项函数
type ConfigOption func(*Config)

// WithServiceTag 设置服务标签
func WithServiceTag(tag string) ConfigOption {
	return func(c *Config) {
		c.ServiceTag = tag
	}
}

// WithInterval 设置广播间隔
func WithInterval(interval time.Duration) ConfigOption {
	return func(c *Config) {
		c.Interval = interval
	}
}

// WithEnabled 设置是否启用
func WithEnabled(enabled bool) ConfigOption {
	return func(c *Config) {
		c.Enabled = enabled
	}
}
