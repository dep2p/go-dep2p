package member

import (
	"container/list"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              LRU + TTL 缓存
// ============================================================================

// Cache 成员缓存（LRU + TTL）
type Cache struct {
	mu          sync.RWMutex
	maxSize     int
	ttl         time.Duration
	data        map[string]*list.Element
	lruList     *list.List
	cleanupStop chan struct{}
	closed      bool
}

// cacheEntry 缓存条目
type cacheEntry struct {
	key     string
	member  *interfaces.MemberInfo
	expires time.Time
}

// NewCache 创建缓存
func NewCache(ttl time.Duration, maxSize int) *Cache {
	cache := &Cache{
		maxSize:     maxSize,
		ttl:         ttl,
		data:        make(map[string]*list.Element),
		lruList:     list.New(),
		cleanupStop: make(chan struct{}),
	}

	// 启动后台清理协程
	go cache.cleanupLoop()

	return cache
}

// Get 获取缓存
func (c *Cache) Get(peerID string) (*interfaces.MemberInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.data[peerID]
	if !ok {
		return nil, false
	}

	entry := element.Value.(*cacheEntry)

	// 检查是否过期
	if time.Now().After(entry.expires) {
		c.removeElement(element)
		return nil, false
	}

	// 移到队首（LRU）
	c.lruList.MoveToFront(element)

	return entry.member, true
}

// Set 设置缓存
func (c *Cache) Set(member *interfaces.MemberInfo) {
	if member == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否已存在
	if element, ok := c.data[member.PeerID]; ok {
		// 更新现有条目
		entry := element.Value.(*cacheEntry)
		entry.member = member
		entry.expires = time.Now().Add(c.ttl)
		c.lruList.MoveToFront(element)
		return
	}

	// 检查容量，淘汰最老的项
	if c.lruList.Len() >= c.maxSize {
		oldest := c.lruList.Back()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}

	// 添加新条目
	entry := &cacheEntry{
		key:     member.PeerID,
		member:  member,
		expires: time.Now().Add(c.ttl),
	}

	element := c.lruList.PushFront(entry)
	c.data[member.PeerID] = element
}

// Delete 删除缓存
func (c *Cache) Delete(peerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.data[peerID]; ok {
		c.removeElement(element)
	}
}

// Clear 清空缓存
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*list.Element)
	c.lruList.Init()
}

// Size 返回缓存大小
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lruList.Len()
}

// Close 关闭缓存
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.cleanupStop)

	// 直接清理，不调用 Clear() 避免死锁
	// 修复：Clear() 会尝试获取同一个锁导致死锁
	c.data = make(map[string]*list.Element)
	c.lruList.Init()
}

// removeElement 移除元素（内部使用，不加锁）
func (c *Cache) removeElement(element *list.Element) {
	if element == nil {
		return
	}

	entry := element.Value.(*cacheEntry)
	delete(c.data, entry.key)
	c.lruList.Remove(element)
}

// cleanupLoop 后台清理过期条目
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.cleanupStop:
			return
		}
	}
}

// cleanupExpired 清理过期条目
func (c *Cache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	toRemove := make([]*list.Element, 0)

	// 找出所有过期条目
	for element := c.lruList.Front(); element != nil; element = element.Next() {
		entry := element.Value.(*cacheEntry)
		if now.After(entry.expires) {
			toRemove = append(toRemove, element)
		}
	}

	// 移除过期条目
	for _, element := range toRemove {
		c.removeElement(element)
	}
}

// GetStats 获取缓存统计
func (c *Cache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Size:     c.lruList.Len(),
		MaxSize:  c.maxSize,
		HitRate:  0.0, // 需要额外的计数器才能计算命中率
		Capacity: float64(c.lruList.Len()) / float64(c.maxSize),
	}
}

// CacheStats 缓存统计
type CacheStats struct {
	Size     int
	MaxSize  int
	HitRate  float64
	Capacity float64
}

// 确保实现接口
var _ interfaces.MemberCache = (*Cache)(nil)
