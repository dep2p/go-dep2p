// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"sync"
	"time"
)

// ============================================================================
//                              消息缓存
// ============================================================================

// MessageCache 消息历史缓存
//
// 使用滑动窗口缓存最近的消息，支持：
// - 按消息 ID 快速查找
// - 按主题获取最近消息 ID（用于 IHAVE）
// - 自动过期清理
type MessageCache struct {
	mu sync.RWMutex

	// history 历史窗口，每个心跳周期一个
	history []map[string]*CacheEntry

	// msgs 消息 ID 到条目的映射
	msgs map[string]*CacheEntry

	// windowSize 窗口大小（心跳周期数）
	windowSize int

	// gossipWindow gossip 窗口大小
	gossipWindow int

	// currentWindow 当前窗口索引
	currentWindow int
}

// NewMessageCache 创建新的消息缓存
func NewMessageCache(windowSize, gossipWindow int) *MessageCache {
	if windowSize <= 0 {
		windowSize = 5
	}
	if gossipWindow <= 0 || gossipWindow > windowSize {
		gossipWindow = 3
	}

	history := make([]map[string]*CacheEntry, windowSize)
	for i := range history {
		history[i] = make(map[string]*CacheEntry)
	}

	return &MessageCache{
		history:       history,
		msgs:          make(map[string]*CacheEntry),
		windowSize:    windowSize,
		gossipWindow:  gossipWindow,
		currentWindow: 0,
	}
}

// Put 添加消息到缓存
func (mc *MessageCache) Put(entry *CacheEntry) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	msgID := string(entry.Message.ID)

	// 检查是否已存在
	if _, exists := mc.msgs[msgID]; exists {
		return
	}

	// 添加到当前窗口和索引
	mc.history[mc.currentWindow][msgID] = entry
	mc.msgs[msgID] = entry
}

// Get 获取消息
func (mc *MessageCache) Get(msgID []byte) (*CacheEntry, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.msgs[string(msgID)]
	return entry, exists
}

// GetMessage 获取消息内容
func (mc *MessageCache) GetMessage(msgID []byte) (*Message, bool) {
	entry, exists := mc.Get(msgID)
	if !exists {
		return nil, false
	}
	return entry.Message, true
}

// Has 检查消息是否存在
func (mc *MessageCache) Has(msgID []byte) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	_, exists := mc.msgs[string(msgID)]
	return exists
}

// GetGossipIDs 获取用于 gossip 的消息 ID 列表
//
// 返回最近 gossipWindow 个周期内的消息 ID
func (mc *MessageCache) GetGossipIDs(topic string) [][]byte {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	msgIDs := make([][]byte, 0)

	// 遍历 gossip 窗口内的历史
	for i := 0; i < mc.gossipWindow; i++ {
		idx := (mc.currentWindow - i + mc.windowSize) % mc.windowSize
		for _, entry := range mc.history[idx] {
			if entry.Message.Topic == topic {
				msgIDs = append(msgIDs, entry.Message.ID)
			}
		}
	}

	return msgIDs
}

// GetRecentMessages 获取最近的消息
func (mc *MessageCache) GetRecentMessages(topic string, limit int) []*Message {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	messages := make([]*Message, 0, limit)

	// 从最近的窗口开始
	for i := 0; i < mc.windowSize && len(messages) < limit; i++ {
		idx := (mc.currentWindow - i + mc.windowSize) % mc.windowSize
		for _, entry := range mc.history[idx] {
			if entry.Message.Topic == topic {
				messages = append(messages, entry.Message)
				if len(messages) >= limit {
					break
				}
			}
		}
	}

	return messages
}

// Shift 移动到下一个窗口（心跳时调用）
func (mc *MessageCache) Shift() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 移动到下一个窗口
	mc.currentWindow = (mc.currentWindow + 1) % mc.windowSize

	// 清理最老的窗口
	oldWindow := mc.history[mc.currentWindow]
	for msgID := range oldWindow {
		delete(mc.msgs, msgID)
	}
	mc.history[mc.currentWindow] = make(map[string]*CacheEntry)
}

// Size 返回缓存大小
func (mc *MessageCache) Size() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.msgs)
}

// Clear 清空缓存
func (mc *MessageCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for i := range mc.history {
		mc.history[i] = make(map[string]*CacheEntry)
	}
	mc.msgs = make(map[string]*CacheEntry)
	mc.currentWindow = 0
}

// ============================================================================
//                              已见消息缓存
// ============================================================================

// SeenCache 已见消息缓存（用于去重）
type SeenCache struct {
	mu sync.RWMutex

	// seen 已见消息 ID -> 首次看到时间
	seen map[string]time.Time

	// ttl 条目存活时间
	ttl time.Duration

	// maxSize 最大缓存大小
	maxSize int
}

// NewSeenCache 创建新的已见消息缓存
func NewSeenCache(ttl time.Duration, maxSize int) *SeenCache {
	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	if maxSize <= 0 {
		maxSize = 100000
	}

	return &SeenCache{
		seen:    make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Add 添加已见消息
func (sc *SeenCache) Add(msgID []byte) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	key := string(msgID)
	if _, exists := sc.seen[key]; exists {
		return false // 已存在
	}

	// 检查大小限制
	if len(sc.seen) >= sc.maxSize {
		sc.cleanup()

		// 如果清理后仍然超过限制，强制删除最老的条目
		if len(sc.seen) >= sc.maxSize {
			sc.forceEvict(sc.maxSize / 10) // 删除 10% 的条目
		}
	}

	sc.seen[key] = time.Now()
	return true
}

// Has 检查是否已见
func (sc *SeenCache) Has(msgID []byte) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	_, exists := sc.seen[string(msgID)]
	return exists
}

// cleanup 清理过期条目
func (sc *SeenCache) cleanup() {
	cutoff := time.Now().Add(-sc.ttl)
	for key, seen := range sc.seen {
		if seen.Before(cutoff) {
			delete(sc.seen, key)
		}
	}
}

// forceEvict 强制驱逐最老的条目
func (sc *SeenCache) forceEvict(count int) {
	if count <= 0 || len(sc.seen) == 0 {
		return
	}

	// 收集所有条目并按时间排序
	type entry struct {
		key  string
		time time.Time
	}
	entries := make([]entry, 0, len(sc.seen))
	for k, t := range sc.seen {
		entries = append(entries, entry{k, t})
	}

	// 按时间排序（最老的在前）
	for i := range entries {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].time.After(entries[j].time) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// 删除最老的 count 个条目
	if count > len(entries) {
		count = len(entries)
	}
	for i := 0; i < count; i++ {
		delete(sc.seen, entries[i].key)
	}
}

// Cleanup 公开的清理方法
func (sc *SeenCache) Cleanup() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cleanup()
}

// Size 返回缓存大小
func (sc *SeenCache) Size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.seen)
}

// Clear 清空缓存
func (sc *SeenCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.seen = make(map[string]time.Time)
}

// ============================================================================
//                              IWANT 追踪
// ============================================================================

// IWantTracker 追踪发送的 IWANT 请求
type IWantTracker struct {
	mu sync.Mutex

	// requests 待处理的 IWANT 请求
	// key: 消息 ID, value: 请求信息
	requests map[string]*iwantRequest

	// timeout 超时时间
	timeout time.Duration
}

// iwantRequest IWANT 请求信息
type iwantRequest struct {
	// msgID 消息 ID
	msgID []byte

	// requestedAt 请求时间
	requestedAt time.Time

	// peers 请求的 peer 列表
	peers map[string]struct{}
}

// NewIWantTracker 创建新的 IWANT 追踪器
func NewIWantTracker(timeout time.Duration) *IWantTracker {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	return &IWantTracker{
		requests: make(map[string]*iwantRequest),
		timeout:  timeout,
	}
}

// Track 记录 IWANT 请求
func (t *IWantTracker) Track(msgID []byte, peer string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := string(msgID)
	if req, exists := t.requests[key]; exists {
		req.peers[peer] = struct{}{}
		return
	}

	t.requests[key] = &iwantRequest{
		msgID:       msgID,
		requestedAt: time.Now(),
		peers:       map[string]struct{}{peer: {}},
	}
}

// Fulfill 消息已收到，移除追踪
func (t *IWantTracker) Fulfill(msgID []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.requests, string(msgID))
}

// GetBrokenPromises 获取超时未响应的 peer
func (t *IWantTracker) GetBrokenPromises() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()

	broken := make(map[string]int)
	cutoff := time.Now().Add(-t.timeout)

	for key, req := range t.requests {
		if req.requestedAt.Before(cutoff) {
			for peer := range req.peers {
				broken[peer]++
			}
			delete(t.requests, key)
		}
	}

	return broken
}

// Clear 清空追踪器
func (t *IWantTracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = make(map[string]*iwantRequest)
}

// ============================================================================
//                              退避追踪
// ============================================================================

// BackoffTracker 退避时间追踪器
type BackoffTracker struct {
	mu sync.RWMutex

	// backoffs peer+topic -> 退避结束时间
	backoffs map[string]time.Time
}

// NewBackoffTracker 创建新的退避追踪器
func NewBackoffTracker() *BackoffTracker {
	return &BackoffTracker{
		backoffs: make(map[string]time.Time),
	}
}

// backoffKey 生成退避 key
func (bt *BackoffTracker) backoffKey(peer, topic string) string {
	return peer + ":" + topic
}

// AddBackoff 添加退避
func (bt *BackoffTracker) AddBackoff(peer, topic string, duration time.Duration) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	key := bt.backoffKey(peer, topic)
	bt.backoffs[key] = time.Now().Add(duration)
}

// IsBackedOff 检查是否处于退避期
func (bt *BackoffTracker) IsBackedOff(peer, topic string) bool {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	key := bt.backoffKey(peer, topic)
	until, exists := bt.backoffs[key]
	if !exists {
		return false
	}
	return time.Now().Before(until)
}

// Cleanup 清理过期退避
func (bt *BackoffTracker) Cleanup() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	now := time.Now()
	for key, until := range bt.backoffs {
		if now.After(until) {
			delete(bt.backoffs, key)
		}
	}
}

// Clear 清空退避追踪器
func (bt *BackoffTracker) Clear() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.backoffs = make(map[string]time.Time)
}

