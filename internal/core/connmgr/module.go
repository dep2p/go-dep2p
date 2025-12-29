// Package connmgr 提供连接管理模块的实现
//
// 连接管理模块负责：
// - 连接数量控制
// - 重要连接保护
// - 智能裁剪
// - 黑名单/连接门控
package connmgr

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *connmgr.Config `optional:"true"`

	// GaterConfig 门控配置（可选）
	GaterConfig *connmgr.GaterConfig `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// ConnectionManager 连接管理器
	ConnectionManager connmgr.ConnectionManager `name:"conn_manager"`

	// ConnectionGater 连接门控
	ConnectionGater connmgr.ConnectionGater `name:"conn_gater"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := connmgr.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	gaterConfig := connmgr.DefaultGaterConfig()
	if input.GaterConfig != nil {
		gaterConfig = *input.GaterConfig
	}

	manager := NewConnectionManager(config)

	gater, err := NewConnectionGater(gaterConfig)
	if err != nil {
		return ModuleOutput{}, err
	}

	// 集成 Gater 到 Manager
	manager.SetConnectionGater(gater)

	return ModuleOutput{
		ConnectionManager: manager,
		ConnectionGater:   gater,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("connmgr",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC                fx.Lifecycle
	ConnectionManager connmgr.ConnectionManager `name:"conn_manager"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("连接管理器启动")
			return input.ConnectionManager.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			log.Info("连接管理器停止")
			return input.ConnectionManager.Close()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "connmgr"
	Description = "连接管理模块，提供连接数量控制和保护机制"
)
