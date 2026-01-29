package dep2p

import (
	"context"
	"fmt"
	"path/filepath"

	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/pkg/lib/log"

	// Core Layer
	"github.com/dep2p/go-dep2p/internal/core/connmgr"
	"github.com/dep2p/go-dep2p/internal/core/eventbus"
	"github.com/dep2p/go-dep2p/internal/core/host"
	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/internal/core/lifecycle" // 生命周期协调器
	"github.com/dep2p/go-dep2p/internal/core/metrics"
	"github.com/dep2p/go-dep2p/internal/core/muxer"
	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/nat/netreport" // P2 修复：合并到 nat 子目录
	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/core/protocol"
	"github.com/dep2p/go-dep2p/internal/core/reachability" // P2 修复
	"github.com/dep2p/go-dep2p/internal/core/recovery"
	"github.com/dep2p/go-dep2p/internal/core/recovery/netmon"
	"github.com/dep2p/go-dep2p/internal/core/recovery/netmon/watcher"
	"github.com/dep2p/go-dep2p/internal/core/relay"
	"github.com/dep2p/go-dep2p/internal/core/resourcemgr"
	"github.com/dep2p/go-dep2p/internal/core/security"
	"github.com/dep2p/go-dep2p/internal/core/storage"
	"github.com/dep2p/go-dep2p/internal/core/swarm"
	"github.com/dep2p/go-dep2p/internal/core/swarm/bandwidth"
	"github.com/dep2p/go-dep2p/internal/core/swarm/pathhealth"
	"github.com/dep2p/go-dep2p/internal/core/transport"
	"github.com/dep2p/go-dep2p/internal/core/upgrader"
	"github.com/dep2p/go-dep2p/internal/debug/introspect" // P3 修复：移至 debug 层

	// Discovery Layer
	"github.com/dep2p/go-dep2p/internal/discovery/bootstrap"
	"github.com/dep2p/go-dep2p/internal/discovery/coordinator"
	"github.com/dep2p/go-dep2p/internal/discovery/dht"
	"github.com/dep2p/go-dep2p/internal/discovery/dns"
	"github.com/dep2p/go-dep2p/internal/discovery/mdns"
	"github.com/dep2p/go-dep2p/internal/discovery/rendezvous"

	// Core Layer - 地址管理（系统协议）
	"github.com/dep2p/go-dep2p/internal/core/reachability/addrmgmt"

	// Realm Layer（只加载 Manager，组件在 JoinRealm 时动态创建）
	"github.com/dep2p/go-dep2p/internal/realm"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// buildFxApp 构建 Fx 应用
//
// 组装所有内部模块，采用条件加载策略：
//   - 核心模块：必须加载（Identity, EventBus, Peerstore, Protocol）
//   - 条件模块：根据配置加载（DHT, mDNS, Relay, NAT 等）
//   - 扩展模块：用户自定义 Fx 选项
//
// 加载顺序（按依赖）：
//  1. Core Layer: Identity → Transport → Security → Muxer → Upgrader → Swarm → Host
//  2. Discovery Layer: DHT → Bootstrap → mDNS → Coordinator
//  3. Realm Layer: Auth → Member → Routing → Gateway → Manager
//  4. Protocol Layer: Messaging → PubSub → Streams → Liveness

var fxLogger = log.Logger("dep2p/fx")

func buildFxApp(cfg *nodeConfig, node *Node) (*fx.App, error) {
	// ════════════════════════════════════════════════════════════════════════
	// 1. 配置验证（前置）
	// ════════════════════════════════════════════════════════════════════════
	if err := cfg.config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// ════════════════════════════════════════════════════════════════════════
	// 2. 核心模块（必须加载）
	// ════════════════════════════════════════════════════════════════════════
	reachabilityConfig := pkgif.DefaultReachabilityConfig()
	if cfg.config != nil && cfg.config.Storage.DataDir != "" {
		reachabilityConfig.DirectAddrStorePath = filepath.Join(cfg.config.Storage.DataDir, "direct_addrs.json")
	}
	// 应用 STUN 信任模式配置
	if cfg.trustSTUNAddresses {
		reachabilityConfig.TrustSTUNAddresses = true
	}
	modules := []fx.Option{
		// 配置注入
		fx.Supply(cfg),
		fx.Supply(cfg.config),
		fx.Supply(reachabilityConfig),

		// 生命周期协调器（全局单例，协调各阶段依赖）
		lifecycle.Module(),

		// 基础组件（必须）
		identity.Module(),  // 身份管理
		eventbus.Module(),  // 事件总线
		storage.Module(),   // 存储引擎 (v1.1.0+ 必需，提供 BadgerDB 持久化)
		peerstore.Module(), // 节点存储 (依赖 storage)
		// 注意：protocol.Module() 移至 Host 之后，因为 registerSystemProtocols 需要 Host
	}

	// ════════════════════════════════════════════════════════════════════════
	// 3. 指标和资源管理（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if cfg.config.Resource.EnableResourceManager {
		modules = append(modules, metrics.Module)
		modules = append(modules, resourcemgr.Module)
	}

	// ════════════════════════════════════════════════════════════════════════
	// 4. 传输层（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if hasAnyTransport(cfg.config) {
		modules = append(modules,
			transport.Module(), // QUIC/TCP 传输
			security.Module(),  // TLS/Noise 安全
			muxer.Module,       // Yamux 多路复用
			upgrader.Module(),  // 连接升级器
			swarm.Module,       // 连接池
		)

		// Host（核心门面，依赖传输层）
		modules = append(modules, host.Module())

		// Protocol（协议注册，依赖 Host 用于系统协议注册）
		modules = append(modules, protocol.Module())
	} else {
		return nil, fmt.Errorf("at least one transport must be enabled (QUIC or TCP)")
	}

	// ════════════════════════════════════════════════════════════════════════
	// 5. 连接管理（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if cfg.config.ConnMgr.HighWater > 0 {
		modules = append(modules, connmgr.Module())
	}

	// ════════════════════════════════════════════════════════════════════════
	// 5.5 网络变化监控（始终加载）
	// ════════════════════════════════════════════════════════════════════════
	modules = append(modules, watcher.Module)

	// ════════════════════════════════════════════════════════════════════════
	// 6. NAT 穿透（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if hasAnyNAT(cfg.config) {
		modules = append(modules, nat.Module())

		// P2 修复：加载可达性协调器模块（与 NAT 配合）
		modules = append(modules, reachability.Module())

		// P2 修复：加载网络诊断模块（提供诊断 API）
		modules = append(modules, netreport.Module())

		// 外部地址发现组件连接（NAT Service → Coordinator, Host → Coordinator）
		modules = append(modules, fx.Invoke(wireAddressDiscovery))

		// 将 HolePuncher 注入到 Swarm
		// 使 Swarm 在直连失败后能够触发 HolePunch 打洞
		modules = append(modules, fx.Invoke(wireHolePuncher))
	}

	// ════════════════════════════════════════════════════════════════════════
	// 7. 中继服务（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if hasAnyRelay(cfg.config) {
		modules = append(modules, relay.Module())
	}

	// ════════════════════════════════════════════════════════════════════════
	// 8. 发现层（条件加载）
	// ════════════════════════════════════════════════════════════════════════
	if cfg.config.Discovery.EnableDHT {
		modules = append(modules, dht.Module)
	}
	if cfg.config.Discovery.EnableBootstrap {
		modules = append(modules, bootstrap.Module)
	}
	if cfg.config.Discovery.EnableMDNS {
		modules = append(modules, mdns.Module)
	}
	if cfg.config.Discovery.EnableDNS {
		modules = append(modules, dns.Module)
	}
	if cfg.config.Discovery.EnableRendezvous {
		modules = append(modules, rendezvous.Module)
	}

	// Discovery 协调器（始终加载，聚合所有发现机制）
	modules = append(modules, coordinator.Module)

	// ════════════════════════════════════════════════════════════════════════
	// 9. 增强功能模块（条件加载）
	// ════════════════════════════════════════════════════════════════════════

	// 9.1 带宽统计（条件加载）
	if cfg.config.Bandwidth.Enabled {
		modules = append(modules,
			fx.Provide(provideBandwidthConfig(cfg.config)),
			bandwidth.Module(),
		)
	}

	// 9.2 & 9.4 连接健康监控 + 网络恢复（统一处理以支持桥接）
	//
	// Phase 8 修复：当两者都启用时，使用 ResilienceModule 自动桥接 Monitor → Recovery
	// 否则单独加载各自的模块
	connectionHealthEnabled := cfg.config.ConnectionHealth.Enabled
	recoveryEnabled := cfg.config.Recovery.Enabled

	if connectionHealthEnabled && recoveryEnabled {
		// 两者都启用：使用完整的弹性模块（包含桥接）
		modules = append(modules,
			fx.Provide(provideConnectionHealthConfig(cfg.config)),
			fx.Provide(provideRecoveryConfig(cfg.config)),
			recovery.ResilienceModule(),
		)
		fxLogger.Debug("已加载 ResilienceModule（带 Monitor→Recovery 桥接）")
	} else {
		// 单独加载
		if connectionHealthEnabled {
			modules = append(modules,
				fx.Provide(provideConnectionHealthConfig(cfg.config)),
				netmon.Module(),
			)
		}
		if recoveryEnabled {
			modules = append(modules,
				fx.Provide(provideRecoveryConfig(cfg.config)),
				recovery.Module(),
			)
		}
	}

	// 9.3 路径健康管理（条件加载）
	if cfg.config.PathHealth.Enabled {
		modules = append(modules,
			fx.Provide(providePathHealthConfig(cfg.config)),
			pathhealth.Module(),
		)
	}

	// P3 修复：9.5 自省服务（条件加载，依赖配置）
	if cfg.config.Diagnostics.EnableIntrospect {
		modules = append(modules, introspect.Module())
	}

	// P1 修复完成：9.6 地址管理协议（始终加载）
	modules = append(modules, addrmgmt.Module())

	// ════════════════════════════════════════════════════════════════════════
	// 10. RealmManager（始终加载，组件在 JoinRealm 时动态创建）
	// ════════════════════════════════════════════════════════════════════════
	// RealmManager 是 Realm 的工厂，Protocol 服务（Messaging、PubSub、Streams、Liveness）
	// 在 JoinRealm() 时动态创建并绑定到特定 Realm
	modules = append(modules, realm.ManagerModule())

	// ════════════════════════════════════════════════════════════════════════
	// 11. 用户扩展（Fx Options）
	// ════════════════════════════════════════════════════════════════════════
	if len(cfg.userFxOptions) > 0 {
		modules = append(modules, cfg.userFxOptions...)
	}

	// ════════════════════════════════════════════════════════════════════════
	// 12. Node 组件注入
	// ════════════════════════════════════════════════════════════════════════
	modules = append(modules, fx.Invoke(injectNodeComponents(node)))

	// ════════════════════════════════════════════════════════════════════════
	// 13. 已知节点连接（可选）
	// ════════════════════════════════════════════════════════════════════════
	if len(cfg.config.KnownPeers) > 0 {
		modules = append(modules, fx.Invoke(wireKnownPeersConnection(cfg.config.KnownPeers)))
	}

	// ════════════════════════════════════════════════════════════════════════
	// 14. Fx 配置
	// ════════════════════════════════════════════════════════════════════════
	modules = append(modules,
		// 禁用 Fx 日志输出（避免干扰用户日志）
		fx.WithLogger(func() fxevent.Logger {
			return &fxevent.ZapLogger{Logger: zap.NewNop()}
		}),
		fx.NopLogger,
	)

	app := fx.New(modules...)
	return app, nil
}

// ════════════════════════════════════════════════════════════════════════════
// 条件检查辅助函数
// ════════════════════════════════════════════════════════════════════════════

// hasAnyTransport 检查是否启用任何传输协议
func hasAnyTransport(cfg *config.Config) bool {
	return cfg.Transport.EnableQUIC ||
		cfg.Transport.EnableTCP ||
		cfg.Transport.EnableWebSocket
}

// hasAnyNAT 检查是否启用任何 NAT 穿透功能
func hasAnyNAT(cfg *config.Config) bool {
	return cfg.NAT.EnableAutoNAT ||
		cfg.NAT.EnableUPnP ||
		cfg.NAT.EnableNATPMP ||
		cfg.NAT.EnableHolePunch
}

// hasAnyRelay 检查是否启用中继服务
func hasAnyRelay(cfg *config.Config) bool {
	return cfg.Relay.EnableClient || cfg.Relay.EnableServer
}

// ════════════════════════════════════════════════════════════════════════════
// 组件注入辅助函数
// ════════════════════════════════════════════════════════════════════════════

// nodeInjectParams Node 组件注入参数
//
// 注意：BandwidthCounter、PathHealthManager 等增强功能组件
// 已通过各自模块的内部依赖注入消费（如 swarm.Module、ResilienceModule），
// 不需要在 Node 层面再次注入。
type nodeInjectParams struct {
	fx.In

	// 核心组件（必需）
	Host           pkgif.Host           // 网络主机
	RealmManager   pkgif.RealmManager   // Realm 管理器
	NetworkMonitor pkgif.NetworkMonitor // 网络监控器

	// 可选组件
	Discovery    pkgif.Discovery  `optional:"true"` // 发现服务
	NATService   pkgif.NATService `optional:"true"` // NAT 穿透服务
	RelayManager *relay.Manager   `optional:"true"` // 中继管理器

	// 能力服务（始终可选，用于能力开关）
	BootstrapService *bootstrap.BootstrapService `name:"bootstrap_service" optional:"true"`

	// P2 修复：网络诊断客户端
	NetReportClient *netreport.Client `optional:"true"` // 网络诊断

	// 自省服务（P3 修复完成）
	IntrospectServer *introspect.Server `optional:"true"`

	// 生命周期协调器（对齐 20260125-node-lifecycle-cross-cutting.md）
	LifecycleCoordinator *lifecycle.Coordinator `optional:"true"`
}

// injectNodeComponents 创建 Node 组件注入函数
//
// 使用统一的注入结构，所有可选组件通过 optional:"true" 标签处理
func injectNodeComponents(node *Node) interface{} {
	return func(params nodeInjectParams) {
		// 核心组件
		node.host = params.Host
		node.realmManager = params.RealmManager
		node.networkMonitor = params.NetworkMonitor

		// 可选组件
		node.discovery = params.Discovery
		node.natService = params.NATService
		node.relayManager = params.RelayManager
		node.bootstrapService = params.BootstrapService

		// P2 修复：网络诊断客户端
		node.netReportClient = params.NetReportClient

		// 自省服务（P3 修复完成）
		node.introspectServer = params.IntrospectServer

		// 生命周期协调器
		node.lifecycleCoordinator = params.LifecycleCoordinator
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 配置转换函数
// ════════════════════════════════════════════════════════════════════════════

// provideBandwidthConfig 提供带宽统计配置
func provideBandwidthConfig(cfg *config.Config) func() *pkgif.BandwidthConfig {
	return func() *pkgif.BandwidthConfig {
		return &pkgif.BandwidthConfig{
			Enabled:         cfg.Bandwidth.Enabled,
			TrackByPeer:     cfg.Bandwidth.EnablePerPeer,
			TrackByProtocol: cfg.Bandwidth.EnablePerProtocol,
			TrimInterval:    cfg.Bandwidth.TrimInterval.Duration(),
			IdleTimeout:     cfg.Bandwidth.IdleTimeout.Duration(),
		}
	}
}

// provideConnectionHealthConfig 提供连接健康监控配置
func provideConnectionHealthConfig(cfg *config.Config) func() *pkgif.ConnectionHealthMonitorConfig {
	return func() *pkgif.ConnectionHealthMonitorConfig {
		return &pkgif.ConnectionHealthMonitorConfig{
			ErrorThreshold:      cfg.ConnectionHealth.ErrorThreshold,
			ProbeInterval:       cfg.ConnectionHealth.ProbeInterval.Duration(),
			ErrorWindow:         cfg.ConnectionHealth.ErrorWindow.Duration(),
			StateChangeDebounce: cfg.ConnectionHealth.DebounceDuration.Duration(),
		}
	}
}

// providePathHealthConfig 提供路径健康管理配置
func providePathHealthConfig(cfg *config.Config) func() *pkgif.PathHealthConfig {
	return func() *pkgif.PathHealthConfig {
		return &pkgif.PathHealthConfig{
			ProbeInterval:        cfg.PathHealth.ProbeInterval.Duration(),
			EWMAAlpha:            cfg.PathHealth.EWMAAlpha,
			SwitchHysteresis:     cfg.PathHealth.SwitchHysteresis,
			StabilityWindow:      cfg.PathHealth.StabilityWindow.Duration(),
			PathExpiry:           cfg.PathHealth.PathExpiry.Duration(),
			DeadFailureThreshold: cfg.PathHealth.DeadThreshold,
		}
	}
}

// provideRecoveryConfig 提供网络恢复配置
func provideRecoveryConfig(cfg *config.Config) func() *pkgif.RecoveryConfig {
	return func() *pkgif.RecoveryConfig {
		return &pkgif.RecoveryConfig{
			RecoveryTimeout:       cfg.Recovery.RecoveryTimeout.Duration(),
			InitialBackoff:        cfg.Recovery.ReconnectBackoff.Duration(),
			MaxBackoff:            cfg.Recovery.MaxReconnectBackoff.Duration(),
			RebindOnCriticalError: cfg.Recovery.RebindEnabled,
			RediscoverAddresses:   cfg.Recovery.DiscoveryEnabled,
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 外部地址发现组件连接
// ════════════════════════════════════════════════════════════════════════════

// addressDiscoveryWireParams 地址发现组件连接参数
type addressDiscoveryWireParams struct {
	fx.In

	NodeConfig           *nodeConfig                   `optional:"true"` // 用户配置（含 advertiseAddrs）
	ReachabilityConfig   *pkgif.ReachabilityConfig     `optional:"true"` // 可达性配置
	Host                 pkgif.Host                    `optional:"true"`
	NATService           *nat.Service                  `optional:"true"`
	Coordinator          pkgif.ReachabilityCoordinator `name:"reachability_coordinator" optional:"true"`
	CoordImpl            *reachability.Coordinator     `name:"reachability_coordinator_impl" optional:"true"`
	Swarm                pkgif.Swarm                   `optional:"true"`
	DialBackService      pkgif.DialBackService         `name:"dialback_service" optional:"true"`
	DHT                  pkgif.DHT                     `optional:"true"`
	LifecycleCoordinator *lifecycle.Coordinator        `optional:"true"` // 生命周期协调器
	RelayManager         *relay.Manager                `optional:"true"` // Relay 管理器（A5 前置依赖）
}

// wireAddressDiscovery 连接地址发现组件
//
// 这个函数在所有组件创建后执行，负责将它们连接起来：
//  1. NAT Service → Coordinator (STUN/UPnP/NAT-PMP 发现的地址上报)
//  2. Host → Coordinator (地址管理整合)
//  3. Coordinator → 监听端口 (用于与发现的外部 IP 组合)
//  4. 用户配置地址 → Coordinator (WithPublicAddr 配置的地址)
//  5. Lifecycle Gate: 地址就绪后触发 DHT 发布
func wireAddressDiscovery(params addressDiscoveryWireParams) {
	fxLogger.Debug("开始连接地址发现组件",
		"hasNATService", params.NATService != nil,
		"hasCoordinator", params.Coordinator != nil,
		"hasHost", params.Host != nil,
		"hasSwarm", params.Swarm != nil,
		"hasCoordImpl", params.CoordImpl != nil,
		"hasLifecycleCoordinator", params.LifecycleCoordinator != nil)

	// 1. 将 Coordinator 设置到 NAT Service
	if params.NATService != nil && params.Coordinator != nil {
		params.NATService.SetReachabilityCoordinator(params.Coordinator)
		fxLogger.Debug("NAT Service 已连接到 Reachability Coordinator")
	}

	// 1.1 设置 STUN 信任模式（如果配置启用）
	if params.NATService != nil && params.ReachabilityConfig != nil && params.ReachabilityConfig.TrustSTUNAddresses {
		params.NATService.SetTrustSTUNAddresses(true)
		fxLogger.Info("STUN 信任模式已启用")
	}

	// 2. 将 Coordinator 设置到 Host
	if params.Host != nil && params.Coordinator != nil {
		params.Host.SetReachabilityCoordinator(params.Coordinator)
		fxLogger.Debug("Host 已连接到 Reachability Coordinator")
	}

	// 3. 从 Swarm 获取监听端口并设置到 Coordinator 和 NAT Service
	if params.Swarm != nil {
		listenAddrs := params.Swarm.ListenAddrs()
		fxLogger.Debug("Swarm 监听地址", "addrs", listenAddrs)

		ports := extractListenPorts(listenAddrs)
		fxLogger.Info("从 Swarm 提取监听端口", "ports", ports)

		if len(ports) > 0 {
			// 设置到 Coordinator（用于网卡发现）
			if params.CoordImpl != nil {
				params.CoordImpl.SetListenPorts(ports)
				fxLogger.Info("Coordinator 监听端口已设置", "ports", ports)
			} else {
				fxLogger.Debug("CoordImpl 为空，跳过监听端口设置")
			}
			// 设置到 NAT Service（用于 STUN 地址组合）
			if params.NATService != nil {
				params.NATService.SetListenPorts(ports)
				fxLogger.Info("NAT Service 监听端口已设置", "ports", ports)
			} else {
				fxLogger.Debug("NATService 为空，跳过监听端口设置")
			}
		} else {
			// 这是预期行为：wireAddressDiscovery 在 Swarm 监听之前调用
			// 监听端口会在 Swarm 启动后通过 EvtLocalAddrsUpdated 事件更新
			fxLogger.Debug("Swarm 尚未监听，监听端口将通过地址更新事件获取",
				"listenAddrs", listenAddrs)
		}
	} else {
		fxLogger.Warn("Swarm 为空，无法获取监听端口")
	}

	// 4. 注入用户配置的公网地址（WithPublicAddr 设置的地址）
	// 这些地址是用户明确声明可用的，优先级最高
	if params.CoordImpl != nil && params.NodeConfig != nil && len(params.NodeConfig.advertiseAddrs) > 0 {
		params.CoordImpl.SetConfiguredAddresses(params.NodeConfig.advertiseAddrs)
		fxLogger.Info("用户配置地址已注入 Coordinator",
			"count", len(params.NodeConfig.advertiseAddrs),
			"addrs", params.NodeConfig.advertiseAddrs)
	}

	// 5. 设置 DialBackService 的 StreamOpener
	// 用于真正的回拨验证而不是模拟验证
	if params.DialBackService != nil && params.Host != nil {
		opener := &hostStreamOpener{host: params.Host}
		params.DialBackService.SetStreamOpener(opener)
		fxLogger.Info("DialBackService StreamOpener 已设置")
	} else {
		if params.DialBackService == nil {
			fxLogger.Debug("DialBackService 为空，跳过 StreamOpener 设置")
		}
		if params.Host == nil {
			fxLogger.Debug("Host 为空，跳过 StreamOpener 设置")
		}
	}

	// 6. 生命周期 Gate: 地址就绪后触发 DHT 发布
	//
	// 关键改进（对齐 20260125-node-lifecycle-cross-cutting.md Section 7.2）:
	// - 引入 NAT 类型驱动的地址发布决策树
	// - Full Cone/RestrictedCone/PortRestricted → 立即发布 direct_addrs
	// - Symmetric NAT → 先发 relay_addrs，后续可达验证再补充 direct_addrs
	// - 设置 lifecycle address ready 信号
	// - DHT 发布由信号驱动，而非盲目重试
	//
	// 时序对齐（Phase A5）：
	// - 必须等待 NAT 类型检测完成（A3 gate）
	// - 如果配置了 Relay，必须等待 Relay 连接完成（A5 前置 gate）
	if params.CoordImpl != nil {
		params.CoordImpl.SetOnAddressChanged(func(addrs []string) {
			// 时序对齐检查：NAT 类型是否就绪
			natTypeReady := params.LifecycleCoordinator == nil || params.LifecycleCoordinator.IsNATTypeReady()
			if !natTypeReady {
				fxLogger.Debug("地址变更但 NAT 类型未就绪，跳过地址就绪判断")
				return
			}

			// 时序对齐检查：如果配置了 Relay，检查是否已连接
			relayConfigured := params.RelayManager != nil && params.RelayManager.HasConfiguredRelay()
			relayConnected := params.LifecycleCoordinator == nil || params.LifecycleCoordinator.IsRelayConnected()
			if relayConfigured && !relayConnected {
				fxLogger.Debug("地址变更但 Relay 未连接，跳过地址就绪判断",
					"relayConfigured", relayConfigured)
				return
			}

			// 分类地址
			var directAddrs, relayAddrs []string
			for _, addr := range addrs {
				if isRelayAddress(addr) {
					relayAddrs = append(relayAddrs, addr)
				} else {
					directAddrs = append(directAddrs, addr)
				}
			}

			hasDirectAddr := len(directAddrs) > 0
			hasRelayAddr := len(relayAddrs) > 0
			hasPublishable := hasDirectAddr || hasRelayAddr

			// 获取 NAT 类型以决定发布策略
			natType := types.NATTypeUnknown
			if params.NATService != nil {
				natType = params.NATService.GetNATType()
			}

			fxLogger.Debug("地址变更检测（NAT 类型决策树）",
				"totalAddrs", len(addrs),
				"directAddrs", len(directAddrs),
				"relayAddrs", len(relayAddrs),
				"natType", natType.String(),
				"relayConfigured", relayConfigured,
				"relayConnected", relayConnected)

			// NAT 类型驱动的地址就绪判断
			// 对齐设计文档 Section 7.2：
			// - None/FullCone: 直接地址立即可用
			// - RestrictedCone/PortRestricted: 直接地址需验证，但可发布
			// - Symmetric: 必须有 Relay 地址才算就绪
			addressReady := false
			switch natType {
			case types.NATTypeNone, types.NATTypeFullCone:
				// 公网或 Full Cone: 有直接地址即可
				addressReady = hasDirectAddr
			case types.NATTypeRestrictedCone, types.NATTypePortRestricted:
				// Restricted/Port Restricted: 有直接地址或 Relay 地址
				addressReady = hasDirectAddr || hasRelayAddr
			case types.NATTypeSymmetric, types.NATTypeUnknown:
				// Symmetric/Unknown: 必须有 Relay 地址（直接地址对外无效）
				// 这是关键改进：Symmetric NAT 的 direct_addrs 对外部节点无效
				addressReady = hasRelayAddr
				if !hasRelayAddr && hasDirectAddr {
					fxLogger.Warn("Symmetric NAT 下仅有直接地址，等待 Relay 地址",
						"natType", natType.String(),
						"directAddrs", len(directAddrs))
				}
			}

			// 设置生命周期 address ready 信号
			if addressReady && params.LifecycleCoordinator != nil {
				if !params.LifecycleCoordinator.IsAddressReady() {
					params.LifecycleCoordinator.SetAddressReady()
					fxLogger.Info("生命周期 A5 gate: 地址就绪信号已设置",
						"natType", natType.String(),
						"directAddrs", len(directAddrs),
						"relayAddrs", len(relayAddrs))
				}
			}

			// v2.0 重构：DHT 发布已改由各组件自行管理
			// - 节点级别：DHT 组件自动定期刷新
			// - Realm 级别：通过 RealmManager.NotifyNetworkChange 处理
			// 此处仅记录地址变更事件
			if hasPublishable {
				fxLogger.Info("地址变更已检测，将由相关组件触发更新",
					"natType", natType.String(),
					"addrs", len(addrs))
			}
		})
	}

	fxLogger.Info("外部地址发现组件已连接")
}

// isRelayAddress 检查地址是否为 Relay 地址
func isRelayAddress(addr string) bool {
	return len(addr) > 0 && (contains(addr, "/p2p-circuit/") || contains(addr, "p2p-circuit"))
}

// contains 简单字符串包含检查
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// hostStreamOpener 将 Host 适配为 StreamOpener 接口
type hostStreamOpener struct {
	host pkgif.Host
}

// OpenStream 打开到指定节点的流
func (h *hostStreamOpener) OpenStream(ctx context.Context, peerID string, protocolID string) (pkgif.StreamReadWriteCloser, error) {
	return h.host.NewStream(ctx, peerID, protocolID)
}

// extractListenPorts 从监听地址中提取端口
func extractListenPorts(addrs []string) []int {
	portSet := make(map[int]struct{})

	for _, addr := range addrs {
		port := extractPortFromMultiaddr(addr)
		if port > 0 {
			portSet[port] = struct{}{}
		}
	}

	ports := make([]int, 0, len(portSet))
	for port := range portSet {
		ports = append(ports, port)
	}
	return ports
}

// extractPortFromMultiaddr 从 multiaddr 中提取端口
func extractPortFromMultiaddr(addr string) int {
	// 简单解析：找到 /udp/ 或 /tcp/ 后面的数字
	parts := splitMultiaddrComponents(addr)
	for i, part := range parts {
		if (part == "udp" || part == "tcp") && i+1 < len(parts) {
			port := 0
			for _, c := range parts[i+1] {
				if c >= '0' && c <= '9' {
					port = port*10 + int(c-'0')
				}
			}
			if port > 0 && port < 65536 {
				return port
			}
		}
	}
	return 0
}

// splitMultiaddrComponents 分割 multiaddr 为组件
func splitMultiaddrComponents(addr string) []string {
	if addr == "" {
		return nil
	}
	if addr[0] == '/' {
		addr = addr[1:]
	}
	var parts []string
	start := 0
	for i := 0; i < len(addr); i++ {
		if addr[i] == '/' {
			if i > start {
				parts = append(parts, addr[start:i])
			}
			start = i + 1
		}
	}
	if start < len(addr) {
		parts = append(parts, addr[start:])
	}
	return parts
}

// ════════════════════════════════════════════════════════════════════════════
// HolePuncher 注入
// ════════════════════════════════════════════════════════════════════════════

// holePuncherWireParams HolePuncher 注入参数
type holePuncherWireParams struct {
	fx.In

	Swarm      pkgif.Swarm  `optional:"true"`
	NATService *nat.Service `optional:"true"`
}

// wireHolePuncher 将 HolePuncher 注入到 Swarm
//
// 使 Swarm 在直连失败后能够触发 HolePunch 打洞
// 这是实现 iroh/go-libp2p 同等 NAT 穿透能力的关键
func wireHolePuncher(params holePuncherWireParams) {
	if params.Swarm == nil || params.NATService == nil {
		fxLogger.Debug("Swarm 或 NATService 为空，跳过 HolePuncher 注入")
		return
	}

	// 获取 HolePuncher
	puncher := params.NATService.HolePuncher()
	if puncher == nil {
		fxLogger.Debug("HolePuncher 为空（可能 HolePunch 未启用）")
		return
	}

	// 将 HolePuncher 注入到 Swarm
	// 注意：需要类型断言获取具体的 Swarm 实现
	if s, ok := params.Swarm.(*swarm.Swarm); ok {
		s.SetHolePuncher(puncher)

		// 设置 DirectDialer（Swarm 实现了 DirectDialer 接口）
		puncher.SetDirectDialer(s)

		fxLogger.Info("HolePuncher 已注入到 Swarm")
	} else {
		fxLogger.Debug("Swarm 类型断言失败，跳过 HolePuncher 注入")
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 已知节点连接
// ════════════════════════════════════════════════════════════════════════════

// knownPeersWireParams 已知节点连接参数
type knownPeersWireParams struct {
	fx.In

	Host                 pkgif.Host             `optional:"true"`
	LifecycleCoordinator *lifecycle.Coordinator `optional:"true"`
}

// wireKnownPeersConnection 创建已知节点连接函数
//
// 启动时直接连接配置的已知节点，不依赖引导节点或 DHT 发现。
// 适用于云服务器部署、私有网络等已知节点地址的场景。
func wireKnownPeersConnection(knownPeers []config.KnownPeer) interface{} {
	return func(params knownPeersWireParams) {
		if params.Host == nil {
			fxLogger.Debug("Host 为空，跳过已知节点连接")
			return
		}

		if len(knownPeers) == 0 {
			return
		}

		fxLogger.Info("配置了已知节点，将在启动后连接", "count", len(knownPeers))

		// 转换为 host.KnownPeer 类型
		hostPeers := make([]host.KnownPeer, len(knownPeers))
		for i, p := range knownPeers {
			hostPeers[i] = host.KnownPeer{
				PeerID: p.PeerID,
				Addrs:  p.Addrs,
			}
		}

		// 在后台连接已知节点（不阻塞启动）
		go func() {
			ctx := context.Background()

			// 如果有 Host 实现了 ConnectKnownPeers 方法，则调用
			if h, ok := params.Host.(*host.Host); ok {
				if err := h.ConnectKnownPeers(ctx, hostPeers); err != nil {
					fxLogger.Warn("连接已知节点时出错", "error", err)
				}
			} else {
				// 回退方案：逐个连接
				for _, peer := range knownPeers {
					if err := params.Host.Connect(ctx, peer.PeerID, peer.Addrs); err != nil {
						fxLogger.Warn("连接已知节点失败",
							"peerID", peer.PeerID,
							"error", err)
					} else {
						fxLogger.Info("连接已知节点成功", "peerID", peer.PeerID)
					}
				}
			}
		}()
	}
}
