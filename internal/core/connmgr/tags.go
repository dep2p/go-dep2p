package connmgr

import (
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// tagStore 存储节点标签信息
type tagStore struct {
	mu        sync.RWMutex
	tags      map[string]map[string]int // peerID -> tag -> value
	firstSeen map[string]time.Time      // peerID -> first seen time
}

// newTagStore 创建标签存储
func newTagStore() *tagStore {
	return &tagStore{
		tags:      make(map[string]map[string]int),
		firstSeen: make(map[string]time.Time),
	}
}

// Set 设置标签值
func (s *tagStore) Set(peer, tag string, value int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tags[peer] == nil {
		s.tags[peer] = make(map[string]int)
		s.firstSeen[peer] = time.Now()
	}
	s.tags[peer][tag] = value
}

// Get 获取标签值
func (s *tagStore) Get(peer, tag string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tags[peer] == nil {
		return 0
	}
	return s.tags[peer][tag]
}

// Delete 删除标签
func (s *tagStore) Delete(peer, tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tags[peer] != nil {
		delete(s.tags[peer], tag)
		// 如果没有标签了，删除整个节点
		if len(s.tags[peer]) == 0 {
			delete(s.tags, peer)
			delete(s.firstSeen, peer)
		}
	}
}

// Sum 计算节点所有标签权重总和
func (s *tagStore) Sum(peer string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sum := 0
	for _, value := range s.tags[peer] {
		sum += value
	}
	return sum
}

// GetInfo 获取节点标签信息
func (s *tagStore) GetInfo(peer string) *pkgif.TagInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tags[peer] == nil {
		return nil
	}

	// 复制标签映射
	tags := make(map[string]int, len(s.tags[peer]))
	sum := 0
	for k, v := range s.tags[peer] {
		tags[k] = v
		sum += v
	}

	return &pkgif.TagInfo{
		FirstSeen: s.firstSeen[peer],
		Value:     sum,
		Tags:      tags,
		Conns:     0, // 连接数由 Manager 填充
	}
}

// Upsert 更新或插入标签
func (s *tagStore) Upsert(peer, tag string, upsert func(int) int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tags[peer] == nil {
		s.tags[peer] = make(map[string]int)
		s.firstSeen[peer] = time.Now()
	}

	oldValue := s.tags[peer][tag]
	s.tags[peer][tag] = upsert(oldValue)
}

// FirstSeen 获取节点首次发现时间
func (s *tagStore) FirstSeen(peer string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.firstSeen[peer]
}

// HasPeer 检查是否有节点的标签
func (s *tagStore) HasPeer(peer string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tags[peer] != nil
}

// Clear 清空所有标签（用于测试）
func (s *tagStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tags = make(map[string]map[string]int)
	s.firstSeen = make(map[string]time.Time)
}

// ============================================================================
//                              预定义保护标签
// ============================================================================

// 保护标签常量
const (
	// TagBootstrap Bootstrap 节点
	TagBootstrap = "bootstrap"

	// TagValidator 验证者节点
	TagValidator = "validator"

	// TagRelay 中继节点
	TagRelay = "relay"

	// TagDHT DHT 邻居
	TagDHT = "dht"

	// TagMDNS mDNS 发现的本地节点
	TagMDNS = "mdns"

	// TagActive 活跃通信节点
	TagActive = "active"

	// TagPersistent 持久连接（长期合作节点）
	TagPersistent = "persistent"
)
