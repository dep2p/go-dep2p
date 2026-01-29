package muxer

import (
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Config 多路复用器配置
type Config struct {
	MaxStreamWindowSize uint32        // 最大流窗口大小
	KeepAliveInterval   time.Duration // 心跳间隔
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxStreamWindowSize: 16 * 1024 * 1024, // 16MB
		KeepAliveInterval:   30 * time.Second, // 30 秒
	}
}

// ConfigFromUnified 从统一配置创建 Muxer 配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}
	// 从传输配置推断 muxer 配置
	return Config{
		MaxStreamWindowSize: 16 * 1024 * 1024, // 默认 16MB
		KeepAliveInterval:   cfg.Transport.QUIC.MaxIdleTimeout.Duration() / 2,
	}
}

// Params Muxer 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Module 是 muxer 的 Fx 模块
var Module = fx.Module("muxer",
	fx.Provide(
		fx.Annotate(
			NewTransportFromParams,
			fx.As(new(pkgif.StreamMuxer)),
		),
	),
)

// NewTransportFromParams 从参数创建 Transport
func NewTransportFromParams(p Params) *Transport {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	return NewTransportWithConfig(cfg)
}

// NewTransportWithConfig 使用配置创建 Transport
func NewTransportWithConfig(_ Config) *Transport {
	t := NewTransport()
	// 应用配置（如果 Transport 支持）
	return t
}
