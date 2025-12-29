package connmgr

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              测试配置
// ============================================================================

func TestDefaultJitterConfig(t *testing.T) {
	config := DefaultJitterConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, 5*time.Second, config.ToleranceWindow)
	assert.Equal(t, 30*time.Second, config.StateHoldTime)
	assert.True(t, config.ReconnectEnabled)
	assert.Equal(t, time.Second, config.InitialReconnectDelay)
	assert.Equal(t, 60*time.Second, config.MaxReconnectDelay)
	assert.Equal(t, 5, config.MaxReconnectAttempts)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
}

func TestJitterConfig_Validate(t *testing.T) {
	// 测试无效值被修正
	config := JitterConfig{
		ToleranceWindow:       -1,
		StateHoldTime:         0,
		InitialReconnectDelay: -1,
		MaxReconnectDelay:     0,
		BackoffMultiplier:     0.5,
	}

	config.Validate()

	assert.Equal(t, 5*time.Second, config.ToleranceWindow)
	assert.Equal(t, 30*time.Second, config.StateHoldTime)
	assert.Equal(t, time.Second, config.InitialReconnectDelay)
	assert.Equal(t, 60*time.Second, config.MaxReconnectDelay)
	assert.Equal(t, 2.0, config.BackoffMultiplier)
}

// ============================================================================
//                              JitterTolerance 基本测试
// ============================================================================

func TestNewJitterTolerance(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	require.NotNil(t, jt)
	assert.NotNil(t, jt.disconnectedPeers)
	assert.NotNil(t, jt.stopCh)
}

func TestJitterTolerance_StartStop(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := jt.Start(ctx)
	require.NoError(t, err)

	// 等待启动
	time.Sleep(100 * time.Millisecond)

	err = jt.Stop()
	require.NoError(t, err)
}

func TestJitterTolerance_StartDisabled(t *testing.T) {
	config := DefaultJitterConfig()
	config.Enabled = false
	jt := NewJitterTolerance(config)

	ctx := context.Background()
	err := jt.Start(ctx)
	require.NoError(t, err)
}

// ============================================================================
//                              断连处理测试
// ============================================================================

func TestJitterTolerance_NotifyDisconnected(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 通知断连
	shouldRemove := jt.NotifyDisconnected(nodeID)
	assert.False(t, shouldRemove, "不应该立即移除")

	// 检查状态
	state, ok := jt.GetState(nodeID)
	assert.True(t, ok)
	assert.Equal(t, StateDisconnected, state)

	// 检查断连节点列表
	peers := jt.GetDisconnectedPeers()
	assert.Len(t, peers, 1)
	assert.Equal(t, nodeID, peers[0])
}

func TestJitterTolerance_NotifyDisconnected_Disabled(t *testing.T) {
	config := DefaultJitterConfig()
	config.Enabled = false
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 禁用时应该直接返回 true
	shouldRemove := jt.NotifyDisconnected(nodeID)
	assert.True(t, shouldRemove)
}

func TestJitterTolerance_NotifyDisconnected_Duplicate(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 第一次断连
	jt.NotifyDisconnected(nodeID)

	// 模拟重连尝试
	jt.mu.Lock()
	if state, ok := jt.disconnectedPeers[nodeID]; ok {
		state.ReconnectAttempts = 3
	}
	jt.mu.Unlock()

	// 再次断连
	shouldRemove := jt.NotifyDisconnected(nodeID)
	assert.False(t, shouldRemove)

	// 验证重连次数保留
	jt.mu.RLock()
	state := jt.disconnectedPeers[nodeID]
	jt.mu.RUnlock()
	assert.Equal(t, 3, state.ReconnectAttempts)
}

func TestJitterTolerance_NotifyReconnected(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 断连
	jt.NotifyDisconnected(nodeID)
	assert.Len(t, jt.GetDisconnectedPeers(), 1)

	// 重连
	jt.NotifyReconnected(nodeID)
	assert.Len(t, jt.GetDisconnectedPeers(), 0)

	// 状态应该是已连接
	state, ok := jt.GetState(nodeID)
	assert.False(t, ok)
	assert.Equal(t, StateConnected, state)
}

// ============================================================================
//                              ShouldRemove 测试
// ============================================================================

func TestJitterTolerance_ShouldRemove_NotDisconnected(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 未断连的节点不应该被移除
	shouldRemove := jt.ShouldRemove(nodeID)
	assert.False(t, shouldRemove)
}

func TestJitterTolerance_ShouldRemove_StateHoldTimeExceeded(t *testing.T) {
	config := DefaultJitterConfig()
	config.StateHoldTime = 100 * time.Millisecond
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 断连
	jt.NotifyDisconnected(nodeID)

	// 在状态保持时间内不应该移除
	assert.False(t, jt.ShouldRemove(nodeID))

	// 等待超过状态保持时间
	time.Sleep(150 * time.Millisecond)

	// 应该移除
	assert.True(t, jt.ShouldRemove(nodeID))
}

func TestJitterTolerance_ShouldRemove_MaxAttemptsExceeded(t *testing.T) {
	config := DefaultJitterConfig()
	config.MaxReconnectAttempts = 3
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 断连
	jt.NotifyDisconnected(nodeID)

	// 模拟多次重连失败
	jt.mu.Lock()
	jt.disconnectedPeers[nodeID].ReconnectAttempts = 3
	jt.mu.Unlock()

	// 应该移除
	assert.True(t, jt.ShouldRemove(nodeID))
}

func TestJitterTolerance_ShouldRemove_Disabled(t *testing.T) {
	config := DefaultJitterConfig()
	config.Enabled = false
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}

	// 禁用时总是返回 true
	assert.True(t, jt.ShouldRemove(nodeID))
}

// ============================================================================
//                              回调测试
// ============================================================================

func TestJitterTolerance_StateChangeCallback(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	var mu sync.Mutex
	states := make(map[types.NodeID]PeerJitterState)

	jt.SetStateChangeCallback(func(nodeID types.NodeID, state PeerJitterState) {
		mu.Lock()
		states[nodeID] = state
		mu.Unlock()
	})

	nodeID := types.NodeID{1, 2, 3, 4}

	// 断连
	jt.NotifyDisconnected(nodeID)

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	state := states[nodeID]
	mu.Unlock()
	assert.Equal(t, StateDisconnected, state)

	// 重连
	jt.NotifyReconnected(nodeID)

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	state = states[nodeID]
	mu.Unlock()
	assert.Equal(t, StateConnected, state)
}

func TestJitterTolerance_ReconnectCallback(t *testing.T) {
	config := DefaultJitterConfig()
	config.InitialReconnectDelay = 10 * time.Millisecond // 更短的初始延迟
	config.StateHoldTime = 10 * time.Second
	jt := NewJitterTolerance(config)

	var mu sync.Mutex
	reconnectAttempts := 0

	jt.SetReconnectCallback(func(ctx context.Context, nodeID types.NodeID) error {
		mu.Lock()
		reconnectAttempts++
		mu.Unlock()
		return errors.New("reconnect failed")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := jt.Start(ctx)
	require.NoError(t, err)

	nodeID := types.NodeID{1, 2, 3, 4}
	jt.NotifyDisconnected(nodeID)

	// 手动设置 NextReconnectAt 为过去时间以触发立即重连
	jt.mu.Lock()
	if state, ok := jt.disconnectedPeers[nodeID]; ok {
		state.NextReconnectAt = time.Now().Add(-time.Second)
	}
	jt.mu.Unlock()

	// 等待监控循环触发重连（监控循环每秒运行一次）
	time.Sleep(1200 * time.Millisecond)

	mu.Lock()
	attempts := reconnectAttempts
	mu.Unlock()

	assert.Greater(t, attempts, 0, "应该有重连尝试")

	jt.Stop()
}

// ============================================================================
//                              退避计算测试
// ============================================================================

func TestJitterTolerance_CalculateBackoff(t *testing.T) {
	config := DefaultJitterConfig()
	config.InitialReconnectDelay = time.Second
	config.MaxReconnectDelay = 30 * time.Second
	config.BackoffMultiplier = 2.0
	jt := NewJitterTolerance(config)

	// 第一次: 1s
	assert.Equal(t, time.Second, jt.calculateBackoff(1))

	// 第二次: 2s
	assert.Equal(t, 2*time.Second, jt.calculateBackoff(2))

	// 第三次: 4s
	assert.Equal(t, 4*time.Second, jt.calculateBackoff(3))

	// 第四次: 8s
	assert.Equal(t, 8*time.Second, jt.calculateBackoff(4))

	// 第五次: 16s
	assert.Equal(t, 16*time.Second, jt.calculateBackoff(5))

	// 超过最大值应该被限制
	assert.Equal(t, 30*time.Second, jt.calculateBackoff(10))
}

func TestJitterTolerance_CalculateBackoff_ZeroAttempt(t *testing.T) {
	config := DefaultJitterConfig()
	config.InitialReconnectDelay = time.Second
	jt := NewJitterTolerance(config)

	// 0 或负数返回初始延迟
	assert.Equal(t, time.Second, jt.calculateBackoff(0))
	assert.Equal(t, time.Second, jt.calculateBackoff(-1))
}

// ============================================================================
//                              统计测试
// ============================================================================

func TestJitterTolerance_Stats(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	// 初始统计
	stats := jt.Stats()
	assert.Equal(t, 0, stats.TotalDisconnected)

	// 添加断连节点
	nodeID1 := types.NodeID{1}
	nodeID2 := types.NodeID{2}
	jt.NotifyDisconnected(nodeID1)
	jt.NotifyDisconnected(nodeID2)

	// 修改一个节点的状态
	jt.mu.Lock()
	jt.disconnectedPeers[nodeID1].State = StateReconnecting
	jt.disconnectedPeers[nodeID1].ReconnectAttempts = 2
	jt.disconnectedPeers[nodeID2].State = StateHeld
	jt.disconnectedPeers[nodeID2].ReconnectAttempts = 3
	jt.mu.Unlock()

	// 验证统计
	stats = jt.Stats()
	assert.Equal(t, 2, stats.TotalDisconnected)
	assert.Equal(t, 1, stats.Reconnecting)
	assert.Equal(t, 1, stats.Held)
	assert.Equal(t, 5, stats.TotalReconnectAttempts)
}

// ============================================================================
//                              PeerJitterState 测试
// ============================================================================

func TestPeerJitterState_String(t *testing.T) {
	tests := []struct {
		state    PeerJitterState
		expected string
	}{
		{StateConnected, "connected"},
		{StateDisconnected, "disconnected"},
		{StateReconnecting, "reconnecting"},
		{StateHeld, "held"},
		{StateRemoved, "removed"},
		{PeerJitterState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestJitterTolerance_ConcurrentAccess(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := jt.Start(ctx)
	require.NoError(t, err)
	defer jt.Stop()

	var wg sync.WaitGroup
	nodeCount := 100

	// 并发断连
	for i := 0; i < nodeCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := types.NodeID{}
			nodeID[0] = byte(id)
			jt.NotifyDisconnected(nodeID)
		}(i)
	}

	// 并发查询
	for i := 0; i < nodeCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := types.NodeID{}
			nodeID[0] = byte(id)
			jt.GetState(nodeID)
			jt.ShouldRemove(nodeID)
		}(i)
	}

	// 并发重连
	for i := 0; i < nodeCount/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := types.NodeID{}
			nodeID[0] = byte(id)
			jt.NotifyReconnected(nodeID)
		}(i)
	}

	wg.Wait()

	// 统计
	stats := jt.Stats()
	assert.LessOrEqual(t, stats.TotalDisconnected, nodeCount)
}

// ============================================================================
//                              ProcessDisconnectedPeers 测试
// ============================================================================

func TestJitterTolerance_ProcessDisconnectedPeers_RemovesExpired(t *testing.T) {
	config := DefaultJitterConfig()
	config.StateHoldTime = 50 * time.Millisecond
	config.ReconnectEnabled = false // 禁用重连以简化测试
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}
	jt.NotifyDisconnected(nodeID)

	// 等待超过状态保持时间
	time.Sleep(100 * time.Millisecond)

	// 手动触发处理
	ctx := context.Background()
	jt.processDisconnectedPeers(ctx)

	// 节点应该被移除
	assert.Len(t, jt.GetDisconnectedPeers(), 0)
}

func TestJitterTolerance_ProcessDisconnectedPeers_RemovesMaxAttempts(t *testing.T) {
	config := DefaultJitterConfig()
	config.MaxReconnectAttempts = 3
	config.ReconnectEnabled = false // 禁用重连以简化测试
	jt := NewJitterTolerance(config)

	nodeID := types.NodeID{1, 2, 3, 4}
	jt.NotifyDisconnected(nodeID)

	// 模拟超过最大重连次数
	jt.mu.Lock()
	jt.disconnectedPeers[nodeID].ReconnectAttempts = 3
	jt.mu.Unlock()

	// 手动触发处理
	ctx := context.Background()
	jt.processDisconnectedPeers(ctx)

	// 节点应该被移除
	assert.Len(t, jt.GetDisconnectedPeers(), 0)
}

// ============================================================================
//                              新增测试 - 审查修复验证
// ============================================================================

// TestJitterTolerance_StopMultipleTimes 测试 Stop 可以安全地多次调用
func TestJitterTolerance_StopMultipleTimes(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	// 启动
	ctx := context.Background()
	err := jt.Start(ctx)
	require.NoError(t, err)

	// 多次调用 Stop 不应该 panic
	for i := 0; i < 5; i++ {
		err := jt.Stop()
		assert.NoError(t, err, "第 %d 次调用 Stop 不应该返回错误", i+1)
	}
}

// TestJitterTolerance_CallbacksThreadSafe 测试回调设置的线程安全性
func TestJitterTolerance_CallbacksThreadSafe(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)

	var wg sync.WaitGroup
	const goroutines = 10
	const iterations = 100

	// 并发设置回调
	for i := 0; i < goroutines; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				jt.SetReconnectCallback(func(ctx context.Context, nodeID types.NodeID) error {
					return nil
				})
			}
		}(i)

		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				jt.SetStateChangeCallback(func(nodeID types.NodeID, state PeerJitterState) {
					// do nothing
				})
			}
		}(i)
	}

	// 并发通知断连（会触发回调读取）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < iterations; j++ {
			nodeID := types.NodeID{byte(j % 256)}
			jt.NotifyDisconnected(nodeID)
		}
	}()

	// 等待所有 goroutine 完成，不应该发生 panic 或数据竞争
	wg.Wait()
}

