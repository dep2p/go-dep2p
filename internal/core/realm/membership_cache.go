// Package realm 提供成员缓存实现
//
// v1.1 新增: MembershipCache 用于 PubSub 消息的成员验证
package realm

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志
var cacheLog = logger.Logger("realm.cache")

// ============================================================================
//                              MembershipCache 实现
// ============================================================================

// MembershipCache 成员缓存
//
// 用于 PubSub 消息验证，缓存已验证的 Realm 成员信息。
// 这是强制隔离检查点 #3 的关键组件。
type MembershipCache struct {
	cache     map[types.NodeID]*cacheEntry
	ttl       time.Duration
	mu        sync.RWMutex
	cleanupCh chan struct{}
}

// cacheEntry 缓存条目
type cacheEntry struct {
	realmID   types.RealmID
	verified  bool
	expiresAt time.Time
	addedAt   time.Time
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewMembershipCache 创建成员缓存
func NewMembershipCache(ttl time.Duration) *MembershipCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute // 默认 5 分钟 TTL
	}

	c := &MembershipCache{
		cache:     make(map[types.NodeID]*cacheEntry),
		ttl:       ttl,
		cleanupCh: make(chan struct{}),
	}

	// 启动清理协程
	go c.cleanupLoop()

	return c
}

// ============================================================================
//                              缓存操作
// ============================================================================

// Add 添加成员到缓存
func (c *MembershipCache) Add(nodeID types.NodeID, realmID types.RealmID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[nodeID] = &cacheEntry{
		realmID:   realmID,
		verified:  true,
		expiresAt: time.Now().Add(c.ttl),
		addedAt:   time.Now(),
	}

	cacheLog.Debug("添加成员到缓存",
		"node", nodeID.ShortString(),
		"realm", string(realmID))
}

// AddWithExpiry 添加成员到缓存（指定过期时间）
func (c *MembershipCache) AddWithExpiry(nodeID types.NodeID, realmID types.RealmID, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[nodeID] = &cacheEntry{
		realmID:   realmID,
		verified:  true,
		expiresAt: expiresAt,
		addedAt:   time.Now(),
	}

	cacheLog.Debug("添加成员到缓存（指定过期）",
		"node", nodeID.ShortString(),
		"realm", string(realmID),
		"expires", expiresAt)
}

// Remove 从缓存中移除成员
func (c *MembershipCache) Remove(nodeID types.NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, nodeID)

	cacheLog.Debug("从缓存中移除成员",
		"node", nodeID.ShortString())
}

// Clear 清空缓存
func (c *MembershipCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[types.NodeID]*cacheEntry)

	cacheLog.Debug("清空成员缓存")
}

// ============================================================================
//                              查询方法
// ============================================================================

// IsMember 检查节点是否是当前 Realm 成员
//
// 这是强制隔离检查点 #3 的核心方法。
// 用于 PubSub 入站消息验证。
func (c *MembershipCache) IsMember(nodeID types.NodeID, realmID types.RealmID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[nodeID]
	if !ok {
		return false
	}

	// 检查是否过期
	if time.Now().After(entry.expiresAt) {
		return false
	}

	// 检查 Realm 是否匹配
	return entry.realmID == realmID && entry.verified
}

// ValidateMessage 验证 PubSub 消息发送者
//
// 这是强制隔离检查点 #3 的入口方法。
// 返回 true 表示消息发送者是有效的 Realm 成员。
func (c *MembershipCache) ValidateMessage(from types.NodeID, realmID types.RealmID) bool {
	valid := c.IsMember(from, realmID)
	if !valid {
		cacheLog.Debug("PubSub 消息验证失败：非成员",
			"from", from.ShortString(),
			"realm", string(realmID))
	}
	return valid
}

// Get 获取缓存条目
func (c *MembershipCache) Get(nodeID types.NodeID) (types.RealmID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[nodeID]
	if !ok {
		return "", false
	}

	// 检查是否过期
	if time.Now().After(entry.expiresAt) {
		return "", false
	}

	return entry.realmID, entry.verified
}

// Size 返回缓存大小
func (c *MembershipCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Members 返回指定 Realm 的所有缓存成员
func (c *MembershipCache) Members(realmID types.RealmID) []types.NodeID {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var members []types.NodeID
	now := time.Now()

	for nodeID, entry := range c.cache {
		if entry.realmID == realmID && entry.verified && now.Before(entry.expiresAt) {
			members = append(members, nodeID)
		}
	}

	return members
}

// ============================================================================
//                              生命周期
// ============================================================================

// Stop 停止缓存清理
func (c *MembershipCache) Stop() {
	close(c.cleanupCh)
}

// cleanupLoop 清理过期条目
func (c *MembershipCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-c.cleanupCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 执行清理
func (c *MembershipCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expired := 0

	for nodeID, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, nodeID)
			expired++
		}
	}

	if expired > 0 {
		cacheLog.Debug("清理过期缓存条目",
			"expired", expired,
			"remaining", len(c.cache))
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// CacheStats 缓存统计
type CacheStats struct {
	// TotalEntries 总条目数
	TotalEntries int

	// ValidEntries 有效条目数
	ValidEntries int

	// ExpiredEntries 过期条目数
	ExpiredEntries int

	// RealmCounts 各 Realm 的成员数
	RealmCounts map[types.RealmID]int
}

// Stats 返回缓存统计
func (c *MembershipCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		TotalEntries: len(c.cache),
		RealmCounts:  make(map[types.RealmID]int),
	}

	now := time.Now()

	for _, entry := range c.cache {
		if now.Before(entry.expiresAt) {
			stats.ValidEntries++
			stats.RealmCounts[entry.realmID]++
		} else {
			stats.ExpiredEntries++
		}
	}

	return stats
}


