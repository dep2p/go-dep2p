// Package components 提供各组件的配置结构和转换逻辑
package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// TransportOptions 传输组件选项
type TransportOptions struct {
	// QUIC 配置
	QUIC QUICOptions

	// MaxConnections 最大连接数
	MaxConnections int

	// MaxStreamsPerConn 每连接最大流数
	MaxStreamsPerConn int

	// DialTimeout 拨号超时
	DialTimeout time.Duration

	// HandshakeTimeout 握手超时
	HandshakeTimeout time.Duration

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// QUICOptions QUIC 选项
type QUICOptions struct {
	// MaxIdleTimeout 最大空闲超时
	MaxIdleTimeout time.Duration

	// MaxIncomingStreams 最大入站流数
	MaxIncomingStreams int64

	// MaxIncomingUniStreams 最大入站单向流数
	MaxIncomingUniStreams int64

	// KeepAlivePeriod 保活周期
	KeepAlivePeriod time.Duration

	// EnableDatagrams 启用数据报
	EnableDatagrams bool
}

// NewTransportOptions 从配置创建传输选项
func NewTransportOptions(cfg *config.TransportConfig) *TransportOptions {
	return &TransportOptions{
		QUIC: QUICOptions{
			MaxIdleTimeout:        cfg.QUIC.MaxIdleTimeout,
			MaxIncomingStreams:    cfg.QUIC.MaxIncomingStreams,
			MaxIncomingUniStreams: cfg.QUIC.MaxIncomingUniStreams,
			KeepAlivePeriod:       cfg.QUIC.KeepAlivePeriod,
			EnableDatagrams:       cfg.QUIC.EnableDatagrams,
		},
		MaxConnections:    cfg.MaxConnections,
		MaxStreamsPerConn: cfg.MaxStreamsPerConn,
		DialTimeout:       cfg.DialTimeout,
		HandshakeTimeout:  cfg.HandshakeTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

// DefaultTransportOptions 默认传输选项
func DefaultTransportOptions() *TransportOptions {
	return &TransportOptions{
		QUIC: QUICOptions{
			MaxIdleTimeout:        30 * time.Second,
			MaxIncomingStreams:    256,
			MaxIncomingUniStreams: 256,
			KeepAlivePeriod:       15 * time.Second,
			EnableDatagrams:       true,
		},
		MaxConnections:    100,
		MaxStreamsPerConn: 256,
		DialTimeout:       10 * time.Second,
		HandshakeTimeout:  10 * time.Second,
		IdleTimeout:       5 * time.Minute,
	}
}

