// Package dht 提供分布式哈希表实现
//
// 基于 Kademlia 协议实现 DHT，支持：
// - 节点发现
// - 值存储和检索
// - Realm 感知的 Key 计算
package dht

import (
	"crypto/rand"
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
	Alpha = 3

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
	// 检查是否已存在
	for i, existing := range b.replacementCache {
		if existing.ID == node.ID {
			b.replacementCache[i] = node
			return
		}
	}

	// 限制缓存大小
	if len(b.replacementCache) >= BucketSize {
		b.replacementCache = b.replacementCache[1:]
	}

	b.replacementCache = append(b.replacementCache, node)
}

// Remove 移除节点
func (b *KBucket) Remove(id types.NodeID) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, node := range b.nodes {
		if node.ID == id {
			b.nodes = append(b.nodes[:i], b.nodes[i+1:]...)

			// 从替换缓存补充
			if len(b.replacementCache) > 0 {
				replacement := b.replacementCache[len(b.replacementCache)-1]
				b.replacementCache = b.replacementCache[:len(b.replacementCache)-1]
				b.nodes = append(b.nodes, replacement)
			}

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

// Contains 检查是否包含节点
func (b *KBucket) Contains(id types.NodeID) bool {
	return b.Get(id) != nil
}

// NeedsRefresh 检查是否需要刷新
func (b *KBucket) NeedsRefresh() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return time.Since(b.lastRefresh) > BucketRefreshInterval
}

// MarkRefreshed 标记为已刷新
func (b *KBucket) MarkRefreshed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastRefresh = time.Now()
}

// Split 分裂桶（用于路由表优化）
func (b *KBucket) Split(localID types.NodeID, bucketIndex int) (*KBucket, *KBucket) {
	b.mu.Lock()
	defer b.mu.Unlock()

	lower := NewKBucket()
	upper := NewKBucket()

	for _, node := range b.nodes {
		distance := XORDistance(localID[:], node.ID[:])
		if len(distance) > bucketIndex/8 {
			bit := (distance[bucketIndex/8] >> (7 - uint(bucketIndex%8))) & 1
			if bit == 0 {
				lower.nodes = append(lower.nodes, node)
			} else {
				upper.nodes = append(upper.nodes, node)
			}
		}
	}

	return lower, upper
}

// ============================================================================
//                              路由表
// ============================================================================

// RoutingTable Kademlia 路由表
type RoutingTable struct {
	// 本地节点 ID
	localID types.NodeID

	// K 桶数组
	buckets [KeySize]*KBucket

	// 所有节点的索引
	nodeIndex map[types.NodeID]int

	// 当前 Realm
	realmID types.RealmID

	mu sync.RWMutex
}

// NewRoutingTable 创建路由表
func NewRoutingTable(localID types.NodeID, realmID types.RealmID) *RoutingTable {
	rt := &RoutingTable{
		localID:   localID,
		realmID:   realmID,
		nodeIndex: make(map[types.NodeID]int),
	}

	// 初始化所有桶
	for i := 0; i < KeySize; i++ {
		rt.buckets[i] = NewKBucket()
	}

	return rt
}

// ============================================================================
//                              节点操作
// ============================================================================

// Update 更新节点（添加或刷新）
func (rt *RoutingTable) Update(node *RoutingNode) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// 不能添加自己
	if node.ID == rt.localID {
		return nil
	}

	// 计算桶索引
	bucketIdx := rt.bucketIndex(node.ID)

	// 添加到对应桶
	added := rt.buckets[bucketIdx].Add(node)
	if added {
		rt.nodeIndex[node.ID] = bucketIdx
	}

	return nil
}

// Remove 移除节点
func (rt *RoutingTable) Remove(id types.NodeID) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if bucketIdx, ok := rt.nodeIndex[id]; ok {
		rt.buckets[bucketIdx].Remove(id)
		delete(rt.nodeIndex, id)
	}
}

// Find 查找节点
func (rt *RoutingTable) Find(id types.NodeID) *RoutingNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if bucketIdx, ok := rt.nodeIndex[id]; ok {
		return rt.buckets[bucketIdx].Get(id)
	}

	return nil
}

// bucketIndex 计算桶索引
func (rt *RoutingTable) bucketIndex(id types.NodeID) int {
	distance := XORDistance(rt.localID[:], id[:])

	// 找到第一个不为0的位
	for i, b := range distance {
		if b != 0 {
			// 找到这个字节中第一个为1的位
			for j := 7; j >= 0; j-- {
				if (b>>j)&1 == 1 {
					return i*8 + (7 - j)
				}
			}
		}
	}

	return KeySize - 1
}

// ============================================================================
//                              查询操作
// ============================================================================

// Size 返回路由表大小
func (rt *RoutingTable) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.nodeIndex)
}

// Peers 返回所有节点
func (rt *RoutingTable) Peers() []types.NodeID {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	peers := make([]types.NodeID, 0, len(rt.nodeIndex))
	for id := range rt.nodeIndex {
		peers = append(peers, id)
	}

	return peers
}

// NearestPeers 返回最近的节点
func (rt *RoutingTable) NearestPeers(key []byte, count int) []types.NodeID {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	type nodeDistance struct {
		id       types.NodeID
		distance []byte
	}

	var candidates []nodeDistance
	for id := range rt.nodeIndex {
		distance := XORDistance(key, id[:])
		candidates = append(candidates, nodeDistance{
			id:       id,
			distance: distance,
		})
	}

	// 按距离排序
	sort.Slice(candidates, func(i, j int) bool {
		return compareBytes(candidates[i].distance, candidates[j].distance) < 0
	})

	// 返回最近的 count 个
	result := make([]types.NodeID, 0, count)
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].id)
	}

	return result
}

// GetBucket 获取指定索引的桶
func (rt *RoutingTable) GetBucket(index int) *KBucket {
	if index < 0 || index >= KeySize {
		return nil
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.buckets[index]
}

// BucketsNeedingRefresh 返回需要刷新的桶索引
func (rt *RoutingTable) BucketsNeedingRefresh() []int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var indices []int
	for i, bucket := range rt.buckets {
		if bucket.NeedsRefresh() {
			indices = append(indices, i)
		}
	}

	return indices
}

// MarkBucketRefreshed 标记桶为已刷新
func (rt *RoutingTable) MarkBucketRefreshed(index int) {
	if index < 0 || index >= KeySize {
		return
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()
	rt.buckets[index].MarkRefreshed()
}

// ============================================================================
//                              清理
// ============================================================================

// Cleanup 清理过期节点
func (rt *RoutingTable) Cleanup() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	removed := 0
	for id, bucketIdx := range rt.nodeIndex {
		node := rt.buckets[bucketIdx].Get(id)
		if node != nil && node.IsExpired() {
			rt.buckets[bucketIdx].Remove(id)
			delete(rt.nodeIndex, id)
			removed++
		}
	}

	return removed
}

// ============================================================================
//                              辅助函数
// ============================================================================

// compareBytes 比较字节数组
func compareBytes(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return len(a) - len(b)
}

// RandomIDInBucket 生成桶范围内的随机 ID
//
// 使用密码学安全的随机数生成器确保生成的 ID 不可预测。
func RandomIDInBucket(localID types.NodeID, bucketIndex int) types.NodeID {
	var id types.NodeID

	// 复制本地 ID
	copy(id[:], localID[:])

	// 翻转第 bucketIndex 位
	byteIdx := bucketIndex / 8
	bitIdx := 7 - (bucketIndex % 8)

	if byteIdx < len(id) {
		id[byteIdx] ^= 1 << bitIdx
	}

	// 使用密码学安全的随机数填充后面的字节
	if byteIdx+1 < len(id) {
		randomBytes := make([]byte, len(id)-byteIdx-1)
		if _, err := rand.Read(randomBytes); err != nil {
			// 如果 crypto/rand 失败（极不可能），使用时间作为回退
			for i := range randomBytes {
				randomBytes[i] = byte(time.Now().UnixNano() >> (i * 8))
			}
		}
		copy(id[byteIdx+1:], randomBytes)
	}

	return id
}

