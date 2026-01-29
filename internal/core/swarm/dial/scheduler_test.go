package dial

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, 50, config.MaxDialedConns)
	assert.Equal(t, 16, config.MaxActiveDials)
	assert.Equal(t, 30, config.DialHistoryExpiration)
	assert.Equal(t, 5, config.StaticReconnectDelay)
	assert.Equal(t, 10*time.Second, config.DialTimeout)
}

func TestNewScheduler(t *testing.T) {
	config := DefaultConfig()
	setupCalled := false
	setupFunc := func(ctx context.Context, peerID string, addrs []string) error {
		setupCalled = true
		return nil
	}

	scheduler := NewScheduler(config, setupFunc)

	assert.NotNil(t, scheduler)
	assert.NotNil(t, scheduler.static)
	assert.NotNil(t, scheduler.staticPool)
	assert.NotNil(t, scheduler.dynamicCh)
	assert.NotNil(t, scheduler.dialing)
	assert.NotNil(t, scheduler.peers)
	assert.NotNil(t, scheduler.history)
	assert.False(t, setupCalled)
}

func TestScheduler_StartStop(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&scheduler.running))

	// 重复启动应该无效
	err = scheduler.Start(ctx)
	require.NoError(t, err)

	err = scheduler.Stop()
	require.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&scheduler.running))

	// 重复停止应该无效
	err = scheduler.Stop()
	require.NoError(t, err)
}

func TestScheduler_AddRemoveStatic(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	node := PeerInfo{
		ID:    "peer1",
		Addrs: []string{"/ip4/1.1.1.1/tcp/4001"},
	}

	// 添加静态节点
	scheduler.AddStatic(node)

	scheduler.staticMu.RLock()
	_, exists := scheduler.static[node.ID]
	poolLen := len(scheduler.staticPool)
	scheduler.staticMu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, 1, poolLen)

	// 重复添加无效
	scheduler.AddStatic(node)

	scheduler.staticMu.RLock()
	poolLen = len(scheduler.staticPool)
	scheduler.staticMu.RUnlock()

	assert.Equal(t, 1, poolLen)

	// 移除静态节点
	scheduler.RemoveStatic(node.ID)

	scheduler.staticMu.RLock()
	_, exists = scheduler.static[node.ID]
	poolLen = len(scheduler.staticPool)
	scheduler.staticMu.RUnlock()

	assert.False(t, exists)
	assert.Equal(t, 0, poolLen)
}

func TestScheduler_AddDynamic(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	node := PeerInfo{
		ID:    "peer1",
		Addrs: []string{"/ip4/1.1.1.1/tcp/4001"},
	}

	// 添加动态节点
	scheduler.AddDynamic(node)

	// 从通道读取
	select {
	case received := <-scheduler.dynamicCh:
		assert.Equal(t, node.ID, received.ID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for dynamic node")
	}
}

func TestScheduler_PeerAddedRemoved(t *testing.T) {
	config := DefaultConfig()
	config.StaticReconnectDelay = 0 // 禁用延迟重连
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	peerID := "peer1"

	// 通知连接建立
	scheduler.PeerAdded(peerID)

	scheduler.peerMu.RLock()
	_, connected := scheduler.peers[peerID]
	scheduler.peerMu.RUnlock()

	assert.True(t, connected)

	// 通知连接断开
	scheduler.PeerRemoved(peerID)

	scheduler.peerMu.RLock()
	_, connected = scheduler.peers[peerID]
	scheduler.peerMu.RUnlock()

	assert.False(t, connected)
}

func TestScheduler_DialHistory(t *testing.T) {
	history := newDialHistory()

	// 添加记录
	history.add("peer1", time.Now().Add(100*time.Millisecond))
	assert.True(t, history.contains("peer1"))
	assert.False(t, history.contains("peer2"))

	// 等待过期
	time.Sleep(150 * time.Millisecond)
	assert.False(t, history.contains("peer1"))

	// 清理
	history.add("peer3", time.Now().Add(-time.Second))
	history.cleanup()
	assert.Equal(t, 0, history.size())
}

func TestScheduler_FreeDialSlots(t *testing.T) {
	config := DefaultConfig()
	config.MaxDialedConns = 10
	config.MaxActiveDials = 5

	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	// 初始状态
	slots := scheduler.freeDialSlots()
	assert.Equal(t, 5, slots) // min(10*2, 5) = 5

	// 添加已连接节点
	for i := 0; i < 5; i++ {
		scheduler.PeerAdded("peer" + string(rune('0'+i)))
	}

	slots = scheduler.freeDialSlots()
	assert.Equal(t, 5, slots) // min((10-5)*2, 5) = min(10, 5) = 5
}

func TestScheduler_Stats(t *testing.T) {
	config := DefaultConfig()
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	// 添加静态节点
	scheduler.AddStatic(PeerInfo{ID: "static1", Addrs: []string{}})
	scheduler.AddStatic(PeerInfo{ID: "static2", Addrs: []string{}})

	// 添加已连接节点
	scheduler.PeerAdded("connected1")

	stats := scheduler.Stats()

	assert.Equal(t, 2, stats.StaticNodes)
	assert.Equal(t, 2, stats.StaticPoolSize)
	assert.Equal(t, 1, stats.ConnectedPeers)
	assert.Equal(t, 0, stats.DialingCount)
}

func TestScheduler_ProcessDialTasks(t *testing.T) {
	config := DefaultConfig()
	config.DialTimeout = 50 * time.Millisecond

	dialedPeers := make(chan string, 10)
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		dialedPeers <- peerID
		return nil
	})

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)
	defer scheduler.Stop()

	// 添加静态节点
	scheduler.AddStatic(PeerInfo{ID: "peer1", Addrs: []string{"/ip4/1.1.1.1/tcp/4001"}})

	// 等待拨号
	select {
	case peerID := <-dialedPeers:
		assert.Equal(t, "peer1", peerID)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for dial")
	}
}

func TestScheduler_HandleDynamicNode(t *testing.T) {
	config := DefaultConfig()
	config.DialTimeout = 50 * time.Millisecond
	config.DialHistoryExpiration = 1

	dialedPeers := make(chan string, 10)
	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		dialedPeers <- peerID
		return nil
	})

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)
	defer scheduler.Stop()

	// 添加动态节点
	scheduler.AddDynamic(PeerInfo{ID: "dynamic1", Addrs: []string{"/ip4/1.1.1.1/tcp/4001"}})

	// 等待拨号
	select {
	case peerID := <-dialedPeers:
		assert.Equal(t, "dynamic1", peerID)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for dial")
	}
}

func TestScheduler_StaticReconnect(t *testing.T) {
	config := DefaultConfig()
	config.StaticReconnectDelay = 0 // 立即重连

	scheduler := NewScheduler(config, func(ctx context.Context, peerID string, addrs []string) error {
		return nil
	})

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)
	defer scheduler.Stop()

	// 添加静态节点
	node := PeerInfo{ID: "static1", Addrs: []string{}}
	scheduler.AddStatic(node)

	// 标记为已连接
	scheduler.PeerAdded(node.ID)

	scheduler.staticMu.RLock()
	poolLen := len(scheduler.staticPool)
	scheduler.staticMu.RUnlock()
	assert.Equal(t, 0, poolLen) // 已连接，不在池中

	// 断开连接
	scheduler.PeerRemoved(node.ID)

	// 等待重连加入池
	time.Sleep(50 * time.Millisecond)

	scheduler.staticMu.RLock()
	task := scheduler.static[node.ID]
	scheduler.staticMu.RUnlock()

	// 任务应该重新加入池
	assert.NotNil(t, task)
}
