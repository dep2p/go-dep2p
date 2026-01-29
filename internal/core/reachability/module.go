// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *interfaces.ReachabilityConfig `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// Coordinator 可达性协调器
	Coordinator interfaces.ReachabilityCoordinator `name:"reachability_coordinator"`

	// CoordinatorImpl 具体实现（仅供内部 wiring 使用）
	CoordinatorImpl *Coordinator `name:"reachability_coordinator_impl"`

	// DialBackService 回拨验证服务
	DialBackService interfaces.DialBackService `name:"dialback_service"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := input.Config
	if config == nil {
		config = interfaces.DefaultReachabilityConfig()
	}

	// 创建协调器
	coordinator := NewCoordinator(config)

	// 创建 dial-back 服务
	dialBackService := NewDialBackService(config)
	coordinator.SetDialBackService(dialBackService)

	// 创建 witness 服务
	witnessService := NewWitnessService(coordinator)
	coordinator.SetWitnessService(witnessService)

	return ModuleOutput{
		Coordinator:     coordinator,
		CoordinatorImpl: coordinator,
		DialBackService: dialBackService,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("reachability",
		fx.Provide(
			ProvideServices,
			ProvideDirectAddrStateMachine,
		),
		fx.Invoke(registerLifecycle),
		fx.Invoke(bindReachabilityCoordinator),
	)
}

// ProvideDirectAddrStateMachine 提供直接地址更新状态机
func ProvideDirectAddrStateMachine() *DirectAddrUpdateStateMachine {
	return NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC           fx.Lifecycle
	Coordinator  *Coordinator                  `name:"reachability_coordinator_impl"`
	StateMachine *DirectAddrUpdateStateMachine `optional:"true"` // 可选的状态机
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	// 集成状态机到 Coordinator（如果存在）
	if input.StateMachine != nil {
		input.Coordinator.SetStateMachine(input.StateMachine)
		logger.Debug("DirectAddrUpdateStateMachine 已注入到 Coordinator")
	}

	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("可达性协调器模块启动")
			return input.Coordinator.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			logger.Info("可达性协调器模块停止")
			return input.Coordinator.Stop()
		},
	})
}

// bindReachabilityCoordinator 将 Coordinator 注入依赖方
func bindReachabilityCoordinator(input struct {
	fx.In
	Coordinator interfaces.ReachabilityCoordinator `name:"reachability_coordinator"`
	Host        interfaces.Host                   `optional:"true"`
	NATService  *nat.Service                      `optional:"true"`
}) {
	if input.Host != nil {
		input.Host.SetReachabilityCoordinator(input.Coordinator)
	}
	if input.NATService != nil {
		input.NATService.SetReachabilityCoordinator(input.Coordinator)
	}
}

// ============================================================================
//                              接口实现检查
// ============================================================================

// 确保 Coordinator 实现 interfaces.ReachabilityCoordinator
var _ interfaces.ReachabilityCoordinator = (*Coordinator)(nil)

// 确保 DialBackService 实现 interfaces.DialBackService
var _ interfaces.DialBackService = (*DialBackService)(nil)
