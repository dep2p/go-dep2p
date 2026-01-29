// Package streams 实现流协议
package streams

import "time"

// Config 流服务配置
type Config struct {
	// ReadTimeout 读超时
	ReadTimeout time.Duration

	// WriteTimeout 写超时
	WriteTimeout time.Duration

	// MaxStreamBuffer 最大流缓冲区大小
	MaxStreamBuffer int

	// DefaultRealmID 默认Realm ID (可选)
	DefaultRealmID string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		MaxStreamBuffer: 4096,
	}
}

// Option 定义配置选项函数
type Option func(*Config)

// WithReadTimeout 设置读超时
func WithReadTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.ReadTimeout = timeout
	}
}

// WithWriteTimeout 设置写超时
func WithWriteTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.WriteTimeout = timeout
	}
}

// WithMaxStreamBuffer 设置最大流缓冲区
func WithMaxStreamBuffer(size int) Option {
	return func(c *Config) {
		c.MaxStreamBuffer = size
	}
}

// WithDefaultRealmID 设置默认Realm ID
func WithDefaultRealmID(realmID string) Option {
	return func(c *Config) {
		c.DefaultRealmID = realmID
	}
}
