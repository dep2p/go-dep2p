// Package netmon 提供网络状态监控功能
package netmon

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              监控配置
// ============================================================================

// Config 网络监控配置
type Config struct {
	// ErrorThreshold 错误阈值
	// 连续 N 次发送失败后认为节点不可达
	// 默认值: 3
	ErrorThreshold int

	// ProbeInterval 健康状态下的探测间隔
	// 默认值: 30s
	ProbeInterval time.Duration

	// RecoveryProbeInterval 恢复状态下的探测间隔
	// 默认值: 1s
	RecoveryProbeInterval time.Duration

	// ErrorWindow 错误统计窗口
	// 只统计窗口期内的错误
	// 默认值: 1m
	ErrorWindow time.Duration

	// CriticalErrors 关键错误类型
	// 匹配到这些错误时立即触发恢复
	// 默认值: ["network is unreachable", "no route to host"]
	CriticalErrors []string

	// MaxRecoveryAttempts 最大恢复尝试次数
	// 默认值: 5
	MaxRecoveryAttempts int

	// InitialBackoff 初始退避时间
	// 默认: 1s
	InitialBackoff time.Duration

	// MaxBackoff 最大退避时间
	// 默认: 30s
	MaxBackoff time.Duration

	// BackoffFactor 退避因子
	// 默认: 1.5
	BackoffFactor float64

	// StateChangeDebounce 状态变更防抖时间
	// 避免状态频繁抖动
	// 默认值: 500ms
	StateChangeDebounce time.Duration

	// EnableAutoRecovery 是否启用自动恢复
	// 默认值: true
	EnableAutoRecovery bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ErrorThreshold:        3,
		ProbeInterval:         30 * time.Second,
		RecoveryProbeInterval: 1 * time.Second,
		ErrorWindow:           1 * time.Minute,
		CriticalErrors: []string{
			"network is unreachable",
			"no route to host",
			"connection refused",
			"host is down",
		},
		MaxRecoveryAttempts: 5,
		InitialBackoff:      1 * time.Second,
		MaxBackoff:          30 * time.Second,
		BackoffFactor:       1.5,
		StateChangeDebounce: 500 * time.Millisecond,
		EnableAutoRecovery:  true,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.ErrorThreshold <= 0 {
		c.ErrorThreshold = 3
	}
	if c.ProbeInterval <= 0 {
		c.ProbeInterval = 30 * time.Second
	}
	if c.RecoveryProbeInterval <= 0 {
		c.RecoveryProbeInterval = 1 * time.Second
	}
	if c.ErrorWindow <= 0 {
		c.ErrorWindow = 1 * time.Minute
	}
	if len(c.CriticalErrors) == 0 {
		c.CriticalErrors = DefaultConfig().CriticalErrors
	}
	if c.MaxRecoveryAttempts <= 0 {
		c.MaxRecoveryAttempts = 5
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
	if c.StateChangeDebounce <= 0 {
		c.StateChangeDebounce = 500 * time.Millisecond
	}
	return nil
}

// ToInterfaceConfig 转换为接口配置
func (c *Config) ToInterfaceConfig() interfaces.ConnectionHealthMonitorConfig {
	return interfaces.ConnectionHealthMonitorConfig{
		ErrorThreshold:        c.ErrorThreshold,
		ProbeInterval:         c.ProbeInterval,
		RecoveryProbeInterval: c.RecoveryProbeInterval,
		ErrorWindow:           c.ErrorWindow,
		CriticalErrors:        c.CriticalErrors,
		MaxRecoveryAttempts:   c.MaxRecoveryAttempts,
		StateChangeDebounce:   c.StateChangeDebounce,
		EnableAutoRecovery:    c.EnableAutoRecovery,
	}
}

// FromInterfaceConfig 从接口配置创建
func FromInterfaceConfig(cfg interfaces.ConnectionHealthMonitorConfig) *Config {
	return &Config{
		ErrorThreshold:        cfg.ErrorThreshold,
		ProbeInterval:         cfg.ProbeInterval,
		RecoveryProbeInterval: cfg.RecoveryProbeInterval,
		ErrorWindow:           cfg.ErrorWindow,
		CriticalErrors:        cfg.CriticalErrors,
		MaxRecoveryAttempts:   cfg.MaxRecoveryAttempts,
		StateChangeDebounce:   cfg.StateChangeDebounce,
		EnableAutoRecovery:    cfg.EnableAutoRecovery,
	}
}

// WithErrorThreshold 设置错误阈值
func (c *Config) WithErrorThreshold(threshold int) *Config {
	c.ErrorThreshold = threshold
	return c
}

// WithProbeInterval 设置探测间隔
func (c *Config) WithProbeInterval(interval time.Duration) *Config {
	c.ProbeInterval = interval
	return c
}

// WithCriticalErrors 设置关键错误类型
func (c *Config) WithCriticalErrors(errors []string) *Config {
	c.CriticalErrors = errors
	return c
}

// WithAutoRecovery 设置是否启用自动恢复
func (c *Config) WithAutoRecovery(enable bool) *Config {
	c.EnableAutoRecovery = enable
	return c
}

// WithStateChangeDebounce 设置状态变更防抖时间
func (c *Config) WithStateChangeDebounce(d time.Duration) *Config {
	c.StateChangeDebounce = d
	return c
}
