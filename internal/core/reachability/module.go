// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"

	"go.uber.org/fx"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// NAT 服务（可选）
	NAT natif.NATService `name:"nat" optional:"true"`

	// 地址管理器（可选）
	AddressManager addressif.AddressManager `name:"address_manager" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// Coordinator 可达性协调器
	Coordinator reachabilityif.Coordinator `name:"reachability_coordinator"`

	// CoordinatorImpl 具体实现（仅供本组件内部 wiring/lifecycle 使用）
	CoordinatorImpl *Coordinator `name:"reachability_coordinator_impl"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	coordinator := NewCoordinator(
		input.NAT,
		nil, // AutoRelay 通过 fx.Invoke 阶段回注入，避免形成 fx 依赖环
		input.AddressManager,
	)

	return ModuleOutput{
		Coordinator:     coordinator,
		CoordinatorImpl: coordinator,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("reachability",
		fx.Provide(ProvideServices),
		fx.Invoke(wireAutoRelay),
		fx.Invoke(wireDialBack),
		fx.Invoke(registerLifecycle),
	)
}

type wireAutoRelayInput struct {
	fx.In

	Coordinator *Coordinator      `name:"reachability_coordinator_impl"`
	AutoRelay   relayif.AutoRelay `name:"auto_relay" optional:"true"`
}

// wireAutoRelay 负责将 auto_relay 注入到 Coordinator，避免 Provide 阶段形成 fx 环
func wireAutoRelay(input wireAutoRelayInput) {
	if input.Coordinator == nil || input.AutoRelay == nil {
		return
	}
	input.Coordinator.SetAutoRelay(input.AutoRelay)
}

type wireInput struct {
	fx.In

	Coordinator *Coordinator           `name:"reachability_coordinator_impl"`
	Endpoint    endpointif.Endpoint    `name:"endpoint" optional:"true"`
	Config      *reachabilityif.Config `optional:"true"`
}

// wireDialBack 负责将 endpoint/config 注入到 Coordinator，避免 Provide 阶段形成 fx 环
func wireDialBack(input wireInput) {
	if input.Coordinator == nil || input.Endpoint == nil {
		return
	}

	cfg := reachabilityif.DefaultConfig()
	if input.Config != nil {
		cfg = input.Config
	}

	input.Coordinator.SetEndpoint(input.Endpoint)
	input.Coordinator.EnableDialBack(cfg.EnableDialBack, cfg)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC          fx.Lifecycle
	Coordinator *Coordinator `name:"reachability_coordinator_impl"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("可达性协调器模块启动")
			return input.Coordinator.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			log.Info("可达性协调器模块停止")
			return input.Coordinator.Stop()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "reachability"
	Description = "可达性协调模块，统一管理地址发布，实现'可达性优先'策略"
)
