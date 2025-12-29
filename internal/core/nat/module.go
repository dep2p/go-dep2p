// Package nat 提供 NAT 穿透模块的实现
//
// NAT 模块负责：
// - NAT 类型检测
// - 外部地址发现
// - 端口映射
// - 打洞协调
package nat

import (
	"context"
	"time"

	"go.uber.org/fx"

	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *natif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// NATService NAT 服务
	NATService natif.NATService `name:"nat"`

	// STUNClient STUN 客户端
	STUNClient natif.STUNClient `name:"stun_client" optional:"true"`

	// HolePuncher 打洞器
	HolePuncher natif.HolePuncher `name:"hole_puncher" optional:"true"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := natif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建 NAT 服务
	service := NewService(config)

	return ModuleOutput{
		NATService:  service,
		STUNClient:  service.STUNClient(),
		HolePuncher: service.HolePuncher(),
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("nat",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC         fx.Lifecycle
	NATService natif.NATService `name:"nat"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 启动时检测 NAT 类型
			go func() {
				detectCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				natType, err := input.NATService.DetectNATType(detectCtx)
				if err != nil {
					log.Debug("NAT 类型检测失败", "err", err)
				} else {
					log.Info("NAT 模块启动", "natType", natType.String())
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("NAT 模块停止")
			if err := input.NATService.Close(); err != nil {
				log.Warn("NAT 服务关闭失败", "err", err)
			}
			return nil
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "nat"
	Description = "NAT 穿透模块，提供 STUN、UPnP、NAT-PMP 和打洞能力"
)
