package introspect

import (
	"context"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// Module 返回自省服务 Fx 模块
func Module() fx.Option {
	return fx.Module("introspect",
		fx.Provide(
			ConfigFromUnified,
			NewFromParams,
		),
		fx.Invoke(registerLifecycle),
	)
}

// IntrospectParams 自省服务依赖参数
type IntrospectParams struct {
	fx.In

	UnifiedCfg        *config.Config    `optional:"true"`
	Host              pkgif.Host        `optional:"true"`
	ConnManager       pkgif.ConnManager `optional:"true"`
	BandwidthReporter BandwidthReporter `optional:"true"`
}

// IntrospectOutput 自省服务输出
type IntrospectOutput struct {
	fx.Out

	Server *Server `optional:"true"`
}

// ConfigFromUnified 从统一配置创建自省服务配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Diagnostics.EnableIntrospect {
		return nil // 禁用时返回 nil
	}
	addr := cfg.Diagnostics.IntrospectAddr
	if addr == "" {
		addr = DefaultAddr
	}
	return &Config{
		Addr: addr,
	}
}

// NewFromParams 从参数创建自省服务
func NewFromParams(params IntrospectParams) IntrospectOutput {
	cfg := ConfigFromUnified(params.UnifiedCfg)
	if cfg == nil {
		return IntrospectOutput{} // 禁用时返回空输出
	}

	// 设置依赖组件
	cfg.Host = params.Host
	cfg.ConnManager = params.ConnManager
	cfg.BandwidthReporter = params.BandwidthReporter

	return IntrospectOutput{
		Server: New(*cfg),
	}
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(lc fx.Lifecycle, server *Server) {
	if server == nil {
		return // 禁用时跳过
	}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return server.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return server.Stop()
		},
	})
}
