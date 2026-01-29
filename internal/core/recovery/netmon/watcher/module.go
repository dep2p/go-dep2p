package watcher

import (
	"context"

	"go.uber.org/fx"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              生命周期注册
// ============================================================================

// registerHandlerLifecycle 注册处理器生命周期
func registerHandlerLifecycle(lc fx.Lifecycle, h *Handler) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return h.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return h.Stop()
		},
	})
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 网络监控模块
//
// 注意：不再提供适配器函数，因为类型别名会导致 Fx 循环依赖。
// Handler 直接通过 optional 标签接收可选依赖。
var Module = fx.Module("network",
	fx.Provide(
		// 提供网络监控器
		fx.Annotate(
			NewMonitor,
			fx.As(new(pkgif.NetworkMonitor)),
		),

		// 提供网络变化处理器
		NewHandler,
	),

	// 注册生命周期
	fx.Invoke(registerHandlerLifecycle),
)
