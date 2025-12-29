// Package messaging 提供消息服务模块的实现
//
// 消息模块负责：
// - 请求响应模式
// - 发布订阅模式
// - 查询模式
package messaging

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
)

// 包级别日志实例
var log = logger.Logger("messaging")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *messagingif.Config `optional:"true"`

	// Endpoint 端点（可选，用于发送消息）
	Endpoint endpoint.Endpoint `name:"endpoint" optional:"true"`

	// Identity 身份（可选，用于消息签名）
	Identity identityif.Identity `name:"identity" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// MessagingService 消息服务
	MessagingService messagingif.MessagingService `name:"messaging"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := messagingif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	service := NewMessagingService(config, input.Endpoint, input.Identity)

	return ModuleOutput{
		MessagingService: service,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("messaging",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC               fx.Lifecycle
	MessagingService messagingif.MessagingService `name:"messaging"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("消息服务启动")
			return input.MessagingService.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			log.Info("消息服务停止")
			return input.MessagingService.Stop()
		},
	})
}

// ============================================================================
//                              模块元信息
// ============================================================================

// 模块元信息常量
const (
	Version     = "1.0.0"
	Name        = "messaging"
	Description = "消息服务模块，提供请求响应、发布订阅和查询模式"
)
