// Package quic 提供基于 QUIC 的传输层实现
//
// Session Store 用于 0-RTT 重连：
// - 存储 Session Tickets
// - 支持 TTL 过期
// - 线程安全
package quic

import (
	"crypto/tls"
	"sync"
	"time"
)

// ============================================================================
//                              SessionStore 实现
// ============================================================================

// SessionStore TLS Session 缓存，用于 0-RTT 重连
type SessionStore struct {
	tickets  map[string]*sessionEntry
	mu       sync.RWMutex
	maxSize  int
	ttl      time.Duration
	antiPlay *AntiReplayCache

	// 生命周期控制
	stopCh   chan struct{}
	stopOnce sync.Once
}

// sessionEntry 会话条目
type sessionEntry struct {
	ticket     *tls.ClientSessionState
	createdAt  time.Time
	usedCount  int
	serverAddr string
}

// SessionStoreConfig Session Store 配置
type SessionStoreConfig struct {
	// MaxSize 最大缓存条目数
	MaxSize int

	// TTL 条目过期时间
	TTL time.Duration

	// EnableAntiReplay 启用重放攻击防护
	EnableAntiReplay bool

	// AntiReplayWindow 重放攻击防护窗口
	AntiReplayWindow time.Duration
}

// DefaultSessionStoreConfig 返回默认配置
func DefaultSessionStoreConfig() SessionStoreConfig {
	return SessionStoreConfig{
		MaxSize:          1000,
		TTL:              24 * time.Hour,
		EnableAntiReplay: true,
		AntiReplayWindow: 10 * time.Second,
	}
}

// NewSessionStore 创建 Session Store
func NewSessionStore(config SessionStoreConfig) *SessionStore {
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}
	if config.TTL <= 0 {
		config.TTL = 24 * time.Hour
	}

	store := &SessionStore{
		tickets: make(map[string]*sessionEntry),
		maxSize: config.MaxSize,
		ttl:     config.TTL,
		stopCh:  make(chan struct{}),
	}

	if config.EnableAntiReplay {
		store.antiPlay = NewAntiReplayCache(config.AntiReplayWindow)
	}

	// 启动清理协程
	go store.cleanupLoop()

	return store
}

// ============================================================================
//                              tls.ClientSessionCache 接口实现
// ============================================================================

// Get 获取缓存的 session
// 使用写锁确保原子性，避免 TOCTOU 竞态条件
func (s *SessionStore) Get(sessionKey string) (*tls.ClientSessionState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.tickets[sessionKey]
	if !ok {
		return nil, false
	}

	// 检查是否过期
	if time.Since(entry.createdAt) > s.ttl {
		delete(s.tickets, sessionKey)
		return nil, false
	}

	// 更新使用计数
	entry.usedCount++

	return entry.ticket, true
}

// Put 存储 session
func (s *SessionStore) Put(sessionKey string, cs *tls.ClientSessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果达到最大容量，删除最旧的条目
	if len(s.tickets) >= s.maxSize {
		s.evictOldest()
	}

	s.tickets[sessionKey] = &sessionEntry{
		ticket:     cs,
		createdAt:  time.Now(),
		usedCount:  0,
		serverAddr: sessionKey,
	}
}

// ============================================================================
//                              扩展方法
// ============================================================================

// Remove 删除指定的 session
func (s *SessionStore) Remove(sessionKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tickets, sessionKey)
}

// Size 返回缓存大小
func (s *SessionStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tickets)
}

// Clear 清空缓存
func (s *SessionStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tickets = make(map[string]*sessionEntry)
}

// Stats 返回统计信息
func (s *SessionStore) Stats() SessionStoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := SessionStoreStats{
		TotalEntries: len(s.tickets),
	}

	for _, entry := range s.tickets {
		stats.TotalUses += entry.usedCount
		if entry.usedCount > 0 {
			stats.UsedEntries++
		}
	}

	return stats
}

// SessionStoreStats Session Store 统计
type SessionStoreStats struct {
	TotalEntries int
	UsedEntries  int
	TotalUses    int
}

// ============================================================================
//                              内部方法
// ============================================================================

// evictOldest 驱逐最旧的条目
func (s *SessionStore) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range s.tickets {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(s.tickets, oldestKey)
	}
}

// cleanupLoop 清理过期条目
func (s *SessionStore) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// Close 关闭 Session Store，停止后台清理协程
func (s *SessionStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		if s.antiPlay != nil {
			s.antiPlay.Close()
		}
	})
}

// cleanup 清理过期条目
func (s *SessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.tickets {
		if now.Sub(entry.createdAt) > s.ttl {
			delete(s.tickets, key)
		}
	}
}

// ============================================================================
//                              AntiReplayCache 重放攻击防护
// ============================================================================

// AntiReplayCache 重放攻击防护缓存
type AntiReplayCache struct {
	window   time.Duration
	seen     map[string]time.Time
	mu       sync.RWMutex
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewAntiReplayCache 创建重放攻击防护缓存
func NewAntiReplayCache(window time.Duration) *AntiReplayCache {
	if window <= 0 {
		window = 10 * time.Second
	}

	cache := &AntiReplayCache{
		window: window,
		seen:   make(map[string]time.Time),
		stopCh: make(chan struct{}),
	}

	go cache.cleanupLoop()

	return cache
}

// Check 检查并记录 nonce
//
// 返回 true 表示是新的（非重放），false 表示可能是重放
func (c *AntiReplayCache) Check(nonce string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// 检查是否已经见过
	if seenAt, ok := c.seen[nonce]; ok {
		// 在窗口期内见过，可能是重放
		if now.Sub(seenAt) < c.window {
			return false
		}
	}

	// 记录
	c.seen[nonce] = now
	return true
}

// cleanupLoop 清理过期条目
func (c *AntiReplayCache) cleanupLoop() {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup 清理过期条目
func (c *AntiReplayCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for nonce, seenAt := range c.seen {
		if now.Sub(seenAt) > c.window {
			delete(c.seen, nonce)
		}
	}
}

// Close 关闭缓存，停止后台清理协程
func (c *AntiReplayCache) Close() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// ============================================================================
//                              全局 Session Store
// ============================================================================

var (
	globalSessionStore     *SessionStore
	globalSessionStoreOnce sync.Once
)

// GetGlobalSessionStore 获取全局 Session Store
func GetGlobalSessionStore() *SessionStore {
	globalSessionStoreOnce.Do(func() {
		globalSessionStore = NewSessionStore(DefaultSessionStoreConfig())
	})
	return globalSessionStore
}

// SetGlobalSessionStore 设置全局 Session Store
func SetGlobalSessionStore(store *SessionStore) {
	globalSessionStore = store
}

