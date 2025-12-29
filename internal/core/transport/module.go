// Package transport 提供传输层模块的实现
//
// 传输模块负责：
// - 底层网络连接
// - QUIC/TCP 传输
// - 连接管理
package transport

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/transport/quic"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

var log = logger.Logger("transport")

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// 身份模块（必需，用于 TLS 证书）
	Identity identityif.Identity `name:"identity"`

	// 配置（可选，使用默认配置）
	Config *transportif.Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// 公共接口导出（命名依赖）
	Transport transportif.Transport `name:"transport"`

	// QUIC 传输层（内部使用）
	QUICTransport *quic.Transport `name:"quic_transport"`

	// QUIC Muxer 工厂（供 muxer 模块注入使用，避免跨组件实现依赖）
	QuicMuxerFactory muxerif.MuxerFactory `name:"quic_muxer_factory"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	// 验证必需依赖
	if input.Identity == nil {
		return ModuleOutput{}, fmt.Errorf("identity is required")
	}

	// 使用默认配置或输入配置
	config := transportif.DefaultConfig()
	if input.Config != nil {
		config = *input.Config
	}

	// 创建 QUIC 传输实例
	transport, err := quic.NewTransport(config, input.Identity)
	if err != nil {
		return ModuleOutput{}, fmt.Errorf("创建 QUIC 传输层失败: %w", err)
	}

	// 创建 QUIC Muxer 工厂（供 muxer 模块注入使用）
	quicMuxerFactory := &quic.MuxerAdapterFactory{}

	return ModuleOutput{
		Transport:        transport,
		QUICTransport:    transport,
		QuicMuxerFactory: quicMuxerFactory,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("transport",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC        fx.Lifecycle
	Transport transportif.Transport `name:"transport"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("传输模块启动")
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("传输模块停止")
			// 关闭传输层，记录错误但不阻止应用关闭
			if err := input.Transport.Close(); err != nil {
				log.Warn("关闭传输层时出错", "err", err)
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
	Name        = "transport"
	Description = "传输层模块，提供 QUIC 传输能力"
)
