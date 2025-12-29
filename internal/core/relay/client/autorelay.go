// Package client 提供中继客户端实现
//
// AutoRelay 自动管理中继连接，确保节点可达性：
// - 自动发现中继服务器
// - 自动预留和续期
// - 自动故障恢复
package client

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var log = logger.Logger("relay.client")

// ============================================================================
//                              常量
// ============================================================================

const (
	// DefaultMinRelays 默认最小中继数量
	DefaultMinRelays = 2
	// DefaultMaxRelays 默认最大中继数量
	DefaultMaxRelays = 4

	// DefaultRefreshInterval 刷新间隔
	DefaultRefreshInterval = 5 * time.Minute

	// DefaultDiscoveryInterval 发现间隔
	// 可达性优先：需要更快发现可用中继，避免长时间"无兜底地址"
	DefaultDiscoveryInterval = 3 * time.Second

	// ReservationRefreshBefore 预留提前刷新时间
	ReservationRefreshBefore = 10 * time.Minute

	// 推断候选（来自"已连接节点"）的优先级：低于真实发现来源，且失败后应更长时间黑名单
	inferredRelayPriority = -100

	blacklistShort = 5 * time.Minute
	blacklistLong  = 1 * time.Hour
)

// ============================================================================
//                              配置
// ============================================================================

// AutoRelayConfig AutoRelay 配置
type AutoRelayConfig struct {
	// MinRelays 最小中继数
	MinRelays int

	// MaxRelays 最大中继数
	MaxRelays int

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// DiscoveryInterval 发现间隔
	DiscoveryInterval time.Duration

	// StaticRelays 静态中继列表
	StaticRelays []types.NodeID

	// EnableBackoff 启用退避
	EnableBackoff bool

	// MaxBackoff 最大退避时间
	MaxBackoff time.Duration
}

// DefaultAutoRelayConfig 默认配置
func DefaultAutoRelayConfig() AutoRelayConfig {
	return AutoRelayConfig{
		MinRelays:         DefaultMinRelays,
		MaxRelays:         DefaultMaxRelays,
		RefreshInterval:   DefaultRefreshInterval,
		DiscoveryInterval: DefaultDiscoveryInterval,
		StaticRelays:      nil,
		EnableBackoff:     true,
		MaxBackoff:        5 * time.Minute,
	}
}

// ============================================================================
//                              AutoRelay 实现
// ============================================================================

// AutoRelay 自动中继管理器
type AutoRelay struct {
	config   AutoRelayConfig
	client   relayif.RelayClient
	endpoint endpointif.Endpoint

	// 活跃中继
	activeRelays   map[types.NodeID]*activeRelay
	activeRelaysMu sync.RWMutex

	// 候选中继
	candidates   map[types.NodeID]*relayCandidate
	candidatesMu sync.RWMutex

	// 黑名单
	blacklist   map[types.NodeID]time.Time
	blacklistMu sync.RWMutex

	// 连接升级器
	upgrader *ConnectionUpgrader

	// 中继连接（用于升级）
	relayConns   map[types.NodeID]endpointif.Connection
	relayConnsMu sync.RWMutex

	// 地址变更回调（可达性优先策略）
	onAddrsChanged   func([]endpointif.Address)
	onAddrsChangedMu sync.RWMutex

	// 状态
	enabled int32
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// activeRelay 活跃中继
type activeRelay struct {
	nodeID      types.NodeID
	reservation relayif.Reservation
	addrs       []endpointif.Address
	lastRefresh time.Time
	failCount   int
}

// relayCandidate 候选中继
type relayCandidate struct {
	nodeID   types.NodeID
	addrs    []endpointif.Address
	latency  time.Duration
	lastSeen time.Time
	priority int
}

// NewAutoRelay 创建 AutoRelay
func NewAutoRelay(config AutoRelayConfig, client relayif.RelayClient, endpoint endpointif.Endpoint) *AutoRelay {
	return &AutoRelay{
		config:       config,
		client:       client,
		endpoint:     endpoint,
		activeRelays: make(map[types.NodeID]*activeRelay),
		candidates:   make(map[types.NodeID]*relayCandidate),
		blacklist:    make(map[types.NodeID]time.Time),
		relayConns:   make(map[types.NodeID]endpointif.Connection),
	}
}

// SetUpgrader 设置连接升级器
func (ar *AutoRelay) SetUpgrader(upgrader *ConnectionUpgrader) {
	ar.upgrader = upgrader
	if upgrader != nil {
		upgrader.OnUpgraded(ar.onConnectionUpgraded)
	}
}

// onConnectionUpgraded 连接升级成功回调
func (ar *AutoRelay) onConnectionUpgraded(remoteID types.NodeID, directAddr endpointif.Address) {
	log.Info("中继连接已升级为直连",
		"remoteID", remoteID.ShortString(),
		"directAddr", directAddr.String())

	// 移除中继连接
	ar.relayConnsMu.Lock()
	if conn, ok := ar.relayConns[remoteID]; ok {
		_ = conn.Close()
		delete(ar.relayConns, remoteID)
	}
	ar.relayConnsMu.Unlock()
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 AutoRelay
func (ar *AutoRelay) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&ar.running, 0, 1) {
		return nil
	}

	// 重要：不要使用 fx OnStart 传入的 ctx 作为长期运行 ctx。
	// 原因：fx OnStart 的 ctx 可能在启动阶段结束后被取消（例如 Bootstrap.Build() 返回时）。
	// AutoRelay 是后台常驻任务，应由 Stop() 或模块 OnStop 负责关闭。
	ar.ctx, ar.cancel = context.WithCancel(context.Background())

	// 添加静态中继（需要持有锁保护以防止竞态条件）
	ar.candidatesMu.Lock()
	for _, relayID := range ar.config.StaticRelays {
		ar.candidates[relayID] = &relayCandidate{
			nodeID:   relayID,
			priority: 100, // 高优先级
			lastSeen: time.Now(),
		}
	}
	ar.candidatesMu.Unlock()

	// 注册连接事件回调，监听 relay 连接断开事件
	// 当检测到 relay 连接断开时，触发重新预留
	ar.registerConnectionEventCallback()

	// 启动后台任务
	go ar.refreshLoop()
	go ar.discoveryLoop()
	go ar.maintenanceLoop()

	log.Info("AutoRelay 已启动",
		"static_relays", len(ar.config.StaticRelays))

	return nil
}

// registerConnectionEventCallback 注册连接事件回调
//
// 监听连接关闭事件，特别是 relay 连接断开时触发重新预留
func (ar *AutoRelay) registerConnectionEventCallback() {
	if ar.endpoint == nil {
		return
	}

	ar.endpoint.RegisterConnectionEventCallback(func(event interface{}) {
		// 类型断言获取连接关闭事件
		closed, ok := event.(endpointif.ConnectionClosedEvent)
		if !ok {
			return
		}

		// 只处理 relay 连接断开
		if !closed.IsRelayConn {
			return
		}

		// 检查是否为我们正在使用的 relay
		relayID := closed.RelayID
		if relayID.IsEmpty() {
			return
		}

		ar.handleRelayDisconnect(relayID, closed.Reason)
	})
}

// handleRelayDisconnect 处理 relay 连接断开
//
// 当检测到 relay 连接断开时：
//  1. 移除失效的中继
//  2. 触发重新预留流程
func (ar *AutoRelay) handleRelayDisconnect(relayID types.NodeID, reason error) {
	// 检查是否为活跃中继
	ar.activeRelaysMu.RLock()
	_, isActive := ar.activeRelays[relayID]
	ar.activeRelaysMu.RUnlock()

	if !isActive {
		return
	}

	log.Info("检测到 relay 连接断开，触发重新预留",
		"relay", relayID.ShortString(),
		"reason", reason)

	// 移除失效的中继
	ar.removeRelay(relayID)

	// 如果启用了自动中继，触发立即建立新连接
	if ar.IsEnabled() {
		go ar.ensureRelays()
	}
}

// Stop 停止 AutoRelay
func (ar *AutoRelay) Stop() error {
	if !atomic.CompareAndSwapInt32(&ar.running, 1, 0) {
		return nil
	}

	// 取消所有预留
	ar.activeRelaysMu.Lock()
	for _, relay := range ar.activeRelays {
		if relay.reservation != nil {
			relay.reservation.Cancel()
		}
	}
	ar.activeRelays = make(map[types.NodeID]*activeRelay)
	ar.activeRelaysMu.Unlock()

	if ar.cancel != nil {
		ar.cancel()
	}

	log.Info("AutoRelay 已停止")
	return nil
}

// ============================================================================
//                              启用/禁用
// ============================================================================

// Enable 启用 AutoRelay
func (ar *AutoRelay) Enable() {
	atomic.StoreInt32(&ar.enabled, 1)
	log.Info("AutoRelay 已启用")

	// 触发立即建立连接
	go ar.ensureRelays()
}

// Disable 禁用 AutoRelay
func (ar *AutoRelay) Disable() {
	atomic.StoreInt32(&ar.enabled, 0)
	log.Info("AutoRelay 已禁用")
}

// IsEnabled 是否已启用
func (ar *AutoRelay) IsEnabled() bool {
	return atomic.LoadInt32(&ar.enabled) == 1
}

// ============================================================================
//                              查询接口
// ============================================================================

// Relays 返回当前使用的中继
func (ar *AutoRelay) Relays() []types.NodeID {
	ar.activeRelaysMu.RLock()
	defer ar.activeRelaysMu.RUnlock()

	relays := make([]types.NodeID, 0, len(ar.activeRelays))
	for id := range ar.activeRelays {
		relays = append(relays, id)
	}
	return relays
}

// Status 返回状态
func (ar *AutoRelay) Status() relayif.AutoRelayStatus {
	ar.activeRelaysMu.RLock()
	activeCount := len(ar.activeRelays)
	var relayAddrs []string
	for _, relay := range ar.activeRelays {
		for _, addr := range relay.addrs {
			relayAddrs = append(relayAddrs, addr.String())
		}
	}
	ar.activeRelaysMu.RUnlock()

	return relayif.AutoRelayStatus{
		Enabled:    ar.IsEnabled(),
		NumRelays:  activeCount,
		RelayAddrs: relayAddrs,
	}
}

// ============================================================================
//                              后台任务
// ============================================================================

// refreshLoop 刷新循环
func (ar *AutoRelay) refreshLoop() {
	ticker := time.NewTicker(ar.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ticker.C:
			if ar.IsEnabled() {
				ar.refreshReservations()
			}
		}
	}
}

// discoveryLoop 发现循环
func (ar *AutoRelay) discoveryLoop() {
	ticker := time.NewTicker(ar.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ticker.C:
			if ar.IsEnabled() {
				ar.discoverRelays()
			}
		}
	}
}

// maintenanceLoop 维护循环
func (ar *AutoRelay) maintenanceLoop() {
	// 可达性优先：更快触发 ensureRelays，缩短 Relay 兜底准备时间
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ar.ctx.Done():
			return
		case <-ticker.C:
			if ar.IsEnabled() {
				ar.ensureRelays()
				ar.cleanupBlacklist()
			}
		}
	}
}

// ============================================================================
//                              核心逻辑
// ============================================================================

// ensureRelays 确保有足够的中继
func (ar *AutoRelay) ensureRelays() {
	ar.activeRelaysMu.RLock()
	activeCount := len(ar.activeRelays)
	ar.activeRelaysMu.RUnlock()

	if activeCount >= ar.config.MinRelays {
		return
	}

	needed := ar.config.MinRelays - activeCount
	log.Debug("需要更多中继",
		"active", activeCount,
		"needed", needed)

	// 获取候选列表
	candidates := ar.getCandidates(needed * 2)

	// 尝试连接
	for _, candidate := range candidates {
		if ar.isBlacklisted(candidate.nodeID) {
			continue
		}

		if ar.tryRelay(candidate.nodeID) {
			needed--
			if needed <= 0 {
				break
			}
		}
	}
}

// tryRelay 尝试使用中继
func (ar *AutoRelay) tryRelay(relayID types.NodeID) bool {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&ar.running) == 0 || ar.ctx == nil {
		return false
	}

	// 检查是否已经在使用
	ar.activeRelaysMu.RLock()
	_, exists := ar.activeRelays[relayID]
	ar.activeRelaysMu.RUnlock()

	if exists {
		return false
	}

	log.Debug("尝试预留中继",
		"relay", relayID.ShortString())

	ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
	defer cancel()

	// 预留
	reservation, err := ar.client.Reserve(ctx, relayID)
	if err != nil {
		log.Debug("预留失败",
			"relay", relayID.ShortString(),
			"err", err)
		ar.addToBlacklist(relayID)
		return false
	}

	// 添加到活跃列表
	ar.activeRelaysMu.Lock()
	ar.activeRelays[relayID] = &activeRelay{
		nodeID:      relayID,
		reservation: reservation,
		addrs:       reservation.Addrs(),
		lastRefresh: time.Now(),
	}
	ar.activeRelaysMu.Unlock()

	log.Info("中继预留成功",
		"relay", relayID.ShortString(),
		"expiry", reservation.Expiry())

	// 触发地址变更通知（可达性优先策略）
	ar.notifyAddrsChanged()

	// 通告中继地址到 Endpoint（使 Relay 地址可被其他节点通过 DHT 发现）
	ar.announceRelayAddresses(relayID, reservation.Addrs())

	return true
}

// refreshReservations 刷新预留
func (ar *AutoRelay) refreshReservations() {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&ar.running) == 0 || ar.ctx == nil {
		return
	}

	ar.activeRelaysMu.RLock()
	relays := make([]*activeRelay, 0, len(ar.activeRelays))
	for _, relay := range ar.activeRelays {
		relays = append(relays, relay)
	}
	ar.activeRelaysMu.RUnlock()

	for _, relay := range relays {
		// 检查是否需要刷新
		if relay.reservation != nil {
			expiry := relay.reservation.Expiry()
			if time.Until(expiry) > ReservationRefreshBefore {
				continue
			}
		}

		log.Debug("刷新中继预留",
			"relay", relay.nodeID.ShortString())

		ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
		err := relay.reservation.Refresh(ctx)
		cancel()

		if err != nil {
			log.Warn("刷新预留失败",
				"relay", relay.nodeID.ShortString(),
				"err", err)

			relay.failCount++
			if relay.failCount >= 3 {
				// 移除失败的中继
				ar.removeRelay(relay.nodeID)
			}
		} else {
			relay.lastRefresh = time.Now()
			relay.failCount = 0
		}
	}
}

// removeRelay 移除中继
func (ar *AutoRelay) removeRelay(relayID types.NodeID) {
	ar.activeRelaysMu.Lock()
	if relay, ok := ar.activeRelays[relayID]; ok {
		if relay.reservation != nil {
			relay.reservation.Cancel()
		}
		delete(ar.activeRelays, relayID)
	}
	ar.activeRelaysMu.Unlock()

	log.Info("移除中继",
		"relay", relayID.ShortString())

	// 触发地址变更通知（可达性优先策略）
	ar.notifyAddrsChanged()
}

// discoverRelays 发现中继
func (ar *AutoRelay) discoverRelays() {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&ar.running) == 0 || ar.ctx == nil {
		return
	}

	if ar.client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
	defer cancel()

	foundRelays, err := ar.client.FindRelays(ctx)
	if err != nil {
		log.Debug("发现中继失败", "err", err)
		// 继续尝试从当前连接中“推断候选中继”，避免纯静态发现失败导致永远无候选
	}

	// 额外来源：从已建立连接中推断候选（最保守策略：由 Reserve 验证是否真是中继）
	// 注意：该来源容易产生假阳性，因此会显式降权，并在失败时使用更长黑名单。
	var inferredRelays []types.NodeID
	if ar.endpoint != nil {
		conns := ar.endpoint.Connections()
		for _, c := range conns {
			if c == nil {
				continue
			}
			inferredRelays = append(inferredRelays, c.RemoteID())
		}
	}

	ar.candidatesMu.Lock()
	for _, relayID := range foundRelays {
		if cand, exists := ar.candidates[relayID]; exists && cand != nil {
			cand.lastSeen = time.Now()
			// 保留更高优先级（真实发现来源）
			if cand.priority < 0 {
				cand.priority = 0
			}
			continue
		}
		ar.candidates[relayID] = &relayCandidate{
			nodeID:   relayID,
			lastSeen: time.Now(),
			priority: 0,
		}
	}

	for _, relayID := range inferredRelays {
		// 仅在不存在候选时才加入推断候选（避免把真实候选降权）
		if cand, exists := ar.candidates[relayID]; exists && cand != nil {
			cand.lastSeen = time.Now()
			continue
		}
		ar.candidates[relayID] = &relayCandidate{
			nodeID:   relayID,
			lastSeen: time.Now(),
			priority: inferredRelayPriority,
		}
	}
	ar.candidatesMu.Unlock()

	log.Debug("发现中继",
		"countFound", len(foundRelays),
		"countInferred", len(inferredRelays))

	// 发现候选后立即尝试补足中继（可达性优先）
	ar.ensureRelays()
}

// getCandidates 获取候选列表
func (ar *AutoRelay) getCandidates(count int) []*relayCandidate {
	ar.candidatesMu.RLock()
	defer ar.candidatesMu.RUnlock()

	candidates := make([]*relayCandidate, 0, len(ar.candidates))
	for _, c := range ar.candidates {
		candidates = append(candidates, c)
	}

	// 按优先级和延迟排序
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		return candidates[i].latency < candidates[j].latency
	})

	if len(candidates) > count {
		candidates = candidates[:count]
	}

	return candidates
}

// ============================================================================
//                              黑名单
// ============================================================================

// addToBlacklist 添加到黑名单
func (ar *AutoRelay) addToBlacklist(relayID types.NodeID) {
	// 对“推断候选”（非明确发现来源）的失败，采用更长黑名单，避免反复浪费资源
	ttl := blacklistShort
	ar.candidatesMu.RLock()
	if cand, ok := ar.candidates[relayID]; ok && cand != nil && cand.priority <= inferredRelayPriority {
		ttl = blacklistLong
	}
	ar.candidatesMu.RUnlock()

	ar.blacklistMu.Lock()
	ar.blacklist[relayID] = time.Now().Add(ttl)
	ar.blacklistMu.Unlock()
}

// isBlacklisted 检查是否在黑名单
func (ar *AutoRelay) isBlacklisted(relayID types.NodeID) bool {
	ar.blacklistMu.RLock()
	expiry, exists := ar.blacklist[relayID]
	ar.blacklistMu.RUnlock()

	if !exists {
		return false
	}
	return time.Now().Before(expiry)
}

// cleanupBlacklist 清理过期黑名单
func (ar *AutoRelay) cleanupBlacklist() {
	ar.blacklistMu.Lock()
	defer ar.blacklistMu.Unlock()

	now := time.Now()
	for id, expiry := range ar.blacklist {
		if now.After(expiry) {
			delete(ar.blacklist, id)
		}
	}
}

// ============================================================================
//                              地址管理
// ============================================================================

// RelayAddrs 返回中继地址
func (ar *AutoRelay) RelayAddrs() []endpointif.Address {
	ar.activeRelaysMu.RLock()
	defer ar.activeRelaysMu.RUnlock()

	var addrs []endpointif.Address
	for _, relay := range ar.activeRelays {
		addrs = append(addrs, relay.addrs...)
	}
	return addrs
}

// SetOnAddrsChanged 设置地址变更回调（可达性优先策略）
//
// 当 Relay 地址发生变化时（预留成功/失败/过期），会调用此回调。
// ReachabilityCoordinator 通过此回调感知 Relay 地址的变化。
func (ar *AutoRelay) SetOnAddrsChanged(callback func([]endpointif.Address)) {
	ar.onAddrsChangedMu.Lock()
	ar.onAddrsChanged = callback
	ar.onAddrsChangedMu.Unlock()
}

// notifyAddrsChanged 通知地址变更
func (ar *AutoRelay) notifyAddrsChanged() {
	ar.onAddrsChangedMu.RLock()
	callback := ar.onAddrsChanged
	ar.onAddrsChangedMu.RUnlock()

	if callback != nil {
		addrs := ar.RelayAddrs()
		// 异步调用，避免阻塞
		go callback(addrs)
	}
}

// announceRelayAddresses 通告中继地址
//
// 将成功预留的中继地址添加到 Endpoint 的通告地址列表，
// 使其他节点可以通过 DHT 发现本节点的中继地址。
//
// 这是 Relay Transport Integration 的关键步骤：
// - 其他节点查询 DHT 时会获取到这些中继地址
// - 当它们调用 Connect() 时，TransportRegistry 会选择 RelayTransport 进行拨号
func (ar *AutoRelay) announceRelayAddresses(relayID types.NodeID, reservationAddrs []endpointif.Address) {
	if ar.endpoint == nil {
		return
	}

	// 构建中继地址格式：/relay-base-addr/p2p/<relay-id>/p2p-circuit/p2p/<local-id>
	localID := ar.endpoint.ID()
	if localID.IsEmpty() {
		return
	}

	// 获取中继节点的可拨号地址
	relayAddrs := ar.getRelayNodeAddresses(relayID)
	if len(relayAddrs) == 0 {
		// 使用预留返回的地址（可能已经是 circuit 格式）
		for _, addr := range reservationAddrs {
			log.Debug("通告中继地址（来自预留）",
				"relay", relayID.ShortString(),
				"addr", addr.String())
		}
		return
	}

	// 收集所有构建的中继地址
	var builtAddrs []endpointif.Address

	// 为每个中继地址构建完整的 circuit 地址
	for _, relayBaseAddr := range relayAddrs {
		// 跳过已经是 circuit 地址的
		if types.IsRelayAddr(relayBaseAddr.String()) {
			continue
		}

		// 使用 multiaddr 格式构建 relay circuit 地址，确保 DHT 验证通过
		relayAddrStr := types.BuildRelayAddr(
			toMultiaddr(relayBaseAddr)+"/p2p/"+relayID.String(),
			localID,
		)

		// 创建地址对象
		circuitAddr := &relayCircuitAddress{addr: relayAddrStr}
		builtAddrs = append(builtAddrs, circuitAddr)

		log.Info("通告中继地址",
			"relay", relayID.ShortString(),
			"addr", relayAddrStr)

		// 将中继地址添加到 Endpoint 的通告地址列表
		ar.endpoint.AddAdvertisedAddr(circuitAddr)
	}

	// 触发发现服务刷新，使其他节点可以通过 DHT 发现这些中继地址
	if len(builtAddrs) > 0 {
		discovery := ar.endpoint.Discovery()
		if discovery != nil {
			discovery.RefreshAnnounce(builtAddrs)
			log.Info("已触发中继地址发布到 DHT",
				"relay", relayID.ShortString(),
				"addrs", len(builtAddrs))
		}
	}
}

// relayCircuitAddress 中继 circuit 地址实现
type relayCircuitAddress struct {
	addr string
}

func (a *relayCircuitAddress) Network() string   { return "p2p-circuit" }
func (a *relayCircuitAddress) String() string    { return a.addr }
func (a *relayCircuitAddress) Bytes() []byte     { return []byte(a.addr) }
// p2p-circuit 地址不含 IP，视为 unknown（保守处理）
func (a *relayCircuitAddress) IsPublic() bool    { return false }
func (a *relayCircuitAddress) IsPrivate() bool   { return false }
func (a *relayCircuitAddress) IsLoopback() bool  { return false }
func (a *relayCircuitAddress) Equal(other endpointif.Address) bool {
	if other == nil {
		return false
	}
	return a.addr == other.String()
}

// Multiaddr 返回 multiaddr 格式
// relay circuit 地址本身就是 multiaddr 格式
func (a *relayCircuitAddress) Multiaddr() string { return a.addr }

// ============================================================================
//                              地址格式转换
// ============================================================================

// toMultiaddr 将地址转换为 multiaddr 格式
//
// 如果地址实现了 Multiaddr() 方法，直接调用；
// 否则尝试解析 host:port 格式并转换为 /ip4/.../udp/.../quic-v1 格式。
func toMultiaddr(addr endpointif.Address) string {
	if addr == nil {
		return ""
	}

	// 优先使用 Multiaddr() 方法（如果实现了）
	if m, ok := addr.(interface{ Multiaddr() string }); ok {
		return m.Multiaddr()
	}

	// 如果地址字符串已经是 multiaddr 格式（以 / 开头），直接返回
	addrStr := addr.String()
	if strings.HasPrefix(addrStr, "/") {
		return addrStr
	}

	// 解析 host:port 格式并转换为 multiaddr
	return convertHostPortToMultiaddr(addrStr, addr.Network())
}

// convertHostPortToMultiaddr 将 host:port 格式转换为 multiaddr 格式
//
// 参数:
//   - addrStr: host:port 格式的地址字符串，如 "1.2.3.4:8000" 或 "[::1]:8000"
//   - network: 网络类型提示，如 "ip4", "ip6", "quic"
//
// 返回:
//   - multiaddr 格式，如 "/ip4/1.2.3.4/udp/8000/quic-v1"
func convertHostPortToMultiaddr(addrStr string, network string) string {
	host, port, err := net.SplitHostPort(addrStr)
	if err != nil {
		// 无法解析，返回原字符串
		return addrStr
	}

	// 解析 IP 地址
	ip := net.ParseIP(host)
	if ip == nil {
		// 可能是域名，使用 dns4
		return fmt.Sprintf("/dns4/%s/udp/%s/quic-v1", host, port)
	}

	// 判断 IPv4 还是 IPv6
	ipType := "ip4"
	if ip.To4() == nil {
		ipType = "ip6"
	}

	// 构建 multiaddr（默认使用 UDP/QUIC-v1，因为这是当前主要传输协议）
	return fmt.Sprintf("/%s/%s/udp/%s/quic-v1", ipType, host, port)
}

// getRelayNodeAddresses 获取中继节点的可拨号地址
func (ar *AutoRelay) getRelayNodeAddresses(relayID types.NodeID) []endpointif.Address {
	// 从候选列表获取
	ar.candidatesMu.RLock()
	if cand, ok := ar.candidates[relayID]; ok && cand != nil && len(cand.addrs) > 0 {
		addrs := make([]endpointif.Address, len(cand.addrs))
		copy(addrs, cand.addrs)
		ar.candidatesMu.RUnlock()
		return addrs
	}
	ar.candidatesMu.RUnlock()

	// 从已建立的连接获取
	if ar.endpoint != nil {
		if conn, ok := ar.endpoint.Connection(relayID); ok {
			remoteAddrs := conn.RemoteAddrs()
			if len(remoteAddrs) > 0 {
				return remoteAddrs
			}
		}
	}

	return nil
}

// TryUpgradeConnection 尝试将中继连接升级为直连
//
// 设计依据：docs/01-design/protocols/network/03-relay.md 第 300-329 行
// 流程：
// 1. 通过中继连接交换地址信息
// 2. 并行尝试打洞（10s 超时）
// 3. 成功后迁移到直连，释放中继资源
func (ar *AutoRelay) TryUpgradeConnection(ctx context.Context, remoteID types.NodeID, relayConn endpointif.Connection) (endpointif.Address, error) {
	if ar.upgrader == nil {
		return nil, ErrNoPuncher
	}

	// 保存中继连接
	ar.relayConnsMu.Lock()
	ar.relayConns[remoteID] = relayConn
	ar.relayConnsMu.Unlock()

	// 尝试升级
	return ar.upgrader.TryUpgrade(ctx, remoteID, relayConn)
}

// StartAutoUpgrade 启动自动升级（连接后立即尝试）
func (ar *AutoRelay) StartAutoUpgrade(remoteID types.NodeID, relayConn endpointif.Connection) {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&ar.running) == 0 || ar.ctx == nil {
		return
	}

	if ar.upgrader == nil || !ar.upgrader.config.EnableAutoUpgrade {
		return
	}

	go func() {
		// 再次检查上下文是否有效
		if ar.ctx == nil {
			return
		}
		ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
		defer cancel()

		addr, err := ar.TryUpgradeConnection(ctx, remoteID, relayConn)
		if err != nil {
			log.Debug("自动升级失败，继续使用中继",
				"remoteID", remoteID.ShortString(),
				"err", err)
		} else {
			log.Info("自动升级成功",
				"remoteID", remoteID.ShortString(),
				"directAddr", addr.String())
		}
	}()
}

// AddCandidate 添加候选中继
func (ar *AutoRelay) AddCandidate(relayID types.NodeID, addrs []endpointif.Address, priority int) {
	ar.candidatesMu.Lock()
	defer ar.candidatesMu.Unlock()

	ar.candidates[relayID] = &relayCandidate{
		nodeID:   relayID,
		addrs:    addrs,
		priority: priority,
		lastSeen: time.Now(),
	}
}

// RemoveCandidate 移除候选中继
func (ar *AutoRelay) RemoveCandidate(relayID types.NodeID) {
	ar.candidatesMu.Lock()
	defer ar.candidatesMu.Unlock()

	delete(ar.candidates, relayID)
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ relayif.AutoRelay = (*AutoRelay)(nil)

