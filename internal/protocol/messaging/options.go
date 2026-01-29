// Package messaging 实现点对点消息传递协议
package messaging

import "time"

// Config Messaging 服务配置
type Config struct {
	// Timeout 默认请求超时时间
	Timeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryDelay 重试延迟
	RetryDelay time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		RetryDelay: time.Second,
	}
}

// Option 配置选项函数
type Option func(*Config)

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(maxRetries int) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
	}
}

// WithRetryDelay 设置重试延迟
func WithRetryDelay(delay time.Duration) Option {
	return func(c *Config) {
		c.RetryDelay = delay
	}
}
