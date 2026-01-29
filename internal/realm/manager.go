package realm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	coreRelay "github.com/dep2p/go-dep2p/internal/core/relay"
	"github.com/dep2p/go-dep2p/internal/core/relay/addressbook"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/realm/auth"
	"github.com/dep2p/go-dep2p/internal/realm/connector"
	"github.com/dep2p/go-dep2p/internal/realm/gateway"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/internal/realm/member"
	"github.com/dep2p/go-dep2p/internal/realm/protocol"
	"github.com/dep2p/go-dep2p/internal/realm/routing"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// logger 在 doc.go 中定义

// ============================================================================
//                              Manager 结构体
// ============================================================================

// Manager Realm 管理器
//
// Manager 是 Realm 的工厂，负责创建和管理 Realm 实例。
// Protocol 服务（Messaging、PubSub、Streams、Liveness）在 JoinRealm() 时动态创建，
// 并绑定到特定的 Realm 实例。
type Manager struct {
	mu sync.RWMutex

	// 配置
	config *ManagerConfig

	// 依赖注入（Core 层）
	host      pkgif.Host
	discovery pkgif.Discovery
	dht       pkgif.DHT //使用统一的 pkgif.DHT 接口
	peerstore pkgif.Peerstore
	eventBus  pkgif.EventBus // 事件总线（用于订阅连接事件）

	// Connector 和 RelayService 所需依赖
	swarm         pkgif.Swarm            // 网络 Swarm（用于 RelayService）
	storageEngine engine.InternalEngine  // 存储引擎（用于地址簿持久化）
	holePuncher   *holepunch.HolePuncher // NAT 打洞器（用于 Connector）

	// Phase 8 修复：添加可选的 ConnectionHealthMonitor（用于 PubSub 错误上报）
	healthMonitor pkgif.ConnectionHealthMonitor

	// P0 修复：NAT 服务（用于 Capability 广播）
	nat pkgif.NATService

	// 生命周期协调器（对齐 20260125-node-lifecycle-cross-cutting.md）
	lifecycleCoordinator *lifecycle.Coordinator

	// 子模块工厂（用于创建 Realm 时实例化）
	authFactory    func(realmID string, psk []byte) (interfaces.Authenticator, error)
	memberFactory  func(realmID string) (interfaces.MemberManager, error)
	routingFactory func(realmID string) (interfaces.Router, error)
	gatewayFactory func(realmID string, auth interfaces.Authenticator) (interfaces.Gateway, error)

	// Realm 管理
	realms  map[string]*realmImpl // realmID -> Realm
	current *realmImpl            // 当前激活的 Realm

	// 状态
	started atomic.Bool
	closed  atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// addressBookClient 适配 AddressBookService 为 Resolver 客户端接口
type addressBookClient struct {
	service *addressbook.AddressBookService
}

func (c *addressBookClient) Query(ctx context.Context, relayPeerID, targetID string) ([]types.Multiaddr, error) {
	if c == nil || c.service == nil {
		return nil, nil
	}
	entry, err := c.service.Query(ctx, relayPeerID, targetID)
	if err != nil || entry == nil {
		return nil, err
	}
	return entry.DirectAddrs, nil
}

type dhtAdapter struct {
	dht pkgif.DHT //使用统一的 pkgif.DHT 接口
}

func (d *dhtAdapter) FindPeer(ctx context.Context, id types.PeerID) (types.PeerInfo, error) {
	if d == nil || d.dht == nil {
		return types.PeerInfo{}, fmt.Errorf("dht not available")
	}
	//FindPeer 接口接受 string 类型
	return d.dht.FindPeer(ctx, string(id))
}

// ============================================================================
//                              构造函数
// ============================================================================

// ManagerDeps Manager 所有必需依赖（彻底重构：不使用 optional 模式）
type ManagerDeps struct {
	// 核心依赖（必需）
	Host      pkgif.Host
	Discovery pkgif.Discovery
	DHT       pkgif.DHT //使用统一的 pkgif.DHT 接口
	Peerstore pkgif.Peerstore
	EventBus  pkgif.EventBus

	// 连接能力依赖（必需）
	Swarm         pkgif.Swarm
	StorageEngine engine.InternalEngine
	HolePuncher   *holepunch.HolePuncher

	// 可选依赖（Phase 8 修复：支持网络健康监控）
	HealthMonitor pkgif.ConnectionHealthMonitor

	// P0 修复：可选 NAT 服务（用于 Capability 广播）
	NATService pkgif.NATService

	// 配置
	Config *ManagerConfig
}

// Validate 验证依赖完整性
func (d *ManagerDeps) Validate() error {
	if d.Host == nil {
		return fmt.Errorf("host is required")
	}
	if d.Peerstore == nil {
		return fmt.Errorf("peerstore is required")
	}
	if d.EventBus == nil {
		return fmt.Errorf("eventbus is required")
	}
	if d.Swarm == nil {
		return fmt.Errorf("swarm is required")
	}
	// StorageEngine 和 HolePuncher 可以为 nil（降级运行）
	return nil
}

// NewManager 创建 Manager（彻底重构版本）
//
// 所有依赖在构造时传入，不再使用 setter 方法。
// Protocol 服务（Messaging、PubSub、Streams、Liveness）在 JoinRealm() 时动态创建。
func NewManager(deps ManagerDeps) (*Manager, error) {
	// 验证依赖
	if err := deps.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manager dependencies: %w", err)
	}

	config := deps.Config
	if config == nil {
		config = DefaultManagerConfig()
	}

	return &Manager{
		config:        config,
		host:          deps.Host,
		discovery:     deps.Discovery,
		dht:           deps.DHT,
		peerstore:     deps.Peerstore,
		eventBus:      deps.EventBus,
		swarm:         deps.Swarm,
		storageEngine: deps.StorageEngine,
		holePuncher:   deps.HolePuncher,
		healthMonitor: deps.HealthMonitor, // Phase 8 修复：设置可选的健康监控器
		nat:           deps.NATService,    // P0 修复：NAT 服务
		realms:        make(map[string]*realmImpl),
	}, nil
}

// NewManagerMinimal 创建 Manager（向后兼容，已废弃）
//
// Deprecated: 请使用 NewManager(ManagerDeps{...}) 代替
func NewManagerMinimal(
	host pkgif.Host,
	discovery pkgif.Discovery,
	peerstore pkgif.Peerstore,
	eventBus pkgif.EventBus,
	config *ManagerConfig,
) *Manager {
	if config == nil {
		config = DefaultManagerConfig()
	}

	return &Manager{
		config:    config,
		host:      host,
		discovery: discovery,
		peerstore: peerstore,
		eventBus:  eventBus,
		realms:    make(map[string]*realmImpl),
	}
}


// ============================================================================
//                              加入 Realm
// ============================================================================

// Join 加入 Realm
func (m *Manager) Join(ctx context.Context, realmID string, psk []byte) (interfaces.Realm, error) {
	logger.Info("正在加入 Realm", "realmID", realmID)

	// 1. 验证状态
	if !m.started.Load() {
		logger.Warn("Manager 未启动，无法加入 Realm")
		return nil, ErrNotStarted
	}

	if m.closed.Load() {
		logger.Warn("Manager 已关闭，无法加入 Realm")
		return nil, ErrClosed
	}

	// 2. 验证输入
	if err := m.validateJoinInput(realmID, psk); err != nil {
		logger.Error("加入 Realm 输入验证失败", "error", err)
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 3. 如果已在其他 Realm，先 Leave
	if m.current != nil && m.current.id != realmID {
		logger.Debug("离开当前 Realm", "currentRealm", m.current.id)
		if err := m.leaveCurrentRealm(ctx); err != nil {
			logger.Error("离开当前 Realm 失败", "error", err)
			return nil, fmt.Errorf("failed to leave current realm: %w", err)
		}
	}

	// 4. 检查是否已存在
	if realm, ok := m.realms[realmID]; ok {
		logger.Debug("Realm 已存在，切换到该 Realm", "realmID", realmID)
		m.current = realm
		return realm, nil
	}

	// 5. 创建 Realm
	logger.Debug("创建新 Realm", "realmID", realmID)
	realm, err := m.createRealm(ctx, realmID, psk)
	if err != nil {
		logger.Error("创建 Realm 失败", "realmID", realmID, "error", err)
		return nil, fmt.Errorf("failed to create realm: %w", err)
	}

	// 6. 启动 Realm
	logger.Debug("启动 Realm", "realmID", realmID)
	if err := realm.start(ctx); err != nil {
		realm.Close()
		logger.Error("启动 Realm 失败", "realmID", realmID, "error", err)
		return nil, fmt.Errorf("failed to start realm: %w", err)
	}

	// 7. 注册并设置为 current
	m.realms[realmID] = realm
	m.current = realm

	logger.Info("成功加入 Realm", "realmID", realmID)
	return realm, nil
}

// validateJoinInput 验证输入
func (m *Manager) validateJoinInput(realmID string, psk []byte) error {
	if realmID == "" {
		return ErrInvalidRealmID
	}

	if len(psk) == 0 {
		return ErrInvalidPSK
	}

	return nil
}

// createRealm 创建 Realm
//
// 此方法创建 Realm 及其所有组件（Auth、Member、Routing、Gateway、Protocol 服务）
// Protocol 服务（Messaging、PubSub、Streams、Liveness）动态创建并绑定到此 Realm
//
// 现在还会创建 Connector 和 RelayService
func (m *Manager) createRealm(ctx context.Context, realmID string, psk []byte) (*realmImpl, error) {
	// 1. 创建 Authenticator
	var authenticator interfaces.Authenticator
	var err error

	if m.authFactory != nil {
		authenticator, err = m.authFactory(realmID, psk)
	} else {
		// 使用默认工厂
		authenticator, err = m.defaultAuthFactory(realmID, psk)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create auth: %w", err)
	}

	// 2. 创建 Member
	var memberMgr interfaces.MemberManager

	if m.memberFactory != nil {
		memberMgr, err = m.memberFactory(realmID)
	} else {
		memberMgr, err = m.defaultMemberFactory(realmID)
	}

	if err != nil {
		if authenticator != nil {
			authenticator.Close()
		}
		return nil, fmt.Errorf("failed to create member: %w", err)
	}

	// 3. 创建 Routing
	var router interfaces.Router

	if m.routingFactory != nil {
		router, err = m.routingFactory(realmID)
	} else {
		router, err = m.defaultRoutingFactory(realmID)
	}

	if err != nil {
		if authenticator != nil {
			authenticator.Close()
		}
		if memberMgr != nil {
			memberMgr.Close()
		}
		return nil, fmt.Errorf("failed to create routing: %w", err)
	}

	// 4. 创建 Gateway
	var gw interfaces.Gateway

	if m.gatewayFactory != nil {
		gw, err = m.gatewayFactory(realmID, authenticator)
	} else {
		gw, err = m.defaultGatewayFactory(realmID, authenticator)
	}

	if err != nil {
		if authenticator != nil {
			authenticator.Close()
		}
		if memberMgr != nil {
			memberMgr.Close()
		}
		if router != nil {
			router.Close()
		}
		return nil, fmt.Errorf("failed to create gateway: %w", err)
	}

	// 5. 创建 AuthHandler（用于自动认证新连接）
	var authHandler *protocol.AuthHandler
	if m.host != nil {
		// 从 PSK 派生认证密钥
		authKey := auth.DeriveAuthKey(psk, realmID)
		authHandler = protocol.NewAuthHandler(m.host, realmID, authKey, authenticator, nil)
	}

	// ════════════════════════════════════════════════════════════════════════════
	// P0 修复：创建 AddressResolver 和 Connector（"仅 ID 连接"核心组件）
	// ════════════════════════════════════════════════════════════════════════════
	var realmConnector *connector.Connector
	addressBookSvc := m.newAddressBookService(types.RealmID(realmID))
	if m.peerstore != nil {
		// 创建 AddressResolver
		resolverConfig := connector.DefaultResolverConfig()
		resolverConfig.Peerstore = m.peerstore
		if m.dht != nil {
			resolverConfig.DHT = &dhtAdapter{dht: m.dht}
		}

		// v2.0 新增：Relay 地址簿回退（仅在配置了 Relay 时启用）
		relayPeerID := ""
		if m.config != nil && len(m.config.RelayPeers) > 0 {
			relayPeerID = m.config.RelayPeers[0]
		}

		if relayPeerID != "" {
			resolverConfig.RelayPeerID = relayPeerID
			resolverConfig.EnableRelayFallback = true
		}

		// v2.0 新增：地址簿客户端（用于 Relay 地址簿回退）
		if addressBookSvc != nil && relayPeerID != "" {
			resolverConfig.AddressBookClient = &addressBookClient{service: addressBookSvc}
		}
		resolver := connector.NewAddressResolver(resolverConfig)

		// 创建 Connector
		connectorConfig := connector.DefaultConnectorConfig()
		connectorConfig.EnableHolePunch = m.holePuncher != nil
		realmConnector = connector.NewConnector(
			resolver,
			m.host,
			m.holePuncher,
			connectorConfig,
		)
		logger.Debug("已创建 Connector", "realmID", realmID, "holePunch", connectorConfig.EnableHolePunch)
	} else {
		logger.Debug("跳过 Connector 创建（缺少 Peerstore）", "realmID", realmID)
	}

	// Step B2 对齐：创建成员同步器（基于 DHT FindRealmMembers）
	// 使用 DHT 而非 Discovery，确保成员发现使用正确的 RealmMembersKey
	var synchronizer *member.Synchronizer
	if m.dht != nil {
		// 尝试获取底层 member.Manager 来创建 Synchronizer
		if mgr, ok := memberMgr.(*member.Manager); ok {
			synchronizer = member.NewSynchronizer(mgr, m.dht)
			logger.Debug("已创建成员同步器", "realmID", realmID)
		} else {
			logger.Debug("跳过成员同步器创建（MemberManager 类型不匹配）", "realmID", realmID)
		}
	} else {
		logger.Debug("跳过成员同步器创建（DHT 不可用）", "realmID", realmID)
	}

	// 6. 创建 Realm 实例
	realm := &realmImpl{
		id:                  realmID,
		name:                realmID, // 简化实现，使用 ID 作为名称
		psk:                 psk,
		auth:                authenticator,
		member:              memberMgr,
		routing:             router,
		gateway:             gw,
		manager:             m,
		authHandler:         authHandler,
		eventBus:            m.eventBus,
		authenticatingPeers: make(map[string]struct{}), // 认证去重
		synchronizer:        synchronizer,              // Step B2 对齐
		discovery:           m.discovery,               // Step B2 对齐
		// Protocol 服务在下面动态创建
	}

	// 注入 Connector
	if realmConnector != nil {
		realm.SetConnector(realmConnector)
	}

	// v2.0 新增：注入地址簿服务（用于网络变化时通知 Relay）
	if addressBookSvc != nil {
		// 预设 relayPeerID，使首次 registerToRelay 能正确获取配置
		if m.config != nil && len(m.config.RelayPeers) > 0 {
			relayPeerIDForABS := m.config.RelayPeers[0]
			addressBookSvc.SetRelayPeerID(relayPeerIDForABS)
		}
		realm.SetAddressBookService(addressBookSvc)
	}

	// 优化：注入基础设施节点列表（Bootstrap、Relay）
	// 避免对这些节点发起无效的 Realm 认证尝试
	if m.config != nil && len(m.config.InfrastructurePeers) > 0 {
		realm.SetInfrastructurePeers(m.config.InfrastructurePeers)
	}

	// 生命周期重构：注入 lifecycle coordinator 和 DHT
	// 用于 Join 协议等待 A5 gate 和 DHT 权威解析入口节点
	if m.lifecycleCoordinator != nil {
		realm.SetLifecycleCoordinator(m.lifecycleCoordinator)
	}
	if m.dht != nil {
		realm.SetDHT(m.dht)
	}

	// ════════════════════════════════════════════════════════════════════════════
	// P0 修复：创建 CapabilityManager（能力广播）并设置回调
	// ════════════════════════════════════════════════════════════════════════════
	if m.host != nil {
		capMgr := protocol.NewCapabilityManager(
			types.RealmID(realmID),
			m.host,
			m.eventBus,
			m.nat, // P0 修复：注入 NAT 服务
		)

		// P0 修复：设置成员能力回调，将地址写入 Peerstore
		if m.peerstore != nil {
			capMgr.SetMemberCapabilityHandler(func(nodeID string, reachability string, addrs []string) {
				// 跳过自己
				if nodeID == m.host.ID() {
					return
				}

				if len(addrs) == 0 {
					return
				}

				// 将地址写入 Peerstore
				var maddrs []types.Multiaddr
				for _, addrStr := range addrs {
					ma, err := types.NewMultiaddr(addrStr)
					if err != nil {
						continue
					}
					maddrs = append(maddrs, ma)
				}

				if len(maddrs) > 0 {
					// 使用 DiscoveredAddrTTL（10 分钟）
					m.peerstore.AddAddrs(types.PeerID(nodeID), maddrs, 10*time.Minute)
					logger.Debug("Capability 回调写入 Peerstore",
						"peerID", nodeID[:8],
						"reachability", reachability,
						"addrs", len(maddrs))
				}
			})
		}

		// P0 修复：设置成员提供者，用于能力广播
		capMgr.SetMemberProvider(func() []string {
			return realm.Members()
		})

		// P0 补修：注入连接函数，确保能力公告前建立连接
		capMgr.SetConnectFunc(func(ctx context.Context, peerID string) error {
			_, err := realm.Connect(ctx, peerID)
			return err
		})

		realm.SetCapabilityManager(capMgr)
		logger.Debug("已创建 CapabilityManager", "realmID", realmID, "natService", m.nat != nil)
	}

	// 7. 动态创建 Protocol 服务（绑定到此 Realm）
	if err := m.createProtocolServices(ctx, realm); err != nil {
		// 清理已创建的组件
		if authenticator != nil {
			authenticator.Close()
		}
		if memberMgr != nil {
			memberMgr.Close()
		}
		if router != nil {
			router.Close()
		}
		if gw != nil {
			gw.Close()
		}
		if authHandler != nil {
			authHandler.Close()
		}
		if realmConnector != nil {
			realmConnector.Close()
		}
		return nil, fmt.Errorf("failed to create protocol services: %w", err)
	}

	return realm, nil
}

// ============================================================================
//                              离开 Realm
// ============================================================================

// Leave 离开当前 Realm
func (m *Manager) Leave(ctx context.Context) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.leaveCurrentRealm(ctx)
}

// leaveCurrentRealm 离开当前 Realm（内部方法，需持有锁）
func (m *Manager) leaveCurrentRealm(ctx context.Context) error {
	if m.current == nil {
		return ErrNotInRealm
	}

	// 停止并清理 Realm
	if err := m.current.stop(ctx); err != nil {
		// 即使失败也要清理
		m.current = nil
		return fmt.Errorf("failed to stop realm: %w", err)
	}

	// 清理 current
	m.current = nil

	return nil
}

// ============================================================================
//                              查询 Realm
// ============================================================================

// Current 返回当前 Realm
//
// 返回 pkgif.Realm 以满足 pkg/interfaces.RealmManager 接口
func (m *Manager) Current() pkgif.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.current == nil {
		return nil
	}

	return m.current
}

// Get 获取指定 Realm
func (m *Manager) Get(realmID string) (interfaces.Realm, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realm, ok := m.realms[realmID]
	if !ok {
		return nil, false
	}

	return realm, true
}

// List 列出所有 Realm
func (m *Manager) List() []interfaces.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realms := make([]interfaces.Realm, 0, len(m.realms))
	for _, r := range m.realms {
		realms = append(realms, r)
	}

	return realms
}

// ============================================================================
//                              生命周期管理
// ============================================================================

// Start 启动 Manager
func (m *Manager) Start(_ context.Context) error {
	if m.started.Load() {
		return ErrAlreadyStarted
	}

	if m.closed.Load() {
		return ErrClosed
	}

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.started.Store(true)

	return nil
}

// Stop 停止 Manager
func (m *Manager) Stop(ctx context.Context) error {
	if !m.started.Load() {
		return ErrNotStarted
	}

	m.started.Store(false)

	// 停止当前 Realm
	if m.current != nil {
		if err := m.Leave(ctx); err != nil {
			// 记录错误但继续
			_ = err
		}
	}

	if m.cancel != nil {
		m.cancel()
	}

	return nil
}

// Create 创建 Realm（满足 interfaces.Manager 接口）
func (m *Manager) Create(ctx context.Context, id, _ string, psk []byte) (interfaces.Realm, error) {
	// 委托给 Join
	return m.Join(ctx, id, psk)
}

// CreateWithOpts 创建 Realm（满足 pkgif.RealmManager 接口）
//
// 使用选项模式创建 Realm
func (m *Manager) CreateWithOpts(ctx context.Context, opts ...pkgif.RealmOption) (pkgif.Realm, error) {
	// 解析选项
	cfg := &pkgif.RealmConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// 验证必要参数
	if cfg.ID == "" {
		return nil, ErrInvalidRealmID
	}
	if len(cfg.PSK) == 0 {
		return nil, ErrInvalidPSK
	}

	// 委托给 Join，然后类型断言为 pkgif.Realm
	internalRealm, err := m.Join(ctx, cfg.ID, cfg.PSK)
	if err != nil {
		return nil, err
	}

	// realmImpl 实现了 pkgif.Realm 接口
	return internalRealm.(pkgif.Realm), nil
}

// GetRealm 获取 Realm（满足 pkgif.RealmManager 接口）
func (m *Manager) GetRealm(realmID string) (pkgif.Realm, bool) {
	internalRealm, ok := m.Get(realmID)
	if !ok {
		return nil, false
	}
	return internalRealm.(pkgif.Realm), true
}

// ListRealms 列出所有 Realm（满足 pkgif.RealmManager 接口）
func (m *Manager) ListRealms() []pkgif.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realms := make([]pkgif.Realm, 0, len(m.realms))
	for _, r := range m.realms {
		// realmImpl 实现了 pkgif.Realm 接口
		realms = append(realms, r)
	}

	return realms
}

// Close 关闭 Manager
func (m *Manager) Close() error {
	if m.closed.Load() {
		return nil
	}

	m.closed.Store(true)

	// 停止 Manager
	if m.started.Load() {
		ctx := context.Background()
		m.Stop(ctx)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭所有 Realm
	for _, realm := range m.realms {
		realm.Close()
	}

	m.realms = nil
	m.current = nil

	return nil
}

// ============================================================================
//                              默认工厂实现
// ============================================================================

// defaultAuthFactory 默认 Auth 工厂
func (m *Manager) defaultAuthFactory(_ string, psk []byte) (interfaces.Authenticator, error) {
	// 创建 PSK 认证器
	if m.host == nil {
		// 没有 host，无法创建认证器（需要本地 PeerID）
		return nil, nil
	}

	peerID := m.host.ID()
	authenticator, err := auth.NewPSKAuthenticator(psk, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to create PSK authenticator: %w", err)
	}

	return authenticator, nil
}

// defaultMemberFactory 默认 Member 工厂
func (m *Manager) defaultMemberFactory(realmID string) (interfaces.MemberManager, error) {
	// 创建真实的 MemberManager（不带持久化存储，仅内存模式）
	// 生产环境应该配置 MemberStore 进行持久化
	mgr := member.NewManager(
		realmID,
		nil,         // cache: 可选
		nil,         // store: 可选（不持久化）
		m.eventBus,  // eventBus: 用于事件发布
	)
	return mgr, nil
}

// defaultRoutingFactory 默认 Routing 工厂
func (m *Manager) defaultRoutingFactory(realmID string) (interfaces.Router, error) {
	// 创建真实的 Router 实例
	// DHT 参数可选，传 nil 时路由器仍可工作（仅使用本地路由表）
	router := routing.NewRouter(realmID, nil, nil)
	return router, nil
}

// defaultGatewayFactory 默认 Gateway 工厂
func (m *Manager) defaultGatewayFactory(realmID string, authenticator interfaces.Authenticator) (interfaces.Gateway, error) {
	// 创建真实的 Gateway 实例
	// host 可能为 nil（单元测试场景），Gateway 会优雅处理
	gw := gateway.NewGateway(realmID, m.host, authenticator, nil)
	return gw, nil
}

// ============================================================================
//                              工厂设置
// ============================================================================

// SetAuthFactory 设置 Auth 工厂
func (m *Manager) SetAuthFactory(factory func(realmID string, psk []byte) (interfaces.Authenticator, error)) {
	m.authFactory = factory
}

// SetMemberFactory 设置 Member 工厂
func (m *Manager) SetMemberFactory(factory func(realmID string) (interfaces.MemberManager, error)) {
	m.memberFactory = factory
}

// SetRoutingFactory 设置 Routing 工厂
func (m *Manager) SetRoutingFactory(factory func(realmID string) (interfaces.Router, error)) {
	m.routingFactory = factory
}

// SetGatewayFactory 设置 Gateway 工厂
func (m *Manager) SetGatewayFactory(factory func(realmID string, auth interfaces.Authenticator) (interfaces.Gateway, error)) {
	m.gatewayFactory = factory
}

// SetLifecycleCoordinator 设置生命周期协调器
//
// 生命周期重构：用于协调 Realm Join 等操作与 A5/B3 gate 的依赖关系。
func (m *Manager) SetLifecycleCoordinator(lc *lifecycle.Coordinator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleCoordinator = lc
}

// ============================================================================
//                              网络变化通知
// ============================================================================

// newAddressBookService 创建地址簿服务（用于 Relay 地址簿回退与注册）
func (m *Manager) newAddressBookService(realmID types.RealmID) *addressbook.AddressBookService {
	if m.host == nil || m.storageEngine == nil {
		return nil
	}

	svc, err := addressbook.NewAddressBookService(addressbook.ServiceConfig{
		RealmID:  realmID,
		LocalID:  types.NodeID(m.host.ID()),
		Host:     m.host,
		Engine:   m.storageEngine,
		EventBus: m.eventBus,
		DefaultTTL: coreRelay.DefaultAddressEntryTTL,
		AddrProvider: func() []types.Multiaddr {
			return parseMultiaddrs(m.host.AdvertisedAddrs())
		},
		NATTypeProvider: func() types.NATType {
			if m.nat != nil {
				return m.nat.GetNATType()
			}
			return types.NATTypeUnknown
		},
	})
	if err != nil {
		logger.Warn("创建地址簿服务失败", "realmID", realmID, "err", err)
		return nil
	}

	return svc
}

func parseMultiaddrs(addrs []string) []types.Multiaddr {
	if len(addrs) == 0 {
		return nil
	}
	result := make([]types.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		ma, err := types.ParseMultiaddr(addr)
		if err != nil || ma == nil {
			continue
		}
		result = append(result, ma)
	}
	return result
}

// NotifyNetworkChange 通知所有活跃的 Realm 网络已变化
//
// 遍历所有活跃的 Realm，调用其 OnNetworkChange 方法，
// 触发 Capability 重新广播和成员地址刷新。
func (m *Manager) NotifyNetworkChange(ctx context.Context, event pkgif.NetworkChangeEvent) error {
	m.mu.RLock()
	realms := make([]*realmImpl, 0, len(m.realms))
	for _, r := range m.realms {
		realms = append(realms, r)
	}
	m.mu.RUnlock()
	
	if len(realms) == 0 {
		return nil
	}
	
	logger.Info("通知 Realm 网络变化", "count", len(realms))
	
	var lastErr error
	for _, realm := range realms {
		if err := realm.OnNetworkChange(ctx, event); err != nil {
			logger.Warn("Realm 处理网络变化失败",
				"realmID", realm.ID(),
				"err", err)
			lastErr = err
		}
	}
	
	return lastErr
}

// 确保实现接口
var _ interfaces.Manager = (*Manager)(nil)
