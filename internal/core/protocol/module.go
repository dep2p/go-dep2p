// Package protocol 实现协议注册与路由
package protocol

import (
	"context"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/protocol/system/identify"
	"github.com/dep2p/go-dep2p/internal/core/protocol/system/ping"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Params Protocol 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("protocol",
		fx.Provide(
			ProvideConfig,
			ProvideRegistry,
			ProvideNegotiator,
			ProvideRouter,
			ProvideIdentifySubscriber,
		),
		// 注册系统协议
		fx.Invoke(registerSystemProtocols),
		// P0 修复：启动 Identify 订阅器
		fx.Invoke(startIdentifySubscriber),
	)
}

// identifySubscriberParams Identify 订阅器依赖参数
type identifySubscriberParams struct {
	fx.In

	Host        pkgif.Host
	Coordinator pkgif.ReachabilityCoordinator `name:"reachability_coordinator" optional:"true"` // 
}

// ProvideIdentifySubscriber 提供 Identify 订阅器
//
// 
func ProvideIdentifySubscriber(params identifySubscriberParams) *IdentifySubscriber {
	return NewIdentifySubscriber(params.Host, params.Coordinator)
}

// identifySubscriberInput Identify 订阅器启动输入
type identifySubscriberInput struct {
	fx.In

	Lifecycle  fx.Lifecycle
	Subscriber *IdentifySubscriber
}

// startIdentifySubscriber 启动 Identify 订阅器
//
// P0 修复：订阅连接事件，主动调用 Identify 客户端获取远端地址并写入 Peerstore
func startIdentifySubscriber(input identifySubscriberInput) {
	input.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Subscriber.Start(ctx)
		},
		OnStop: func(_ context.Context) error {
			return input.Subscriber.Stop()
		},
	})
}

// ConfigFromUnified 从统一配置创建协议配置
func ConfigFromUnified(cfg *config.Config) Config {
	if cfg == nil {
		return DefaultConfig()
	}
	return Config{
		NegotiationTimeout: cfg.Security.NegotiateTimeout.Duration(),
	}
}

// NewConfig 创建默认配置（用于测试和直接调用）
func NewConfig() Config {
	return DefaultConfig()
}

// ProvideConfig 从统一配置提供协议配置
func ProvideConfig(p Params) Config {
	return ConfigFromUnified(p.UnifiedCfg)
}

// ProvideRegistry 提供协议注册表
func ProvideRegistry() pkgif.ProtocolRegistry {
	return NewRegistry()
}

// ProvideNegotiator 提供协议协商器
func ProvideNegotiator(registry pkgif.ProtocolRegistry) pkgif.ProtocolNegotiator {
	return NewNegotiator(registry.(*Registry))
}

// ProvideRouter 提供协议路由器
// 注意：返回具体类型 *Router 而非接口，确保 Fx 能正确注入到 host.ModuleInput
func ProvideRouter(registry pkgif.ProtocolRegistry, negotiator pkgif.ProtocolNegotiator) *Router {
	return NewRouter(registry.(*Registry), negotiator.(*Negotiator))
}

// systemProtocolsInput 系统协议注册输入
type systemProtocolsInput struct {
	fx.In

	Registry pkgif.ProtocolRegistry
	Host     pkgif.Host // 移除 name 标签
}

// registerSystemProtocols 注册系统协议
//
// 
// SetStreamHandler 内部会调用 router.AddRoute -> registry.Register，
// 无需重复调用 registry.Register，避免 "protocol already registered" 警告
//
// 验证：所有节点类型（包括基础设施节点）都必须注册 Ping 和 Identify
func registerSystemProtocols(input systemProtocolsInput) error {
	registry := input.Registry
	host := input.Host

	logger.Info("开始注册系统协议", "nodeID", host.ID())

	// Ping 协议 - 只通过 SetStreamHandler 注册（统一入口）
	pingService := ping.NewService()
	host.SetStreamHandler(ping.ProtocolID, pingService.Handler)
	logger.Debug("Ping 协议已注册", "protocolID", ping.ProtocolID)

	// Identify 协议 - 只通过 SetStreamHandler 注册（统一入口）
	idService := identify.NewService(host, registry)
	host.SetStreamHandler(identify.ProtocolID, idService.Handler)
	logger.Debug("Identify 协议已注册", "protocolID", identify.ProtocolID)

	// 验证注册成功
	registeredProtocols := registry.Protocols()
	if len(registeredProtocols) < 2 {
		logger.Warn("系统协议注册数量不足",
			"expected", 2,
			"actual", len(registeredProtocols),
			"protocols", registeredProtocols)
	}

	logger.Info("系统协议注册完成",
		"nodeID", host.ID(),
		"protocols", []string{ping.ProtocolID, identify.ProtocolID})

	return nil
}
