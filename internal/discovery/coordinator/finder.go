// Package coordinator 提供发现协调功能
//
// PeerFinder 实现多级缓存的节点查找：
//   - 优先级 1: 内存缓存 (knownPeers)
//   - 优先级 2: Peerstore 持久缓存
//   - 优先级 3: 已连接节点
//   - 优先级 4: 网络发现 (DHT/mDNS)
package coordinator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var finderLogger = log.Logger("discovery/finder")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrPeerNotFound 未找到节点
	ErrPeerNotFound = errors.New("peer not found")

	// ErrFinderClosed 查找器已关闭
	ErrFinderClosed = errors.New("finder closed")

	// ErrNoDiscoverySource 无可用发现源
	ErrNoDiscoverySource = errors.New("no discovery source available")
)

// ============================================================================
//                              缓存条目
// ============================================================================

// PeerCacheEntry 节点缓存条目
type PeerCacheEntry struct {
	// PeerID 节点 ID
	PeerID types.PeerID

	// Addrs 节点地址列表
	Addrs []string

	// LastSeen 最后发现时间
	LastSeen time.Time

	// Source 发现来源
	Source string

	// TTL 缓存有效期
	TTL time.Duration
}

// IsExpired 检查缓存是否过期
func (e *PeerCacheEntry) IsExpired() bool {
	if e.TTL <= 0 {
		return false
	}
	return time.Since(e.LastSeen) > e.TTL
}

// ============================================================================
//                              配置
// ============================================================================

// PeerFinderConfig 节点查找器配置
type PeerFinderConfig struct {
	// CacheTTL 缓存有效期
	CacheTTL time.Duration

	// MaxCacheSize 最大缓存大小
	MaxCacheSize int

	// NetworkTimeout 网络查询超时
	NetworkTimeout time.Duration

	// EnableLocalPriority 启用本地优先
	EnableLocalPriority bool

	// CacheCleanupInterval 缓存清理间隔
	CacheCleanupInterval time.Duration
}

// DefaultPeerFinderConfig 返回默认配置
func DefaultPeerFinderConfig() PeerFinderConfig {
	return PeerFinderConfig{
		CacheTTL:             10 * time.Minute,
		MaxCacheSize:         1000,
		NetworkTimeout:       30 * time.Second,
		EnableLocalPriority:  true,
		CacheCleanupInterval: 5 * time.Minute,
	}
}

// ============================================================================
//                              PeerFinder 结构
// ============================================================================

// PeerFinder 节点查找器
//
// 实现多级缓存的节点查找，按优先级顺序尝试：
//   1. 内存缓存（最快）
//   2. Peerstore 持久缓存
//   3. 已连接节点
//   4. 网络发现（DHT/mDNS）
type PeerFinder struct {
	// 配置
	config PeerFinderConfig

	// 依赖组件
	peerstore pkgif.Peerstore   // 节点存储
	host      pkgif.Host        // 本地主机
	swarm     pkgif.Swarm       // 网络层

	// 发现源
	discoveries   map[string]pkgif.Discovery
	discoveriesMu sync.RWMutex

	// 内存缓存
	cache      map[types.PeerID]*PeerCacheEntry
	cacheOrder []types.PeerID // LRU 顺序
	cacheMu    sync.RWMutex

	// 状态
	// FIX #B40: 改用 atomic.Int32 避免 Race 条件
	// 原先的 bool 类型在并发读写时会导致数据竞争
	closed  int32  // 0=open, 1=closed
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewPeerFinder 创建节点查找器
func NewPeerFinder(config PeerFinderConfig) *PeerFinder {
	if config.CacheTTL <= 0 {
		config.CacheTTL = DefaultPeerFinderConfig().CacheTTL
	}
	if config.MaxCacheSize <= 0 {
		config.MaxCacheSize = DefaultPeerFinderConfig().MaxCacheSize
	}
	if config.NetworkTimeout <= 0 {
		config.NetworkTimeout = DefaultPeerFinderConfig().NetworkTimeout
	}
	if config.CacheCleanupInterval <= 0 {
		config.CacheCleanupInterval = DefaultPeerFinderConfig().CacheCleanupInterval
	}

	pf := &PeerFinder{
		config:      config,
		discoveries: make(map[string]pkgif.Discovery),
		cache:       make(map[types.PeerID]*PeerCacheEntry),
		cacheOrder:  make([]types.PeerID, 0),
		closeCh:     make(chan struct{}),
	}

	finderLogger.Info("节点查找器已创建",
		"cacheTTL", config.CacheTTL,
		"maxCacheSize", config.MaxCacheSize)

	return pf
}

// SetPeerstore 设置 Peerstore
func (pf *PeerFinder) SetPeerstore(ps pkgif.Peerstore) {
	pf.peerstore = ps
}

// SetHost 设置 Host
func (pf *PeerFinder) SetHost(h pkgif.Host) {
	pf.host = h
}

// SetSwarm 设置 Swarm
func (pf *PeerFinder) SetSwarm(s pkgif.Swarm) {
	pf.swarm = s
}

// ============================================================================
//                              发现源管理
// ============================================================================

// RegisterDiscovery 注册发现源
func (pf *PeerFinder) RegisterDiscovery(name string, d pkgif.Discovery) {
	if d == nil {
		return
	}

	pf.discoveriesMu.Lock()
	defer pf.discoveriesMu.Unlock()

	pf.discoveries[name] = d
	finderLogger.Debug("注册发现源", "name", name)
}

// UnregisterDiscovery 注销发现源
func (pf *PeerFinder) UnregisterDiscovery(name string) {
	pf.discoveriesMu.Lock()
	defer pf.discoveriesMu.Unlock()

	delete(pf.discoveries, name)
}

// ============================================================================
//                              核心查找功能
// ============================================================================

// FindPeer 查找节点
//
// 按优先级顺序尝试多个来源：
//   1. 内存缓存
//   2. Peerstore
//   3. 已连接节点
//   4. 网络发现
//
// 参数：
//   - ctx: 上下文
//   - peerID: 目标节点 ID
//
// 返回：
//   - []string: 节点地址列表
//   - error: 错误
func (pf *PeerFinder) FindPeer(ctx context.Context, peerID types.PeerID) ([]string, error) {
	// FIX #B40: 使用 atomic.LoadInt32 避免 Race
	if atomic.LoadInt32(&pf.closed) == 1 {
		return nil, ErrFinderClosed
	}

	finderLogger.Debug("开始查找节点", "peer", peerID)

	// 优先级 1: 内存缓存
	if pf.config.EnableLocalPriority {
		if addrs := pf.findInCache(peerID); len(addrs) > 0 {
			finderLogger.Debug("从缓存找到节点", "peer", peerID, "addrs", len(addrs))
			return addrs, nil
		}
	}

	// 优先级 2: Peerstore
	if pf.peerstore != nil {
		if addrs := pf.findInPeerstore(peerID); len(addrs) > 0 {
			finderLogger.Debug("从 Peerstore 找到节点", "peer", peerID, "addrs", len(addrs))
			// 缓存结果
			pf.cacheAddrs(peerID, addrs, "peerstore")
			return addrs, nil
		}
	}

	// 优先级 3: 已连接节点
	if pf.swarm != nil {
		if addrs := pf.findInConnections(peerID); len(addrs) > 0 {
			finderLogger.Debug("从已连接节点找到", "peer", peerID, "addrs", len(addrs))
			pf.cacheAddrs(peerID, addrs, "connection")
			return addrs, nil
		}
	}

	// 优先级 4: 网络发现
	addrs, err := pf.findFromNetwork(ctx, peerID)
	if err != nil {
		finderLogger.Debug("网络查找失败", "peer", peerID, "error", err)
		return nil, err
	}

	if len(addrs) > 0 {
		finderLogger.Debug("从网络找到节点", "peer", peerID, "addrs", len(addrs))
		pf.cacheAddrs(peerID, addrs, "network")
		return addrs, nil
	}

	return nil, ErrPeerNotFound
}

// FindPeerInfo 查找节点完整信息
func (pf *PeerFinder) FindPeerInfo(ctx context.Context, peerID types.PeerID) (types.PeerInfo, error) {
	addrs, err := pf.FindPeer(ctx, peerID)
	if err != nil {
		return types.PeerInfo{}, err
	}

	// 转换字符串地址为 Multiaddr
	multiaddrs := make([]types.Multiaddr, 0, len(addrs))
	for _, addrStr := range addrs {
		if ma, err := types.NewMultiaddr(addrStr); err == nil {
			multiaddrs = append(multiaddrs, ma)
		}
	}

	return types.PeerInfo{
		ID:    peerID,
		Addrs: multiaddrs,
	}, nil
}

// ============================================================================
//                              查找方法实现
// ============================================================================

// findInCache 从缓存查找
func (pf *PeerFinder) findInCache(peerID types.PeerID) []string {
	pf.cacheMu.RLock()
	defer pf.cacheMu.RUnlock()

	entry, ok := pf.cache[peerID]
	if !ok {
		return nil
	}

	// 检查是否过期
	if entry.IsExpired() {
		return nil
	}

	return entry.Addrs
}

// findInPeerstore 从 Peerstore 查找
func (pf *PeerFinder) findInPeerstore(peerID types.PeerID) []string {
	if pf.peerstore == nil {
		return nil
	}

	multiaddrs := pf.peerstore.Addrs(peerID)
	if len(multiaddrs) == 0 {
		return nil
	}

	// 转换为字符串
	addrs := make([]string, 0, len(multiaddrs))
	for _, ma := range multiaddrs {
		addrs = append(addrs, ma.String())
	}

	return addrs
}

// findInConnections 从已连接节点查找
func (pf *PeerFinder) findInConnections(peerID types.PeerID) []string {
	if pf.swarm == nil {
		return nil
	}

	conns := pf.swarm.ConnsToPeer(string(peerID))
	if len(conns) == 0 {
		return nil
	}

	// 从连接获取远程地址
	addrs := make([]string, 0, len(conns))
	for _, conn := range conns {
		if remoteAddr := conn.RemoteMultiaddr(); remoteAddr != nil {
			addrs = append(addrs, remoteAddr.String())
		}
	}

	return addrs
}

// findFromNetwork 从网络发现
func (pf *PeerFinder) findFromNetwork(ctx context.Context, peerID types.PeerID) ([]string, error) {
	pf.discoveriesMu.RLock()
	discoveries := make(map[string]pkgif.Discovery)
	for k, v := range pf.discoveries {
		discoveries[k] = v
	}
	pf.discoveriesMu.RUnlock()

	if len(discoveries) == 0 {
		return nil, ErrNoDiscoverySource
	}

	// 设置超时
	queryCtx, cancel := context.WithTimeout(ctx, pf.config.NetworkTimeout)
	defer cancel()

	// 并行查询所有发现源
	var wg sync.WaitGroup
	resultCh := make(chan []string, len(discoveries))

	for name, d := range discoveries {
		wg.Add(1)
		go func(_ string, disc pkgif.Discovery) {
			defer wg.Done()

			// 尝试使用 DHTDiscovery.FindPeer（如果支持）
			if dhtDisc, ok := disc.(pkgif.DHTDiscovery); ok {
				info, err := dhtDisc.FindPeer(queryCtx, peerID)
				if err == nil && len(info.Addrs) > 0 {
					addrs := make([]string, 0, len(info.Addrs))
					for _, ma := range info.Addrs {
						addrs = append(addrs, ma.String())
					}
					resultCh <- addrs
					return
				}
			}

			// 回退到 FindPeers（使用 PeerID 作为命名空间）
			peerCh, err := disc.FindPeers(queryCtx, string(peerID))
			if err != nil {
				return
			}

			for info := range peerCh {
				if info.ID == peerID && len(info.Addrs) > 0 {
					addrs := make([]string, 0, len(info.Addrs))
					for _, ma := range info.Addrs {
						addrs = append(addrs, ma.String())
					}
					resultCh <- addrs
					return
				}
			}
		}(name, d)
	}

	// 等待所有查询完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	var allAddrs []string
	seen := make(map[string]bool)

	for addrs := range resultCh {
		for _, addr := range addrs {
			if !seen[addr] {
				seen[addr] = true
				allAddrs = append(allAddrs, addr)
			}
		}
	}

	return allAddrs, nil
}

// ============================================================================
//                              缓存管理
// ============================================================================

// cacheAddrs 缓存地址
func (pf *PeerFinder) cacheAddrs(peerID types.PeerID, addrs []string, source string) {
	pf.cacheMu.Lock()
	defer pf.cacheMu.Unlock()

	// 检查缓存大小
	if len(pf.cache) >= pf.config.MaxCacheSize {
		pf.evictOldest()
	}

	// 更新或添加缓存条目
	entry := &PeerCacheEntry{
		PeerID:   peerID,
		Addrs:    addrs,
		LastSeen: time.Now(),
		Source:   source,
		TTL:      pf.config.CacheTTL,
	}

	if _, exists := pf.cache[peerID]; !exists {
		pf.cacheOrder = append(pf.cacheOrder, peerID)
	}

	pf.cache[peerID] = entry
}

// evictOldest 驱逐最旧的缓存条目
func (pf *PeerFinder) evictOldest() {
	if len(pf.cacheOrder) == 0 {
		return
	}

	// 移除最旧的条目
	oldest := pf.cacheOrder[0]
	pf.cacheOrder = pf.cacheOrder[1:]
	delete(pf.cache, oldest)

	finderLogger.Debug("驱逐缓存条目", "peer", oldest)
}

// GetCachedPeer 获取缓存的节点信息
func (pf *PeerFinder) GetCachedPeer(peerID types.PeerID) (*PeerCacheEntry, bool) {
	pf.cacheMu.RLock()
	defer pf.cacheMu.RUnlock()

	entry, ok := pf.cache[peerID]
	if !ok || entry.IsExpired() {
		return nil, false
	}

	return entry, true
}

// ClearCache 清空缓存
func (pf *PeerFinder) ClearCache() {
	pf.cacheMu.Lock()
	defer pf.cacheMu.Unlock()

	pf.cache = make(map[types.PeerID]*PeerCacheEntry)
	pf.cacheOrder = make([]types.PeerID, 0)

	finderLogger.Debug("缓存已清空")
}

// CacheSize 返回缓存大小
func (pf *PeerFinder) CacheSize() int {
	pf.cacheMu.RLock()
	defer pf.cacheMu.RUnlock()

	return len(pf.cache)
}

// ============================================================================
//                              后台任务
// ============================================================================

// Start 启动查找器
func (pf *PeerFinder) Start(_ context.Context) error {
	pf.wg.Add(1)
	go pf.cleanupLoop()

	finderLogger.Info("节点查找器已启动")
	return nil
}

// cleanupLoop 缓存清理循环
func (pf *PeerFinder) cleanupLoop() {
	defer pf.wg.Done()

	ticker := time.NewTicker(pf.config.CacheCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pf.closeCh:
			return
		case <-ticker.C:
			pf.cleanupExpired()
		}
	}
}

// cleanupExpired 清理过期缓存
func (pf *PeerFinder) cleanupExpired() {
	pf.cacheMu.Lock()
	defer pf.cacheMu.Unlock()

	now := time.Now()
	var newOrder []types.PeerID
	cleaned := 0

	for _, peerID := range pf.cacheOrder {
		entry, ok := pf.cache[peerID]
		if !ok {
			continue
		}

		if entry.TTL > 0 && now.Sub(entry.LastSeen) > entry.TTL {
			delete(pf.cache, peerID)
			cleaned++
		} else {
			newOrder = append(newOrder, peerID)
		}
	}

	pf.cacheOrder = newOrder

	if cleaned > 0 {
		finderLogger.Debug("清理过期缓存", "count", cleaned)
	}
}

// Close 关闭查找器
func (pf *PeerFinder) Close() error {
	// FIX #B40: 使用 atomic.CompareAndSwapInt32 避免 Race 和重复关闭
	if !atomic.CompareAndSwapInt32(&pf.closed, 0, 1) {
		return nil  // 已经关闭
	}

	close(pf.closeCh)
	pf.wg.Wait()

	finderLogger.Info("节点查找器已关闭")
	return nil
}

// ============================================================================
//                              统计信息
// ============================================================================

// PeerFinderStats 查找器统计
type PeerFinderStats struct {
	// CacheSize 缓存大小
	CacheSize int

	// DiscoverySourceCount 发现源数量
	DiscoverySourceCount int

	// HasPeerstore 是否有 Peerstore
	HasPeerstore bool

	// HasSwarm 是否有 Swarm
	HasSwarm bool
}

// Stats 返回统计信息
func (pf *PeerFinder) Stats() PeerFinderStats {
	pf.cacheMu.RLock()
	cacheSize := len(pf.cache)
	pf.cacheMu.RUnlock()

	pf.discoveriesMu.RLock()
	discCount := len(pf.discoveries)
	pf.discoveriesMu.RUnlock()

	return PeerFinderStats{
		CacheSize:            cacheSize,
		DiscoverySourceCount: discCount,
		HasPeerstore:         pf.peerstore != nil,
		HasSwarm:             pf.swarm != nil,
	}
}
