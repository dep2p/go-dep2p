package routing

import (
	"container/list"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              路由缓存
// ============================================================================

// RouteCache 路由缓存（LRU + TTL）
type RouteCache struct {
	mu sync.RWMutex

	// LRU
	maxSize int
	data    map[string]*list.Element
	lruList *list.List

	// TTL
	ttl time.Duration

	// 统计
	hits   int64
	misses int64
}

// cacheEntry 缓存条目
type cacheEntry struct {
	key     string
	route   *interfaces.Route
	expires time.Time
}

// NewRouteCache 创建路由缓存
func NewRouteCache(maxSize int, ttl time.Duration) *RouteCache {
	return &RouteCache{
		maxSize: maxSize,
		ttl:     ttl,
		data:    make(map[string]*list.Element),
		lruList: list.New(),
	}
}

// Get 获取缓存
func (rc *RouteCache) Get(targetPeerID string) (*interfaces.Route, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	element, ok := rc.data[targetPeerID]
	if !ok {
		rc.misses++
		return nil, false
	}

	entry := element.Value.(*cacheEntry)

	// 检查过期
	if time.Now().After(entry.expires) {
		rc.removeElement(element)
		rc.misses++
		return nil, false
	}

	// 移到队首
	rc.lruList.MoveToFront(element)
	rc.hits++

	return entry.route, true
}

// Set 设置缓存
func (rc *RouteCache) Set(route *interfaces.Route) {
	if route == nil {
		return
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// 检查是否已存在
	if element, ok := rc.data[route.TargetPeerID]; ok {
		entry := element.Value.(*cacheEntry)
		entry.route = route
		entry.expires = time.Now().Add(rc.ttl)
		rc.lruList.MoveToFront(element)
		return
	}

	// 检查容量
	if rc.lruList.Len() >= rc.maxSize {
		oldest := rc.lruList.Back()
		if oldest != nil {
			rc.removeElement(oldest)
		}
	}

	// 添加新条目
	entry := &cacheEntry{
		key:     route.TargetPeerID,
		route:   route,
		expires: time.Now().Add(rc.ttl),
	}

	element := rc.lruList.PushFront(entry)
	rc.data[route.TargetPeerID] = element
}

// Invalidate 使缓存失效
func (rc *RouteCache) Invalidate(targetPeerID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if element, ok := rc.data[targetPeerID]; ok {
		rc.removeElement(element)
	}
}

// Clear 清空缓存
func (rc *RouteCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.data = make(map[string]*list.Element)
	rc.lruList.Init()
	rc.hits = 0
	rc.misses = 0
}

// GetStats 获取统计
func (rc *RouteCache) GetStats() RouteCacheStats {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	total := rc.hits + rc.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(rc.hits) / float64(total)
	}

	return RouteCacheStats{
		Size:    rc.lruList.Len(),
		MaxSize: rc.maxSize,
		Hits:    rc.hits,
		Misses:  rc.misses,
		HitRate: hitRate,
	}
}

// removeElement 移除元素
func (rc *RouteCache) removeElement(element *list.Element) {
	entry := element.Value.(*cacheEntry)
	delete(rc.data, entry.key)
	rc.lruList.Remove(element)
}

// RouteCacheStats 路由缓存统计
type RouteCacheStats struct {
	Size    int
	MaxSize int
	Hits    int64
	Misses  int64
	HitRate float64
}
