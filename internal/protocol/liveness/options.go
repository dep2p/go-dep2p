// Package liveness 实现存活检测服务
package liveness

import "time"

// Option 定义配置选项函数
type Option func(*Config)

// Config Liveness配置
type Config struct {
	// Interval 检测间隔
	Interval time.Duration

	// Timeout 单次检测超时
	Timeout time.Duration

	// FailThreshold 判定下线的失败阈值
	FailThreshold int

	// RTTWindowSize RTT滑动窗口大小
	RTTWindowSize int

	// DefaultRealmID 默认Realm ID (可选)
	DefaultRealmID string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Interval:      30 * time.Second,
		Timeout:       5 * time.Second,
		FailThreshold: 3,
		RTTWindowSize: 10,
	}
}

// WithInterval 设置检测间隔
func WithInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.Interval = interval
	}
}

// WithTimeout 设置超时
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithFailThreshold 设置失败阈值
func WithFailThreshold(threshold int) Option {
	return func(c *Config) {
		c.FailThreshold = threshold
	}
}

// WithRTTWindowSize 设置RTT窗口大小
func WithRTTWindowSize(size int) Option {
	return func(c *Config) {
		c.RTTWindowSize = size
	}
}

// WithDefaultRealmID 设置默认Realm ID
func WithDefaultRealmID(realmID string) Option {
	return func(c *Config) {
		c.DefaultRealmID = realmID
	}
}
