// Package netmon 提供网络状态监控功能
package netmon

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("netmon",
		fx.Provide(ProvideMonitor),
		fx.Invoke(registerLifecycle),
	)
}

// monitorParams 监控器依赖参数
type monitorParams struct {
	fx.In

	Config         *interfaces.ConnectionHealthMonitorConfig `optional:"true"`
	NetworkMonitor interfaces.NetworkMonitor                 `optional:"true"` // 用于订阅系统网络变化
}

// ProvideMonitor 提供网络监控器
func ProvideMonitor(params monitorParams) interfaces.ConnectionHealthMonitor {
	var config *Config
	if params.Config != nil {
		config = FromInterfaceConfig(*params.Config)
	} else {
		config = DefaultConfig()
	}
	m := NewMonitor(config)

	// 如果有 NetworkMonitor，设置为系统网络事件源
	if params.NetworkMonitor != nil {
		m.SetNetworkMonitor(params.NetworkMonitor)
	}

	return m
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Monitor interfaces.ConnectionHealthMonitor
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Monitor.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.Monitor.Stop()
		},
	})
}
