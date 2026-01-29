// Package relay 提供中继服务实现
//
// RelayDiscovery 提供中继服务的发现和发布功能：
//   - 通过 DHT 发现公共中继服务器
//   - 通过 Realm 发现私有中继服务器
//   - 发布本节点的中继服务能力
//   - 基于延迟/负载选择最优中继
package relay

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var discoveryLogger = log.Logger("relay/discovery")

// safePeerIDPrefix 安全获取 PeerID 前缀，避免切片越界 panic
func safePeerIDPrefix(peerID types.PeerID) string {
	s := string(peerID)
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrDiscoveryClosed 发现服务已关闭
	ErrDiscoveryClosed = errors.New("relay discovery closed")

	// ErrNoDiscoverySource 没有可用的发现源
	ErrNoDiscoverySource = errors.New("no discovery source available")

	// ErrAdvertiseFailed 发布失败
	ErrAdvertiseFailed = errors.New("advertise failed")
)

// ============================================================================
//                              常量定义
// ============================================================================

// 协议常量（使用统一定义）
var (
	// RelayNamespace DHT 中继服务命名空间（作为 provider payload）
	RelayNamespaceLocal = protocol.RelayNamespace

	// HopProtocolIDForDiscovery Relay HOP 协议 ID（用于验证是否为真正的 Relay 服务器）
	HopProtocolIDForDiscovery = string(protocol.RelayHop)
)

const (

	// DefaultDiscoveryInterval 默认发现间隔
	DefaultDiscoveryInterval = 5 * time.Minute

	// DefaultAdvertiseInterval 默认发布间隔
	DefaultAdvertiseInterval = 10 * time.Minute

	// DefaultDiscoveryTimeout 默认发现超时
	// v2.0.1: 从 30s 增加到 60s，配合 DHT QueryTimeout 增加，减少超时风险
	DefaultDiscoveryTimeout = 60 * time.Second

	// DefaultMaxRelays 默认最大中继数
	DefaultMaxRelays = 10

	// RelayTTL 中继记录 TTL
	RelayTTL = 30 * time.Minute
)

// ============================================================================
//                              DiscoveredRelay 结构
// ============================================================================

// DiscoveredRelay 发现的中继信息
type DiscoveredRelay struct {
	// PeerID 中继节点 ID
	PeerID types.PeerID

	// Addrs 中继地址列表
	Addrs []string

	// Latency 延迟（毫秒）
	Latency time.Duration

	// Load 当前负载（0-100）
	Load int

	// Capacity 最大容量
	Capacity int

	// LastSeen 最后发现时间
	LastSeen time.Time

	// Source 发现来源（"dht" 或 "static"）
	Source string

	// Score 综合评分
	Score float64
}

// ============================================================================
//                              RelayDiscovery 结构
// ============================================================================

// RelayDiscovery 中继发现服务
//
// 负责发现和发布中继服务，通过 DHT 或显式配置。
type RelayDiscovery struct {
	// 依赖组件
	dht       pkgif.Discovery  // DHT 发现接口
	host      pkgif.Host       // 本地主机
	peerstore pkgif.Peerstore  // 节点存储
	liveness  pkgif.Liveness   // Liveness 服务（可选，用于精确延迟测量）

	// 配置
	discoveryInterval time.Duration
	advertiseInterval time.Duration
	discoveryTimeout  time.Duration
	maxRelays         int

	// v2.0.1: 静态 Relay 地址（用于 DHT 发现失败时的回退）
	staticRelayAddrs []string

	// 发现的中继缓存
	relays   map[types.PeerID]*DiscoveredRelay
	relaysMu sync.RWMutex

	// Phase 9 修复：延迟缓存
	latencyCache   map[types.PeerID]time.Duration
	latencyCacheMu sync.RWMutex

	// 本地中继服务状态
	isRelayServer     bool
	relayServerConfig RelayServerConfig

	// 状态
	running int32
	closed  int32

	// 同步
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// RelayServerConfig 中继服务器配置
type RelayServerConfig struct {
	// MaxConnections 最大连接数
	MaxConnections int

	// MaxDuration 最大连接时长
	MaxDuration time.Duration

	// MaxData 最大传输数据量
	MaxData int64
}

// RelayDiscoveryConfig 发现服务配置
type RelayDiscoveryConfig struct {
	// DiscoveryInterval 发现间隔
	DiscoveryInterval time.Duration

	// AdvertiseInterval 发布间隔
	AdvertiseInterval time.Duration

	// DiscoveryTimeout 发现超时
	DiscoveryTimeout time.Duration

	// MaxRelays 最大中继数
	MaxRelays int

	// StaticRelayAddrs 静态 Relay 地址
	// v2.0.1: 当 DHT 发现失败或超时时，使用静态配置作为回退
	// 格式: multiaddr 字符串，如 "/ip4/1.2.3.4/tcp/4001/p2p/Qm..."
	StaticRelayAddrs []string
}

// DefaultRelayDiscoveryConfig 返回默认配置
func DefaultRelayDiscoveryConfig() RelayDiscoveryConfig {
	return RelayDiscoveryConfig{
		DiscoveryInterval: DefaultDiscoveryInterval,
		AdvertiseInterval: DefaultAdvertiseInterval,
		DiscoveryTimeout:  DefaultDiscoveryTimeout,
		MaxRelays:         DefaultMaxRelays,
	}
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewRelayDiscovery 创建中继发现服务
func NewRelayDiscovery(dht pkgif.Discovery, host pkgif.Host, peerstore pkgif.Peerstore, config RelayDiscoveryConfig) *RelayDiscovery {
	ctx, cancel := context.WithCancel(context.Background())

	// 应用默认值
	if config.DiscoveryInterval <= 0 {
		config.DiscoveryInterval = DefaultDiscoveryInterval
	}
	if config.AdvertiseInterval <= 0 {
		config.AdvertiseInterval = DefaultAdvertiseInterval
	}
	if config.DiscoveryTimeout <= 0 {
		config.DiscoveryTimeout = DefaultDiscoveryTimeout
	}
	if config.MaxRelays <= 0 {
		config.MaxRelays = DefaultMaxRelays
	}

	rd := &RelayDiscovery{
		dht:               dht,
		host:              host,
		peerstore:         peerstore,
		discoveryInterval: config.DiscoveryInterval,
		advertiseInterval: config.AdvertiseInterval,
		discoveryTimeout:  config.DiscoveryTimeout,
		maxRelays:         config.MaxRelays,
		staticRelayAddrs:  config.StaticRelayAddrs, // v2.0.1: 静态 Relay 回退
		relays:            make(map[types.PeerID]*DiscoveredRelay),
		latencyCache:      make(map[types.PeerID]time.Duration), // Phase 9 修复：初始化延迟缓存
		ctx:               ctx,
		cancel:            cancel,
	}

	discoveryLogger.Info("中继发现服务已创建",
		"discoveryInterval", config.DiscoveryInterval,
		"maxRelays", config.MaxRelays,
		"staticRelayCount", len(config.StaticRelayAddrs))

	return rd
}

// ============================================================================
//                              发现功能
// ============================================================================

// Discover 立即发现中继
//
// 从所有可用的发现源查找中继服务器。
//
// 返回：
//   - []DiscoveredRelay: 发现的中继列表
//   - error: 错误
func (rd *RelayDiscovery) Discover(ctx context.Context) ([]DiscoveredRelay, error) {
	if atomic.LoadInt32(&rd.closed) == 1 {
		return nil, ErrDiscoveryClosed
	}

	discoveryLogger.Debug("开始发现中继")

	// 并行从多个源发现
	var wg sync.WaitGroup
	results := make(chan *DiscoveredRelay, rd.maxRelays*2)

	// DHT 发现（v2.0: 统一通过 DHT 或显式配置发现）
	if rd.dht != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rd.discoverFromDHT(ctx, results)
		}()
	}

	// v2.0.1: 静态 Relay 回退（与 DHT 并行加载）
	if len(rd.staticRelayAddrs) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rd.loadStaticRelays(ctx, results)
		}()
	}

	// 等待所有发现完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 收集结果
	found := make(map[types.PeerID]*DiscoveredRelay)
	for info := range results {
		if info != nil {
			// 如果已存在，合并信息
			if existing, ok := found[info.PeerID]; ok {
				// 保留延迟较低的
				if info.Latency < existing.Latency {
					found[info.PeerID] = info
				}
			} else {
				found[info.PeerID] = info
			}
		}
	}

	// 更新缓存
	rd.relaysMu.Lock()
	for id, info := range found {
		rd.relays[id] = info
	}
	rd.relaysMu.Unlock()

	// 转换为列表并排序
	relays := make([]DiscoveredRelay, 0, len(found))
	for _, info := range found {
		relays = append(relays, *info)
	}

	// 按评分排序
	rd.sortRelays(relays)

	// 限制数量
	if len(relays) > rd.maxRelays {
		relays = relays[:rd.maxRelays]
	}

	discoveryLogger.Info("中继发现完成", "count", len(relays))

	return relays, nil
}

// discoverFromDHT 从 DHT 发现中继
//
// DHT FindPeers 返回所有查询该命名空间的节点，需要通过协议检查过滤非 Relay 节点
func (rd *RelayDiscovery) discoverFromDHT(ctx context.Context, results chan<- *DiscoveredRelay) {
	discoverCtx, cancel := context.WithTimeout(ctx, rd.discoveryTimeout)
	defer cancel()

	peers, err := rd.dht.FindPeers(discoverCtx, RelayNamespaceLocal)
	if err != nil {
		discoveryLogger.Debug("DHT 发现失败", "error", err)
		return
	}

	for peer := range peers {
		start := time.Now()

		// 检查 Peerstore 中的协议信息，过滤不支持 HOP 协议的节点
		if rd.peerstore != nil {
			supportedProtos, err := rd.peerstore.SupportsProtocols(peer.ID, types.ProtocolID(HopProtocolIDForDiscovery))
			if err == nil && len(supportedProtos) == 0 {
				// 检查是否有该节点的协议信息
				allProtos, _ := rd.peerstore.GetProtocols(peer.ID)
				if len(allProtos) > 0 {
					// 有协议信息但不支持 HOP 协议，说明不是 Relay 服务器
					discoveryLogger.Debug("过滤非 Relay 节点（不支持 HOP 协议）",
						"peer", safePeerIDPrefix(peer.ID))
					continue
				}
				// 没有协议信息，可能是新节点，先加入候选
			}
		}

		// 计算延迟（通过 ping 或估算）
		latency := rd.estimateLatency(peer.ID)

		// 转换地址为字符串
		addrs := make([]string, 0, len(peer.Addrs))
		for _, addr := range peer.Addrs {
			addrs = append(addrs, addr.String())
		}

		info := &DiscoveredRelay{
			PeerID:   peer.ID,
			Addrs:    addrs,
			Latency:  latency,
			LastSeen: time.Now(),
			Source:   "dht",
		}

		// 计算评分
		info.Score = rd.calculateScore(info)

		select {
		case results <- info:
		case <-ctx.Done():
			return
		}

		discoveryLogger.Debug("发现中继（DHT）",
			"peer", safePeerIDPrefix(peer.ID),
			"latency", time.Since(start))
	}
}

// loadStaticRelays 加载静态配置的 Relay
//
// v2.0.1: 从静态配置加载 Relay 地址，作为 DHT 发现失败时的回退
// 静态配置的 Relay 评分较低（50），优先使用 DHT 发现的 Relay
func (rd *RelayDiscovery) loadStaticRelays(ctx context.Context, results chan<- *DiscoveredRelay) {
	for _, addr := range rd.staticRelayAddrs {
		// 解析 multiaddr 获取 PeerID
		peerID, addrs := extractPeerIDFromMultiaddr(addr)
		if peerID == "" {
			discoveryLogger.Debug("静态 Relay 地址解析失败", "addr", addr)
			continue
		}

		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			return
		default:
		}

		info := &DiscoveredRelay{
			PeerID:   peerID,
			Addrs:    addrs,
			Latency:  100 * time.Millisecond, // 静态配置使用默认延迟
			LastSeen: time.Now(),
			Source:   "static",
			Score:    50, // 静态配置评分较低，优先使用 DHT 发现的
		}

		select {
		case results <- info:
			discoveryLogger.Debug("加载静态 Relay",
				"peer", safePeerIDPrefix(peerID),
				"addr", addr)
		case <-ctx.Done():
			return
		}
	}
}

// extractPeerIDFromMultiaddr 从 multiaddr 中提取 PeerID 和地址
//
// v2.0.1: 使用 types.AddrInfoFromString 进行完整的 multiaddr 解析
// 支持所有标准 multiaddr 格式，包括复杂的 /p2p-circuit/ 地址
//
// 输入: "/ip4/1.2.3.4/tcp/4001/p2p/QmXXX"
// 输出: ("QmXXX", ["/ip4/1.2.3.4/tcp/4001/p2p/QmXXX"])
func extractPeerIDFromMultiaddr(addr string) (types.PeerID, []string) {
	// 使用 types 包的 AddrInfoFromString 进行完整解析
	addrInfo, err := types.AddrInfoFromString(addr)
	if err != nil {
		discoveryLogger.Debug("multiaddr 解析失败",
			"addr", addr,
			"error", err)
		return "", nil
	}

	// 返回解析出的 PeerID 和原始地址
	return addrInfo.ID, []string{addr}
}

// discoverFromRealm 从 Realm 发现中继
//
// 注意：当前 Realm 接口不支持 FindPeers，此功能预留。
// ============================================================================
//                              发布功能
// ============================================================================

// Advertise 发布本节点为中继服务器
//
// 向 DHT 和 Realm 发布中继服务能力。
//
// 参数：
//   - ctx: 上下文
//
// 返回：
//   - error: 错误
func (rd *RelayDiscovery) Advertise(ctx context.Context) error {
	if atomic.LoadInt32(&rd.closed) == 1 {
		return ErrDiscoveryClosed
	}

	if !rd.isRelayServer {
		return errors.New("not configured as relay server")
	}

	discoveryLogger.Debug("发布中继服务")

	var errs []error

	// 发布到 DHT（v2.0: 统一通过 DHT 发布）
	if rd.dht != nil {
		_, err := rd.dht.Advertise(ctx, RelayNamespaceLocal)
		if err != nil {
			errs = append(errs, err)
			discoveryLogger.Warn("DHT 发布失败", "error", err)
		}
	}

	if len(errs) > 0 && rd.dht == nil {
		return ErrNoDiscoverySource
	}

	discoveryLogger.Info("中继服务已发布")
	return nil
}

// EnableRelayServer 启用中继服务器模式
func (rd *RelayDiscovery) EnableRelayServer(config RelayServerConfig) {
	rd.isRelayServer = true
	rd.relayServerConfig = config
	discoveryLogger.Info("已启用中继服务器模式",
		"maxConnections", config.MaxConnections)
}

// DisableRelayServer 禁用中继服务器模式
func (rd *RelayDiscovery) DisableRelayServer() {
	rd.isRelayServer = false
	discoveryLogger.Info("已禁用中继服务器模式")
}

// ============================================================================
//                              缓存管理
// ============================================================================

// GetRelays 获取缓存的中继列表
func (rd *RelayDiscovery) GetRelays() []DiscoveredRelay {
	rd.relaysMu.RLock()
	defer rd.relaysMu.RUnlock()

	relays := make([]DiscoveredRelay, 0, len(rd.relays))
	for _, info := range rd.relays {
		relays = append(relays, *info)
	}

	rd.sortRelays(relays)
	return relays
}

// GetRelay 获取指定中继信息
func (rd *RelayDiscovery) GetRelay(peerID types.PeerID) (*DiscoveredRelay, bool) {
	rd.relaysMu.RLock()
	defer rd.relaysMu.RUnlock()

	info, ok := rd.relays[peerID]
	if !ok {
		return nil, false
	}
	return info, true
}

// AddRelay 手动添加中继
func (rd *RelayDiscovery) AddRelay(info DiscoveredRelay) {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	info.LastSeen = time.Now()
	if info.Score == 0 {
		info.Score = rd.calculateScore(&info)
	}
	rd.relays[info.PeerID] = &info
}

// RemoveRelay 移除中继
func (rd *RelayDiscovery) RemoveRelay(peerID types.PeerID) {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	delete(rd.relays, peerID)
}

// ClearRelays 清空缓存
func (rd *RelayDiscovery) ClearRelays() {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	rd.relays = make(map[types.PeerID]*DiscoveredRelay)
}

// ============================================================================
//                              评分和排序
// ============================================================================

// calculateScore 计算中继评分
//
// 评分因素：
//   - 延迟（越低越好）
//   - 负载（越低越好）
//   - 来源（Realm > DHT）
func (rd *RelayDiscovery) calculateScore(info *DiscoveredRelay) float64 {
	var score float64 = 100

	// 延迟惩罚（每 100ms 扣 10 分）
	latencyMs := float64(info.Latency.Milliseconds())
	score -= latencyMs / 10

	// 负载惩罚
	score -= float64(info.Load)

	// 来源加分
	if info.Source == "realm" {
		score += 20
	}

	// 确保评分不为负
	if score < 0 {
		score = 0
	}

	return score
}

// sortRelays 按评分排序中继
func (rd *RelayDiscovery) sortRelays(relays []DiscoveredRelay) {
	sort.Slice(relays, func(i, j int) bool {
		return relays[i].Score > relays[j].Score
	})
}

// SetLiveness 设置 Liveness 服务（用于精确延迟测量）
//
// Phase 9 修复：添加可选的 Liveness 依赖，支持真实延迟测量
func (rd *RelayDiscovery) SetLiveness(liveness pkgif.Liveness) {
	rd.liveness = liveness
	discoveryLogger.Debug("已设置 Liveness 服务，启用精确延迟测量")
}

// estimateLatency 估算延迟
//
// Phase 9 修复：实现真实延迟测量
// 优先级：1. 缓存中的值 2. Liveness Ping 3. 默认值
func (rd *RelayDiscovery) estimateLatency(peerID types.PeerID) time.Duration {
	// 1. 先检查缓存
	rd.latencyCacheMu.RLock()
	if latency, ok := rd.latencyCache[peerID]; ok {
		rd.latencyCacheMu.RUnlock()
		return latency
	}
	rd.latencyCacheMu.RUnlock()

	// 2. 如果有 Liveness 服务，尝试 Ping
	if rd.liveness != nil {
		ctx, cancel := context.WithTimeout(rd.ctx, 5*time.Second)
		defer cancel()

		latency, err := rd.liveness.Ping(ctx, string(peerID))
		if err == nil {
			// 缓存结果
			rd.latencyCacheMu.Lock()
			rd.latencyCache[peerID] = latency
			rd.latencyCacheMu.Unlock()

			discoveryLogger.Debug("延迟测量成功",
				"peerID", peerID,
				"latency", latency,
			)
			return latency
		}
		discoveryLogger.Debug("延迟测量失败，使用默认值",
			"peerID", peerID,
			"error", err,
		)
	}

	// 3. 返回默认值
	return 100 * time.Millisecond
}

// MeasureLatencyAsync 异步测量延迟并更新缓存
//
// Phase 9 修复：支持批量异步延迟测量
func (rd *RelayDiscovery) MeasureLatencyAsync(peerIDs []types.PeerID) {
	if rd.liveness == nil {
		return
	}

	for _, peerID := range peerIDs {
		go func(pid types.PeerID) {
			ctx, cancel := context.WithTimeout(rd.ctx, 5*time.Second)
			defer cancel()

			latency, err := rd.liveness.Ping(ctx, string(pid))
			if err == nil {
				rd.latencyCacheMu.Lock()
				rd.latencyCache[pid] = latency
				rd.latencyCacheMu.Unlock()
			}
		}(peerID)
	}
}

// ClearLatencyCache 清除延迟缓存
func (rd *RelayDiscovery) ClearLatencyCache() {
	rd.latencyCacheMu.Lock()
	rd.latencyCache = make(map[types.PeerID]time.Duration)
	rd.latencyCacheMu.Unlock()
}

// ============================================================================
//                              后台任务
// ============================================================================

// Start 启动发现服务
func (rd *RelayDiscovery) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&rd.running, 0, 1) {
		return nil
	}

	// 延迟首次发现，等待 Coordinator 等依赖组件完成启动
	go func() {
		select {
		case <-rd.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		_, _ = rd.Discover(rd.ctx)
	}()

	// 启动定期发现
	rd.wg.Add(1)
	go rd.discoveryLoop()

	// 如果是中继服务器，启动定期发布
	if rd.isRelayServer {
		rd.wg.Add(1)
		go rd.advertiseLoop()
	}

	discoveryLogger.Info("中继发现服务已启动")
	return nil
}

// discoveryLoop 发现循环
func (rd *RelayDiscovery) discoveryLoop() {
	defer rd.wg.Done()

	ticker := time.NewTicker(rd.discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rd.ctx.Done():
			return
		case <-ticker.C:
			_, _ = rd.Discover(rd.ctx)
			rd.cleanupExpired()
		}
	}
}

// advertiseLoop 发布循环
func (rd *RelayDiscovery) advertiseLoop() {
	defer rd.wg.Done()

	ticker := time.NewTicker(rd.advertiseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rd.ctx.Done():
			return
		case <-ticker.C:
			_ = rd.Advertise(rd.ctx)
		}
	}
}

// cleanupExpired 清理过期中继
func (rd *RelayDiscovery) cleanupExpired() {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	now := time.Now()
	for id, info := range rd.relays {
		if now.Sub(info.LastSeen) > RelayTTL {
			delete(rd.relays, id)
			discoveryLogger.Debug("移除过期中继", "peer", safePeerIDPrefix(id))
		}
	}
}

// Close 关闭发现服务
func (rd *RelayDiscovery) Close() error {
	if !atomic.CompareAndSwapInt32(&rd.closed, 0, 1) {
		return nil
	}

	rd.cancel()
	rd.wg.Wait()

	discoveryLogger.Info("中继发现服务已关闭")
	return nil
}

// ============================================================================
//                              统计信息
// ============================================================================

// RelayDiscoveryStats 发现服务统计
type RelayDiscoveryStats struct {
	// CachedRelays 缓存的中继数
	CachedRelays int

	// IsRelayServer 是否为中继服务器
	IsRelayServer bool

	// LastDiscovery 上次发现时间
	LastDiscovery time.Time
}

// Stats 返回统计信息
func (rd *RelayDiscovery) Stats() RelayDiscoveryStats {
	rd.relaysMu.RLock()
	count := len(rd.relays)
	rd.relaysMu.RUnlock()

	return RelayDiscoveryStats{
		CachedRelays:  count,
		IsRelayServer: rd.isRelayServer,
	}
}
