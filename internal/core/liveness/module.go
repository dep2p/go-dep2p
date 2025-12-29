// Package liveness 实现节点存活检测模块
//
// Liveness 模块负责：
// - 节点状态检测（Online/Degraded/Offline/Unknown）
// - 心跳机制
// - 优雅下线（Goodbye 协议）
// - 健康评分
package liveness

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
)

// 包级别日志实例
var log = logger.Logger("liveness")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置
	Config *config.Config

	// Endpoint 网络端点（可选，用于发送 Ping/Goodbye）
	Endpoint endpoint.Endpoint `name:"endpoint" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// LivenessService 存活检测服务
	LivenessService livenessif.LivenessService `name:"liveness"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	cfg := input.Config.Liveness

	// 创建存活检测服务
	service := NewService(cfg, input.Endpoint)

	return ModuleOutput{
		LivenessService: service,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("liveness",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In

	LC              fx.Lifecycle
	LivenessService livenessif.LivenessService `name:"liveness"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("存活检测模块启动")
			return input.LivenessService.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			log.Info("存活检测模块停止")
			return input.LivenessService.Stop()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	// Version 模块版本
	Version = "1.0.0"
	// Name 模块名称
	Name = "liveness"
	// Description 模块描述
	Description = "节点存活检测模块，提供 Ping、心跳和 Goodbye 协议能力"
)
