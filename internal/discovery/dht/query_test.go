package dht

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// iterativeQuery 基础测试
// ============================================================================

// TestNewIterativeQuery 测试创建迭代查询
func TestNewIterativeQuery(t *testing.T) {
	dht := &DHT{}
	target := types.NodeID("target")
	
	q := newIterativeQuery(dht, target, MessageTypeFindNode, "")
	
	require.NotNil(t, q)
	assert.Equal(t, target, q.target)
	assert.Equal(t, MessageTypeFindNode, q.queryType)
	assert.NotNil(t, q.queried)
	assert.NotNil(t, q.pending)
	assert.NotNil(t, q.result)
	assert.NotNil(t, q.done)
	
	t.Log("✅ 创建迭代查询成功")
}

// ============================================================================
// getNextBatch 测试 - 重点测试并发安全和去重逻辑
// ============================================================================

// TestGetNextBatch_EmptyPending 测试空待查询列表
func TestGetNextBatch_EmptyPending(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	batch := q.getNextBatch()
	
	assert.Empty(t, batch, "空列表应该返回空批次")
	
	t.Log("✅ 空待查询列表正确处理")
}

// TestGetNextBatch_FoundValue 测试找到值后不再查询
func TestGetNextBatch_FoundValue(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindValue, "key")
	
	// 添加待查询节点
	q.pending = []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
	}
	
	// 标记找到值
	q.foundValue = true
	
	batch := q.getNextBatch()
	
	assert.Empty(t, batch, "找到值后不应该继续查询")
	
	t.Log("✅ 找到值后正确停止查询")
}

// TestGetNextBatch_MaxConcurrent 测试并发限制
func TestGetNextBatch_MaxConcurrent(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	// 添加多个待查询节点
	for i := 0; i < 10; i++ {
		q.pending = append(q.pending, &RoutingNode{
			ID: types.NodeID("peer-" + string(rune('a'+i))),
		})
	}
	
	// 设置已有Alpha个查询在运行
	q.queryRunning = Alpha
	
	batch := q.getNextBatch()
	
	assert.Empty(t, batch, "达到并发上限时不应该返回新节点")
	
	t.Log("✅ 并发限制正确工作")
}

// TestGetNextBatch_AlphaLimit 测试Alpha限制
func TestGetNextBatch_AlphaLimit(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	// 添加多于Alpha个节点
	for i := 0; i < Alpha+5; i++ {
		q.pending = append(q.pending, &RoutingNode{
			ID: types.NodeID("peer-" + string(rune('a'+i))),
		})
	}
	
	batch := q.getNextBatch()
	
	assert.LessOrEqual(t, len(batch), Alpha, "单批次不应该超过Alpha个节点")
	assert.Equal(t, Alpha, len(batch), "应该返回Alpha个节点")
	
	t.Log("✅ Alpha限制正确工作")
}

// TestGetNextBatch_SkipQueried 测试跳过已查询节点
func TestGetNextBatch_SkipQueried(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	// 添加待查询节点
	q.pending = []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
		{ID: "peer-3"},
	}
	
	// 标记peer-2已查询
	q.queried["peer-2"] = struct{}{}
	
	batch := q.getNextBatch()
	
	// 应该跳过peer-2
	assert.Equal(t, 2, len(batch))
	for _, node := range batch {
		assert.NotEqual(t, "peer-2", string(node.ID), "不应该返回已查询的节点")
	}
	
	t.Log("✅ 正确跳过已查询节点")
}

// ============================================================================
// containsNode 测试
// ============================================================================

// TestContainsNode_Found 测试节点存在
func TestContainsNode_Found(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	nodes := []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
		{ID: "peer-3"},
	}
	
	assert.True(t, q.containsNode(nodes, "peer-2"))
	assert.True(t, q.containsNode(nodes, "peer-1"))
	assert.True(t, q.containsNode(nodes, "peer-3"))
	
	t.Log("✅ 节点存在检测正确")
}

// TestContainsNode_NotFound 测试节点不存在
func TestContainsNode_NotFound(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	nodes := []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
	}
	
	assert.False(t, q.containsNode(nodes, "peer-3"))
	assert.False(t, q.containsNode(nodes, "peer-nonexistent"))
	
	t.Log("✅ 节点不存在检测正确")
}

// TestContainsNode_EmptyList 测试空列表
func TestContainsNode_EmptyList(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	var nodes []*RoutingNode
	
	assert.False(t, q.containsNode(nodes, "peer-1"))
	
	t.Log("✅ 空列表正确处理")
}

// ============================================================================
// addToPending 测试 - 重点测试排序逻辑
// ============================================================================

// TestAddToPending_SingleNode 测试添加单个节点
func TestAddToPending_SingleNode(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeFindNode, "")
	
	node := &RoutingNode{ID: "peer-1"}
	q.addToPending(node)
	
	assert.Equal(t, 1, len(q.pending))
	assert.Equal(t, "peer-1", string(q.pending[0].ID))
	
	t.Log("✅ 添加单个节点成功")
}

// TestAddToPending_Sorting 测试节点按距离排序
func TestAddToPending_Sorting(t *testing.T) {
	target := types.NodeID("target-node")
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, target, MessageTypeFindNode, "")
	
	// 添加多个节点
	nodes := []*RoutingNode{
		{ID: "peer-c"},
		{ID: "peer-a"},
		{ID: "peer-b"},
	}
	
	for _, node := range nodes {
		q.addToPending(node)
	}
	
	// 验证按距离排序（距离target最近的在前）
	assert.Equal(t, 3, len(q.pending))
	
	// 验证排序：每个节点应该比后面的节点更近或相等
	for i := 0; i < len(q.pending)-1; i++ {
		cmp := CompareDistance(q.pending[i].ID, q.pending[i+1].ID, target)
		assert.LessOrEqual(t, cmp, 0, "节点应该按距离排序")
	}
	
	t.Log("✅ 节点按距离正确排序")
}

// TestAddToPending_Duplicate 测试重复节点
func TestAddToPending_Duplicate(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeFindNode, "")
	
	node := &RoutingNode{ID: "peer-1"}
	
	// 添加两次相同节点
	q.addToPending(node)
	q.addToPending(node)
	
	// 应该只有一个（去重）
	assert.Equal(t, 1, len(q.pending))
	
	t.Log("✅ 重复节点正确去重")
}

// ============================================================================
// GetResult/GetValue/GetProviders 测试
// ============================================================================

// TestGetResult 测试获取查询结果
func TestGetResult(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	// 添加结果节点
	q.result = []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
		{ID: "peer-3"},
	}
	
	result := q.GetResult()
	
	assert.Equal(t, 3, len(result))
	assert.Equal(t, "peer-1", string(result[0].ID))
	
	t.Log("✅ 获取查询结果成功")
}

// TestGetValue 测试获取值
func TestGetValue(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindValue, "key")
	
	// 设置值
	expectedValue := []byte("test-value")
	q.value = expectedValue
	
	value := q.GetValue()
	
	assert.Equal(t, expectedValue, value)
	
	t.Log("✅ 获取值成功")
}

// TestGetValue_NotFound 测试值未找到
func TestGetValue_NotFound(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindValue, "key")
	
	value := q.GetValue()
	
	assert.Nil(t, value)
	
	t.Log("✅ 值未找到返回nil")
}

// TestGetProviders 测试获取提供者列表
func TestGetProviders(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeGetProviders, "key")
	
	// 添加提供者
	q.providers = []types.PeerInfo{
		{ID: "provider-1"},
		{ID: "provider-2"},
	}
	
	providers := q.GetProviders()
	
	assert.Equal(t, 2, len(providers))
	
	t.Log("✅ 获取提供者列表成功")
}

// ============================================================================
// processResponse 测试 - 重点测试BUG: 重复close(done)
// ============================================================================

// TestProcessResponse_FindNode 测试处理FindNode响应
func TestProcessResponse_FindNode(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeFindNode, "")
	
	q.queryRunning = 1
	
	node := &RoutingNode{ID: "peer-1"}
	response := &Message{
		Type: MessageTypeFindNodeResponse,
		CloserPeers: []PeerRecord{
			{ID: "peer-2", Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}},
			{ID: "peer-3", Addrs: []string{"/ip4/5.6.7.8/tcp/4001"}},
		},
	}
	
	q.processResponse(node, response)
	
	// 验证节点被添加到结果
	assert.Equal(t, 1, len(q.result))
	assert.Equal(t, "peer-1", string(q.result[0].ID))
	
	// 验证新节点被添加到pending
	assert.Equal(t, 2, len(q.pending))
	
	t.Log("✅ FindNode响应处理正确")
}

// TestProcessResponse_FindValue_FoundValue 测试找到值
func TestProcessResponse_FindValue_FoundValue(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeFindValue, "key")
	
	q.queryRunning = 1
	
	node := &RoutingNode{ID: "peer-1"}
	expectedValue := []byte("found-value")
	response := &Message{
		Type:  MessageTypeFindValueResponse,
		Value: expectedValue,
	}
	
	q.processResponse(node, response)
	
	// 验证值被设置
	assert.Equal(t, expectedValue, q.value)
	assert.True(t, q.foundValue)
	
	t.Log("✅ FindValue找到值时处理正确")
}

// TestProcessResponse_GetProviders 测试处理GetProviders响应
func TestProcessResponse_GetProviders(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeGetProviders, "key")
	
	q.queryRunning = 1
	
	node := &RoutingNode{ID: "peer-1"}
	response := &Message{
		Type: MessageTypeGetProvidersResponse,
		Providers: []PeerRecord{
			{ID: "provider-1", Addrs: []string{"/ip4/1.2.3.4/tcp/4001"}},
			{ID: "provider-2", Addrs: []string{"/ip4/5.6.7.8/tcp/4001"}},
		},
		CloserPeers: []PeerRecord{
			{ID: "peer-2", Addrs: []string{"/ip4/9.10.11.12/tcp/4001"}},
		},
	}
	
	q.processResponse(node, response)
	
	// 验证提供者被收集
	assert.Equal(t, 2, len(q.providers))
	
	// 验证新节点被添加到pending
	assert.Equal(t, 1, len(q.pending))
	
	t.Log("✅ GetProviders响应处理正确")
}

// ============================================================================
// 并发安全测试 - 发现潜在BUG
// ============================================================================

// TestProcessResponse_ConcurrentSafety 测试并发安全
// 潜在BUG: 多个goroutine同时完成可能导致重复close(done)
func TestProcessResponse_ConcurrentSafety(t *testing.T) {
	dht := &DHT{
		host: &mockHost{id: "local"},
	}
	q := newIterativeQuery(dht, "target", MessageTypeFindNode, "")
	
	// 启动多个并发查询
	q.queryRunning = 3
	
	node1 := &RoutingNode{ID: "peer-1"}
	node2 := &RoutingNode{ID: "peer-2"}
	node3 := &RoutingNode{ID: "peer-3"}
	
	response := &Message{
		Type:        MessageTypeFindNodeResponse,
		CloserPeers: []PeerRecord{},
	}
	
	// 并发处理响应
	done := make(chan bool, 3)
	
	processWithRecovery := func(node *RoutingNode) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("❌ Panic detected: %v (可能是重复close(done))", r)
			}
			done <- true
		}()
		
		// 模拟查询完成
		q.mu.Lock()
		q.queryRunning--
		isLast := q.queryRunning == 0
		q.mu.Unlock()
		
		q.processResponse(node, response)
		
		// BUG检测：如果多个goroutine都认为自己是最后一个，会重复close
		if isLast {
			t.Logf("  goroutine %s 认为自己是最后一个", node.ID)
		}
	}
	
	go processWithRecovery(node1)
	go processWithRecovery(node2)
	go processWithRecovery(node3)
	
	// 等待所有goroutine完成
	for i := 0; i < 3; i++ {
		<-done
	}
	
	t.Log("✅ 并发处理没有panic")
}

// ============================================================================
// 边界条件测试
// ============================================================================

// TestGetNextBatch_PartialAvailable 测试部分节点可用
func TestGetNextBatch_PartialAvailable(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	// 只添加2个节点，但Alpha=3
	q.pending = []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
	}
	
	batch := q.getNextBatch()
	
	assert.Equal(t, 2, len(batch), "应该返回所有可用节点")
	
	t.Log("✅ 部分节点可用时正确处理")
}

// TestGetNextBatch_AllQueried 测试所有节点都已查询
func TestGetNextBatch_AllQueried(t *testing.T) {
	q := newIterativeQuery(&DHT{}, "target", MessageTypeFindNode, "")
	
	q.pending = []*RoutingNode{
		{ID: "peer-1"},
		{ID: "peer-2"},
	}
	
	// 标记所有节点已查询
	q.queried["peer-1"] = struct{}{}
	q.queried["peer-2"] = struct{}{}
	
	batch := q.getNextBatch()
	
	assert.Empty(t, batch, "所有节点已查询时应该返回空")
	
	t.Log("✅ 所有节点已查询时正确处理")
}

// mockHost 在 dht_test.go 中已定义，这里不需要重复定义
