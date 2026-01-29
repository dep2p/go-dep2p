package realm

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
)

// ============================================================================
//
//	Fx 模块定义（彻底重构版本）
//
// ============================================================================

// ManagerModule Realm Manager Fx 模块
//
// 彻底重构：所有依赖为必需，不使用 optional 模式。
// Protocol 服务在 JoinRealm() 时由 Manager 动态创建并绑定到特定 Realm。
func ManagerModule() fx.Option {
	return fx.Module("realm_manager",
		fx.Provide(provideManager),
		fx.Invoke(registerLifecycle),
	)
}

// ManagerResult Manager 提供结果
type ManagerResult struct {
	fx.Out
	Manager      *Manager           // 内部使用
	RealmManager pkgif.RealmManager // 外部接口
}

// provideManager 提供 Manager
func provideManager(p ManagerParams) (ManagerResult, error) {
	manager, err := NewManagerFromParams(p)
	if err != nil {
		return ManagerResult{}, err
	}
	return ManagerResult{
		Manager:      manager,
		RealmManager: manager,
	}, nil
}

// ============================================================================
//
//	Fx 参数（彻底重构：必需依赖）
//
// ============================================================================

// ManagerParams Manager 依赖参数（彻底重构：核心依赖为必需）
type ManagerParams struct {
	fx.In

	// 核心依赖（必需）
	Host      pkgif.Host      // 主机
	Peerstore pkgif.Peerstore // Peerstore
	EventBus  pkgif.EventBus  // 事件总线
	Swarm     pkgif.Swarm     // 网络 Swarm

	// 可选依赖（有合理默认值或可降级）
	Discovery            pkgif.Discovery                 `optional:"true"` // 发现服务（可选，某些场景不需要）
	DHT                  pkgif.DHT                       `optional:"true"` // 使用统一的 pkgif.DHT 接口
	StorageEngine        engine.InternalEngine           `optional:"true"` // 存储引擎（可选，无持久化时降级）
	HolePuncher          *holepunch.HolePuncher          `optional:"true"` // NAT 打洞器（可选，无 NAT 时降级）
	UnifiedCfg           *config.Config                  `optional:"true"` // 统一配置
	HealthMonitor        pkgif.ConnectionHealthMonitor   `optional:"true"` // Phase 8 修复：网络健康监控器（用于 PubSub 错误上报）
	LifecycleCoordinator *lifecycle.Coordinator          `optional:"true"` // 生命周期协调器

	// 子模块工厂（可选，有默认实现）
	AuthFactory    func(realmID string, psk []byte) (interfaces.Authenticator, error)              `optional:"true"`
	MemberFactory  func(realmID string) (interfaces.MemberManager, error)                          `optional:"true"`
	RoutingFactory func(realmID string) (interfaces.Router, error)                                 `optional:"true"`
	GatewayFactory func(realmID string, auth interfaces.Authenticator) (interfaces.Gateway, error) `optional:"true"`
}

// ============================================================================
//
//	配置转换
//
// ============================================================================

// ConfigFromUnified 从统一配置创建 Realm Manager 配置
func ConfigFromUnified(cfg *config.Config) *ManagerConfig {
	if cfg == nil {
		return DefaultManagerConfig()
	}

	mgrCfg := &ManagerConfig{
		DefaultRealmName: "default",
		MaxRealms:        1,
		AuthTimeout:      cfg.Realm.Auth.Timeout,
		LeaveTimeout:     cfg.Realm.Auth.Timeout,
		SyncInterval:     cfg.Realm.Member.SyncInterval,
	}

	// 提取基础设施节点 PeerID（Bootstrap + Relay）
	// 这些节点不是 Realm 成员，跳过对它们的认证尝试
	mgrCfg.InfrastructurePeers = extractInfrastructurePeers(cfg)
	mgrCfg.RelayPeers = extractRelayPeers(cfg)

	return mgrCfg
}

// extractInfrastructurePeers 从配置中提取基础设施节点 PeerID
//
// 基础设施节点包括：
//   - Bootstrap 节点（来自 cfg.Discovery.Bootstrap.Peers）
//   - Relay 节点（来自 cfg.Relay.RelayAddr）
//   - Static Relay 节点（来自 cfg.Relay.StaticRelays）
//
// 这些节点不参与 Realm 认证，连接时跳过认证可以避免不必要的网络请求和错误日志。
func extractInfrastructurePeers(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}

	// 使用 map 去重
	peers := make(map[string]struct{})

	// 1. 提取 Bootstrap 节点 PeerID
	for _, addrStr := range cfg.Discovery.Bootstrap.Peers {
		if peerID := extractPeerIDFromAddr(addrStr); peerID != "" {
			peers[peerID] = struct{}{}
		}
	}

	// 2. 提取 Relay 节点 PeerID
	if cfg.Relay.RelayAddr != "" {
		if peerID := extractPeerIDFromAddr(cfg.Relay.RelayAddr); peerID != "" {
			peers[peerID] = struct{}{}
		}
	}

	// 3. 提取 Static Relay 节点 PeerID
	for _, addrStr := range cfg.Relay.StaticRelays {
		if peerID := extractPeerIDFromAddr(addrStr); peerID != "" {
			peers[peerID] = struct{}{}
		}
	}

	// 转换为切片
	result := make([]string, 0, len(peers))
	for peerID := range peers {
		result = append(result, peerID)
	}

	return result
}

// extractRelayPeers 从配置中提取 Relay 节点 PeerID
//
// 仅包含 Relay 节点：
//   - Relay 节点（来自 cfg.Relay.RelayAddr）
//   - Static Relay 节点（来自 cfg.Relay.StaticRelays）
func extractRelayPeers(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}

	peers := make(map[string]struct{})

	// 1. 提取 Relay 节点 PeerID
	if cfg.Relay.RelayAddr != "" {
		if peerID := extractPeerIDFromAddr(cfg.Relay.RelayAddr); peerID != "" {
			peers[peerID] = struct{}{}
		}
	}

	// 2. 提取 Static Relay 节点 PeerID
	for _, addrStr := range cfg.Relay.StaticRelays {
		if peerID := extractPeerIDFromAddr(addrStr); peerID != "" {
			peers[peerID] = struct{}{}
		}
	}

	result := make([]string, 0, len(peers))
	for peerID := range peers {
		result = append(result, peerID)
	}

	return result
}

// extractPeerIDFromAddr 从多地址字符串中提取 PeerID
func extractPeerIDFromAddr(addrStr string) string {
	if addrStr == "" {
		return ""
	}

	ma, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		logger.Debug("解析多地址失败，跳过", "addr", addrStr, "error", err)
		return ""
	}

	peerID, err := multiaddr.GetPeerID(ma)
	if err != nil {
		logger.Debug("提取 PeerID 失败，跳过", "addr", addrStr, "error", err)
		return ""
	}

	return peerID
}

// ============================================================================
//
//	构造函数（彻底重构版本）
//
// ============================================================================

// NewManagerFromParams 从 Fx 参数创建 Manager（彻底重构版本）
func NewManagerFromParams(p ManagerParams) (*Manager, error) {
	// 验证必需依赖
	if p.Host == nil {
		return nil, fmt.Errorf("host is required for RealmManager")
	}
	if p.Peerstore == nil {
		return nil, fmt.Errorf("peerstore is required for RealmManager")
	}
	if p.EventBus == nil {
		return nil, fmt.Errorf("eventbus is required for RealmManager")
	}
	if p.Swarm == nil {
		return nil, fmt.Errorf("swarm is required for RealmManager")
	}

	mgrConfig := ConfigFromUnified(p.UnifiedCfg)
	if err := mgrConfig.Validate(); err != nil {
		return nil, err
	}

	// 使用新构造函数
	manager, err := NewManager(ManagerDeps{
		Host:          p.Host,
		Discovery:     p.Discovery,
		DHT:           p.DHT,
		Peerstore:     p.Peerstore,
		EventBus:      p.EventBus,
		Swarm:         p.Swarm,
		StorageEngine: p.StorageEngine,
		HolePuncher:   p.HolePuncher,
		HealthMonitor: p.HealthMonitor, // Phase 8 修复：传递可选的健康监控器
		Config:        mgrConfig,
	})
	if err != nil {
		return nil, err
	}

	// 设置工厂（如果提供）
	if p.AuthFactory != nil {
		manager.SetAuthFactory(p.AuthFactory)
	}
	if p.MemberFactory != nil {
		manager.SetMemberFactory(p.MemberFactory)
	}
	if p.RoutingFactory != nil {
		manager.SetRoutingFactory(p.RoutingFactory)
	}
	if p.GatewayFactory != nil {
		manager.SetGatewayFactory(p.GatewayFactory)
	}

	// 生命周期重构：注入 lifecycle coordinator
	if p.LifecycleCoordinator != nil {
		manager.SetLifecycleCoordinator(p.LifecycleCoordinator)
	}

	return manager, nil
}

// ============================================================================
//
//	生命周期管理
//
// ============================================================================

// lifecycleInput Lifecycle 注册输入
type lifecycleInput struct {
	fx.In
	LC      fx.Lifecycle
	Manager *Manager
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return input.Manager.Start(ctx)
		},
		OnStop: func(ctx context.Context) error {
			if err := input.Manager.Stop(ctx); err != nil {
				return err
			}
			return input.Manager.Close()
		},
	})
}
