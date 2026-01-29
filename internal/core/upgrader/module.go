// Package upgrader 实现连接升级器
package upgrader

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Params Upgrader 依赖参数
type Params struct {
	fx.In

	Identity   pkgif.Identity
	Security   pkgif.SecureTransport
	Muxer      pkgif.StreamMuxer
	UnifiedCfg *config.Config `optional:"true"`
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("upgrader",
		fx.Provide(
			ProvideUpgrader,
		),
	)
}

// ConfigFromUnified 从统一配置创建 Upgrader 配置
func ConfigFromUnified(cfg *config.Config, security pkgif.SecureTransport, muxer pkgif.StreamMuxer) Config {
	baseCfg := NewConfig()

	// 设置安全传输和多路复用器
	baseCfg.SecurityTransports = []pkgif.SecureTransport{security}
	baseCfg.StreamMuxers = []pkgif.StreamMuxer{muxer}

	if cfg != nil {
		baseCfg.NegotiateTimeout = cfg.Security.NegotiateTimeout.Duration()
		baseCfg.HandshakeTimeout = cfg.Security.NegotiateTimeout.Duration() / 2
	}

	return baseCfg
}

// ProvideUpgrader 提供 Upgrader（依赖注入）
func ProvideUpgrader(params Params) (pkgif.Upgrader, error) {
	// 从统一配置创建配置
	cfg := ConfigFromUnified(params.UnifiedCfg, params.Security, params.Muxer)
	return New(params.Identity, cfg)
}
