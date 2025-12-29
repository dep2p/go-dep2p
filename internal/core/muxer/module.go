// Package muxer 提供多路复用模块的实现
//
// 多路复用模块负责：
// - 在单个连接上创建多个流
// - 流量控制
// - 优先级调度
package muxer

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/muxer/yamux"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// 包级别日志实例
var log = logger.Logger("muxer")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Config 配置（可选）
	Config *muxerif.Config `optional:"true"`

	// QuicMuxerFactory QUIC 连接的 muxer 工厂（可选，由 transport 模块提供）
	// 通过 Fx 注入，避免 muxer 组件直接依赖 transport/quic 实现
	QuicMuxerFactory muxerif.MuxerFactory `name:"quic_muxer_factory" optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// MuxerFactory 多路复用器工厂
	MuxerFactory muxerif.MuxerFactory `name:"muxer_factory"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := muxerif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建 yamux 工厂作为默认实现
	yamuxFactory := yamux.NewFactory(config)

	// 使用复合工厂，自动检测 QUIC 连接并使用适配器
	// quicMuxerFactory 通过 Fx 注入（可为 nil），避免跨组件实现依赖
	factory := NewCompositeFactory(input.QuicMuxerFactory, yamuxFactory)

	return ModuleOutput{
		MuxerFactory: factory,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("muxer",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC           fx.Lifecycle
	MuxerFactory muxerif.MuxerFactory `name:"muxer_factory"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("多路复用模块启动",
				"protocol", input.MuxerFactory.Protocol())
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("多路复用模块停止")
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
	Name        = "muxer"
	Description = "多路复用模块，提供 yamux 多路复用支持"
)
