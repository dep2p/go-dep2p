// Package protocol 提供协议管理模块的实现
//
// 协议模块负责：
// - 协议注册
// - 协议协商
// - 处理器分发
package protocol

import (
	"context"
	"errors"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrNoHandler 没有注册协议处理器
	ErrNoHandler = errors.New("no handler registered for protocol")

	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream is closed")

	// ErrHandlerPanic 处理器崩溃
	ErrHandlerPanic = errors.New("handler panicked")

	// ErrProtocolInvalid 无效的协议 ID
	ErrProtocolInvalid = errors.New("invalid protocol ID")

	// ErrRealmAuthRequired 需要 Realm 验证 (v1.1 新增)
	// 非系统协议在连接未通过 RealmAuth 验证时返回
	ErrRealmAuthRequired = errors.New("realm authentication required for non-system protocol")
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// Handlers 预注册的处理器（value groups 不能是 optional，FX 会自动处理空切片）
	Handlers []ProtocolHandlerEntry `group:"protocol_handlers"`
}

// ProtocolHandlerEntry 协议处理器条目
type ProtocolHandlerEntry struct {
	Protocol types.ProtocolID
	Handler  endpoint.ProtocolHandler
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// ProtocolRouter 协议路由器
	ProtocolRouter protocolif.Router `name:"protocol_router"`
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	router := NewRouter()

	// 注册预提供的处理器
	for _, entry := range input.Handlers {
		router.AddHandler(entry.Protocol, entry.Handler)
	}

	return ModuleOutput{
		ProtocolRouter: router,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("protocol",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC     fx.Lifecycle
	Router protocolif.Router `name:"protocol_router"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("协议路由器启动",
				"protocols", len(input.Router.Protocols()))
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("协议路由器停止")
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
	Name        = "protocol"
	Description = "协议管理模块，提供协议注册、协商和分发能力"
)
