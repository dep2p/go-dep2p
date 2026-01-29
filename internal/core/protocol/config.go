package protocol

import "time"

// Config 协议模块配置
type Config struct {
	// NegotiationTimeout 协议协商超时时间
	NegotiationTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		NegotiationTimeout: 10 * time.Second,
	}
}

// Validate 验证配置
func (c Config) Validate() error {
	if c.NegotiationTimeout <= 0 {
		return ErrInvalidProtocolID
	}
	return nil
}

// WithNegotiationTimeout 设置协商超时
func (c Config) WithNegotiationTimeout(timeout time.Duration) Config {
	c.NegotiationTimeout = timeout
	return c
}
