package swarm

import (
	"time"
)

// Config Swarm 配置
type Config struct {
	// DialTimeout 拨号超时（远程网络）
	DialTimeout time.Duration

	// DialTimeoutLocal 本地网络拨号超时
	DialTimeoutLocal time.Duration

	// NewStreamTimeout 创建流超时
	NewStreamTimeout time.Duration

	// MaxConcurrentDials 最大并发拨号数
	MaxConcurrentDials int

	// 连接健康检测配置
	// 用于检测 QUIC 直连的健康状态，加速离线检测

	// ConnHealthInterval 连接健康检测间隔
	// 设置为 0 则禁用健康检测
	ConnHealthInterval time.Duration

	// ConnHealthTimeout 单次健康检测超时
	ConnHealthTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		DialTimeout:        15 * time.Second,
		DialTimeoutLocal:   5 * time.Second,
		NewStreamTimeout:   15 * time.Second,
		MaxConcurrentDials: 100,

		// 连接健康检测默认配置
		ConnHealthInterval: 30 * time.Second,
		ConnHealthTimeout:  10 * time.Second,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.DialTimeout <= 0 {
		return ErrInvalidConfig
	}
	if c.DialTimeoutLocal <= 0 {
		return ErrInvalidConfig
	}
	if c.NewStreamTimeout <= 0 {
		return ErrInvalidConfig
	}
	if c.MaxConcurrentDials <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

// Option Swarm 选项函数
type Option func(*Swarm) error

// WithConfig 设置配置
func WithConfig(config *Config) Option {
	return func(s *Swarm) error {
		if config == nil {
			return ErrInvalidConfig
		}
		if err := config.Validate(); err != nil {
			return err
		}
		s.config = config
		return nil
	}
}
