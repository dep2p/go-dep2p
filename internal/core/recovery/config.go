// Package recovery 提供网络恢复功能
package recovery

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              恢复配置
// ============================================================================

// Config 恢复管理器配置
type Config struct {
	// MaxAttempts 最大恢复尝试次数
	// 默认值: 5
	MaxAttempts int

	// InitialBackoff 初始退避时间
	// 默认值: 1s
	InitialBackoff time.Duration

	// MaxBackoff 最大退避时间
	// 默认值: 30s
	MaxBackoff time.Duration

	// BackoffFactor 退避因子
	// 默认值: 1.5
	BackoffFactor float64

	// RecoveryTimeout 单次恢复超时
	// 默认值: 30s
	RecoveryTimeout time.Duration

	// CriticalPeers 关键节点列表
	// 恢复时优先重连这些节点
	CriticalPeers []string

	// CriticalPeersAddrs 关键节点地址列表
	// 优先使用地址直拨，失败再使用 PeerID
	CriticalPeersAddrs []string

	// RebindOnCriticalError 关键错误时是否 rebind
	// 默认值: true
	RebindOnCriticalError bool

	// RediscoverAddresses 恢复时是否重新发现地址
	// 默认值: true
	RediscoverAddresses bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts:           5,
		InitialBackoff:        1 * time.Second,
		MaxBackoff:            30 * time.Second,
		BackoffFactor:         1.5,
		RecoveryTimeout:       30 * time.Second,
		CriticalPeers:         nil,
		RebindOnCriticalError: true,
		RediscoverAddresses:   true,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.InitialBackoff <= 0 {
		c.InitialBackoff = 1 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
	if c.BackoffFactor <= 1.0 {
		c.BackoffFactor = 1.5
	}
	if c.RecoveryTimeout <= 0 {
		c.RecoveryTimeout = 30 * time.Second
	}
	return nil
}

// ToInterfaceConfig 转换为接口配置
func (c *Config) ToInterfaceConfig() interfaces.RecoveryConfig {
	return interfaces.RecoveryConfig{
		MaxAttempts:           c.MaxAttempts,
		InitialBackoff:        c.InitialBackoff,
		MaxBackoff:            c.MaxBackoff,
		BackoffFactor:         c.BackoffFactor,
		RecoveryTimeout:       c.RecoveryTimeout,
		RebindOnCriticalError: c.RebindOnCriticalError,
		RediscoverAddresses:   c.RediscoverAddresses,
	}
}

// FromInterfaceConfig 从接口配置创建
func FromInterfaceConfig(cfg interfaces.RecoveryConfig) *Config {
	return &Config{
		MaxAttempts:           cfg.MaxAttempts,
		InitialBackoff:        cfg.InitialBackoff,
		MaxBackoff:            cfg.MaxBackoff,
		BackoffFactor:         cfg.BackoffFactor,
		RecoveryTimeout:       cfg.RecoveryTimeout,
		RebindOnCriticalError: cfg.RebindOnCriticalError,
		RediscoverAddresses:   cfg.RediscoverAddresses,
	}
}

// WithCriticalPeers 设置关键节点
func (c *Config) WithCriticalPeers(peers ...string) *Config {
	c.CriticalPeers = peers
	return c
}

// WithCriticalPeersAddrs 设置关键节点地址
func (c *Config) WithCriticalPeersAddrs(addrs ...string) *Config {
	c.CriticalPeersAddrs = addrs
	return c
}

// WithMaxAttempts 设置最大尝试次数
func (c *Config) WithMaxAttempts(n int) *Config {
	c.MaxAttempts = n
	return c
}

// WithRebindOnCriticalError 设置关键错误时是否 rebind
func (c *Config) WithRebindOnCriticalError(enable bool) *Config {
	c.RebindOnCriticalError = enable
	return c
}

// WithRediscoverAddresses 设置是否重新发现地址
func (c *Config) WithRediscoverAddresses(enable bool) *Config {
	c.RediscoverAddresses = enable
	return c
}

// WithRecoveryTimeout 设置恢复超时
func (c *Config) WithRecoveryTimeout(timeout time.Duration) *Config {
	c.RecoveryTimeout = timeout
	return c
}
