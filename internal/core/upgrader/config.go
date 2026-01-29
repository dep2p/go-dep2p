// Package upgrader 实现连接升级器
package upgrader

import (
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Config 升级器配置
type Config struct {
	// SecurityTransports 安全传输列表（按优先级排序）
	SecurityTransports []pkgif.SecureTransport

	// StreamMuxers 流多路复用器列表（按优先级排序）
	StreamMuxers []pkgif.StreamMuxer

	// ResourceManager 资源管理器（可选）
	ResourceManager pkgif.ResourceManager

	// NegotiateTimeout 协议协商超时（默认 60s）
	NegotiateTimeout time.Duration

	// HandshakeTimeout 握手超时（默认 30s）
	HandshakeTimeout time.Duration
}

// NewConfig 创建默认配置
func NewConfig() Config {
	return Config{
		NegotiateTimeout: 60 * time.Second,
		HandshakeTimeout: 30 * time.Second,
	}
}
