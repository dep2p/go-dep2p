// Package bootstrap 提供引导节点发现服务
//
// 本文件实现 ExtendedNodeStore，用于持久化存储 Bootstrap 节点信息。
package bootstrap

import (
	"container/list"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// 使用 bootstrap.go 中定义的 logger

// ════════════════════════════════════════════════════════════════════════════
// NodeEntry 节点条目
// ════════════════════════════════════════════════════════════════════════════

// NodeEntry 存储的节点信息
type NodeEntry struct {
	// ID 节点 ID
	ID types.NodeID `json:"id"`

	// Addrs 节点地址列表
	Addrs []string `json:"addrs"`

	// LastSeen 最后活跃时间
	LastSeen time.Time `json:"last_seen"`

	// LastProbe 最后探测时间
	LastProbe time.Time `json:"last_probe"`

	// FailCount 连续探测失败次数
	FailCount int `json:"fail_count"`

	// Status 节点状态
	Status NodeStatus `json:"status"`

	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired 检查节点是否已过期
func (e *NodeEntry) IsExpired(expireTime time.Duration) bool {
	return time.Since(e.LastSeen) > expireTime
}

// IsOffline 检查节点是否离线
func (e *NodeEntry) IsOffline(threshold int) bool {
	return e.FailCount >= threshold
}

// ════════════════════════════════════════════════════════════════════════════
// ExtendedNodeStore 扩展节点存储
// ════════════════════════════════════════════════════════════════════════════

// ExtendedNodeStore 扩展节点存储
// 支持大容量存储、LRU 缓存和持久化
type ExtendedNodeStore struct {
	mu sync.RWMutex

	// 内存存储（主存储）
	nodes map[string]*NodeEntry

	// LRU 缓存
	cache     map[string]*list.Element
	cacheList *list.List
	cacheSize int

	// 配置
	maxNodes         int
	expireTime       time.Duration
	offlineThreshold int

	// 持久化接口（可选）
	persister NodePersister

	// 统计信息
	stats StoreStats
}

// StoreStats 存储统计信息
type StoreStats struct {
	TotalNodes   int
	OnlineNodes  int
	OfflineNodes int
	CacheHits    int64
	CacheMisses  int64
}

// NodePersister 节点持久化接口
type NodePersister interface {
	// Save 保存节点
	Save(entry *NodeEntry) error

	// Load 加载节点
	Load(id types.NodeID) (*NodeEntry, error)

	// Delete 删除节点
	Delete(id types.NodeID) error

	// LoadAll 加载所有节点
	LoadAll() ([]*NodeEntry, error)

	// Close 关闭持久化
	Close() error
}

// StoreOption 存储选项
type StoreOption func(*ExtendedNodeStore)

// WithMaxNodes 设置最大节点数
func WithMaxNodes(max int) StoreOption {
	return func(s *ExtendedNodeStore) {
		s.maxNodes = max
	}
}

// WithCacheSize 设置缓存大小
func WithCacheSize(size int) StoreOption {
	return func(s *ExtendedNodeStore) {
		s.cacheSize = size
	}
}

// WithExpireTime 设置过期时间
func WithExpireTime(d time.Duration) StoreOption {
	return func(s *ExtendedNodeStore) {
		s.expireTime = d
	}
}

// WithOfflineThreshold 设置离线阈值
func WithOfflineThreshold(threshold int) StoreOption {
	return func(s *ExtendedNodeStore) {
		s.offlineThreshold = threshold
	}
}

// WithPersister 设置持久化器
func WithPersister(p NodePersister) StoreOption {
	return func(s *ExtendedNodeStore) {
		s.persister = p
	}
}

// NewExtendedNodeStore 创建扩展节点存储
func NewExtendedNodeStore(opts ...StoreOption) *ExtendedNodeStore {
	defaults := GetDefaults()

	s := &ExtendedNodeStore{
		nodes:            make(map[string]*NodeEntry),
		cache:            make(map[string]*list.Element),
		cacheList:        list.New(),
		maxNodes:         defaults.MaxNodes,
		cacheSize:        defaults.CacheSize,
		expireTime:       defaults.NodeExpireTime,
		offlineThreshold: defaults.OfflineThreshold,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ════════════════════════════════════════════════════════════════════════════
// 基础操作
// ════════════════════════════════════════════════════════════════════════════

// Put 添加或更新节点
func (s *ExtendedNodeStore) Put(entry *NodeEntry) error {
	if entry == nil || entry.ID == "" {
		return fmt.Errorf("invalid entry: nil or empty ID")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(entry.ID)

	// 检查是否超过最大容量
	if _, exists := s.nodes[key]; !exists && len(s.nodes) >= s.maxNodes {
		// 驱逐最旧的节点
		s.evictOldest()
	}

	// 更新时间戳
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// 存储到内存
	s.nodes[key] = entry

	// 更新缓存
	s.updateCache(key, entry)

	// 持久化（如果配置了）
	if s.persister != nil {
		if err := s.persister.Save(entry); err != nil {
			// 记录错误但不影响内存操作
			logger.Debug("持久化节点失败（内存操作已完成）", "id", entry.ID, "error", err)
		}
	}

	// 更新统计
	s.updateStats()

	return nil
}

// Get 获取节点
func (s *ExtendedNodeStore) Get(id types.NodeID) (*NodeEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := string(id)

	// 先查缓存
	if elem, ok := s.cache[key]; ok {
		s.stats.CacheHits++
		s.cacheList.MoveToFront(elem)
		return elem.Value.(*NodeEntry), true
	}

	// 查内存存储
	entry, ok := s.nodes[key]
	if ok {
		s.stats.CacheMisses++
		// 添加到缓存（需要升级锁，这里简化处理）
		return entry, true
	}

	// 尝试从持久化加载
	if s.persister != nil {
		entry, err := s.persister.Load(id)
		if err == nil && entry != nil {
			s.stats.CacheMisses++
			// 注意：这里不能直接修改 s.nodes，因为持有读锁
			return entry, true
		}
	}

	return nil, false
}

// Delete 删除节点
func (s *ExtendedNodeStore) Delete(id types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(id)

	// 从缓存删除
	if elem, ok := s.cache[key]; ok {
		s.cacheList.Remove(elem)
		delete(s.cache, key)
	}

	// 从内存删除
	delete(s.nodes, key)

	// 从持久化删除
	if s.persister != nil {
		if err := s.persister.Delete(id); err != nil {
			// 记录错误但不影响内存操作
			logger.Debug("删除节点持久化记录失败", "id", id, "error", err)
		}
	}

	// 更新统计
	s.updateStats()

	return nil
}

// ════════════════════════════════════════════════════════════════════════════
// 批量操作
// ════════════════════════════════════════════════════════════════════════════

// GetAll 获取所有节点
func (s *ExtendedNodeStore) GetAll() []*NodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*NodeEntry, 0, len(s.nodes))
	for _, entry := range s.nodes {
		entries = append(entries, entry)
	}
	return entries
}

// GetOnline 获取所有在线节点
func (s *ExtendedNodeStore) GetOnline() []*NodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*NodeEntry, 0)
	for _, entry := range s.nodes {
		if entry.Status == NodeStatusOnline {
			entries = append(entries, entry)
		}
	}
	return entries
}

// GetForProbe 获取需要探测的节点
// 返回最久未探测的节点，最多 batchSize 个
func (s *ExtendedNodeStore) GetForProbe(batchSize int) []*NodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 收集所有节点并按 LastProbe 排序
	entries := make([]*NodeEntry, 0, len(s.nodes))
	for _, entry := range s.nodes {
		entries = append(entries, entry)
	}

	// 按 LastProbe 时间排序（最久未探测的在前）
	// 简单的选择排序，对于大数据集可优化为堆
	for i := 0; i < len(entries) && i < batchSize; i++ {
		minIdx := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].LastProbe.Before(entries[minIdx].LastProbe) {
				minIdx = j
			}
		}
		entries[i], entries[minIdx] = entries[minIdx], entries[i]
	}

	if len(entries) > batchSize {
		entries = entries[:batchSize]
	}

	return entries
}

// ════════════════════════════════════════════════════════════════════════════
// XOR 距离查找
// ════════════════════════════════════════════════════════════════════════════

// FindClosest 查找 XOR 距离最近的 K 个节点
func (s *ExtendedNodeStore) FindClosest(target types.NodeID, k int) []*NodeEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.nodes) == 0 {
		return nil
	}

	// 收集所有节点
	entries := make([]*NodeEntry, 0, len(s.nodes))
	for _, entry := range s.nodes {
		entries = append(entries, entry)
	}

	// 按 XOR 距离排序
	targetBytes := []byte(target)
	for i := 0; i < len(entries) && i < k; i++ {
		minIdx := i
		minDist := xorDistance(targetBytes, []byte(entries[i].ID))
		for j := i + 1; j < len(entries); j++ {
			dist := xorDistance(targetBytes, []byte(entries[j].ID))
			if compareDistance(dist, minDist) < 0 {
				minIdx = j
				minDist = dist
			}
		}
		entries[i], entries[minIdx] = entries[minIdx], entries[i]
	}

	if len(entries) > k {
		entries = entries[:k]
	}

	return entries
}

// xorDistance 计算 XOR 距离
func xorDistance(a, b []byte) []byte {
	// 确保长度一致
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	result := make([]byte, maxLen)
	for i := 0; i < maxLen; i++ {
		var ai, bi byte
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		result[i] = ai ^ bi
	}
	return result
}

// compareDistance 比较两个距离
// 返回 -1 如果 a < b，0 如果 a == b，1 如果 a > b
func compareDistance(a, b []byte) int {
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	for i := 0; i < maxLen; i++ {
		var ai, bi byte
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// ════════════════════════════════════════════════════════════════════════════
// 状态更新
// ════════════════════════════════════════════════════════════════════════════

// MarkOnline 标记节点在线
func (s *ExtendedNodeStore) MarkOnline(id types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(id)
	entry, ok := s.nodes[key]
	if !ok {
		return fmt.Errorf("node not found: %s", id)
	}

	entry.Status = NodeStatusOnline
	entry.LastSeen = time.Now()
	entry.LastProbe = time.Now()
	entry.FailCount = 0

	// 持久化
	if s.persister != nil {
		if err := s.persister.Save(entry); err != nil {
			logger.Debug("持久化在线状态失败", "id", id, "error", err)
		}
	}

	s.updateStats()
	return nil
}

// MarkOffline 标记节点离线
func (s *ExtendedNodeStore) MarkOffline(id types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := string(id)
	entry, ok := s.nodes[key]
	if !ok {
		return fmt.Errorf("node not found: %s", id)
	}

	entry.FailCount++
	entry.LastProbe = time.Now()

	if entry.FailCount >= s.offlineThreshold {
		entry.Status = NodeStatusOffline
	}

	// 持久化
	if s.persister != nil {
		if err := s.persister.Save(entry); err != nil {
			logger.Debug("持久化离线状态失败", "id", id, "error", err)
		}
	}

	s.updateStats()
	return nil
}

// ════════════════════════════════════════════════════════════════════════════
// 维护操作
// ════════════════════════════════════════════════════════════════════════════

// Cleanup 清理过期节点
func (s *ExtendedNodeStore) Cleanup() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	removed := 0
	for key, entry := range s.nodes {
		if entry.IsExpired(s.expireTime) {
			// 从缓存删除
			if elem, ok := s.cache[key]; ok {
				s.cacheList.Remove(elem)
				delete(s.cache, key)
			}
			// 从内存删除
			delete(s.nodes, key)
			// 从持久化删除
			if s.persister != nil {
				if err := s.persister.Delete(entry.ID); err != nil {
					logger.Debug("删除过期节点持久化记录失败", "id", entry.ID, "error", err)
				}
			}
			removed++
		}
	}

	s.updateStats()
	return removed
}

// Stats 返回统计信息
func (s *ExtendedNodeStore) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Size 返回存储大小
func (s *ExtendedNodeStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// ════════════════════════════════════════════════════════════════════════════
// 持久化
// ════════════════════════════════════════════════════════════════════════════

// LoadFromPersister 从持久化加载所有节点
func (s *ExtendedNodeStore) LoadFromPersister() error {
	if s.persister == nil {
		return nil
	}

	entries, err := s.persister.LoadAll()
	if err != nil {
		return fmt.Errorf("load from persister: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, entry := range entries {
		if entry != nil && entry.ID != "" {
			key := string(entry.ID)
			s.nodes[key] = entry
		}
	}

	s.updateStats()
	return nil
}

// Close 关闭存储
func (s *ExtendedNodeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.persister != nil {
		return s.persister.Close()
	}
	return nil
}

// ════════════════════════════════════════════════════════════════════════════
// 内部方法
// ════════════════════════════════════════════════════════════════════════════

// updateCache 更新 LRU 缓存
func (s *ExtendedNodeStore) updateCache(key string, entry *NodeEntry) {
	if elem, ok := s.cache[key]; ok {
		// 已存在，移动到前面
		s.cacheList.MoveToFront(elem)
		elem.Value = entry
	} else {
		// 新条目
		if s.cacheList.Len() >= s.cacheSize {
			// 缓存已满，删除最旧的
			oldest := s.cacheList.Back()
			if oldest != nil {
				oldEntry := oldest.Value.(*NodeEntry)
				delete(s.cache, string(oldEntry.ID))
				s.cacheList.Remove(oldest)
			}
		}
		elem := s.cacheList.PushFront(entry)
		s.cache[key] = elem
	}
}

// evictOldest 驱逐最旧的节点
func (s *ExtendedNodeStore) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range s.nodes {
		if oldestKey == "" || entry.LastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastSeen
		}
	}

	if oldestKey != "" {
		// 从缓存删除
		if elem, ok := s.cache[oldestKey]; ok {
			s.cacheList.Remove(elem)
			delete(s.cache, oldestKey)
		}
		// 从内存删除
		entry := s.nodes[oldestKey]
		delete(s.nodes, oldestKey)
		// 从持久化删除
		if s.persister != nil && entry != nil {
			if err := s.persister.Delete(entry.ID); err != nil {
				logger.Debug("淘汰最旧节点持久化记录失败", "id", entry.ID, "error", err)
			}
		}
	}
}

// updateStats 更新统计信息
func (s *ExtendedNodeStore) updateStats() {
	s.stats.TotalNodes = len(s.nodes)
	s.stats.OnlineNodes = 0
	s.stats.OfflineNodes = 0

	for _, entry := range s.nodes {
		switch entry.Status {
		case NodeStatusOnline:
			s.stats.OnlineNodes++
		case NodeStatusOffline:
			s.stats.OfflineNodes++
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// 内存持久化器（用于测试）
// ════════════════════════════════════════════════════════════════════════════

// MemoryPersister 内存持久化器（用于测试）
type MemoryPersister struct {
	mu    sync.RWMutex
	nodes map[string][]byte
}

// NewMemoryPersister 创建内存持久化器
func NewMemoryPersister() *MemoryPersister {
	return &MemoryPersister{
		nodes: make(map[string][]byte),
	}
}

// Save 保存节点
func (p *MemoryPersister) Save(entry *NodeEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	p.nodes[string(entry.ID)] = data
	return nil
}

// Load 加载节点
func (p *MemoryPersister) Load(id types.NodeID) (*NodeEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, ok := p.nodes[string(id)]
	if !ok {
		return nil, fmt.Errorf("node not found")
	}

	var entry NodeEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// Delete 删除节点
func (p *MemoryPersister) Delete(id types.NodeID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.nodes, string(id))
	return nil
}

// LoadAll 加载所有节点
func (p *MemoryPersister) LoadAll() ([]*NodeEntry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	entries := make([]*NodeEntry, 0, len(p.nodes))
	for _, data := range p.nodes {
		var entry NodeEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

// Close 关闭
func (p *MemoryPersister) Close() error {
	return nil
}
