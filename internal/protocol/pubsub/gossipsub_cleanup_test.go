// Package pubsub 实现发布订阅协议
//
// 本文件包含 P2 修复（Mesh 清理）的测试用例。
package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestGossipSub_FilterConnectedPeers 测试 P2-L3 修复：过滤已连接节点
func TestGossipSub_FilterConnectedPeers(t *testing.T) {
	// 创建 mock swarm
	mockSwarm := mocks.NewMockSwarm("local-peer")

	// 设置连接状态：peer1 已连接，peer2/peer3 未连接
	mockSwarm.ConnectednessFunc = func(peerID string) interfaces.Connectedness {
		if peerID == "peer1" {
			return interfaces.Connected
		}
		return interfaces.NotConnected
	}

	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return mockSwarm
	}

	// 创建 GossipSub
	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}

	// 测试过滤
	candidates := []string{"peer1", "peer2", "peer3"}
	filtered := gs.filterConnectedPeers(candidates)

	// 只有 peer1 应该被保留
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "peer1")
	assert.NotContains(t, filtered, "peer2")
	assert.NotContains(t, filtered, "peer3")
}

// TestGossipSub_CleanupDisconnectedPeers 测试 P2-L2 修复：心跳清理断开节点
func TestGossipSub_CleanupDisconnectedPeers(t *testing.T) {
	// 创建 mock swarm
	mockSwarm := mocks.NewMockSwarm("local-peer")

	// peer1 已连接，peer2/peer3 断开
	mockSwarm.ConnectednessFunc = func(peerID string) interfaces.Connectedness {
		if peerID == "peer1" {
			return interfaces.Connected
		}
		return interfaces.NotConnected
	}

	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return mockSwarm
	}

	// 创建 GossipSub
	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}

	// 添加节点到 Mesh
	topicName := "test-topic"
	gs.mesh.Add(topicName, "peer1")
	gs.mesh.Add(topicName, "peer2")
	gs.mesh.Add(topicName, "peer3")
	assert.Equal(t, 3, gs.mesh.Count(topicName))

	// 执行清理
	gs.cleanupDisconnectedPeers(topicName)

	// peer2 和 peer3 应该被移除
	assert.Equal(t, 1, gs.mesh.Count(topicName))
	assert.True(t, gs.mesh.Has(topicName, "peer1"))
	assert.False(t, gs.mesh.Has(topicName, "peer2"))
	assert.False(t, gs.mesh.Has(topicName, "peer3"))
}

// TestGossipSub_HandlePeerDisconnected 测试 P2-L1 修复：事件驱动断开处理
func TestGossipSub_HandlePeerDisconnected(t *testing.T) {
	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")

	// 创建 GossipSub
	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}

	// 创建 topics（handlePeerDisconnected 通过 topics 遍历）
	gs.topics["topic1"] = newTopic("topic1", nil, gs)
	gs.topics["topic2"] = newTopic("topic2", nil, gs)

	// 添加节点到多个 topic 的 Mesh
	gs.mesh.Add("topic1", "peer1")
	gs.mesh.Add("topic1", "peer2")
	gs.mesh.Add("topic2", "peer1")
	gs.mesh.Add("topic2", "peer3")

	// 模拟 peer1 断开
	gs.handlePeerDisconnected("peer1")

	// peer1 应该从所有 topic 中移除
	assert.False(t, gs.mesh.Has("topic1", "peer1"))
	assert.False(t, gs.mesh.Has("topic2", "peer1"))

	// 其他节点不受影响
	assert.True(t, gs.mesh.Has("topic1", "peer2"))
	assert.True(t, gs.mesh.Has("topic2", "peer3"))
}

// TestGossipSub_HandlePeerDisconnected_WithScorer 测试断开处理时通知评分器
func TestGossipSub_HandlePeerDisconnected_WithScorer(t *testing.T) {
	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")

	// 创建 GossipSub 和评分器
	config := DefaultConfig()
	config.PeerScoring.Enabled = true
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
		scorer: NewPeerScorer(nil),
	}

	// 创建 topic
	gs.topics["topic1"] = newTopic("topic1", nil, gs)

	// 添加节点到评分器和 Mesh
	gs.scorer.AddPeer("peer1", "")
	gs.scorer.Graft("peer1", "topic1")
	gs.mesh.Add("topic1", "peer1")

	// 验证初始状态
	gs.scorer.mu.RLock()
	stats := gs.scorer.peerStats["peer1"]
	tStats := stats.topicStats["topic1"]
	initialInMesh := tStats.inMesh
	gs.scorer.mu.RUnlock()
	assert.True(t, initialInMesh)

	// 模拟断开
	gs.handlePeerDisconnected("peer1")

	// 验证评分器状态更新
	gs.scorer.mu.RLock()
	stats = gs.scorer.peerStats["peer1"]
	if stats != nil && stats.topicStats["topic1"] != nil {
		tStats = stats.topicStats["topic1"]
		assert.False(t, tStats.inMesh, "Scorer should mark peer as not in mesh")
	}
	gs.scorer.mu.RUnlock()
}

// TestGossipSub_MaintainMesh_CleansDisconnected 测试 maintainMesh 集成清理
func TestGossipSub_MaintainMesh_CleansDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 mock swarm
	mockSwarm := mocks.NewMockSwarm("local-peer")
	mockSwarm.ConnectednessFunc = func(peerID string) interfaces.Connectedness {
		// peer1 已连接，其他断开
		if peerID == "peer1" {
			return interfaces.Connected
		}
		return interfaces.NotConnected
	}

	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return mockSwarm
	}

	// 创建 GossipSub
	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}
	gs.ctx = ctx

	// 创建 topic
	topicName := "test-topic"
	gs.topics[topicName] = &topic{name: topicName}

	// 添加节点到 Mesh（包括断开的）
	gs.mesh.Add(topicName, "peer1")
	gs.mesh.Add(topicName, "peer2-disconnected")
	gs.mesh.Add(topicName, "peer3-disconnected")

	// 执行 maintainMesh
	gs.maintainMesh()

	// 断开的节点应该被清理
	assert.True(t, gs.mesh.Has(topicName, "peer1"))
	assert.False(t, gs.mesh.Has(topicName, "peer2-disconnected"))
	assert.False(t, gs.mesh.Has(topicName, "peer3-disconnected"))
}

// TestPeerScorer_DeliveryFailed 测试 P2-L4 修复：发送失败记录到评分器
func TestPeerScorer_DeliveryFailed(t *testing.T) {
	ps := NewPeerScorer(nil)
	ps.AddPeer("peer1", "")

	// 初始行为惩罚应该为 0
	ps.mu.RLock()
	initialPenalty := ps.peerStats["peer1"].behaviourPenalty
	ps.mu.RUnlock()
	assert.Equal(t, float64(0), initialPenalty)

	// 记录发送失败
	ps.DeliveryFailed("peer1", "topic1")

	// 行为惩罚应该增加
	ps.mu.RLock()
	newPenalty := ps.peerStats["peer1"].behaviourPenalty
	ps.mu.RUnlock()
	assert.Equal(t, 0.5, newPenalty)

	// 再次失败
	ps.DeliveryFailed("peer1", "topic1")

	ps.mu.RLock()
	finalPenalty := ps.peerStats["peer1"].behaviourPenalty
	ps.mu.RUnlock()
	assert.Equal(t, 1.0, finalPenalty)
}

// TestGossipSub_GraftPeers_FiltersDisconnected 测试 graftPeers 过滤未连接节点
func TestGossipSub_GraftPeers_FiltersDisconnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 mock realm
	mockRealm := mocks.NewMockRealm("test-realm")
	mockRealm.MemberList = []string{
		"local-peer",
		"peer1-connected",
		"peer2-disconnected",
		"peer3-disconnected",
	}

	// 创建 mock swarm
	mockSwarm := mocks.NewMockSwarm("local-peer")
	mockSwarm.ConnectednessFunc = func(peerID string) interfaces.Connectedness {
		if peerID == "peer1-connected" {
			return interfaces.Connected
		}
		return interfaces.NotConnected
	}

	// 创建 mock host
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return mockSwarm
	}

	// 创建 GossipSub
	config := DefaultConfig()
	config.D = 3 // 目标度数
	gs := &gossipSub{
		host:    mockHost,
		realm:   mockRealm,
		realmID: mockRealm.ID(),
		config:  config,
		mesh:    newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics:  make(map[string]*topic),
	}
	gs.ctx = ctx

	// 创建 topic
	topicName := "test-topic"
	gs.topics[topicName] = newTopic(topicName, nil, gs)

	// 执行 graftPeers
	gs.graftPeers(topicName)

	// 只有已连接的节点应该被添加到 Mesh
	assert.True(t, gs.mesh.Has(topicName, "peer1-connected"))
	assert.False(t, gs.mesh.Has(topicName, "peer2-disconnected"))
	assert.False(t, gs.mesh.Has(topicName, "peer3-disconnected"))
}

// TestGossipSub_SubscribeDisconnectEvents 测试事件订阅（集成测试）
func TestGossipSub_SubscribeDisconnectEvents(t *testing.T) {
	// 此测试需要完整的 EventBus 实现
	// 这里只测试方法存在性和基本逻辑

	// 创建 mock EventBus（返回 nil subscription 来测试错误处理）
	mockEventBus := mocks.NewMockEventBus()
	mockEventBus.SubscribeFunc = func(eventType interface{}, opts ...interfaces.SubscriptionOpt) (interfaces.Subscription, error) {
		return nil, nil
	}

	mockHost := mocks.NewMockHost("local-peer")
	mockHost.EventBusFunc = func() interfaces.EventBus {
		return mockEventBus
	}

	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	gs.ctx = ctx

	// 启动订阅协程
	done := make(chan struct{})
	go func() {
		gs.subscribeDisconnectEvents()
		close(done)
	}()

	// 立即取消 context
	cancel()

	// 等待协程退出
	select {
	case <-done:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscribeDisconnectEvents did not exit after context cancel")
	}
}

// TestGossipSub_CleanupWithNilNetwork 测试 Network 为 nil 时的容错
func TestGossipSub_CleanupWithNilNetwork(t *testing.T) {
	// 创建 mock host，Network 返回 nil
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return nil
	}

	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
		mesh:   newMeshPeers(config.D, config.Dlo, config.Dhi),
		topics: make(map[string]*topic),
	}

	// 添加节点到 Mesh
	topicName := "test-topic"
	gs.mesh.Add(topicName, "peer1")
	gs.mesh.Add(topicName, "peer2")

	// 执行清理 - 不应该 panic
	gs.cleanupDisconnectedPeers(topicName)

	// 由于无法检查连接状态，节点应该保持不变
	assert.Equal(t, 2, gs.mesh.Count(topicName))
}

// TestGossipSub_FilterConnectedPeers_WithNilNetwork 测试过滤时 Network 为 nil
func TestGossipSub_FilterConnectedPeers_WithNilNetwork(t *testing.T) {
	mockHost := mocks.NewMockHost("local-peer")
	mockHost.NetworkFunc = func() interfaces.Swarm {
		return nil
	}

	config := DefaultConfig()
	gs := &gossipSub{
		host:   mockHost,
		config: config,
	}

	candidates := []string{"peer1", "peer2", "peer3"}
	filtered := gs.filterConnectedPeers(candidates)

	// 无法检查连接状态时，返回原列表
	require.Len(t, filtered, 3)
	assert.Equal(t, candidates, filtered)
}
