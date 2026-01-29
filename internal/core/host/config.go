package host

import (
	"errors"
	"time"
	
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Config Host 配置
type Config struct {
	// 基础配置
	UserAgent       string // 用户代理标识
	ProtocolVersion string // 协议版本
	
	// 地址配置
	AddrsFactory AddrsFactory // 地址过滤器函数
	
	// 超时配置
	NegotiationTimeout time.Duration // 协议协商超时（默认 10s）
	
	// 功能开关
	EnableMetrics bool // 启用指标监控
}

// AddrsFactory 地址工厂函数类型
// 用于过滤或转换监听地址列表
type AddrsFactory func([]types.Multiaddr) []types.Multiaddr

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		UserAgent:          "dep2p/1.0.0",
		ProtocolVersion:    "dep2p/1.0.0",
		AddrsFactory:       DefaultAddrsFactory,
		NegotiationTimeout: 10 * time.Second,
		EnableMetrics:      false,
	}
}

// DefaultAddrsFactory 默认地址工厂（不过滤）
func DefaultAddrsFactory(addrs []types.Multiaddr) []types.Multiaddr {
	return addrs
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.UserAgent == "" {
		return errors.New("UserAgent cannot be empty")
	}
	
	if c.ProtocolVersion == "" {
		return errors.New("ProtocolVersion cannot be empty")
	}
	
	if c.NegotiationTimeout < 0 {
		return errors.New("NegotiationTimeout cannot be negative")
	}
	
	if c.AddrsFactory == nil {
		c.AddrsFactory = DefaultAddrsFactory
	}
	
	return nil
}

// WithUserAgent 设置用户代理
func WithUserAgent(ua string) ConfigOption {
	return func(c *Config) {
		c.UserAgent = ua
	}
}

// WithProtocolVersion 设置协议版本
func WithProtocolVersion(pv string) ConfigOption {
	return func(c *Config) {
		c.ProtocolVersion = pv
	}
}

// WithAddrsFactory 设置地址工厂
func WithAddrsFactory(f AddrsFactory) ConfigOption {
	return func(c *Config) {
		c.AddrsFactory = f
	}
}

// WithNegotiationTimeout 设置协议协商超时
func WithNegotiationTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.NegotiationTimeout = timeout
	}
}

// WithMetrics 启用指标监控
func WithMetrics() ConfigOption {
	return func(c *Config) {
		c.EnableMetrics = true
	}
}

// ConfigOption 配置选项函数类型
type ConfigOption func(*Config)

// ApplyOptions 应用配置选项
func (c *Config) ApplyOptions(opts ...ConfigOption) {
	for _, opt := range opts {
		opt(c)
	}
}
