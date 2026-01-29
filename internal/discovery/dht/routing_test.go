package dht

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// KBucket 基础功能测试
// ============================================================================

// TestKBucket_NewKBucket 测试创建新的 KBucket
func TestKBucket_NewKBucket(t *testing.T) {
	bucket := NewKBucket()
	
	require.NotNil(t, bucket)
	assert.Equal(t, 0, bucket.Size())
	assert.False(t, bucket.IsFull())
	assert.Empty(t, bucket.Nodes())
	
	t.Log("✅ 新建 KBucket 初始化正确")
}

// TestKBucket_AddFirstNode 测试添加第一个节点
func TestKBucket_AddFirstNode(t *testing.T) {
	bucket := NewKBucket()
	
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	
	added := bucket.Add(node)
	
	assert.True(t, added)
	assert.Equal(t, 1, bucket.Size())
	assert.False(t, bucket.IsFull())
	
	t.Log("✅ 添加第一个节点成功")
}

// TestKBucket_AddMultipleNodes 测试添加多个节点
func TestKBucket_AddMultipleNodes(t *testing.T) {
	bucket := NewKBucket()
	
	// 添加10个节点
	for i := 0; i < 10; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('0'+i))),
			LastSeen: time.Now(),
		}
		added := bucket.Add(node)
		assert.True(t, added)
	}
	
	assert.Equal(t, 10, bucket.Size())
	assert.False(t, bucket.IsFull())
	
	t.Log("✅ 添加多个节点成功")
}

// TestKBucket_IsFull 测试桶满检测
func TestKBucket_IsFull(t *testing.T) {
	bucket := NewKBucket()
	
	// 添加到恰好满（BucketSize = 20）
	for i := 0; i < BucketSize; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		added := bucket.Add(node)
		assert.True(t, added, "节点 %d 应该添加成功", i)
	}
	
	assert.Equal(t, BucketSize, bucket.Size())
	assert.True(t, bucket.IsFull(), "桶应该已满")
	
	t.Log("✅ 桶满检测正确")
}

// TestKBucket_AddWhenFull 测试桶满时添加节点
func TestKBucket_AddWhenFull(t *testing.T) {
	bucket := NewKBucket()
	
	// 填满桶
	for i := 0; i < BucketSize; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		bucket.Add(node)
	}
	
	// 尝试添加新节点（应该进入替换缓存）
	newNode := &RoutingNode{
		ID:       "peer-new",
		LastSeen: time.Now(),
	}
	added := bucket.Add(newNode)
	
	assert.False(t, added, "桶满时添加应该返回false")
	assert.Equal(t, BucketSize, bucket.Size(), "桶大小不应该改变")
	
	t.Log("✅ 桶满时添加节点正确处理")
}

// TestKBucket_AddDuplicateNode 测试添加重复节点
func TestKBucket_AddDuplicateNode(t *testing.T) {
	bucket := NewKBucket()
	
	node1 := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	
	// 第一次添加
	added := bucket.Add(node1)
	assert.True(t, added)
	assert.Equal(t, 1, bucket.Size())
	
	// 添加相同ID的节点（应该更新并移到前端）
	node2 := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now().Add(time.Hour),
	}
	
	added = bucket.Add(node2)
	assert.True(t, added, "更新节点应该返回true")
	assert.Equal(t, 1, bucket.Size(), "大小不应该改变")
	
	// 验证节点被更新
	nodes := bucket.Nodes()
	assert.Equal(t, "peer-1", string(nodes[0].ID))
	
	t.Log("✅ 重复节点正确更新")
}

// ============================================================================
// KBucket 移除功能测试
// ============================================================================

// TestKBucket_Remove 测试移除节点
func TestKBucket_Remove(t *testing.T) {
	bucket := NewKBucket()
	
	// 添加3个节点
	for i := 0; i < 3; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('1'+i))),
			LastSeen: time.Now(),
		}
		bucket.Add(node)
	}
	
	assert.Equal(t, 3, bucket.Size())
	
	// 移除中间的节点
	removed := bucket.Remove("peer-2")
	
	assert.True(t, removed)
	assert.Equal(t, 2, bucket.Size())
	
	// 验证节点确实被移除
	node := bucket.Get("peer-2")
	assert.Nil(t, node)
	
	t.Log("✅ 移除节点成功")
}

// TestKBucket_RemoveNonExistent 测试移除不存在的节点
func TestKBucket_RemoveNonExistent(t *testing.T) {
	bucket := NewKBucket()
	
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	bucket.Add(node)
	
	// 尝试移除不存在的节点
	removed := bucket.Remove("peer-nonexistent")
	
	assert.False(t, removed)
	assert.Equal(t, 1, bucket.Size())
	
	t.Log("✅ 移除不存在的节点正确处理")
}

// TestKBucket_RemoveWithReplacement 测试移除节点时从替换缓存提升
func TestKBucket_RemoveWithReplacement(t *testing.T) {
	bucket := NewKBucket()
	
	// 填满桶
	for i := 0; i < BucketSize; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		bucket.Add(node)
	}
	
	// 添加一个节点到替换缓存
	replacementNode := &RoutingNode{
		ID:       "peer-replacement",
		LastSeen: time.Now(),
	}
	bucket.Add(replacementNode)
	
	initialSize := bucket.Size()
	
	// 移除第一个节点（应该从替换缓存提升）
	removed := bucket.Remove("peer-a")
	
	assert.True(t, removed)
	assert.Equal(t, initialSize, bucket.Size(), "移除后应该从替换缓存提升，大小不变")
	
	// 验证替换节点被提升
	node := bucket.Get("peer-replacement")
	assert.NotNil(t, node, "替换节点应该被提升到桶中")
	
	t.Log("✅ 移除时从替换缓存提升节点")
}

// ============================================================================
// KBucket Get/Update 测试
// ============================================================================

// TestKBucket_Get 测试获取节点
func TestKBucket_Get(t *testing.T) {
	bucket := NewKBucket()
	
	node := &RoutingNode{
		ID:       "peer-1",
		Addrs:    []string{"addr1"},
		LastSeen: time.Now(),
	}
	bucket.Add(node)
	
	// 获取存在的节点
	retrieved := bucket.Get("peer-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "peer-1", string(retrieved.ID))
	
	// 获取不存在的节点
	notFound := bucket.Get("peer-nonexistent")
	assert.Nil(t, notFound)
	
	t.Log("✅ 获取节点功能正常")
}

// TestKBucket_Update 测试更新节点
func TestKBucket_Update(t *testing.T) {
	bucket := NewKBucket()
	
	oldTime := time.Now().Add(-time.Hour)
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: oldTime,
	}
	bucket.Add(node)
	
	// 更新节点
	newTime := time.Now()
	updatedNode := &RoutingNode{
		ID:       "peer-1",
		LastSeen: newTime,
	}
	bucket.Update(updatedNode)
	
	// 验证更新
	retrieved := bucket.Get("peer-1")
	assert.NotNil(t, retrieved)
	assert.True(t, retrieved.LastSeen.After(oldTime))
	
	t.Log("✅ 更新节点成功")
}

// TestKBucket_UpdateNonExistent 测试更新不存在的节点
func TestKBucket_UpdateNonExistent(t *testing.T) {
	bucket := NewKBucket()
	
	// Update 方法不返回值，所以只测试不panic
	updatedNode := &RoutingNode{
		ID:       "peer-nonexistent",
		LastSeen: time.Now(),
	}
	bucket.Update(updatedNode)
	
	// 节点不应该被添加
	node := bucket.Get("peer-nonexistent")
	assert.Nil(t, node)
	
	t.Log("✅ 更新不存在的节点正确处理")
}

// ============================================================================
// KBucket 刷新和过期测试
// ============================================================================

// TestKBucket_NeedRefresh 测试需要刷新检测
func TestKBucket_NeedRefresh(t *testing.T) {
	bucket := NewKBucket()
	
	// 新建的桶不需要刷新
	assert.False(t, bucket.NeedRefresh())
	
	// 修改最后刷新时间为很久以前
	bucket.mu.Lock()
	bucket.lastRefresh = time.Now().Add(-2 * BucketRefreshInterval)
	bucket.mu.Unlock()
	
	// 现在应该需要刷新
	assert.True(t, bucket.NeedRefresh())
	
	t.Log("✅ 刷新检测正确")
}

// TestKBucket_MarkRefreshed 测试标记已刷新
func TestKBucket_MarkRefreshed(t *testing.T) {
	bucket := NewKBucket()
	
	// 设置为需要刷新
	bucket.mu.Lock()
	bucket.lastRefresh = time.Now().Add(-2 * BucketRefreshInterval)
	bucket.mu.Unlock()
	
	require.True(t, bucket.NeedRefresh())
	
	// 标记已刷新
	bucket.MarkRefreshed()
	
	// 不应该再需要刷新
	assert.False(t, bucket.NeedRefresh())
	
	t.Log("✅ 标记刷新成功")
}

// TestRoutingTable_RemoveExpiredNodes 测试移除过期节点
func TestRoutingTable_RemoveExpiredNodes(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	// 添加一个正常节点
	freshNode := &RoutingNode{
		ID:       "peer-fresh",
		LastSeen: time.Now(),
	}
	rt.Add(freshNode)
	
	// 添加一个过期节点
	expiredNode := &RoutingNode{
		ID:       "peer-expired",
		LastSeen: time.Now().Add(-NodeExpireTime - time.Hour),
	}
	rt.Add(expiredNode)
	
	assert.Equal(t, 2, rt.Size())
	
	// 移除过期节点
	removed := rt.RemoveExpiredNodes()
	
	assert.Equal(t, 1, removed, "应该移除1个过期节点")
	assert.Equal(t, 1, rt.Size(), "应该剩余1个节点")
	
	// 验证正确的节点被保留
	assert.NotNil(t, rt.Get("peer-fresh"))
	assert.Nil(t, rt.Get("peer-expired"))
	
	t.Log("✅ 移除过期节点成功")
}

// ============================================================================
// RoutingTable 基础功能测试
// ============================================================================

// TestRoutingTable_New 测试创建路由表
func TestRoutingTable_New(t *testing.T) {
	localID := types.NodeID("local-peer")
	
	rt := NewRoutingTable(localID)
	
	require.NotNil(t, rt)
	assert.Equal(t, 0, rt.Size())
	
	t.Log("✅ 路由表创建成功")
}

// TestRoutingTable_AddNode 测试添加节点到路由表
func TestRoutingTable_AddNode(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	
	added := rt.Add(node)
	
	assert.True(t, added)
	assert.Equal(t, 1, rt.Size())
	
	t.Log("✅ 路由表添加节点成功")
}

// TestRoutingTable_AddSelf 测试添加本地节点（应该被拒绝）
func TestRoutingTable_AddSelf(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	selfNode := &RoutingNode{
		ID:       localID,
		LastSeen: time.Now(),
	}
	
	added := rt.Add(selfNode)
	
	assert.False(t, added, "不应该添加本地节点")
	assert.Equal(t, 0, rt.Size())
	
	t.Log("✅ 正确拒绝添加本地节点")
}

// TestRoutingTable_AddMultipleNodes 测试添加多个节点到不同的桶
func TestRoutingTable_AddMultipleNodes(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	// 添加多个节点
	for i := 0; i < 10; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		rt.Add(node)
	}
	
	assert.Equal(t, 10, rt.Size())
	
	t.Log("✅ 路由表添加多个节点成功")
}

// TestRoutingTable_Remove 测试从路由表移除节点
func TestRoutingTable_Remove(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	rt.Add(node)
	
	assert.Equal(t, 1, rt.Size())
	
	// 移除节点
	removed := rt.Remove("peer-1")
	
	assert.True(t, removed)
	assert.Equal(t, 0, rt.Size())
	
	t.Log("✅ 路由表移除节点成功")
}

// TestRoutingTable_Update 测试更新路由表中的节点
func TestRoutingTable_Update(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	oldTime := time.Now().Add(-time.Hour)
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: oldTime,
	}
	rt.Add(node)
	
	// 更新节点
	newTime := time.Now()
	updatedNode := &RoutingNode{
		ID:       "peer-1",
		LastSeen: newTime,
	}
	rt.Update(updatedNode)
	
	// 验证更新
	retrieved := rt.Get("peer-1")
	assert.NotNil(t, retrieved)
	assert.True(t, retrieved.LastSeen.After(oldTime))
	
	t.Log("✅ 路由表更新节点成功")
}

// TestRoutingTable_AllNodes 测试获取所有节点
func TestRoutingTable_AllNodes(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	// 添加5个节点
	expectedIDs := make(map[types.NodeID]bool)
	for i := 0; i < 5; i++ {
		id := types.NodeID("peer-" + string(rune('a'+i)))
		node := &RoutingNode{
			ID:       id,
			LastSeen: time.Now(),
		}
		rt.Add(node)
		expectedIDs[id] = true
	}
	
	// 获取所有节点
	allNodes := rt.AllNodes()
	
	assert.Equal(t, 5, len(allNodes))
	
	// 验证所有节点都被返回
	for _, node := range allNodes {
		assert.True(t, expectedIDs[node.ID], "节点 %s 应该在预期列表中", node.ID)
	}
	
	t.Log("✅ 获取所有节点成功")
}

// ============================================================================
// RoutingTable 刷新测试
// ============================================================================

// TestRoutingTable_BucketsNeedingRefresh 测试获取需要刷新的桶
func TestRoutingTable_BucketsNeedingRefresh(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	// 添加一些节点
	for i := 0; i < 5; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		rt.Add(node)
	}
	
	// 获取需要刷新的桶（新路由表应该没有需要刷新的）
	buckets := rt.BucketsNeedingRefresh()
	assert.Equal(t, 0, len(buckets), "新路由表不应该有需要刷新的桶")
	
	t.Log("✅ 获取需要刷新的桶成功")
}

// TestRoutingTable_MarkBucketRefreshed 测试标记桶已刷新
func TestRoutingTable_MarkBucketRefreshed(t *testing.T) {
	localID := types.NodeID("local-peer")
	rt := NewRoutingTable(localID)
	
	node := &RoutingNode{
		ID:       "peer-1",
		LastSeen: time.Now(),
	}
	rt.Add(node)
	
	// 计算桶索引
	bucketIndex := BucketIndex(localID, node.ID)
	
	// 标记桶已刷新
	rt.MarkBucketRefreshed(bucketIndex)
	
	// 该桶不应该需要刷新
	buckets := rt.BucketsNeedingRefresh()
	for _, idx := range buckets {
		assert.NotEqual(t, bucketIndex, idx, "已标记的桶不应该需要刷新")
	}
	
	t.Log("✅ 标记桶刷新成功")
}

// ============================================================================
// 综合场景测试
// ============================================================================

// TestRoutingTable_RealWorldScenario 测试真实场景
func TestRoutingTable_RealWorldScenario(t *testing.T) {
	localID := types.NodeID("local-node")
	rt := NewRoutingTable(localID)
	
	// 场景1：添加多个节点
	nodes := []types.NodeID{
		"peer-alpha",
		"peer-beta",
		"peer-gamma",
		"peer-delta",
	}
	
	for _, id := range nodes {
		node := &RoutingNode{
			ID:       id,
			Addrs:    []string{"addr1", "addr2"},
			LastSeen: time.Now(),
		}
		added := rt.Add(node)
		assert.True(t, added)
	}
	
	assert.Equal(t, len(nodes), rt.Size())
	
	// 场景2：查找最近的节点
	target := types.NodeID("target-node")
	nearest := rt.NearestPeers(target, 3)
	
	assert.LessOrEqual(t, len(nearest), 3, "最多返回3个节点")
	
	// 场景3：移除一个节点
	removed := rt.Remove("peer-beta")
	assert.True(t, removed)
	assert.Equal(t, len(nodes)-1, rt.Size())
	
	// 场景4：获取所有节点
	allNodes := rt.AllNodes()
	assert.Equal(t, len(nodes)-1, len(allNodes))
	
	t.Log("✅ 真实场景测试通过")
}

// TestKBucket_ReplacementCache 测试替换缓存机制
func TestKBucket_ReplacementCache(t *testing.T) {
	bucket := NewKBucket()
	
	// 填满桶
	for i := 0; i < BucketSize; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("peer-" + string(rune('a'+i))),
			LastSeen: time.Now(),
		}
		added := bucket.Add(node)
		assert.True(t, added)
	}
	
	assert.True(t, bucket.IsFull())
	
	// 添加多个节点到替换缓存
	for i := 0; i < 5; i++ {
		node := &RoutingNode{
			ID:       types.NodeID("replacement-" + string(rune('1'+i))),
			LastSeen: time.Now(),
		}
		added := bucket.Add(node)
		assert.False(t, added, "桶满时添加应该失败")
	}
	
	// 移除一个节点，应该从替换缓存提升
	bucket.Remove("peer-a")
	
	// 验证桶仍然满
	assert.True(t, bucket.IsFull())
	
	// 验证替换节点被提升
	found := false
	nodes := bucket.Nodes()
	for _, node := range nodes {
		nodeIDStr := string(node.ID)
		if len(nodeIDStr) >= 12 && nodeIDStr[:12] == "replacement-" {
			found = true
			break
		}
	}
	assert.True(t, found, "应该有替换节点被提升")
	
	t.Log("✅ 替换缓存机制正常工作")
}
