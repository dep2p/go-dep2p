// Package eventbus 实现事件总线
package eventbus

import (
	"context"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// ============================================================================
// Fx 模块
// ============================================================================

// Result Fx 模块输出结果
type Result struct {
	fx.Out

	EventBus pkgif.EventBus // 移除 name 标签
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("eventbus",
		fx.Provide(ProvideEventBus),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideEventBus 提供 EventBus 实例
func ProvideEventBus() Result {
	return Result{
		EventBus: NewBus(),
	}
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC       fx.Lifecycle
	EventBus pkgif.EventBus // 移除 name 标签
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// EventBus 启动（当前无需特殊启动逻辑）
			return nil
		},
		OnStop: func(_ context.Context) error {
			// EventBus 停止（当前无需特殊停止逻辑）
			return nil
		},
	})
}

// ============================================================================
// 模块元信息
// ============================================================================

const (
	// Version 模块版本
	Version = "1.0.0"
	// Name 模块名称
	Name = "eventbus"
	// Description 模块描述
	Description = "事件总线模块，提供类型安全的事件发布/订阅机制"
)
