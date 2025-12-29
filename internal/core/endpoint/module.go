// Package endpoint 提供 Endpoint 聚合模块的实现
//
// Endpoint 模块是 DeP2P 的核心入口点，负责：
// - 聚合所有子模块
// - 提供统一的 API
// - 管理节点生命周期
package endpoint

import (
	"context"
	"time"

	"go.uber.org/fx"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"

	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
)

// ============================================================================
//                              模块输入依赖
// ============================================================================

// ModuleInput 定义模块输入依赖
type ModuleInput struct {
	fx.In

	// 核心依赖
	Identity  identityif.Identity   `name:"identity"`
	Transport transportif.Transport `name:"transport"`

	// 安全和多路复用
	Security     securityif.SecureTransport `name:"secure_transport" optional:"true"`
	MuxerFactory muxerif.MuxerFactory       `name:"muxer_factory" optional:"true"`

	// 可选依赖
	Discovery      coreif.DiscoveryService  `name:"discovery" optional:"true"`
	NAT            natif.NATService         `name:"nat" optional:"true"`
	// 注意：Relay 不在此处依赖，避免 relay ↔ endpoint 循环依赖
	// RelayClient 通过 relay 模块的 lifecycle 注入到 Endpoint
	AddressBook    addressif.AddressBook    `name:"address_book" optional:"true"`
	AddressManager addressif.AddressManager `name:"address_manager" optional:"true"`

	// 可达性协调器（可选，但在启用 Relay/NAT 时建议启用）
	Reachability reachabilityif.Coordinator `name:"reachability_coordinator" optional:"true"`

	// 协议路由器 - 用于统一管理协议处理器
	ProtocolRouter protocolif.Router `name:"protocol_router" optional:"true"`

	// 连接管理 - 水位线控制和连接保护
	ConnManager connmgrif.ConnectionManager `name:"conn_manager" optional:"true"`

	// 连接门控 - 黑名单和连接拦截
	ConnGater connmgrif.ConnectionGater `name:"conn_gater" optional:"true"`

	// 配置
	Config *Config `optional:"true"`
}

// ============================================================================
//                              模块输出服务
// ============================================================================

// ModuleOutput 定义模块输出服务
type ModuleOutput struct {
	fx.Out

	// 公共接口导出
	Endpoint coreif.Endpoint `name:"endpoint"`
}

// ============================================================================
//                              配置
// ============================================================================

// Config Endpoint 配置
type Config struct {
	// ListenAddrs 监听地址列表
	ListenAddrs []string

	// DialTimeout 拨号超时
	DialTimeout int // 秒

	// MaxConnections 最大连接数
	MaxConnections int

	// InboundRateLimit 入站连接速率限制（每秒允许的新连接数）
	// 0 表示不限制
	InboundRateLimit int

	// InboundRateBurst 入站连接速率限制的突发容量
	// 允许短时间内超过速率限制的连接数
	InboundRateBurst int

	// ExternalAddrs 用户显式声明的公网地址（作为候选地址）
	// 用于公网服务器场景：当节点有独立公网IP但无法通过UPnP/STUN自动发现时
	// 这些地址将作为 Candidate 输入到 dial-back 验证流程
	ExternalAddrs []string

	// StartedAt 启动时间（由 Listen 设置）
	// REQ-OPS-001: 用于诊断报告中的 Uptime 计算
	StartedAt time.Time
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ListenAddrs:      []string{"0.0.0.0:0"},
		DialTimeout:      30,
		MaxConnections:   100,
		InboundRateLimit: 50,  // 默认每秒 50 个新连接
		InboundRateBurst: 100, // 允许突发 100 个
	}
}

// ============================================================================
//                              服务提供
// ============================================================================

// ProvideServices 提供模块服务
func ProvideServices(input ModuleInput) (ModuleOutput, error) {
	config := DefaultConfig()
	if input.Config != nil {
		config = input.Config
	}

	// 创建 Endpoint 实例
	// 注意：Relay 通过 TransportRegistry + RelayTransport 透明工作，无需在此传入
	ep := NewEndpointWithConfig(
		input.Identity,
		input.Transport,
		input.Security,
		input.MuxerFactory,
		input.Discovery,
		input.NAT,
		input.ProtocolRouter,
		input.ConnManager,
		input.ConnGater,
		config,
	)

	// 注入可达性协调器（可达性优先：通告地址以协调器为准）
	if input.Reachability != nil {
		ep.SetReachabilityCoordinator(input.Reachability)
	}

	return ModuleOutput{
		Endpoint: ep,
	}, nil
}

// ============================================================================
//                              模块定义
// ============================================================================

// Module 返回 fx 模块配置
func Module() fx.Option {
	return fx.Module("endpoint",
		fx.Provide(ProvideServices),
		fx.Invoke(registerLifecycle),
	)
}

// lifecycleInput 生命周期输入参数
type lifecycleInput struct {
	fx.In
	LC       fx.Lifecycle
	Endpoint coreif.Endpoint `name:"endpoint"`
}

// registerLifecycle 注册生命周期
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			log.Info("Endpoint 已初始化",
				"nodeID", input.Endpoint.ID().ShortString())
			// 注意：不在此处调用 Listen()，由用户代码或 dep2p.Start() 显式控制
			return nil
		},
		OnStop: func(_ context.Context) error {
			log.Info("Endpoint 关闭中")
			return input.Endpoint.Close()
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
	Name = "endpoint"
	// Description 模块描述
	Description = "Endpoint 聚合模块，提供统一的 P2P 入口"
)
