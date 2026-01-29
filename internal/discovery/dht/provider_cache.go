package dht

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                           Provider 查询缓存
// ============================================================================

const (
	// ProviderCacheTTL 缓存 TTL
	// v2.0.1: 默认 5 分钟，避免频繁查询同一命名空间
	ProviderCacheTTL = 5 * time.Minute

	// ProviderCacheMaxSize 最大缓存条目数
	ProviderCacheMaxSize = 100
)

// ProviderCacheEntry 缓存条目
type ProviderCacheEntry struct {
	// Providers 缓存的 Provider 列表
	Providers []types.PeerInfo

	// CachedAt 缓存时间
	CachedAt time.Time

	// TTL 过期时间
	TTL time.Duration
}

// IsExpired 检查是否过期
func (e *ProviderCacheEntry) IsExpired() bool {
	return time.Since(e.CachedAt) > e.TTL
}

// ProviderCache Provider 查询结果缓存
//
// v2.0.1: 缓存 DHT Provider 查询结果，减少重复查询
// 特别适用于 Relay 发现等高频查询场景
type ProviderCache struct {
	cache map[string]*ProviderCacheEntry
	mu    sync.RWMutex
}

// NewProviderCache 创建 Provider 缓存
func NewProviderCache() *ProviderCache {
	return &ProviderCache{
		cache: make(map[string]*ProviderCacheEntry),
	}
}

// Get 获取缓存的 Provider 列表
//
// 如果缓存存在且未过期，返回缓存的 Provider 列表和 true
// 否则返回 nil 和 false
func (c *ProviderCache) Get(key string) ([]types.PeerInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}

	if entry.IsExpired() {
		return nil, false
	}

	// 返回副本，避免并发修改
	result := make([]types.PeerInfo, len(entry.Providers))
	copy(result, entry.Providers)
	return result, true
}

// Set 设置缓存
//
// 将 Provider 列表缓存到指定 key
func (c *ProviderCache) Set(key string, providers []types.PeerInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 限制缓存大小
	if len(c.cache) >= ProviderCacheMaxSize {
		c.evictExpired()
		// 如果还是超过限制，删除最旧的条目
		if len(c.cache) >= ProviderCacheMaxSize {
			c.evictOldest()
		}
	}

	// 复制 providers，避免外部修改影响缓存
	cached := make([]types.PeerInfo, len(providers))
	copy(cached, providers)

	c.cache[key] = &ProviderCacheEntry{
		Providers: cached,
		CachedAt:  time.Now(),
		TTL:       ProviderCacheTTL,
	}
}

// SetWithTTL 设置缓存（自定义 TTL）
func (c *ProviderCache) SetWithTTL(key string, providers []types.PeerInfo, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 限制缓存大小
	if len(c.cache) >= ProviderCacheMaxSize {
		c.evictExpired()
		if len(c.cache) >= ProviderCacheMaxSize {
			c.evictOldest()
		}
	}

	// 复制 providers
	cached := make([]types.PeerInfo, len(providers))
	copy(cached, providers)

	c.cache[key] = &ProviderCacheEntry{
		Providers: cached,
		CachedAt:  time.Now(),
		TTL:       ttl,
	}
}

// Delete 删除缓存条目
func (c *ProviderCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// Clear 清空缓存
func (c *ProviderCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*ProviderCacheEntry)
}

// Size 返回缓存条目数
func (c *ProviderCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// evictExpired 清理过期条目（需要持有锁）
func (c *ProviderCache) evictExpired() {
	for key, entry := range c.cache {
		if entry.IsExpired() {
			delete(c.cache, key)
		}
	}
}

// evictOldest 删除最旧的条目（需要持有锁）
func (c *ProviderCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// CleanupExpired 清理所有过期条目
//
// 返回清理的条目数
func (c *ProviderCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key, entry := range c.cache {
		if entry.IsExpired() {
			delete(c.cache, key)
			count++
		}
	}
	return count
}

// StartCleanupLoop 启动后台清理循环
//
// v2.0.1: 定时清理过期条目，避免长时间运行后内存膨胀
// 通过 context 取消来停止清理循环
//
// 参数：
//   - ctx: 上下文，用于控制循环停止
//   - interval: 清理间隔（建议 1 分钟）
func (c *ProviderCache) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Debug("Provider 缓存清理循环已启动",
		"interval", interval)

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Provider 缓存清理循环已停止")
			return
		case <-ticker.C:
			cleaned := c.CleanupExpired()
			if cleaned > 0 {
				logger.Debug("Provider 缓存已清理过期条目",
					"cleaned", cleaned,
					"remaining", c.Size())
			}
		}
	}
}
