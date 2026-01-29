package dht

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	coreRelay "github.com/dep2p/go-dep2p/internal/core/relay"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Module DHT Fx 模块
var Module = fx.Module("discovery_dht",
	fx.Provide(
		NewFromParams,
	),
	fx.Invoke(registerDHTLifecycle),
)

// Params DHT 依赖参数
type Params struct {
	fx.In

	Host        pkgif.Host                      // 网络主机
	Peerstore   pkgif.Peerstore                 `optional:"true"`
	EventBus    pkgif.EventBus                  `optional:"true"` // 事件总线，用于监听连接事件
	UnifiedCfg  *config.Config                  `optional:"true"`
	Identity    pkgif.Identity                  `optional:"true"` // Step A5: 用于签名 PeerRecord
	Coordinator pkgif.ReachabilityCoordinator   `name:"reachability_coordinator" optional:"true"` // Step A5: 可达性协调器
}

// Result DHT 导出结果
//添加 pkgif.DHT 接口导出供 RealmManager 使用
type Result struct {
	fx.Out

	DHT            *DHT
	DHTInterface   pkgif.DHT                       // 导出 pkgif.DHT 接口供 RealmManager 使用
	Discovery      pkgif.Discovery `name:"dht"`           // 带名称，供 Coordinator 收集
	GroupDiscovery pkgif.Discovery `group:"discoveries"`  // 也添加到 group 中
}

// ConfigFromUnified 从统一配置创建 DHT 配置
func ConfigFromUnified(cfg *config.Config) *Config {
	if cfg == nil || !cfg.Discovery.EnableDHT {
		// 返回禁用的配置
		c := DefaultConfig()
		c.EnableValueStore = false
		return c
	}
	if cfg.Discovery.DHT.ProviderTTL.Duration() > coreRelay.DefaultAddressEntryTTL {
		logger.Warn("DHT ProviderTTL 大于 Relay 地址簿 TTL",
			"providerTTL", cfg.Discovery.DHT.ProviderTTL.Duration(),
			"relayAddressBookTTL", coreRelay.DefaultAddressEntryTTL)
	}
	
	// 解析引导节点配置
	var bootstrapPeers []types.PeerInfo
	if cfg.Discovery.EnableBootstrap && len(cfg.Discovery.Bootstrap.Peers) > 0 {
		bootstrapPeers = parseBootstrapPeers(cfg.Discovery.Bootstrap.Peers)
		if len(bootstrapPeers) > 0 {
			logger.Info("DHT 从统一配置解析引导节点",
				"configuredCount", len(cfg.Discovery.Bootstrap.Peers),
				"parsedCount", len(bootstrapPeers))
		}
	}
	
	return &Config{
		BucketSize:        cfg.Discovery.DHT.BucketSize,
		Alpha:             cfg.Discovery.DHT.Alpha,
		QueryTimeout:      cfg.Discovery.DHT.QueryTimeout.Duration(),
		RefreshInterval:   cfg.Discovery.DHT.RefreshInterval.Duration(),
		ReplicationFactor: cfg.Discovery.DHT.ReplicationFactor,
		EnableValueStore:  cfg.Discovery.DHT.EnableValueStore,
		MaxRecordAge:      cfg.Discovery.DHT.MaxRecordAge.Duration(),
		BootstrapPeers:    bootstrapPeers, // 从统一配置解析
		ProviderTTL:       cfg.Discovery.DHT.ProviderTTL.Duration(),
		PeerRecordTTL:     cfg.Discovery.DHT.PeerRecordTTL.Duration(),
		CleanupInterval:   cfg.Discovery.DHT.CleanupInterval.Duration(),
		RepublishInterval: cfg.Discovery.DHT.RepublishInterval.Duration(),
	}
}

// parseBootstrapPeers 解析引导节点字符串为 PeerInfo
//
// 将 multiaddr 字符串（如 "/ip4/1.2.3.4/tcp/4001/p2p/QmXXX"）
// 解析为 PeerInfo 结构。
func parseBootstrapPeers(addrs []string) []types.PeerInfo {
	var peers []types.PeerInfo
	
	for _, addr := range addrs {
		// 解析 multiaddr 字符串为 AddrInfo
		addrInfo, err := types.AddrInfoFromString(addr)
		if err != nil {
			logger.Debug("解析 DHT 引导节点地址失败", "addr", addr, "error", err)
			continue
		}
		
		// 转换为 PeerInfo
		peerInfo := addrInfo.ToPeerInfo()
		peerInfo.Source = types.SourceBootstrap
		
		peers = append(peers, peerInfo)
	}
	
	return peers
}

// NewFromParams 从 Fx 参数创建 DHT
func NewFromParams(p Params) (Result, error) {
	// 从统一配置获取 DHT 配置
	cfg := ConfigFromUnified(p.UnifiedCfg)

	// 创建 DHT（使用配置）
	dht, err := NewWithConfig(p.Host, p.Peerstore, cfg)
	if err != nil {
		return Result{}, err
	}

	// 设置 EventBus 用于监听连接事件
	if p.EventBus != nil {
		dht.SetEventBus(p.EventBus)
		logger.Info("DHT EventBus 已设置，将自动处理连接事件")
	}

	// Step A5 对齐：初始化 LocalPeerRecordManager
	// 使用 Identity 的私钥进行 PeerRecord 签名
	if p.Identity != nil {
		privKey := p.Identity.PrivateKey()
		// 全局模式：RealmID 为空字符串
		// 使用 InitializeLocalRecordManager 方法进行类型转换
		if err := dht.InitializeLocalRecordManager(privKey, ""); err != nil {
			logger.Warn("LocalPeerRecordManager 初始化失败", "error", err)
		} else {
			logger.Info("LocalPeerRecordManager 已初始化（全局模式）",
				"nodeID", p.Identity.PeerID())
		}
	}

	// Step A5 对齐：绑定 ReachabilityCoordinator
	// 用于发布前的可达性验证
	if p.Coordinator != nil {
		adapter := NewCoordinatorReachabilityAdapter(p.Coordinator)
		dht.SetReachabilityChecker(adapter)
		logger.Info("ReachabilityChecker 已绑定到 DHT")
	}

	return Result{
		DHT:            dht,
		DHTInterface:   dht, //导出 pkgif.DHT 接口
		Discovery:      dht,
		GroupDiscovery: dht,
	}, nil
}

// NewWithConfig 使用配置创建 DHT
func NewWithConfig(host pkgif.Host, peerstore pkgif.Peerstore, cfg *Config) (*DHT, error) {
	dht, err := New(host, peerstore)
	if err != nil {
		return nil, err
	}

	// 应用配置
	if cfg != nil {
		dht.config = cfg
	}

	return dht, nil
}

// lifecycleParams DHT 生命周期参数
type lifecycleParams struct {
	fx.In

	LC                   fx.Lifecycle
	DHT                  *DHT
	UnifiedCfg           *config.Config `optional:"true"`
	LifecycleCoordinator AddressReadyWaiter `name:"lifecycle_coordinator" optional:"true"` // Step A5: 等待地址就绪
}

// AddressReadyWaiter 地址就绪等待接口
//
// 用于 DHT 模块等待地址就绪后再发布 PeerRecord
type AddressReadyWaiter interface {
	WaitAddressReady(ctx context.Context) error
	IsAddressReady() bool
}

// registerDHTLifecycle 注册 DHT 生命周期钩子
func registerDHTLifecycle(p lifecycleParams) {
	p.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			//日志已在 DHT.Start() 中打印，此处移除重复日志
			if err := p.DHT.Start(ctx); err != nil {
				logger.Error("DHT 启动失败", "error", err)
				return err
			}
			
			// Step A4 对齐：启动后自动触发 DHT Bootstrap（异步，不阻塞启动）
			// 条件：DHT 启用 且 Bootstrap 启用
			if shouldAutoBootstrap(p.UnifiedCfg) {
				go scheduleDHTBootstrap(p.DHT, p.LifecycleCoordinator)
			}
			
			return nil
		},
		OnStop: func(ctx context.Context) error {
			//日志已在 DHT.Stop() 中打印，此处移除重复日志
			if err := p.DHT.Stop(ctx); err != nil {
				logger.Error("DHT 停止失败", "error", err)
				return err
			}
			return nil
		},
	})
}

// shouldAutoBootstrap 判断是否应该自动触发 DHT Bootstrap
//
// 引导节点应跳过 DHT 自动 Bootstrap
// 引导节点作为网络入口，不应依赖外部引导节点列表
func shouldAutoBootstrap(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	
	// 如果本节点启用了 Bootstrap 服务能力（作为引导节点运行），
	// 则跳过 DHT 自动 Bootstrap，因为引导节点是入口，不应依赖其他引导节点
	if cfg.Discovery.Bootstrap.EnableService {
		logger.Info("跳过 DHT 自动 Bootstrap（本节点为引导节点）",
			"reason", "引导节点作为入口，不依赖外部引导节点")
		return false
	}
	
	// DHT 启用 且 Bootstrap 启用（连接到引导节点）
	return cfg.Discovery.EnableDHT && cfg.Discovery.EnableBootstrap
}

// scheduleDHTBootstrap 调度 DHT Bootstrap
//
// 延迟执行 Bootstrap，确保 Host 完全就绪。
// 不阻塞 Fx 启动流程，失败只记录日志。
//
// v2.0 时序对齐：传入 LifecycleCoordinator 用于等待地址就绪
func scheduleDHTBootstrap(dht *DHT, lc AddressReadyWaiter) {
	// 延迟 500ms，确保 Host 和其他组件就绪
	time.Sleep(500 * time.Millisecond)
	
	// 使用 30 秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	logger.Info("DHT 自动 Bootstrap 开始")
	
	if err := dht.Bootstrap(ctx); err != nil {
		logger.Warn("DHT 自动 Bootstrap 失败", "error", err)
		return
	}
	
	logger.Info("DHT 自动 Bootstrap 完成",
		"routingTableSize", dht.routingTable.Size())
	
	// v2.0.1: 等待路由表达到最小节点数，确保后续 FindPeers 查询有效
	// 这修复了 
	minRoutingTableSize := 3
	waitTimeout := 10 * time.Second
	if !waitForMinRoutingTable(dht, minRoutingTableSize, waitTimeout) {
		logger.Warn("路由表未达到最小节点数，继续执行",
			"minRequired", minRoutingTableSize,
			"actual", dht.routingTable.Size(),
			"timeout", waitTimeout)
	} else {
		logger.Info("路由表已达到最小节点数",
			"size", dht.routingTable.Size())
	}
	
	// Step A5 对齐：Bootstrap 完成后自动发布全局 NodeRecord
	// 条件：LocalPeerRecordManager 已初始化
	// v2.0 时序对齐：传入 LifecycleCoordinator 用于等待地址就绪
	scheduleGlobalPeerRecordPublish(dht, lc)
}

// waitForMinRoutingTable 等待路由表达到最小节点数
//
// v2.0.1: 修复 Bootstrap 时序问题，确保路由表有足够节点后再通知下游
// 这对于 RelayDiscovery 等依赖 DHT FindPeers 的组件至关重要
//
// 返回：
//   - true: 路由表已达到最小节点数
//   - false: 超时未达到
func waitForMinRoutingTable(dht *DHT, minSize int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if dht.routingTable.Size() >= minSize {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// scheduleGlobalPeerRecordPublish 调度全局 PeerRecord 发布
//
// Step A5 对齐：在 DHT Bootstrap 完成后发布全局 NodeRecord
// 格式：/dep2p/v2/node/<NodeID>
//
// v2.0 时序对齐：
//   - 必须等待地址就绪（包括 NAT 类型检测 + Relay 连接）
//   - 确保发布的 PeerRecord 包含完整的可达地址
//
// 条件：
//   - LocalPeerRecordManager 已初始化
//   - DHT 已启动
//   - 地址就绪（addressReady 信号）
func scheduleGlobalPeerRecordPublish(dht *DHT, lc AddressReadyWaiter) {
	if dht.localRecordManager == nil || !dht.localRecordManager.IsInitialized() {
		logger.Debug("跳过全局 PeerRecord 发布：LocalPeerRecordManager 未初始化")
		return
	}
	
	// v2.0 时序对齐：等待地址就绪（A5 gate）
	// 这确保 NAT 类型检测完成 且 Relay 连接成功（如果配置）
	if lc != nil {
		logger.Info("等待地址就绪信号（A5 gate）")
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := lc.WaitAddressReady(waitCtx); err != nil {
			logger.Warn("等待地址就绪超时，继续发布",
				"error", err)
		} else {
			logger.Info("地址就绪信号已收到")
		}
		waitCancel()
	}
	
	// 额外延迟 200ms，等待路由表稳定和地址同步
	time.Sleep(200 * time.Millisecond)
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	logger.Info("全局 PeerRecord 发布开始")
	
	// 使用带可达性验证的发布方法
	decision, err := dht.PublishLocalPeerRecordWithVerification(ctx)
	if err != nil {
		logger.Warn("全局 PeerRecord 发布失败", "error", err)
		return
	}
	
	if decision != nil && !decision.ShouldPublish {
		logger.Info("全局 PeerRecord 跳过发布",
			"reason", decision.Reason)
		return
	}
	
	logger.Info("全局 PeerRecord 发布完成",
		"directAddrs", len(decision.DirectAddrs),
		"relayAddrs", len(decision.RelayAddrs),
		"ttl", decision.TTL)
}
