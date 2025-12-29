// Package netreport 提供网络诊断模块的实现
//
// 网络诊断模块负责：
// - IPv4/IPv6 连通性检测
// - NAT 类型检测
// - 中继延迟测量
package netreport

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// 包级别日志实例
var log = logger.Logger("netreport")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *netreportif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// Client 网络诊断客户端
	Client netreportif.Client `name:"netreport_client"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 使用默认配置或输入配置
	config := netreportif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建客户端
	client := NewClient(config)

	return ModuleOutput{
		Client: client,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("netreport",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC     fx.Lifecycle
	Client netreportif.Client `name:"netreport_client"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("网络诊断模块启动")

			// 可选：启动时自动生成初始报告
			go func() {
				_, err := input.Client.GetReport(context.Background())
				if err != nil {
					log.Warn("初始网络诊断失败", "err", err)
				}
			}()

			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("网络诊断模块停止")
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
	Name        = "netreport"
	Description = "网络诊断模块，提供 IPv4/IPv6 连通性检测、NAT 类型检测和中继延迟测量"
)

