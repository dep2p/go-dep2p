// Package recovery 提供网络恢复功能
package recovery

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/recovery/netmon"
	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ResilienceModule 返回网络弹性完整 Fx 模块
//
// Phase 5.3: IMPL-NETWORK-RESILIENCE 完整集成
// 包含:
// - NetworkMonitor: 网络状态监控
// - RecoveryManager: 网络恢复管理
// - MonitorBridge: 监控与恢复桥接
// - SystemWatcher: 系统网络事件监听
func ResilienceModule() fx.Option {
	return fx.Module("network-resilience",
		// 提供 NetworkMonitor
		fx.Provide(provideMonitorWithWatcher),
		// 提供 RecoveryManager
		fx.Provide(ProvideManager),
		// 提供桥接器
		fx.Provide(ProvideBridge),
		// 注册生命周期
		fx.Invoke(registerResilienceLifecycle),
	)
}

// provideMonitorWithWatcher 提供带 SystemWatcher 的 NetworkMonitor
func provideMonitorWithWatcher(cfg *interfaces.ConnectionHealthMonitorConfig) interfaces.ConnectionHealthMonitor {
	var config *netmon.Config
	if cfg != nil {
		config = netmon.FromInterfaceConfig(*cfg)
	} else {
		config = netmon.DefaultConfig()
	}

	monitor := netmon.NewMonitor(config)

	// Phase 5.2: 创建并设置 SystemWatcher
	watcherConfig := netmon.DefaultWatcherConfig()
	watcher := netmon.NewSystemWatcher(watcherConfig)
	monitor.SetSystemWatcher(watcher)

	return monitor
}

// resilienceLifecycleInput 网络弹性生命周期输入
type resilienceLifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Monitor interfaces.ConnectionHealthMonitor
	Manager interfaces.RecoveryManager
	Bridge  *MonitorBridge
}

// registerResilienceLifecycle 注册网络弹性生命周期
func registerResilienceLifecycle(input resilienceLifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("启动网络弹性模块")

			// 1. 启动 NetworkMonitor
			if err := input.Monitor.Start(ctx); err != nil {
				logger.Error("启动 NetworkMonitor 失败", "error", err)
				return err
			}

			// 2. 启动 RecoveryManager
			if err := input.Manager.Start(ctx); err != nil {
				logger.Error("启动 RecoveryManager 失败", "error", err)
				return err
			}

			// 3. 启动桥接器
			input.Bridge.Start(ctx)

			logger.Info("网络弹性模块启动完成")
			return nil
		},
		OnStop: func(_ context.Context) error {
			logger.Info("停止网络弹性模块")

			// 逆序停止
			// 1. 停止桥接器
			input.Bridge.Stop()

			// 2. 停止 RecoveryManager
			if err := input.Manager.Stop(); err != nil {
				logger.Warn("停止 RecoveryManager 失败", "error", err)
			}

			// 3. 停止 NetworkMonitor
			if err := input.Monitor.Stop(); err != nil {
				logger.Warn("停止 NetworkMonitor 失败", "error", err)
			}

			logger.Info("网络弹性模块已停止")
			return nil
		},
	})
}

// ResilienceConfig 网络弹性配置
type ResilienceConfig struct {
	// MonitorConfig 监控配置
	MonitorConfig *interfaces.ConnectionHealthMonitorConfig

	// RecoveryConfig 恢复配置
	RecoveryConfig *interfaces.RecoveryConfig

	// EnableSystemWatcher 是否启用系统网络监听
	EnableSystemWatcher bool
}

// DefaultResilienceConfig 返回默认配置
func DefaultResilienceConfig() *ResilienceConfig {
	return &ResilienceConfig{
		EnableSystemWatcher: true,
	}
}
