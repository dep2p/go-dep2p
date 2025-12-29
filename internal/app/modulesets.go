// Package app 提供模块集合清单
//
// modulesets.go 集中维护"哪些模块属于哪个 Tier"，是 Bootstrap 组装的唯一模块来源。
// 这样模块归属清单统一在 internal/app，避免双入口漂移。
package app

import (
	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/core/bandwidth"
	"github.com/dep2p/go-dep2p/internal/core/connmgr"
	"github.com/dep2p/go-dep2p/internal/core/discovery"
	"github.com/dep2p/go-dep2p/internal/core/endpoint"
	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/internal/core/liveness"
	"github.com/dep2p/go-dep2p/internal/core/messaging"
	"github.com/dep2p/go-dep2p/internal/core/muxer"
	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/netreport"
	"github.com/dep2p/go-dep2p/internal/core/protocol"
	"github.com/dep2p/go-dep2p/internal/core/reachability"
	"github.com/dep2p/go-dep2p/internal/core/realm"
	"github.com/dep2p/go-dep2p/internal/core/relay"
	"github.com/dep2p/go-dep2p/internal/core/security"
	"github.com/dep2p/go-dep2p/internal/core/transport"
)

// ============================================================================
//                              固定必选模块集合（不依赖配置裁剪）
// ============================================================================

// FoundationModules 基础层模块组合 (Tier 1)
//
// 包含身份和地址管理，是所有其他模块的基础。
// 这些模块始终加载，不受配置开关控制。
func FoundationModules() fx.Option {
	return fx.Options(
		identity.Module(),
		address.Module(),
	)
}

// TransportModules 传输层模块组合 (Tier 2)
//
// 包含传输、安全和多路复用功能。
// 这些模块始终加载，不受配置开关控制。
func TransportModules() fx.Option {
	return fx.Options(
		transport.Module(),
		security.Module(),
		muxer.Module(),
	)
}

// ApplicationModules 应用层模块组合 (Tier 5)
//
// 包含协议、连接管理和消息传递功能。
// 这些模块始终加载，不受配置开关控制。
func ApplicationModules() fx.Option {
	return fx.Options(
		protocol.Module(),
		connmgr.Module(),
		messaging.Module(),
	)
}

// MonitoringModules 监控模块组合
//
// 包含带宽统计和网络报告功能。
// 这些模块始终加载，不受配置开关控制。
func MonitoringModules() fx.Option {
	return fx.Options(
		bandwidth.Module(),
		netreport.Module(),
	)
}

// EndpointModules 端点层模块组合 (Tier 6)
//
// 包含 API 聚合层。
// 这些模块始终加载，不受配置开关控制。
func EndpointModules() fx.Option {
	return fx.Options(
		endpoint.Module(),
	)
}

// ============================================================================
//                              可选模块单模块入口（供 Bootstrap 按配置选择）
// ============================================================================

// NATModule NAT 穿透模块
//
// 包含 STUN/UPnP/NAT-PMP 等 NAT 穿透功能。
// 由 Bootstrap 根据 config.NAT.Enable 决定是否加载。
func NATModule() fx.Option {
	return nat.Module()
}

// DiscoveryModule 发现服务模块
//
// 包含 DHT/mDNS/Bootstrap/Rendezvous 等节点发现功能。
// v1.1+：强制内建，由 Bootstrap 始终加载。
func DiscoveryModule() fx.Option {
	return discovery.Module()
}

// RelayModule 中继服务模块
//
// 包含中继客户端和服务器功能。
// 由 Bootstrap 根据 config.Relay.Enable 决定是否加载。
func RelayModule() fx.Option {
	return relay.Module()
}

// ReachabilityModule 可达性协调模块
//
// 统一管理 NAT/Relay/AddressManager 的地址发布，实现“可达性优先”策略。
// 该模块对依赖均为 optional，安全可插拔；建议在 Relay 之后装配，以便接入 auto_relay。
func ReachabilityModule() fx.Option {
	return reachability.Module()
}

// LivenessModule 存活检测模块
//
// 包含心跳、Ping、Goodbye 等存活检测功能。
// 由 Bootstrap 根据 config.Liveness.Enable 决定是否加载。
func LivenessModule() fx.Option {
	return liveness.Module()
}

// RealmModule 领域管理模块
//
// 包含业务网络隔离功能。
// v1.1+：强制内建，由 Bootstrap 始终加载。
func RealmModule() fx.Option {
	return realm.Module()
}

// ============================================================================
//                              组合模块集合（供测试/特殊场景使用）
// ============================================================================

// CoreModules 核心模块组合
//
// 包含 DeP2P 运行所必需的核心模块（Foundation + Transport）。
func CoreModules() fx.Option {
	return fx.Options(
		FoundationModules(),
		TransportModules(),
	)
}

// AllModules 所有模块组合
//
// 包含 DeP2P 的所有功能模块（不按配置裁剪）。
// 主要用于测试或需要完整功能的场景。
func AllModules() fx.Option {
	return fx.Options(
		FoundationModules(),
		TransportModules(),
		NATModule(),
		DiscoveryModule(),
		RelayModule(),
		ReachabilityModule(),
		LivenessModule(),
		RealmModule(),
		ApplicationModules(),
		MonitoringModules(),
		EndpointModules(),
	)
}

// MinimalModules 最小模块配置
//
// 仅包含最基本的连接功能，适用于简单场景或测试。
// 包含：Foundation + Transport + Endpoint
func MinimalModules() fx.Option {
	return fx.Options(
		FoundationModules(),
		TransportModules(),
		EndpointModules(),
	)
}
