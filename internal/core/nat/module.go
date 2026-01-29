package nat

import (
	"context"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// Params Fx 依赖注入参数（创建时不依赖 Host，避免循环依赖）
type Params struct {
	fx.In
	Swarm      pkgif.Swarm    `optional:"true"`
	EventBus   pkgif.EventBus `optional:"true"`
	UnifiedCfg *config.Config `optional:"true"`
	// 注意：Host 不在这里，因为 Host 依赖 NAT（可选），会形成循环
	// Host 在 lifecycle 的 fx.Invoke 中注入
}

// ConfigFromUnified 从统一配置创建 NAT 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return &Config{
		EnableAutoNAT:          cfg.NAT.EnableAutoNAT,
		EnableUPnP:             cfg.NAT.EnableUPnP,
		EnableNATPMP:           cfg.NAT.EnableNATPMP,
		EnableHolePunch:        cfg.NAT.EnableHolePunch,
		STUNServers:            cfg.NAT.STUNServers,
		ProbeInterval:          cfg.NAT.AutoNAT.ProbeInterval,
		ProbeTimeout:           cfg.NAT.AutoNAT.ProbeTimeout,
		MappingDuration:        cfg.NAT.UPnP.MappingDuration,
		ConfidenceThreshold:    cfg.NAT.AutoNAT.ConfidenceThreshold,
		ProbeSuccessThreshold:  cfg.NAT.AutoNAT.SuccessThreshold,
		ProbeFailureThreshold:  cfg.NAT.AutoNAT.FailureThreshold,
		STUNCacheDuration:      cfg.NAT.UPnP.CacheDuration,
		MappingRenewalInterval: cfg.NAT.UPnP.RenewalInterval,
		// 
		LockReachabilityPublic: cfg.NAT.LockReachabilityPublic,
	}
}

// NewServiceFromParams 从参数创建服务
func NewServiceFromParams(params Params) (*Service, error) {
	cfg := ConfigFromUnified(params.UnifiedCfg)
	return NewService(cfg, params.Swarm, params.EventBus)
}

// Result NAT 模块导出结果
type Result struct {
	fx.Out
	Service     *Service
	NATService  pkgif.NATService
	HolePuncher *holepunch.HolePuncher `optional:"true"` // P0 修复：导出 HolePuncher 供 Realm Connector 使用
}

// provideNATService 提供 NAT 服务
func provideNATService(params Params) (Result, error) {
	svc, err := NewServiceFromParams(params)
	if err != nil {
		return Result{}, err
	}
	
	return Result{
		Service:     svc,
		NATService:  svc,           // 同一个实例，但类型不同
		HolePuncher: svc.HolePuncher(), // P0 修复：导出 HolePuncher
	}, nil
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("nat",
		fx.Provide(
			provideNATService,
		),
		fx.Invoke(registerLifecycle),
		fx.Invoke(bindLifecycleCoordinator),
	)
}

type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Service *Service
	Host    pkgif.Host `optional:"true"`
}

func registerLifecycle(input lifecycleInput) {
	if input.Host == nil {
		return
	}
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Service.Start(ctx, input.Host)
		},
		OnStop: func(_ context.Context) error {
			return input.Service.Stop()
		},
	})
}

// lifecycleCoordInput 生命周期协调器绑定参数
type lifecycleCoordInput struct {
	fx.In
	Service              *Service
	LifecycleCoordinator *lifecycle.Coordinator `optional:"true"`
}

// bindLifecycleCoordinator 绑定生命周期协调器
//
// 用于 NAT 类型检测完成后通知 A3 gate 解除
func bindLifecycleCoordinator(input lifecycleCoordInput) {
	if input.LifecycleCoordinator != nil {
		input.Service.SetLifecycleCoordinator(input.LifecycleCoordinator)
	}
}
