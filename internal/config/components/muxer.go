package components

import (
	"github.com/dep2p/go-dep2p/internal/config"
)

// MuxerOptions 多路复用选项
type MuxerOptions struct {
	// Protocol 多路复用协议
	// 可选: yamux (默认), quic (QUIC 原生)
	Protocol string

	// StreamReceiveWindow 流接收窗口大小
	StreamReceiveWindow uint32

	// ConnectionReceiveWindow 连接接收窗口大小
	ConnectionReceiveWindow uint32
}

// NewMuxerOptions 从配置创建多路复用选项
func NewMuxerOptions(cfg *config.MuxerConfig) *MuxerOptions {
	return &MuxerOptions{
		Protocol:                cfg.Protocol,
		StreamReceiveWindow:     cfg.StreamReceiveWindow,
		ConnectionReceiveWindow: cfg.ConnectionReceiveWindow,
	}
}

// DefaultMuxerOptions 默认多路复用选项
func DefaultMuxerOptions() *MuxerOptions {
	return &MuxerOptions{
		Protocol:                "yamux",
		StreamReceiveWindow:     256 * 1024,      // 256 KB
		ConnectionReceiveWindow: 16 * 1024 * 1024, // 16 MB
	}
}

