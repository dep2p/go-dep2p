// Package yamux 提供基于 yamux 的多路复用实现
package yamux

import (
	"io"
	"time"

	"github.com/hashicorp/yamux"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// DefaultYamuxConfig 返回默认的 yamux 配置
func DefaultYamuxConfig() *yamux.Config {
	return &yamux.Config{
		AcceptBacklog:          256,
		EnableKeepAlive:        true,
		KeepAliveInterval:      30 * time.Second,
		ConnectionWriteTimeout: 10 * time.Second,
		MaxStreamWindowSize:    256 * 1024,       // 256 KB
		StreamOpenTimeout:      75 * time.Second,
		StreamCloseTimeout:     5 * time.Minute,
		LogOutput:              io.Discard, // 禁用日志输出
	}
}

// ConfigToYamux 将 muxer.Config 转换为 yamux.Config
func ConfigToYamux(cfg muxerif.Config) *yamux.Config {
	yamuxCfg := DefaultYamuxConfig()

	if cfg.MaxStreamWindowSize > 0 {
		yamuxCfg.MaxStreamWindowSize = cfg.MaxStreamWindowSize
	}

	if cfg.MaxStreams > 0 {
		yamuxCfg.AcceptBacklog = cfg.MaxStreams
	}

	if cfg.KeepAliveInterval > 0 {
		yamuxCfg.KeepAliveInterval = cfg.KeepAliveInterval
	}

	if cfg.KeepAliveTimeout > 0 {
		yamuxCfg.ConnectionWriteTimeout = cfg.KeepAliveTimeout
	}

	yamuxCfg.EnableKeepAlive = cfg.EnableKeepAlive

	return yamuxCfg
}

// YamuxToConfig 将 yamux.Config 转换为 muxer.Config
func YamuxToConfig(yamuxCfg *yamux.Config) muxerif.Config {
	return muxerif.Config{
		MaxStreams:          yamuxCfg.AcceptBacklog,
		MaxStreamWindowSize: yamuxCfg.MaxStreamWindowSize,
		KeepAliveInterval:   yamuxCfg.KeepAliveInterval,
		KeepAliveTimeout:    yamuxCfg.ConnectionWriteTimeout,
		EnableKeepAlive:     yamuxCfg.EnableKeepAlive,
	}
}

