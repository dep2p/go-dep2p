package dht

import (
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// KeySize 密钥大小（256 位）
	KeySize = 256

	// BucketSize K 桶大小
	BucketSize = 20

	// Alpha 并发查询参数
	// v2.0.1: 从 3 增加到 5，提升 DHT 查询并发度，减少超时风险
	Alpha = 5

	// BucketRefreshInterval 桶刷新间隔
	BucketRefreshInterval = 1 * time.Hour

	// NodeExpireTime 节点过期时间
	NodeExpireTime = 24 * time.Hour
)

// ============================================================================
//                              路由表节点
// ============================================================================

// RoutingNode 路由表节点
type RoutingNode struct {
	// ID 节点 ID
	ID types.NodeID

	// Addrs 节点地址
	Addrs []string

	// LastSeen 最后一次见到的时间
	LastSeen time.Time

	// LastQuery 最后一次查询的时间
	LastQuery time.Time

	// RTT 往返时间
	RTT time.Duration

	// FailCount 连续失败次数
	FailCount int

	// RealmID 所属 Realm
	RealmID types.RealmID
}

// IsExpired 检查节点是否过期
func (n *RoutingNode) IsExpired() bool {
	return time.Since(n.LastSeen) > NodeExpireTime
}

// ============================================================================
//                              K 桶
// ============================================================================

// KBucket K 桶
type KBucket struct {
	// 节点列表（最近活跃的在前）
	nodes []*RoutingNode

	// 替换缓存（当桶满时存储候选节点）
	replacementCache []*RoutingNode

	// 最后刷新时间
	lastRefresh time.Time

	mu sync.RWMutex
}

// NewKBucket 创建新的 K 桶
func NewKBucket() *KBucket {
	return &KBucket{
		nodes:            make([]*RoutingNode, 0, BucketSize),
		replacementCache: make([]*RoutingNode, 0, BucketSize),
		lastRefresh:      time.Now(),
	}
}

// Size 返回桶中节点数量
func (b *KBucket) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.nodes)
}

// IsFull 检查桶是否已满
func (b *KBucket) IsFull() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.nodes) >= BucketSize
}

// Nodes 返回所有节点
func (b *KBucket) Nodes() []*RoutingNode {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*RoutingNode, len(b.nodes))
	copy(result, b.nodes)
	return result
}

// Add 添加节点
func (b *KBucket) Add(node *RoutingNode) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查节点是否已存在
	for i, existing := range b.nodes {
		if existing.ID == node.ID {
			// 移动到列表前端（最近活跃）
			b.nodes = append(b.nodes[:i], b.nodes[i+1:]...)
			b.nodes = append([]*RoutingNode{node}, b.nodes...)
			return true
		}
	}

	// 如果桶未满，直接添加到前端
	if len(b.nodes) < BucketSize {
		b.nodes = append([]*RoutingNode{node}, b.nodes...)
		return true
	}

	// 桶已满，添加到替换缓存
	b.addToReplacementCache(node)
	return false
}

// addToReplacementCache 添加到替换缓存
func (b *KBucket) addToReplacementCache(node *RoutingNode) {
	// 检查是否已在缓存中
	for i, existing := range b.replacementCache {
		if existing.ID == node.ID {
			// 移动到列表前端
			b.replacementCache = append(b.replacementCache[:i], b.replacementCache[i+1:]...)
			b.replacementCache = append([]*RoutingNode{node}, b.replacementCache...)
			return
		}
	}

	// 添加到前端
	b.replacementCache = append([]*RoutingNode{node}, b.replacementCache...)

	// 限制缓存大小
	if len(b.replacementCache) > BucketSize {
		b.replacementCache = b.replacementCache[:BucketSize]
	}
}

// Remove 移除节点
func (b *KBucket) Remove(id types.NodeID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 从节点列表中移除
	for i, node := range b.nodes {
		if node.ID == id {
			b.nodes = append(b.nodes[:i], b.nodes[i+1:]...)

			// 从替换缓存中提升一个节点
			if len(b.replacementCache) > 0 {
				replacement := b.replacementCache[0]
				b.replacementCache = b.replacementCache[1:]
				b.nodes = append(b.nodes, replacement)
			}

			return true
		}
	}

	// 从替换缓存中移除
	for i, node := range b.replacementCache {
		if node.ID == id {
			b.replacementCache = append(b.replacementCache[:i], b.replacementCache[i+1:]...)
			return true
		}
	}

	return false
}

// Get 获取节点
func (b *KBucket) Get(id types.NodeID) *RoutingNode {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, node := range b.nodes {
		if node.ID == id {
			return node
		}
	}

	return nil
}

// Update 更新节点
func (b *KBucket) Update(node *RoutingNode) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, existing := range b.nodes {
		if existing.ID == node.ID {
			b.nodes[i] = node
			return
		}
	}
}

// NeedRefresh 检查是否需要刷新
func (b *KBucket) NeedRefresh() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return time.Since(b.lastRefresh) > BucketRefreshInterval
}

// MarkRefreshed 标记已刷新
func (b *KBucket) MarkRefreshed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastRefresh = time.Now()
}

// ============================================================================
//                              路由表
// ============================================================================

// RoutingTable 路由表
type RoutingTable struct {
	// 本地节点 ID
	localID types.NodeID

	// K 桶数组（256 个桶）
	buckets []*KBucket

	mu sync.RWMutex
}

// NewRoutingTable 创建新的路由表
func NewRoutingTable(localID types.NodeID) *RoutingTable {
	rt := &RoutingTable{
		localID: localID,
		buckets: make([]*KBucket, KeySize),
	}

	// 初始化所有桶
	for i := 0; i < KeySize; i++ {
		rt.buckets[i] = NewKBucket()
	}

	return rt
}

// Add 添加节点
func (rt *RoutingTable) Add(node *RoutingNode) bool {
	if node.ID == rt.localID {
		return false // 不添加自己
	}

	idx := BucketIndex(rt.localID, node.ID)
	return rt.buckets[idx].Add(node)
}

// Remove 移除节点
func (rt *RoutingTable) Remove(id types.NodeID) bool {
	if id == rt.localID {
		return false
	}

	idx := BucketIndex(rt.localID, id)
	return rt.buckets[idx].Remove(id)
}

// Get 获取节点
func (rt *RoutingTable) Get(id types.NodeID) *RoutingNode {
	if id == rt.localID {
		return nil
	}

	idx := BucketIndex(rt.localID, id)
	return rt.buckets[idx].Get(id)
}

// Update 更新节点
func (rt *RoutingTable) Update(node *RoutingNode) {
	if node.ID == rt.localID {
		return
	}

	idx := BucketIndex(rt.localID, node.ID)
	rt.buckets[idx].Update(node)
}

// Size 返回路由表中的节点总数
func (rt *RoutingTable) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	total := 0
	for _, bucket := range rt.buckets {
		total += bucket.Size()
	}
	return total
}

// NearestPeers 查找最近的 N 个节点
func (rt *RoutingTable) NearestPeers(target types.NodeID, count int) []*RoutingNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// 收集所有节点
	var allNodes []*RoutingNode
	for _, bucket := range rt.buckets {
		allNodes = append(allNodes, bucket.Nodes()...)
	}

	// 按距离排序
	sort.Slice(allNodes, func(i, j int) bool {
		return CompareDistance(allNodes[i].ID, allNodes[j].ID, target) < 0
	})

	// 返回前 N 个
	if len(allNodes) > count {
		allNodes = allNodes[:count]
	}

	return allNodes
}

// AllNodes 返回所有节点
func (rt *RoutingTable) AllNodes() []*RoutingNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var allNodes []*RoutingNode
	for _, bucket := range rt.buckets {
		allNodes = append(allNodes, bucket.Nodes()...)
	}

	return allNodes
}

// BucketsNeedingRefresh 返回需要刷新的桶索引
func (rt *RoutingTable) BucketsNeedingRefresh() []int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var indices []int
	for i, bucket := range rt.buckets {
		if bucket.NeedRefresh() {
			indices = append(indices, i)
		}
	}

	return indices
}

// MarkBucketRefreshed 标记桶已刷新
func (rt *RoutingTable) MarkBucketRefreshed(idx int) {
	if idx >= 0 && idx < KeySize {
		rt.buckets[idx].MarkRefreshed()
	}
}

// RemoveExpiredNodes 移除过期节点
func (rt *RoutingTable) RemoveExpiredNodes() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	count := 0
	for _, bucket := range rt.buckets {
		nodes := bucket.Nodes()
		for _, node := range nodes {
			if node.IsExpired() {
				if bucket.Remove(node.ID) {
					count++
				}
			}
		}
	}

	return count
}
