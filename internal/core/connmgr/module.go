// Package connmgr 实现连接管理器
package connmgr

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("connmgr",
		fx.Provide(
			ConfigFromUnified,
			ProvideManager,
			ProvideGater,
			ProvideScheduler,      // P2 修复完成：拨号调度器
			ProvideSubnetLimiter,  // P4 新增：子网限制器
		),
		fx.Invoke(registerLifecycle),
		fx.Invoke(registerSchedulerLifecycle),  // P2 修复完成
		fx.Invoke(registerSubnetLimiterLifecycle), // P4 新增
	)
}

// ProvideSubnetLimiter 提供子网限制器（P4 新增）
func ProvideSubnetLimiter() *SubnetLimiter {
	return NewSubnetLimiter(DefaultSubnetLimiterConfig())
}

// subnetLimiterLifecycleInput 子网限制器生命周期输入（P4 新增）
type subnetLimiterLifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Limiter *SubnetLimiter `optional:"true"`
}

// registerSubnetLimiterLifecycle 注册子网限制器生命周期（P4 新增）
func registerSubnetLimiterLifecycle(input subnetLimiterLifecycleInput) {
	if input.Limiter == nil {
		return
	}

	input.LC.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			input.Limiter.Close()
			return nil
		},
	})
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager pkgif.ConnManager
	Config  Config
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	// 类型断言获取Manager实现
	mgr, ok := input.Manager.(*Manager)
	if !ok {
		return
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// 启动后台裁剪循环
			mgr.startTrimLoop(ctx)
			return nil
		},
		OnStop: func(_ context.Context) error {
			// Manager.Close() 会由Fx自动调用
			return nil
		},
	})
}

// schedulerLifecycleInput 调度器生命周期输入（P2 修复完成）
type schedulerLifecycleInput struct {
	fx.In
	LC        fx.Lifecycle
	Scheduler *Scheduler `optional:"true"`
}

// registerSchedulerLifecycle 注册调度器生命周期（P2 修复完成）
func registerSchedulerLifecycle(input schedulerLifecycleInput) {
	if input.Scheduler == nil {
		return
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Scheduler.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.Scheduler.Stop()
		},
	})
}

// ConfigFromUnified 从统一配置创建连接管理配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return Config{
		LowWater:      cfg.ConnMgr.LowWater,
		HighWater:     cfg.ConnMgr.HighWater,
		GracePeriod:   cfg.ConnMgr.GracePeriod.Duration(),
		DecayInterval: cfg.ConnMgr.DecayInterval.Duration(),
	}
}

// ProvideManager 提供连接管理器
func ProvideManager(cfg Config) (pkgif.ConnManager, error) {
	return New(cfg)
}

// ProvideGater 提供连接门控器
func ProvideGater() pkgif.ConnGater {
	return NewGater()
}

// ProvideScheduler 提供拨号调度器（P2 修复完成）
func ProvideScheduler(host pkgif.Host, _ Config) *Scheduler {
	if host == nil {
		return nil
	}

	schedulerCfg := DefaultSchedulerConfig()
	// 可从 cfg 中读取调度器相关配置

	return NewScheduler(host, schedulerCfg)
}
