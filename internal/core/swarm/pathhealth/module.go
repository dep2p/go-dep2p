// Package pathhealth 提供路径健康管理功能
package pathhealth

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// Module 提供路径健康管理的 Fx 模块
func Module() fx.Option {
	return fx.Module("pathhealth",
		fx.Provide(ProvideManager),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideManager 提供路径健康管理器
func ProvideManager(cfg *interfaces.PathHealthConfig) interfaces.PathHealthManager {
	var config *Config
	if cfg != nil {
		config = FromInterfaceConfig(*cfg)
	} else {
		config = DefaultConfig()
	}
	return NewManager(config)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager interfaces.PathHealthManager
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Manager.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.Manager.Stop()
		},
	})
}
