// Package netreport 提供网络诊断功能
package netreport

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/netreport")

// ============================================================================
//                              Fx 模块定义
// ============================================================================

// Params 模块依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Result 模块提供的结果
type Result struct {
	fx.Out

	Client *Client
}

// Module 返回 Fx 模块配置
//
// 提供:
//   - *Client: 网络诊断客户端
//
// 生命周期:
//   - 无特殊启动/停止逻辑
func Module() fx.Option {
	return fx.Module("netreport",
		fx.Provide(ProvideClient),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideClient 提供网络诊断客户端
func ProvideClient(p Params) (Result, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)
	client := NewClient(cfg)

	return Result{
		Client: client,
	}, nil
}

// ConfigFromUnified 从统一配置创建网络诊断配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}

	// 使用默认配置作为基础
	result := DefaultConfig()

	// 如果配置了 NAT STUN 服务器，使用相同的服务器
	if len(cfg.NAT.STUNServers) > 0 {
		result.STUNServers = cfg.NAT.STUNServers
	}

	return result
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC     fx.Lifecycle
	Client *Client
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			logger.Info("网络诊断模块已启动")
			return nil
		},
		OnStop: func(_ context.Context) error {
			logger.Info("网络诊断模块已停止")
			return nil
		},
	})
}
