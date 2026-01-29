// Package recovery 提供网络恢复功能
package recovery

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("recovery",
		fx.Provide(ProvideManager),
		fx.Invoke(registerLifecycle),
	)
}

// ModuleWithBridge 返回带桥接器的 Fx 模块
//
// Phase 5.3: 提供 NetworkMonitor 与 RecoveryManager 的自动桥接
// 当 NetworkMonitor 检测到网络故障时，自动触发 RecoveryManager 恢复
func ModuleWithBridge() fx.Option {
	return fx.Module("recovery",
		fx.Provide(ProvideManager),
		fx.Provide(ProvideBridge),
		fx.Invoke(registerLifecycleWithBridge),
	)
}

// ProvideManager 提供恢复管理器
func ProvideManager(cfg *interfaces.RecoveryConfig) interfaces.RecoveryManager {
	var config *Config
	if cfg != nil {
		config = FromInterfaceConfig(*cfg)
	} else {
		config = DefaultConfig()
	}
	return NewManager(config)
}

// ProvideBridge 提供监控桥接器
//
// Phase 5.3: 桥接 NetworkMonitor 和 RecoveryManager
func ProvideBridge(
	monitor interfaces.ConnectionHealthMonitor,
	manager interfaces.RecoveryManager,
) *MonitorBridge {
	return NewMonitorBridge(monitor, manager)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager interfaces.RecoveryManager
}

// registerLifecycle 注册生命周期
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

// lifecycleWithBridgeInput 带桥接器的生命周期输入参数
type lifecycleWithBridgeInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager interfaces.RecoveryManager
	Bridge  *MonitorBridge
}

// registerLifecycleWithBridge 注册带桥接器的生命周期
func registerLifecycleWithBridge(input lifecycleWithBridgeInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动 RecoveryManager
			if err := input.Manager.Start(ctx); err != nil {
				return err
			}
			// 启动桥接器
			input.Bridge.Start(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 停止桥接器
			input.Bridge.Stop()
			// 停止 RecoveryManager
			return input.Manager.Stop()
		},
	})
}
