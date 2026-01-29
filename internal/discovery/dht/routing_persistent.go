package dht

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// persistedRoutingNode 持久化的路由节点
type persistedRoutingNode struct {
	ID        string   `json:"id"`
	Addrs     []string `json:"addrs"`
	LastSeen  int64    `json:"last_seen"`  // Unix 纳秒
	LastQuery int64    `json:"last_query"` // Unix 纳秒
	RTT       int64    `json:"rtt"`        // 纳秒
	FailCount int      `json:"fail_count"`
	RealmID   string   `json:"realm_id"`
}

// PersistentRoutingTable 持久化路由表
//
// 使用 BadgerDB 存储路由表数据。
// 键格式: {bucketIdx}/{nodeID}
//
// 注意：路由表持久化是可选的优化，因为节点可以在启动时重新发现。
type PersistentRoutingTable struct {
	// store KV 存储（前缀 d/r/）
	store *kv.Store

	// 本地节点 ID
	localID types.NodeID

	// K 桶数组（256 个桶）
	buckets []*KBucket

	mu sync.RWMutex
}

// NewPersistentRoutingTable 创建持久化路由表
//
// 参数:
//   - localID: 本地节点 ID
//   - store: KV 存储实例（已带前缀 d/r/）
func NewPersistentRoutingTable(localID types.NodeID, store *kv.Store) (*PersistentRoutingTable, error) {
	rt := &PersistentRoutingTable{
		store:   store,
		localID: localID,
		buckets: make([]*KBucket, KeySize),
	}

	// 初始化所有桶
	for i := 0; i < KeySize; i++ {
		rt.buckets[i] = NewKBucket()
	}

	// 从存储加载已有数据
	if err := rt.loadFromStore(); err != nil {
		return nil, err
	}

	return rt, nil
}

// makeRoutingKey 生成路由表存储键
func makeRoutingKey(bucketIdx int, nodeID types.NodeID) []byte {
	return []byte(fmt.Sprintf("%03d/%s", bucketIdx, string(nodeID)))
}

// loadFromStore 从存储加载路由表数据
func (rt *PersistentRoutingTable) loadFromStore() error {
	now := time.Now()

	return rt.store.PrefixScan(nil, func(storeKey, value []byte) bool {
		var persisted persistedRoutingNode
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		lastSeen := time.Unix(0, persisted.LastSeen)

		// 跳过过期节点
		if now.Sub(lastSeen) > NodeExpireTime {
			// 删除过期数据
			rt.store.Delete(storeKey)
			return true
		}

		node := &RoutingNode{
			ID:        types.NodeID(persisted.ID),
			Addrs:     persisted.Addrs,
			LastSeen:  lastSeen,
			LastQuery: time.Unix(0, persisted.LastQuery),
			RTT:       time.Duration(persisted.RTT),
			FailCount: persisted.FailCount,
			RealmID:   types.RealmID(persisted.RealmID),
		}

		// 计算桶索引并添加
		idx := BucketIndex(rt.localID, node.ID)
		rt.buckets[idx].Add(node)

		return true
	})
}

// persistNode 持久化单个节点
func (rt *PersistentRoutingTable) persistNode(node *RoutingNode) {
	idx := BucketIndex(rt.localID, node.ID)
	storeKey := makeRoutingKey(idx, node.ID)

	persisted := persistedRoutingNode{
		ID:        string(node.ID),
		Addrs:     node.Addrs,
		LastSeen:  node.LastSeen.UnixNano(),
		LastQuery: node.LastQuery.UnixNano(),
		RTT:       int64(node.RTT),
		FailCount: node.FailCount,
		RealmID:   string(node.RealmID),
	}

	rt.store.PutJSON(storeKey, &persisted)
}

// deleteNode 从存储中删除节点
func (rt *PersistentRoutingTable) deleteNode(id types.NodeID) {
	idx := BucketIndex(rt.localID, id)
	storeKey := makeRoutingKey(idx, id)
	rt.store.Delete(storeKey)
}

// Add 添加节点
func (rt *PersistentRoutingTable) Add(node *RoutingNode) bool {
	if node.ID == rt.localID {
		return false // 不添加自己
	}

	idx := BucketIndex(rt.localID, node.ID)
	added := rt.buckets[idx].Add(node)

	if added {
		// 持久化
		rt.persistNode(node)
	}

	return added
}

// Remove 移除节点
func (rt *PersistentRoutingTable) Remove(id types.NodeID) bool {
	if id == rt.localID {
		return false
	}

	idx := BucketIndex(rt.localID, id)
	removed := rt.buckets[idx].Remove(id)

	if removed {
		// 从存储中删除
		rt.deleteNode(id)
	}

	return removed
}

// Get 获取节点
func (rt *PersistentRoutingTable) Get(id types.NodeID) *RoutingNode {
	if id == rt.localID {
		return nil
	}

	idx := BucketIndex(rt.localID, id)
	return rt.buckets[idx].Get(id)
}

// Update 更新节点
func (rt *PersistentRoutingTable) Update(node *RoutingNode) {
	if node.ID == rt.localID {
		return
	}

	idx := BucketIndex(rt.localID, node.ID)
	rt.buckets[idx].Update(node)

	// 持久化更新
	rt.persistNode(node)
}

// Size 返回路由表中的节点总数
func (rt *PersistentRoutingTable) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	total := 0
	for _, bucket := range rt.buckets {
		total += bucket.Size()
	}
	return total
}

// NearestPeers 查找最近的 N 个节点
func (rt *PersistentRoutingTable) NearestPeers(target types.NodeID, count int) []*RoutingNode {
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
func (rt *PersistentRoutingTable) AllNodes() []*RoutingNode {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	var allNodes []*RoutingNode
	for _, bucket := range rt.buckets {
		allNodes = append(allNodes, bucket.Nodes()...)
	}

	return allNodes
}

// BucketsNeedingRefresh 返回需要刷新的桶索引
func (rt *PersistentRoutingTable) BucketsNeedingRefresh() []int {
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
func (rt *PersistentRoutingTable) MarkBucketRefreshed(idx int) {
	if idx >= 0 && idx < KeySize {
		rt.buckets[idx].MarkRefreshed()
	}
}

// RemoveExpiredNodes 移除过期节点
func (rt *PersistentRoutingTable) RemoveExpiredNodes() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	count := 0
	for _, bucket := range rt.buckets {
		nodes := bucket.Nodes()
		for _, node := range nodes {
			if node.IsExpired() {
				if bucket.Remove(node.ID) {
					rt.deleteNode(node.ID)
					count++
				}
			}
		}
	}

	return count
}

// Clear 清空路由表
func (rt *PersistentRoutingTable) Clear() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// 删除所有存储的节点
	for _, bucket := range rt.buckets {
		for _, node := range bucket.Nodes() {
			rt.deleteNode(node.ID)
		}
	}

	// 重新初始化桶
	for i := 0; i < KeySize; i++ {
		rt.buckets[i] = NewKBucket()
	}
}

// Ensure PersistentRoutingTable implements similar interface as RoutingTable
var _ interface {
	Add(*RoutingNode) bool
	Remove(types.NodeID) bool
	Get(types.NodeID) *RoutingNode
	Update(*RoutingNode)
	Size() int
	NearestPeers(types.NodeID, int) []*RoutingNode
	AllNodes() []*RoutingNode
	BucketsNeedingRefresh() []int
	MarkBucketRefreshed(int)
	RemoveExpiredNodes() int
} = (*PersistentRoutingTable)(nil)
