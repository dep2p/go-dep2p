package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("discovery/coordinator")

// ============================================================================
//                              Coordinator 结构体
// ============================================================================

// Coordinator 发现协调器
type Coordinator struct {
	// 配置
	config *Config

	// 发现器管理
	mu          sync.RWMutex
	discoveries map[string]pkgif.Discovery

	// 状态管理
	started atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc

	// 缓存管理
	cacheMu    sync.RWMutex
	peerCache  map[types.PeerID]*peerCacheEntry
	cacheOrder []types.PeerID // LRU 缓存顺序
}

// peerCacheEntry 节点缓存条目
type peerCacheEntry struct {
	peer      types.PeerInfo
	timestamp time.Time
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewCoordinator 创建协调器
func NewCoordinator(config *Config) *Coordinator {
	if config == nil {
		config = DefaultConfig()
	}

	return &Coordinator{
		config:      config,
		discoveries: make(map[string]pkgif.Discovery),
		peerCache:   make(map[types.PeerID]*peerCacheEntry),
		cacheOrder:  make([]types.PeerID, 0),
	}
}

// ============================================================================
//                              发现器管理
// ============================================================================

// RegisterDiscovery 注册发现器
func (c *Coordinator) RegisterDiscovery(name string, discovery pkgif.Discovery) {
	if discovery == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.discoveries[name] = discovery
	logger.Debug("注册发现器", "name", name)
}

// UnregisterDiscovery 注销发现器
func (c *Coordinator) UnregisterDiscovery(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.discoveries, name)
}

// GetDiscovery 获取发现器
func (c *Coordinator) GetDiscovery(name string) pkgif.Discovery {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.discoveries[name]
}

// ListDiscoveries 列出所有发现器名称
func (c *Coordinator) ListDiscoveries() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.discoveries))
	for name := range c.discoveries {
		names = append(names, name)
	}
	return names
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动协调器
func (c *Coordinator) Start(_ context.Context) error {
	if c.started.Load() {
		return nil
	}

	logger.Info("正在启动 Discovery Coordinator")

	// 创建内部上下文
	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.started.Store(true)

	// 如果启用缓存，启动清理协程
	if c.config.EnableCache {
		go c.cacheCleanupLoop()
		logger.Debug("缓存清理协程已启动")
	}

	logger.Info("Discovery Coordinator 启动成功")

	// 注意：不在这里启动各个发现器
	// 各发现器有自己的 Fx Lifecycle 管理生命周期
	// Coordinator 只是一个聚合器，不负责子发现器的启动/停止

	return nil
}

// Stop 停止协调器
func (c *Coordinator) Stop(_ context.Context) error {
	if !c.started.Load() {
		return nil
	}

	logger.Info("正在停止 Discovery Coordinator")

	// 注意：不在这里停止各个发现器
	// 各发现器有自己的 Fx Lifecycle 管理生命周期
	// Coordinator 只是一个聚合器，不负责子发现器的启动/停止

	// 取消内部上下文
	if c.cancel != nil {
		c.cancel()
	}

	logger.Info("Discovery Coordinator 已停止")

	c.started.Store(false)

	// 清空缓存
	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.peerCache = make(map[types.PeerID]*peerCacheEntry)
		c.cacheOrder = make([]types.PeerID, 0)
		c.cacheMu.Unlock()
	}

	return nil
}

// ============================================================================
//                              Discovery 接口实现
// ============================================================================

// FindPeers 发现节点（实现 Discovery 接口）
func (c *Coordinator) FindPeers(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if !c.started.Load() {
		return nil, ErrNotStarted
	}

	ns = pkgif.NormalizeNamespace(ns)

	// 解析选项
	options := &pkgif.DiscoveryOptions{Limit: 100}
	for _, opt := range opts {
		opt(options)
	}

	// 使用原始上下文，不创建超时（超时由调用者控制）
	findCtx := ctx

	// 获取所有发现器的副本
	c.mu.RLock()
	discoveries := make([]pkgif.Discovery, 0, len(c.discoveries))
	for _, discovery := range c.discoveries {
		discoveries = append(discoveries, discovery)
	}
	c.mu.RUnlock()

	if len(discoveries) == 0 {
		return nil, ErrNoDiscoveries
	}

	// 创建输出通道
	out := make(chan types.PeerInfo, options.Limit)

	// 启动并行发现
	go c.parallelFindPeers(findCtx, ns, discoveries, out, options)

	return out, nil
}

// parallelFindPeers 并行发现节点
func (c *Coordinator) parallelFindPeers(
	ctx context.Context,
	ns string,
	discoveries []pkgif.Discovery,
	out chan<- types.PeerInfo,
	options *pkgif.DiscoveryOptions,
) {
	defer close(out)

	// 用于收集所有发现器的结果
	resultCh := make(chan types.PeerInfo, 100)
	var wg sync.WaitGroup

	// 启动所有发现器
	for _, discovery := range discoveries {
		wg.Add(1)
		go func(d pkgif.Discovery) {
			defer wg.Done()

			ch, err := d.FindPeers(ctx, ns, pkgif.WithLimit(options.Limit))
			if err != nil {
				return
			}

			for peer := range ch {
				select {
				case resultCh <- peer:
				case <-ctx.Done():
					return
				}
			}
		}(discovery)
	}

	// 等待所有发现器完成后关闭结果通道
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 去重并输出
	c.dedupAndOutput(ctx, resultCh, out, options.Limit)
}

// dedupAndOutput 去重并输出结果
func (c *Coordinator) dedupAndOutput(
	ctx context.Context,
	in <-chan types.PeerInfo,
	out chan<- types.PeerInfo,
	limit int,
) {
	seen := make(map[types.PeerID]bool)
	count := 0

	for peer := range in {
		// 检查上下文
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 去重
		if seen[peer.ID] {
			continue
		}
		seen[peer.ID] = true

		// 更新缓存
		if c.config.EnableCache {
			c.updateCache(peer)
		}

		// 输出
		select {
		case out <- peer:
			count++
			if limit > 0 && count >= limit {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// Advertise 广播自身（实现 Discovery 接口）
func (c *Coordinator) Advertise(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (time.Duration, error) {
	if !c.started.Load() {
		return 0, ErrNotStarted
	}

	ns = pkgif.NormalizeNamespace(ns)

	// 解析选项
	options := &pkgif.DiscoveryOptions{TTL: time.Hour}
	for _, opt := range opts {
		opt(options)
	}

	// 创建超时上下文
	advertiseCtx := ctx
	if c.config.AdvertiseTimeout > 0 {
		var cancel context.CancelFunc
		advertiseCtx, cancel = context.WithTimeout(ctx, c.config.AdvertiseTimeout)
		defer cancel()
	}

	// 获取所有发现器的副本
	c.mu.RLock()
	discoveries := make([]pkgif.Discovery, 0, len(c.discoveries))
	for _, discovery := range c.discoveries {
		discoveries = append(discoveries, discovery)
	}
	c.mu.RUnlock()

	if len(discoveries) == 0 {
		return 0, ErrNoDiscoveries
	}

	// 并行广播到所有发现器
	var wg sync.WaitGroup
	var maxTTL time.Duration
	var mu sync.Mutex

	for _, discovery := range discoveries {
		wg.Add(1)
		go func(d pkgif.Discovery) {
			defer wg.Done()

			ttl, err := d.Advertise(advertiseCtx, ns, pkgif.WithTTL(options.TTL))
			if err != nil {
				return
			}

			mu.Lock()
			if ttl > maxTTL {
				maxTTL = ttl
			}
			mu.Unlock()
		}(discovery)
	}

	wg.Wait()

	if maxTTL == 0 {
		return options.TTL, nil
	}

	return maxTTL, nil
}

// ============================================================================
//                              缓存管理
// ============================================================================

// updateCache 更新缓存
func (c *Coordinator) updateCache(peer types.PeerInfo) {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// 检查是否已存在
	if entry, exists := c.peerCache[peer.ID]; exists {
		entry.peer = peer
		entry.timestamp = time.Now()
		return
	}

	// 检查缓存大小限制
	if len(c.peerCache) >= c.config.MaxCacheSize {
		// LRU 淘汰最旧的条目
		if len(c.cacheOrder) > 0 {
			oldestID := c.cacheOrder[0]
			delete(c.peerCache, oldestID)
			c.cacheOrder = c.cacheOrder[1:]
		}
	}

	// 添加新条目
	c.peerCache[peer.ID] = &peerCacheEntry{
		peer:      peer,
		timestamp: time.Now(),
	}
	c.cacheOrder = append(c.cacheOrder, peer.ID)
}

// cacheCleanupLoop 缓存清理循环
func (c *Coordinator) cacheCleanupLoop() {
	ticker := time.NewTicker(c.config.CacheTTL)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpiredCache()
		case <-c.ctx.Done():
			return
		}
	}
}

// cleanupExpiredCache 清理过期缓存
func (c *Coordinator) cleanupExpiredCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	now := time.Now()
	newOrder := make([]types.PeerID, 0, len(c.cacheOrder))

	for _, id := range c.cacheOrder {
		entry, exists := c.peerCache[id]
		if !exists {
			continue
		}

		// 检查是否过期
		if now.Sub(entry.timestamp) > c.config.CacheTTL {
			delete(c.peerCache, id)
		} else {
			newOrder = append(newOrder, id)
		}
	}

	c.cacheOrder = newOrder
}

// ============================================================================
//                              接口验证
// ============================================================================

// 确保 Coordinator 实现 Discovery 接口
var _ pkgif.Discovery = (*Coordinator)(nil)
