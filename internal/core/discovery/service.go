// Package discovery 提供节点发现模块的实现
package discovery

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              地址转换辅助函数
// ============================================================================

// multiaddrsToAddresses 将 Multiaddr 列表转换为 endpoint.Address 列表
func multiaddrsToAddresses(mas []types.Multiaddr) []endpoint.Address {
	addrs := make([]endpoint.Address, len(mas))
	for i, ma := range mas {
		addrs[i] = address.NewAddr(ma)
	}
	return addrs
}

// ============================================================================
//                              DiscoveryService 实现
// ============================================================================

// DiscoveryService 发现服务实现
type DiscoveryService struct {
	transport        transportif.Transport
	realmManager     realmif.RealmManager
	accessController realmif.RealmAccessController
	config           discoveryif.Config

	// 动态发现间隔
	dynamicInterval *DynamicInterval

	// 当前 Realm
	currentRealm types.RealmID

	// 注册的发现器和通告器
	discoverers map[string]discoveryif.Discoverer
	announcers  map[string]discoveryif.Announcer

	// 细粒度发现器
	peerFinders          map[string]discoveryif.PeerFinder
	closestPeerFinders   map[string]discoveryif.ClosestPeerFinder
	namespaceDiscoverers map[string]discoveryif.NamespaceDiscoverer

	// Rendezvous 服务点（可选）
	rendezvousPoint discoveryif.RendezvousPoint

	// 已发现的节点
	knownPeers map[types.NodeID]peerRealmInfo

	// 节点发现回调
	peerDiscoveredCallbacks []func(discoveryif.PeerInfo)
	callbackMu              sync.RWMutex

	// 等待查找的请求
	pendingLookups map[types.NodeID][]chan lookupResult
	lookupMu       sync.Mutex

	// 活跃注册（用于统一 API 自动续约）
	activeRegistrations map[string]*activeRegistration

	// Endpoint 引用（用于查询已连接节点和 AddressBook）
	// 通过 SetEndpoint 在 Fx Invoke 阶段注入，避免构造期循环依赖
	ep   endpoint.Endpoint
	epMu sync.RWMutex

	// Bootstrap 节点管理
	// 用于运行时动态添加/移除引导节点
	bootstrapPeers   []discoveryif.PeerInfo
	bootstrapPeersMu sync.RWMutex

	mu sync.RWMutex

	// 状态
	running int32
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc

	// REQ-DISC-002: 入网状态机
	state           discoveryif.DiscoveryState
	stateMu         sync.RWMutex
	stateCallbacks  []func(discoveryif.StateChangeEvent)
	stateCallbackMu sync.RWMutex
	readyCh         chan struct{} // 等待就绪的通道
	readyOnce       sync.Once

	// REQ-DISC-006: 递归发现防护
	// 追踪当前正在进行的发现操作，防止自递归闭环
	inFlightDiscoveries sync.Map // map[types.NodeID]bool
	recursionDepth      int32    // 当前递归深度（原子操作）
}

// lookupResult 查找结果
type lookupResult struct {
	addrs []endpoint.Address
	err   error
}

// peerRealmInfo 节点 Realm 信息
type peerRealmInfo struct {
	RealmID   types.RealmID
	Addresses []endpoint.Address
	LastSeen  time.Time
}

// NewDiscoveryService 创建发现服务
func NewDiscoveryService(transport transportif.Transport, realmManager realmif.RealmManager, config discoveryif.Config) *DiscoveryService {
	intervalConfig := DefaultIntervalConfig()
	if config.TargetPeers > 0 {
		intervalConfig.TargetPeerCount = config.TargetPeers
	}

	// 复制 bootstrap peers 到本地管理
	bootstrapPeers := make([]discoveryif.PeerInfo, len(config.BootstrapPeers))
	copy(bootstrapPeers, config.BootstrapPeers)

	return &DiscoveryService{
		transport:            transport,
		realmManager:         realmManager,
		config:               config,
		dynamicInterval:      NewDynamicInterval(intervalConfig),
		discoverers:          make(map[string]discoveryif.Discoverer),
		announcers:           make(map[string]discoveryif.Announcer),
		peerFinders:          make(map[string]discoveryif.PeerFinder),
		closestPeerFinders:   make(map[string]discoveryif.ClosestPeerFinder),
		namespaceDiscoverers: make(map[string]discoveryif.NamespaceDiscoverer),
		knownPeers:           make(map[types.NodeID]peerRealmInfo),
		pendingLookups:       make(map[types.NodeID][]chan lookupResult),
		bootstrapPeers:       bootstrapPeers,
		// REQ-DISC-002: 初始化入网状态机
		state:   discoveryif.StateNotStarted,
		readyCh: make(chan struct{}),
	}
}

// SetRealmManager 设置 RealmManager（用于 Realm 隔离）。
//
// 注意：为避免 Fx 依赖环，discovery 模块构造期不直接依赖 realm 模块。
// RealmManager 通过 fx.Invoke 在构造后阶段可选注入。
func (s *DiscoveryService) SetRealmManager(rm realmif.RealmManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.realmManager = rm
}

// SetEndpoint 设置 Endpoint 引用（用于本地优先查找）。
//
// 注意：为避免 Fx 依赖环，discovery 模块构造期不直接依赖 endpoint 模块。
// Endpoint 通过 fx.Invoke 在构造后阶段注入。
//
// 设置后，FindPeer 会优先检查：
// 1. 本地 knownPeers 缓存
// 2. AddressBook 缓存
// 3. 已连接节点的地址
// 4. 最后才调用 DHT/mDNS 等网络发现
func (s *DiscoveryService) SetEndpoint(ep endpoint.Endpoint) {
	s.epMu.Lock()
	defer s.epMu.Unlock()
	s.ep = ep
	log.Debug("Endpoint 已注入到 DiscoveryService", "hasEndpoint", ep != nil)
}

// 确保实现接口
var _ discoveryif.DiscoveryService = (*DiscoveryService)(nil)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动发现服务
func (s *DiscoveryService) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}

	// 使用 context.Background() 而非 ctx，因为 Fx OnStart 的 ctx 在 OnStart 返回后会被取消
	// 这会导致后台循环 (discoveryLoop, announceLoop, cleanupLoop) 提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())

	log.Info("发现服务启动中")

	// REQ-DISC-002: 状态转移到 Bootstrapping
	s.transitionState(discoveryif.StateBootstrapping, "Start() called")

	// 获取当前 Realm
	// IMPL-1227: CurrentRealm() 现在返回 Realm 对象
	if s.realmManager != nil {
		if realm := s.realmManager.CurrentRealm(); realm != nil {
			s.currentRealm = realm.ID()
		}
	}

	// 启动 Rendezvous 服务点（如果有）
	s.mu.RLock()
	rvPoint := s.rendezvousPoint
	s.mu.RUnlock()

	if rvPoint != nil {
		// 使用 s.ctx 而非传入的 Fx OnStart ctx，避免 Fx OnStart 完成后 ctx 被 cancel 导致服务提前退出
		if err := rvPoint.Start(s.ctx); err != nil {
			log.Error("启动 Rendezvous 服务点失败", "err", err)
			// 不阻止服务启动，仅记录错误
		} else {
			log.Info("Rendezvous 服务点已启动")
		}
	}

	// 启动所有发现器（包括 DHT、mDNS 等）
	// 使用 s.ctx 而非传入的 Fx OnStart ctx，避免 Fx OnStart 完成后 ctx 被 cancel
	// 导致 DHT 的 refreshLoop/cleanupLoop 等后台循环提前退出
	s.mu.RLock()
	for name, discoverer := range s.discoverers {
		if starter, ok := discoverer.(interface{ Start(context.Context) error }); ok {
			if err := starter.Start(s.ctx); err != nil {
				log.Error("启动发现器失败",
					"name", name,
					"err", err)
			}
		}
	}
	s.mu.RUnlock()

	// 启动发现循环
	go s.discoveryLoop()

	// 启动通告循环
	go s.announceLoop()

	// 启动清理循环
	go s.cleanupLoop()

	// 启动统一 API 续约循环
	go s.renewalLoop()

	// REQ-DISC-002: 启动入网状态监控
	go s.stateMonitorLoop()

	log.Info("发现服务已启动",
		"realm", string(s.currentRealm),
		"discoverers", len(s.discoverers),
		"announcers", len(s.announcers))

	return nil
}

// Stop 停止发现服务
func (s *DiscoveryService) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}

	log.Info("发现服务停止中")

	// 先复制需要操作的对象，再释放锁后操作
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	announcers := make(map[string]discoveryif.Announcer, len(s.announcers))
	for k, v := range s.announcers {
		announcers[k] = v
	}
	rvPoint := s.rendezvousPoint
	s.mu.RUnlock()

	// 停止所有发现器（在锁外操作）
	for name, discoverer := range discoverers {
		// 首先尝试 Stop 方法
		if stopper, ok := discoverer.(interface{ Stop() error }); ok {
			if err := stopper.Stop(); err != nil {
				log.Debug("停止发现器失败",
					"name", name,
					"err", err)
			}
		} else if closer, ok := discoverer.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				log.Debug("关闭发现器失败",
					"name", name,
					"err", err)
			}
		}
	}

	// 停止所有通告（在锁外操作）
	for _, announcer := range announcers {
		_ = announcer.StopAnnounce("") // 停止通告错误可忽略
	}

	// 停止 Rendezvous 服务点
	if rvPoint != nil {
		if err := rvPoint.Stop(); err != nil {
			log.Debug("停止 Rendezvous 服务点失败", "err", err)
		} else {
			log.Info("Rendezvous 服务点已停止")
		}
	}

	// 取消等待中的查找
	s.lookupMu.Lock()
	for _, waiters := range s.pendingLookups {
		for _, ch := range waiters {
			close(ch)
		}
	}
	s.pendingLookups = make(map[types.NodeID][]chan lookupResult)
	s.lookupMu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	atomic.StoreInt32(&s.running, 0)
	log.Info("发现服务已停止")
	return nil
}

// ============================================================================
//                              发现器管理
// ============================================================================

// RegisterDiscoverer 注册发现器
func (s *DiscoveryService) RegisterDiscoverer(name string, discoverer discoveryif.Discoverer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discoverers[name] = discoverer

	log.Debug("注册发现器", "name", name)
}

// RegisterAnnouncer 注册通告器
func (s *DiscoveryService) RegisterAnnouncer(name string, announcer discoveryif.Announcer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.announcers[name] = announcer

	log.Debug("注册通告器", "name", name)
}

// RegisterPeerFinder 注册按 NodeID 查找的发现器
func (s *DiscoveryService) RegisterPeerFinder(name string, finder discoveryif.PeerFinder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerFinders[name] = finder

	log.Debug("注册 PeerFinder", "name", name)
}

// RegisterClosestPeerFinder 注册 DHT 风格的发现器
func (s *DiscoveryService) RegisterClosestPeerFinder(name string, finder discoveryif.ClosestPeerFinder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closestPeerFinders[name] = finder

	log.Debug("注册 ClosestPeerFinder", "name", name)
}

// RegisterNamespaceDiscoverer 注册基于命名空间的发现器
func (s *DiscoveryService) RegisterNamespaceDiscoverer(name string, discoverer discoveryif.NamespaceDiscoverer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.namespaceDiscoverers[name] = discoverer

	log.Debug("注册 NamespaceDiscoverer", "name", name)
}

// SetRendezvousPoint 设置 Rendezvous 服务点
func (s *DiscoveryService) SetRendezvousPoint(point discoveryif.RendezvousPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rendezvousPoint = point
}

// GetRendezvousPoint 获取 Rendezvous 服务点
func (s *DiscoveryService) GetRendezvousPoint() discoveryif.RendezvousPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rendezvousPoint
}

// SetAccessController 设置访问控制器（用于私有 Realm 过滤）
func (s *DiscoveryService) SetAccessController(ac realmif.RealmAccessController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessController = ac
}

// GetAccessController 获取访问控制器
func (s *DiscoveryService) GetAccessController() realmif.RealmAccessController {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accessController
}

// GetDiscoverer 获取指定名称的发现器
func (s *DiscoveryService) GetDiscoverer(name string) discoveryif.Discoverer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.discoverers[name]
}

// ============================================================================
//                              Bootstrap 节点管理
// ============================================================================
//
// SPEC-BOOTSTRAP-002: 动态 Bootstrap 管理规范
//
// AddBootstrapPeer MUST 触发:
//   1. 将 peer 加入内部 Bootstrap 列表
//   2. 异步连接尝试（不阻塞调用者）
//   3. 加入 DHT 路由表候选（如果 DHT 已启动）
//
// RemoveBootstrapPeer MUST 触发:
//   1. 从内部 Bootstrap 列表移除
//   2. 从 DHT 路由表移除（如果存在）
//   3. 断开与该 peer 的连接

// GetBootstrapPeers 获取当前配置的引导节点列表
func (s *DiscoveryService) GetBootstrapPeers(_ context.Context) ([]discoveryif.PeerInfo, error) {
	s.bootstrapPeersMu.RLock()
	defer s.bootstrapPeersMu.RUnlock()

	result := make([]discoveryif.PeerInfo, len(s.bootstrapPeers))
	copy(result, s.bootstrapPeers)
	return result, nil
}

// AddBootstrapPeer 运行时添加引导节点
//
// SPEC-BOOTSTRAP-002: 添加后 MUST 触发异步连接尝试或 DHT 路由表更新
// REQ-BOOT-001: Bootstrap seed 必须是 Full Address
func (s *DiscoveryService) AddBootstrapPeer(peer discoveryif.PeerInfo) {
	// SPEC-BOOTSTRAP-001: NodeID 必须非空
	if peer.ID.IsEmpty() {
		log.Warn("SPEC-BOOTSTRAP-001 违规: 拒绝添加空 NodeID 的引导节点")
		return
	}

	// REQ-BOOT-001: 地址格式校验
	// Bootstrap peer 必须至少有一个有效的 Full Address
	if len(peer.Addrs) == 0 {
		log.Warn("REQ-BOOT-001 违规: 拒绝添加无地址的引导节点",
			"peer", peer.ID.ShortString())
		return
	}

	// 校验每个地址：必须是 Full Address，不能是 RelayCircuit
	validAddrs := make([]types.Multiaddr, 0, len(peer.Addrs))
	for _, addr := range peer.Addrs {
		if addr.IsEmpty() {
			continue
		}

		addrStr := addr.String()

		// 检查是否是 RelayCircuit 地址（不允许作为 bootstrap seed）
		if addr.IsRelay() {
			log.Warn("REQ-BOOT-001 违规: 跳过 RelayCircuit 地址",
				"peer", peer.ID.ShortString(),
				"addr", addrStr)
			continue
		}

		// 检查是否是 Full Address（包含 /p2p/<NodeID>）
		parsedID := addr.PeerID()
		if parsedID.IsEmpty() {
			log.Warn("REQ-BOOT-001 违规: 跳过非 Full Address",
				"peer", peer.ID.ShortString(),
				"addr", addrStr)
			continue
		}

		// 校验地址中的 NodeID 与 peer.ID 一致
		if !parsedID.Equal(peer.ID) {
			log.Warn("REQ-BOOT-001 违规: 地址中的 NodeID 与 peer.ID 不匹配",
				"peer", peer.ID.ShortString(),
				"addr_peer_id", parsedID.ShortString(),
				"addr", addrStr)
			continue
		}

		validAddrs = append(validAddrs, addr)
	}

	// 必须有至少一个有效的 Full Address
	if len(validAddrs) == 0 {
		log.Warn("REQ-BOOT-001 违规: 引导节点没有有效的 Full Address",
			"peer", peer.ID.ShortString(),
			"original_addrs", peer.Addrs)
		return
	}

	// 使用验证后的地址
	validatedPeer := discoveryif.PeerInfo{
		ID:    peer.ID,
		Addrs: validAddrs,
	}

	s.bootstrapPeersMu.Lock()
	// 检查是否已存在
	for _, existing := range s.bootstrapPeers {
		if existing.ID == validatedPeer.ID {
			s.bootstrapPeersMu.Unlock()
			log.Debug("引导节点已存在，跳过添加", "peer", validatedPeer.ID.ShortString())
			return
		}
	}
	s.bootstrapPeers = append(s.bootstrapPeers, validatedPeer)
	s.bootstrapPeersMu.Unlock()

	log.Info("添加引导节点",
		"peer", validatedPeer.ID.ShortString(),
		"addrs", validatedPeer.Addrs,
		"total", len(s.bootstrapPeers))

	// SPEC-BOOTSTRAP-002: 触发异步连接尝试
	go s.connectToBootstrapPeer(validatedPeer)
}

// connectToBootstrapPeer 异步连接引导节点
//
// SPEC-BOOTSTRAP-002: 连接成功后将 peer 加入 DHT 路由表
func (s *DiscoveryService) connectToBootstrapPeer(peer discoveryif.PeerInfo) {
	s.epMu.RLock()
	ep := s.ep
	s.epMu.RUnlock()

	if ep == nil {
		log.Warn("Endpoint 未就绪，无法连接引导节点", "peer", peer.ID.ShortString())
		return
	}

	// 使用服务上下文
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 将 Multiaddr 转换为 endpoint.Address
	addrs := multiaddrsToAddresses(peer.Addrs)

	// 添加地址到地址簿（供后续 Connect 使用）
	if ab := ep.AddressBook(); ab != nil && len(addrs) > 0 {
		ab.Add(peer.ID, addrs...)
	}

	// 尝试连接
	var err error
	if len(addrs) > 0 {
		_, err = ep.ConnectWithAddrs(ctx, peer.ID, addrs)
	} else {
		_, err = ep.Connect(ctx, peer.ID)
	}

	if err != nil {
		log.Warn("连接引导节点失败",
			"peer", peer.ID.ShortString(),
			"error", err,
			"action", "保留在列表中，下次 bootstrap refresh 重试")
		return
	}

	log.Info("连接引导节点成功",
		"peer", peer.ID.ShortString(),
		"action", "已加入路由表")
}

// RemoveBootstrapPeer 运行时移除引导节点
//
// SPEC-BOOTSTRAP-002: 移除后 MUST 断开连接并从 DHT 路由表移除
func (s *DiscoveryService) RemoveBootstrapPeer(id types.NodeID) {
	// SPEC-BOOTSTRAP-001: NodeID 必须非空
	if id.IsEmpty() {
		log.Warn("SPEC-BOOTSTRAP-001 违规: 拒绝移除空 NodeID")
		return
	}

	s.bootstrapPeersMu.Lock()
	found := false
	for i, peer := range s.bootstrapPeers {
		if peer.ID == id {
			s.bootstrapPeers = append(s.bootstrapPeers[:i], s.bootstrapPeers[i+1:]...)
			found = true
			break
		}
	}
	remaining := len(s.bootstrapPeers)
	s.bootstrapPeersMu.Unlock()

	if !found {
		log.Debug("引导节点不在列表中，忽略移除请求", "peer", id.ShortString())
		return
	}

	log.Info("移除引导节点",
		"peer", id.ShortString(),
		"remaining", remaining)

	// SPEC-BOOTSTRAP-002: 断开与该 peer 的连接
	go s.disconnectBootstrapPeer(id)
}

// disconnectBootstrapPeer 断开引导节点连接
func (s *DiscoveryService) disconnectBootstrapPeer(id types.NodeID) {
	s.epMu.RLock()
	ep := s.ep
	s.epMu.RUnlock()

	if ep == nil {
		return
	}

	// 检查是否有连接
	if _, ok := ep.Connection(id); ok {
		if err := ep.Disconnect(id); err != nil {
			log.Warn("断开引导节点连接失败",
				"peer", id.ShortString(),
				"error", err)
		} else {
			log.Info("已断开引导节点连接", "peer", id.ShortString())
		}
	}
}

// ============================================================================
//                              动态发现间隔
// ============================================================================

// GetDiscoveryInterval 获取当前发现间隔
func (s *DiscoveryService) GetDiscoveryInterval() time.Duration {
	s.mu.RLock()
	peerCount := len(s.knownPeers)
	s.mu.RUnlock()

	return s.dynamicInterval.Calculate(peerCount)
}

// IsEmergencyMode 是否处于紧急模式
func (s *DiscoveryService) IsEmergencyMode() bool {
	return s.dynamicInterval.IsEmergency()
}

// ============================================================================
//                              节点管理
// ============================================================================

// AddKnownPeer 添加已知节点
func (s *DiscoveryService) AddKnownPeer(id types.NodeID, realmID types.RealmID, addrs []endpoint.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.knownPeers[id] = peerRealmInfo{
		RealmID:   realmID,
		Addresses: addrs,
		LastSeen:  time.Now(),
	}
}

// RemoveKnownPeer 移除已知节点
func (s *DiscoveryService) RemoveKnownPeer(id types.NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.knownPeers, id)
}

// KnownPeerCount 获取已知节点数量
func (s *DiscoveryService) KnownPeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.knownPeers)
}

// ============================================================================
//                              节点发现回调
// ============================================================================

// OnPeerDiscovered 注册节点发现回调
//
// 当通过 mDNS、DHT 或其他发现机制发现新节点时触发回调。
// 应用可以通过此回调主动连接到发现的节点。
func (s *DiscoveryService) OnPeerDiscovered(callback func(discoveryif.PeerInfo)) {
	if callback == nil {
		return
	}

	s.callbackMu.Lock()
	defer s.callbackMu.Unlock()
	s.peerDiscoveredCallbacks = append(s.peerDiscoveredCallbacks, callback)

	log.Debug("注册节点发现回调")
}

// NotifyPeerDiscovered 通知所有注册的回调有新节点被发现
//
// 此方法由各个发现器（如 mDNS）在发现新节点时调用。
func (s *DiscoveryService) NotifyPeerDiscovered(peer discoveryif.PeerInfo) {
	// 发现事件必须携带可拨号地址；否则会导致上层疯狂尝试 DialByNodeID，
	// 但 AddressBook 里没有地址，最终表现为“发现了很多节点但 no addresses available”。
	if len(peer.Addrs) == 0 {
		return
	}

	s.callbackMu.RLock()
	callbacks := make([]func(discoveryif.PeerInfo), len(s.peerDiscoveredCallbacks))
	copy(callbacks, s.peerDiscoveredCallbacks)
	s.callbackMu.RUnlock()

	if len(callbacks) == 0 {
		return
	}

	log.Debug("触发节点发现回调",
		"peer", peer.ID.ShortString(),
		"callbacks", len(callbacks))

	// 在独立的 goroutine 中调用回调，避免阻塞发现流程
	for _, cb := range callbacks {
		go func(callback func(discoveryif.PeerInfo)) {
			defer func() {
				if r := recover(); r != nil {
					log.Error("节点发现回调 panic",
						"recover", r)
				}
			}()
			callback(peer)
		}(cb)
	}
}

// ============================================================================
//                              连接通知（DHT 引导重试支持）
// ============================================================================

// PeerConnectionNotifier 节点连接通知接口
//
// 发现器可选实现此接口以接收连接建立/断开事件。
// 主要用于 DHT 在新连接时更新路由表并触发引导重试。
type PeerConnectionNotifier interface {
	// NotifyPeerConnected 通知新连接建立
	NotifyPeerConnected(nodeID types.NodeID, addrs []string)

	// NotifyPeerDisconnected 通知连接断开
	NotifyPeerDisconnected(nodeID types.NodeID)
}

// NotifyPeerConnected 通知所有支持的发现器有新连接建立
//
// 此方法应在 Endpoint 新连接建立时调用，用于：
// - 将连接的节点加入 DHT 路由表
// - 触发 seed 节点的 DHT 引导重试
//
// 调用时机：Endpoint.addConnection() 或 ConnectionManager.NotifyConnected()
func (s *DiscoveryService) NotifyPeerConnected(nodeID types.NodeID, addrs []string) {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	// 通知所有支持连接通知的发现器
	for name, discoverer := range discoverers {
		if notifier, ok := discoverer.(PeerConnectionNotifier); ok {
			log.Debug("通知发现器新连接",
				"discoverer", name,
				"nodeID", nodeID.ShortString(),
				"addrs", addrs)
			notifier.NotifyPeerConnected(nodeID, addrs)
		}
	}
}

// NotifyPeerDisconnected 通知所有支持的发现器连接断开
//
// 此方法应在 Endpoint 连接断开时调用。
func (s *DiscoveryService) NotifyPeerDisconnected(nodeID types.NodeID) {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	// 通知所有支持连接通知的发现器
	for name, discoverer := range discoverers {
		if notifier, ok := discoverer.(PeerConnectionNotifier); ok {
			log.Debug("通知发现器连接断开",
				"discoverer", name,
				"nodeID", nodeID.ShortString())
			notifier.NotifyPeerDisconnected(nodeID)
		}
	}
}

// ============================================================================
//                              入网状态机 (REQ-DISC-002)
// ============================================================================

// State 返回当前入网状态
func (s *DiscoveryService) State() discoveryif.DiscoveryState {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// SetOnStateChanged 注册状态变更回调
func (s *DiscoveryService) SetOnStateChanged(callback func(discoveryif.StateChangeEvent)) {
	if callback == nil {
		return
	}

	s.stateCallbackMu.Lock()
	defer s.stateCallbackMu.Unlock()
	s.stateCallbacks = append(s.stateCallbacks, callback)

	log.Debug("注册入网状态变更回调")
}

// WaitReady 等待服务就绪
func (s *DiscoveryService) WaitReady(ctx context.Context) error {
	// 先检查当前状态
	s.stateMu.RLock()
	currentState := s.state
	s.stateMu.RUnlock()

	if currentState.IsReady() {
		return nil
	}
	if currentState == discoveryif.StateFailed {
		return ErrBootstrapFailed
	}

	// 等待就绪或失败
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.readyCh:
		s.stateMu.RLock()
		finalState := s.state
		s.stateMu.RUnlock()

		if finalState == discoveryif.StateFailed {
			return ErrBootstrapFailed
		}
		return nil
	}
}

// RetryBootstrap 重试引导
func (s *DiscoveryService) RetryBootstrap() {
	s.stateMu.RLock()
	currentState := s.state
	s.stateMu.RUnlock()

	if currentState != discoveryif.StateFailed {
		log.Debug("当前不在 StateFailed 状态，忽略 RetryBootstrap 请求",
			"currentState", currentState.String())
		return
	}

	log.Info("重试引导流程")

	// 重置就绪通道
	s.readyOnce = sync.Once{}
	s.readyCh = make(chan struct{})

	// 转移到 Bootstrapping 状态
	s.transitionState(discoveryif.StateBootstrapping, "RetryBootstrap() called")

	// 重新执行引导
	go s.retryBootstrapPeers()
}

// transitionState 状态转移（内部方法）
func (s *DiscoveryService) transitionState(newState discoveryif.DiscoveryState, reason string) {
	s.stateMu.Lock()
	oldState := s.state
	if oldState == newState {
		s.stateMu.Unlock()
		return
	}
	s.state = newState
	s.stateMu.Unlock()

	log.Info("入网状态转移",
		"from", oldState.String(),
		"to", newState.String(),
		"reason", reason)

	// 如果转移到就绪或失败状态，关闭就绪通道
	if newState.IsReady() || newState == discoveryif.StateFailed {
		s.readyOnce.Do(func() {
			close(s.readyCh)
		})
	}

	// 通知回调
	event := discoveryif.StateChangeEvent{
		OldState: oldState,
		NewState: newState,
		Reason:   reason,
	}

	s.stateCallbackMu.RLock()
	callbacks := make([]func(discoveryif.StateChangeEvent), len(s.stateCallbacks))
	copy(callbacks, s.stateCallbacks)
	s.stateCallbackMu.RUnlock()

	for _, cb := range callbacks {
		go func(callback func(discoveryif.StateChangeEvent)) {
			defer func() {
				if r := recover(); r != nil {
					log.Error("状态变更回调 panic", "recover", r)
				}
			}()
			callback(event)
		}(cb)
	}
}

// stateMonitorLoop 入网状态监控循环
func (s *DiscoveryService) stateMonitorLoop() {
	// 首次检查延迟，等待引导节点连接
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			s.checkAndUpdateState()
			timer.Reset(10 * time.Second) // 后续每 10 秒检查一次
		}
	}
}

// checkAndUpdateState 检查并更新入网状态
func (s *DiscoveryService) checkAndUpdateState() {
	s.stateMu.RLock()
	currentState := s.state
	s.stateMu.RUnlock()

	// 已经是终态，不需要检查
	if currentState == discoveryif.StateDiscoverable || currentState == discoveryif.StateFailed {
		return
	}

	// 检查是否有连接
	s.epMu.RLock()
	ep := s.ep
	s.epMu.RUnlock()

	if ep == nil {
		return
	}

	connCount := ep.ConnectionCount()
	hasConnections := connCount > 0

	// 检查 DHT 路由表大小
	routingTableSize := 0
	s.mu.RLock()
	for _, discoverer := range s.discoverers {
		if dht, ok := discoverer.(interface{ RoutingTable() discoveryif.RoutingTable }); ok {
			if rt := dht.RoutingTable(); rt != nil {
				routingTableSize = rt.Size()
				break
			}
		}
	}
	s.mu.RUnlock()

	// 状态转移逻辑
	switch currentState {
	case discoveryif.StateBootstrapping:
		if hasConnections {
			s.transitionState(discoveryif.StateConnected, "first connection established")
		} else {
			// 检查是否有配置的引导节点
			s.bootstrapPeersMu.RLock()
			hasBootstrapPeers := len(s.bootstrapPeers) > 0
			s.bootstrapPeersMu.RUnlock()

			if !hasBootstrapPeers && connCount == 0 {
				// 无引导节点且无连接，检查是否有 mDNS 发现
				s.mu.RLock()
				knownCount := len(s.knownPeers)
				s.mu.RUnlock()

				if knownCount == 0 {
					// 如果 5 秒后仍无任何发现，转移到失败状态
					// 但给予更多时间（由 stateMonitorLoop 控制）
					log.Debug("等待网络发现...",
						"connections", connCount,
						"knownPeers", knownCount)
				}
			}
		}

	case discoveryif.StateConnected:
		// 检查是否可被发现（路由表有足够条目）
		if routingTableSize >= 3 {
			s.transitionState(discoveryif.StateDiscoverable, "routing table populated")
		}
	}
}

// retryBootstrapPeers 重试连接引导节点
func (s *DiscoveryService) retryBootstrapPeers() {
	s.bootstrapPeersMu.RLock()
	peers := make([]discoveryif.PeerInfo, len(s.bootstrapPeers))
	copy(peers, s.bootstrapPeers)
	s.bootstrapPeersMu.RUnlock()

	if len(peers) == 0 {
		log.Warn("无引导节点可重试")
		s.transitionState(discoveryif.StateFailed, "no bootstrap peers available")
		return
	}

	successCount := 0
	for _, peer := range peers {
		if s.connectToBootstrapPeer(peer); successCount == 0 {
			// 检查是否连接成功
			s.epMu.RLock()
			ep := s.ep
			s.epMu.RUnlock()

			if ep != nil {
				if _, ok := ep.Connection(peer.ID); ok {
					successCount++
				}
			}
		}
	}

	if successCount == 0 {
		s.transitionState(discoveryif.StateFailed, "all bootstrap connections failed")
	}
}

// ErrBootstrapFailed 引导失败错误
var ErrBootstrapFailed = discoveryif.ErrBootstrapFailed

// ============================================================================
//                              递归发现防护（REQ-DISC-006）
// ============================================================================

// MaxRecursionDepth 最大递归深度
// 防止发现流程中的无限递归
const MaxRecursionDepth = 3

// ErrRecursiveDiscovery 递归发现错误
var ErrRecursiveDiscovery = discoveryif.ErrRecursiveDiscovery

// enterDiscoveryContext 进入发现上下文
//
// REQ-DISC-006: 检测并防止递归发现
// 返回 true 表示允许继续，返回 false 表示检测到递归应中止
func (s *DiscoveryService) enterDiscoveryContext(nodeID types.NodeID) bool {
	// 检查是否已在进行中
	if _, loaded := s.inFlightDiscoveries.LoadOrStore(nodeID, true); loaded {
		log.Debug("检测到重复发现请求，跳过",
			"nodeID", nodeID.ShortString())
		return false
	}

	// 检查递归深度
	depth := atomic.AddInt32(&s.recursionDepth, 1)
	if depth > MaxRecursionDepth {
		atomic.AddInt32(&s.recursionDepth, -1)
		s.inFlightDiscoveries.Delete(nodeID)
		log.Warn("检测到过深递归，中止发现",
			"nodeID", nodeID.ShortString(),
			"depth", depth)
		return false
	}

	return true
}

// leaveDiscoveryContext 离开发现上下文
func (s *DiscoveryService) leaveDiscoveryContext(nodeID types.NodeID) {
	s.inFlightDiscoveries.Delete(nodeID)
	atomic.AddInt32(&s.recursionDepth, -1)
}

// IsDiscoveryInProgress 检查是否有指定节点的发现正在进行中
//
// 用于诊断和测试
func (s *DiscoveryService) IsDiscoveryInProgress(nodeID types.NodeID) bool {
	_, inProgress := s.inFlightDiscoveries.Load(nodeID)
	return inProgress
}

// CurrentRecursionDepth 返回当前递归深度
//
// 用于诊断和测试
func (s *DiscoveryService) CurrentRecursionDepth() int {
	return int(atomic.LoadInt32(&s.recursionDepth))
}

