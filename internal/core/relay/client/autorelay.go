// Package client 提供中继客户端实现
//
// AutoRelay 自动管理中继连接，确保节点可达性：
//   - 自动发现中继服务器
//   - 自动预留和续期
//   - 自动故障恢复
//   - 健康检查和黑名单管理
package client

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var autorelayLogger = log.Logger("relay/autorelay")

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
	DefaultDiscoveryInterval = 30 * time.Second

	// ReservationRefreshBefore 预留提前刷新时间
	ReservationRefreshBefore = 10 * time.Minute

	// preferredRelayPriority 首选中继优先级
	preferredRelayPriority = 100

	// blacklistShort 短期黑名单时间
	blacklistShort = 5 * time.Minute

	// blacklistLong 长期黑名单时间
	blacklistLong = 1 * time.Hour
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
	StaticRelays []string

	// EnableBackoff 启用退避
	EnableBackoff bool

	// MaxBackoff 最大退避时间
	MaxBackoff time.Duration
}

// DefaultAutoRelayConfig 返回默认配置
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
	config    AutoRelayConfig
	client    pkgif.RelayClient
	host      pkgif.Host
	peerstore pkgif.Peerstore

	// 活跃中继
	activeRelays   map[string]*activeRelay
	activeRelaysMu sync.RWMutex

	// 候选中继
	candidates   map[string]*relayCandidate
	candidatesMu sync.RWMutex

	// 黑名单
	blacklist   map[string]time.Time
	blacklistMu sync.RWMutex

	// 首选中继列表
	preferredRelays   map[string]struct{}
	preferredRelaysMu sync.RWMutex

	// 地址变更回调
	onAddrsChanged   func([]string)
	onAddrsChangedMu sync.RWMutex

	// 日志指数退避
	lastNoRelayLog     time.Time     // 上次打印"需要更多中继"的时间
	noRelayLogInterval time.Duration // 当前日志间隔（指数退避）
	lastActiveCount    int           // 上次活跃中继数量（用于检测状态变化）

	// 状态
	enabled int32
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// activeRelay 活跃中继
type activeRelay struct {
	relayID     string
	reservation pkgif.Reservation
	addrs       []string
	lastRefresh time.Time
	failCount   int
}

// relayCandidate 候选中继
type relayCandidate struct {
	relayID  string
	addrs    []string
	latency  time.Duration
	lastSeen time.Time
	priority int
}

// NewAutoRelay 创建 AutoRelay
func NewAutoRelay(config AutoRelayConfig, client pkgif.RelayClient, host pkgif.Host, peerstore pkgif.Peerstore) *AutoRelay {
	return &AutoRelay{
		config:             config,
		client:             client,
		host:               host,
		peerstore:          peerstore,
		activeRelays:       make(map[string]*activeRelay),
		candidates:         make(map[string]*relayCandidate),
		blacklist:          make(map[string]time.Time),
		preferredRelays:    make(map[string]struct{}),
		noRelayLogInterval: 5 * time.Second, // 初始间隔 5 秒
		lastActiveCount:    -1,              // -1 表示未初始化
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 AutoRelay
func (ar *AutoRelay) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&ar.running, 0, 1) {
		return nil
	}

	// 使用独立的 context，避免 Fx OnStart ctx 取消导致的问题
	ar.ctx, ar.cancel = context.WithCancel(context.Background())

	// 添加静态中继
	ar.candidatesMu.Lock()
	for _, relayID := range ar.config.StaticRelays {
		ar.candidates[relayID] = &relayCandidate{
			relayID:  relayID,
			priority: 100, // 高优先级
			lastSeen: time.Now(),
		}
	}
	ar.candidatesMu.Unlock()

	// 启动后台任务
	ar.wg.Add(3)
	go ar.refreshLoop()
	go ar.discoveryLoop()
	go ar.maintenanceLoop()

	autorelayLogger.Info("AutoRelay 已启动",
		"staticRelays", len(ar.config.StaticRelays))

	return nil
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
	ar.activeRelays = make(map[string]*activeRelay)
	ar.activeRelaysMu.Unlock()

	if ar.cancel != nil {
		ar.cancel()
	}

	ar.wg.Wait()

	autorelayLogger.Info("AutoRelay 已停止")
	return nil
}

// ============================================================================
//                              启用/禁用
// ============================================================================

// Enable 启用 AutoRelay
func (ar *AutoRelay) Enable() {
	atomic.StoreInt32(&ar.enabled, 1)
	autorelayLogger.Info("AutoRelay 已启用")

	// 触发立即建立连接
	go ar.ensureRelays()
}

// Disable 禁用 AutoRelay
func (ar *AutoRelay) Disable() {
	atomic.StoreInt32(&ar.enabled, 0)
	autorelayLogger.Info("AutoRelay 已禁用")
}

// IsEnabled 是否已启用
func (ar *AutoRelay) IsEnabled() bool {
	return atomic.LoadInt32(&ar.enabled) == 1
}

// ============================================================================
//                              查询接口
// ============================================================================

// Relays 返回当前使用的中继
func (ar *AutoRelay) Relays() []string {
	ar.activeRelaysMu.RLock()
	defer ar.activeRelaysMu.RUnlock()

	relays := make([]string, 0, len(ar.activeRelays))
	for id := range ar.activeRelays {
		relays = append(relays, id)
	}
	return relays
}

// RelayAddrs 返回中继地址
func (ar *AutoRelay) RelayAddrs() []string {
	ar.activeRelaysMu.RLock()
	defer ar.activeRelaysMu.RUnlock()

	var addrs []string
	for _, relay := range ar.activeRelays {
		addrs = append(addrs, relay.addrs...)
	}
	return addrs
}

// Status 返回状态
func (ar *AutoRelay) Status() pkgif.AutoRelayStatus {
	ar.activeRelaysMu.RLock()
	activeCount := len(ar.activeRelays)
	var relayAddrs []string
	for _, relay := range ar.activeRelays {
		relayAddrs = append(relayAddrs, relay.addrs...)
	}
	ar.activeRelaysMu.RUnlock()

	ar.candidatesMu.RLock()
	candidateCount := len(ar.candidates)
	ar.candidatesMu.RUnlock()

	ar.blacklistMu.RLock()
	blacklistCount := len(ar.blacklist)
	ar.blacklistMu.RUnlock()

	return pkgif.AutoRelayStatus{
		Enabled:        ar.IsEnabled(),
		NumRelays:      activeCount,
		RelayAddrs:     relayAddrs,
		NumCandidates:  candidateCount,
		NumBlacklisted: blacklistCount,
	}
}

// ============================================================================
//                              后台任务
// ============================================================================

// refreshLoop 刷新循环
func (ar *AutoRelay) refreshLoop() {
	defer ar.wg.Done()

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
	defer ar.wg.Done()

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
	defer ar.wg.Done()

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
		// 有足够中继时重置日志间隔
		ar.noRelayLogInterval = 5 * time.Second
		ar.lastActiveCount = activeCount
		return
	}

	needed := ar.config.MinRelays - activeCount

	// 使用指数退避来减少日志频率
	// 只在状态发生变化或距离上次打印已过足够时间时打印
	shouldLog := false
	if activeCount != ar.lastActiveCount {
		// 状态变化，立即打印并重置间隔
		shouldLog = true
		ar.noRelayLogInterval = 5 * time.Second
	} else if time.Since(ar.lastNoRelayLog) >= ar.noRelayLogInterval {
		// 间隔已过，打印并增加间隔（指数退避，最大 5 分钟）
		shouldLog = true
		ar.noRelayLogInterval = ar.noRelayLogInterval * 2
		if ar.noRelayLogInterval > 5*time.Minute {
			ar.noRelayLogInterval = 5 * time.Minute
		}
	}

	if shouldLog {
		autorelayLogger.Debug("需要更多中继",
			"active", activeCount,
			"needed", needed)
		ar.lastNoRelayLog = time.Now()
	}
	ar.lastActiveCount = activeCount

	// 获取候选列表
	candidates := ar.getCandidates(needed * 2)

	// 尝试连接
	for _, candidate := range candidates {
		if ar.isBlacklisted(candidate.relayID) {
			continue
		}

		if ar.tryRelay(candidate.relayID) {
			needed--
			if needed <= 0 {
				break
			}
		}
	}
}

// tryRelay 尝试使用中继
//
// 对于 "stream canceled" 错误提供更清晰的日志说明，
// 这类错误通常表示远端不支持 Relay 协议。
// 在尝试预留前检查 HOP 协议支持，避免对已知不支持的节点发起请求。
func (ar *AutoRelay) tryRelay(relayID string) bool {
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

	// 预检查 HOP 协议支持，避免对已知不支持的节点发起无效请求
	if ar.peerstore != nil {
		supportedProtos, err := ar.peerstore.SupportsProtocols(
			types.PeerID(relayID),
			types.ProtocolID(HopProtocolID),
		)
		if err == nil && len(supportedProtos) == 0 {
			// 检查是否有该节点的任何协议信息
			allProtos, _ := ar.peerstore.GetProtocols(types.PeerID(relayID))
			if len(allProtos) > 0 {
				// 有协议信息但不支持 HOP 协议，确定不是 Relay 服务器
				autorelayLogger.Debug("跳过非 Relay 节点（不支持 HOP 协议）",
					"relay", relayID,
					"knownProtocols", len(allProtos))
				ar.addToBlacklist(relayID)
				return false
			}
			// 没有协议信息，可能是新节点，继续尝试预留
		}
	}

	autorelayLogger.Debug("尝试预留中继", "relay", relayID)

	ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
	defer cancel()

	// 预留
	reservation, err := ar.client.Reserve(ctx, relayID)
	if err != nil {
		// 分析错误类型，提供更清晰的日志
		errStr := err.Error()
		if isStreamCanceledError(errStr) {
			// "stream X canceled by remote with error code 0" 表示远端关闭了流
			// 这通常意味着远端不支持 Relay 协议（不是真正的 Relay 服务器）
			autorelayLogger.Debug("预留失败: 远端不支持 Relay 协议",
				"relay", relayID,
				"hint", "该节点可能不是 Relay 服务器")
		} else if isProtocolNotSupportedError(errStr) {
			// 协议不支持
			autorelayLogger.Debug("预留失败: 协议协商失败",
				"relay", relayID,
				"hint", "远端不支持 HOP 协议")
		} else {
			autorelayLogger.Debug("预留失败",
				"relay", relayID,
				"err", err)
		}
		ar.addToBlacklist(relayID)
		return false
	}

	// 添加到活跃列表
	ar.activeRelaysMu.Lock()
	ar.activeRelays[relayID] = &activeRelay{
		relayID:     relayID,
		reservation: reservation,
		addrs:       reservation.Addrs(),
		lastRefresh: time.Now(),
	}
	ar.activeRelaysMu.Unlock()

	autorelayLogger.Info("中继预留成功",
		"relay", relayID,
		"expiry", reservation.Expiry())

	// 触发地址变更通知
	ar.notifyAddrsChanged()

	return true
}

// refreshReservations 刷新预留
func (ar *AutoRelay) refreshReservations() {
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
		if relay.reservation != nil {
			expiry := relay.reservation.Expiry()
			// 处理 Expiry() 返回 0 的情况，expiry=0 时总是尝试刷新
			if expiry > 0 {
				// 检查是否需要刷新（距离过期时间小于 ReservationRefreshBefore）
				expiryTime := time.Unix(expiry, 0)
				if time.Until(expiryTime) > ReservationRefreshBefore {
					continue
				}
			}
			// expiry=0 时，总是尝试刷新
		}

		autorelayLogger.Debug("刷新中继预留", "relay", relay.relayID)

		ctx, cancel := context.WithTimeout(ar.ctx, 30*time.Second)
		err := relay.reservation.Refresh(ctx)
		cancel()

		// 使用写锁保护 relay 字段的并发修改
		ar.activeRelaysMu.Lock()
		if err != nil {
			autorelayLogger.Warn("刷新预留失败",
				"relay", relay.relayID,
				"err", err)

			relay.failCount++
			if relay.failCount >= 3 {
				ar.activeRelaysMu.Unlock()
				ar.removeRelay(relay.relayID)
				continue
			}
		} else {
			relay.lastRefresh = time.Now()
			relay.failCount = 0
		}
		ar.activeRelaysMu.Unlock()
	}
}

// removeRelay 移除中继
func (ar *AutoRelay) removeRelay(relayID string) {
	ar.activeRelaysMu.Lock()
	if relay, ok := ar.activeRelays[relayID]; ok {
		if relay.reservation != nil {
			relay.reservation.Cancel()
		}
		delete(ar.activeRelays, relayID)
	}
	ar.activeRelaysMu.Unlock()

	autorelayLogger.Info("移除中继", "relay", relayID)

	// 触发地址变更通知
	ar.notifyAddrsChanged()
}

// discoverRelays 发现中继
func (ar *AutoRelay) discoverRelays() {
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
		autorelayLogger.Debug("发现中继失败", "err", err)
		return
	}

	ar.candidatesMu.Lock()
	for _, relayID := range foundRelays {
		if cand, exists := ar.candidates[relayID]; exists && cand != nil {
			cand.lastSeen = time.Now()
			continue
		}
		ar.candidates[relayID] = &relayCandidate{
			relayID:  relayID,
			lastSeen: time.Now(),
			priority: 0,
		}
	}
	ar.candidatesMu.Unlock()

	autorelayLogger.Debug("发现中继", "count", len(foundRelays))

	// 发现候选后立即尝试补足中继
	ar.ensureRelays()
}

// getCandidates 获取候选列表
func (ar *AutoRelay) getCandidates(count int) []*relayCandidate {
	// 处理 count <= 0 的情况，避免 slice bounds out of range panic
	if count <= 0 {
		return nil
	}

	ar.candidatesMu.RLock()
	defer ar.candidatesMu.RUnlock()

	candidates := make([]*relayCandidate, 0, len(ar.candidates))
	for _, c := range ar.candidates {
		// 如果是首选中继，提升优先级
		if ar.isPreferredRelay(c.relayID) {
			preferredCand := *c
			preferredCand.priority = preferredRelayPriority
			candidates = append(candidates, &preferredCand)
		} else {
			candidates = append(candidates, c)
		}
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
//                              首选中继
// ============================================================================

// SetPreferredRelays 设置首选中继列表
func (ar *AutoRelay) SetPreferredRelays(relayIDs []string) {
	ar.preferredRelaysMu.Lock()
	defer ar.preferredRelaysMu.Unlock()

	ar.preferredRelays = make(map[string]struct{})
	for _, id := range relayIDs {
		ar.preferredRelays[id] = struct{}{}
	}

	autorelayLogger.Debug("设置首选中继", "count", len(relayIDs))
}

// isPreferredRelay 检查是否为首选中继
func (ar *AutoRelay) isPreferredRelay(relayID string) bool {
	ar.preferredRelaysMu.RLock()
	defer ar.preferredRelaysMu.RUnlock()
	_, ok := ar.preferredRelays[relayID]
	return ok
}

// ============================================================================
//                              黑名单
// ============================================================================

// addToBlacklist 添加到黑名单
func (ar *AutoRelay) addToBlacklist(relayID string) {
	ttl := blacklistShort
	ar.candidatesMu.RLock()
	if cand, ok := ar.candidates[relayID]; ok && cand != nil && cand.priority < 0 {
		ttl = blacklistLong
	}
	ar.candidatesMu.RUnlock()

	ar.blacklistMu.Lock()
	ar.blacklist[relayID] = time.Now().Add(ttl)
	ar.blacklistMu.Unlock()
}

// isBlacklisted 检查是否在黑名单
func (ar *AutoRelay) isBlacklisted(relayID string) bool {
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
//                              候选管理
// ============================================================================

// AddCandidate 添加候选中继
func (ar *AutoRelay) AddCandidate(relayID string, addrs []string, priority int) {
	ar.candidatesMu.Lock()
	defer ar.candidatesMu.Unlock()

	ar.candidates[relayID] = &relayCandidate{
		relayID:  relayID,
		addrs:    addrs,
		priority: priority,
		lastSeen: time.Now(),
	}
}

// RemoveCandidate 移除候选中继
func (ar *AutoRelay) RemoveCandidate(relayID string) {
	ar.candidatesMu.Lock()
	defer ar.candidatesMu.Unlock()

	delete(ar.candidates, relayID)
}

// ============================================================================
//                              地址变更通知
// ============================================================================

// SetOnAddrsChanged 设置地址变更回调
func (ar *AutoRelay) SetOnAddrsChanged(callback func([]string)) {
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

// ============================================================================
//                              辅助函数
// ============================================================================

// buildCircuitAddr 构建 circuit 地址
func buildCircuitAddr(relayAddr, localID string) string {
	return relayAddr + "/p2p-circuit/p2p/" + localID
}

// ============================================================================
//                              Reservation 适配器
// ============================================================================

// reservationWrapper 预留包装器
type reservationWrapper struct {
	relayPeer  types.PeerID
	expireTime time.Time
	addrs      []string
	client     *Client
}

// Expiry 返回过期时间戳
func (r *reservationWrapper) Expiry() int64 {
	return r.expireTime.Unix()
}

// Addrs 返回中继地址
func (r *reservationWrapper) Addrs() []string {
	return r.addrs
}

// Refresh 刷新预留
func (r *reservationWrapper) Refresh(ctx context.Context) error {
	if r.client == nil {
		return ErrClientClosed
	}

	// 重新预留
	reservation, err := r.client.Reserve(ctx)
	if err != nil {
		return err
	}

	r.expireTime = reservation.ExpireTime
	return nil
}

// Cancel 取消预留
func (r *reservationWrapper) Cancel() {
	// 目前预留是时间过期的，无需主动取消
}

// ============================================================================
//                              错误分析辅助函数
// ============================================================================

// isStreamCanceledError 检查是否是 "stream canceled by remote" 错误
//
// 这类错误表示远端主动关闭了流，通常发生在：
// - 远端不支持请求的协议
// - 远端没有启用 Relay 服务器功能
// - 协议版本不匹配
func isStreamCanceledError(errStr string) bool {
	// 匹配 "stream X canceled by remote with error code Y" 模式
	return strings.Contains(errStr, "canceled by remote") ||
		strings.Contains(errStr, "stream reset")
}

// isProtocolNotSupportedError 检查是否是协议不支持错误
func isProtocolNotSupportedError(errStr string) bool {
	return strings.Contains(errStr, "protocols not supported") ||
		strings.Contains(errStr, "protocol negotiation failed") ||
		strings.Contains(errStr, "no protocol")
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ pkgif.AutoRelay = (*AutoRelay)(nil)
