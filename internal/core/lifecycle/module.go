package lifecycle

import (
	"context"

	"go.uber.org/fx"
)

// ModuleResult Fx 模块导出结果
type ModuleResult struct {
	fx.Out

	Coordinator      *Coordinator // 默认导出
	NamedCoordinator *Coordinator `name:"lifecycle_coordinator"` // 命名导出，供 DHT 等模块注入
}

// provideCoordinator 提供 Coordinator 实例
func provideCoordinator() ModuleResult {
	c := NewCoordinator()
	return ModuleResult{
		Coordinator:      c,
		NamedCoordinator: c,
	}
}

// Module 返回 Fx 模块
//
// 提供生命周期协调器作为全局单例。
// 同时导出默认和命名（lifecycle_coordinator）两种形式，
// 以支持不同模块的依赖注入需求。
func Module() fx.Option {
	return fx.Module("lifecycle",
		fx.Provide(
			provideCoordinator,
		),
		fx.Invoke(registerLifecycleHooks),
	)
}

// lifecycleHooksParams 生命周期钩子参数
type lifecycleHooksParams struct {
	fx.In

	Lifecycle   fx.Lifecycle
	Coordinator *Coordinator
}

// registerLifecycleHooks 注册生命周期钩子
func registerLifecycleHooks(params lifecycleHooksParams) {
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 初始化时推进到 A1
			return params.Coordinator.AdvanceTo(PhaseA1IdentityInit)
		},
		OnStop: func(_ context.Context) error {
			params.Coordinator.Stop()
			return nil
		},
	})
}
