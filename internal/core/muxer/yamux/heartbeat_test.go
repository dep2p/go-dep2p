package yamux

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultHeartbeatConfig(t *testing.T) {
	config := DefaultHeartbeatConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, 30*time.Second, config.Interval)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxMissed)
}

func TestHeartbeatConfig_Validate(t *testing.T) {
	config := HeartbeatConfig{
		Interval:  -1,
		Timeout:   0,
		MaxMissed: -1,
	}

	config.Validate()

	assert.Equal(t, 30*time.Second, config.Interval)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxMissed)
}

// ============================================================================
//                              HeartbeatMonitor 基本测试
// ============================================================================

func TestNewHeartbeatMonitor(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	require.NotNil(t, monitor)
	assert.Equal(t, 0, monitor.ConnectionCount())
}

func TestHeartbeatMonitor_StartStop(t *testing.T) {
	config := DefaultHeartbeatConfig()
	config.Interval = 100 * time.Millisecond
	monitor := NewHeartbeatMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// 等待启动
	time.Sleep(50 * time.Millisecond)

	err = monitor.Stop()
	require.NoError(t, err)
}

func TestHeartbeatMonitor_StartDisabled(t *testing.T) {
	config := DefaultHeartbeatConfig()
	config.Enabled = false
	monitor := NewHeartbeatMonitor(config)

	ctx := context.Background()
	err := monitor.Start(ctx)
	require.NoError(t, err)
}

func TestHeartbeatMonitor_DoubleStart(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	ctx := context.Background()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// 第二次启动应该是幂等的
	err = monitor.Start(ctx)
	require.NoError(t, err)

	monitor.Stop()
}

func TestHeartbeatMonitor_DoubleStop(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	ctx := context.Background()
	err := monitor.Start(ctx)
	require.NoError(t, err)

	err = monitor.Stop()
	require.NoError(t, err)

	// 第二次停止应该是幂等的
	err = monitor.Stop()
	require.NoError(t, err)
}

// ============================================================================
//                              连接管理测试
// ============================================================================

func TestHeartbeatMonitor_AddConnection(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	monitor.AddConnection("conn1")
	assert.Equal(t, 1, monitor.ConnectionCount())

	monitor.AddConnection("conn2")
	assert.Equal(t, 2, monitor.ConnectionCount())
}

func TestHeartbeatMonitor_RemoveConnection(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	monitor.AddConnection("conn1")
	monitor.AddConnection("conn2")
	assert.Equal(t, 2, monitor.ConnectionCount())

	monitor.RemoveConnection("conn1")
	assert.Equal(t, 1, monitor.ConnectionCount())

	// 移除不存在的连接不应该出错
	monitor.RemoveConnection("nonexistent")
	assert.Equal(t, 1, monitor.ConnectionCount())
}

// ============================================================================
//                              心跳记录测试
// ============================================================================

func TestHeartbeatMonitor_RecordHeartbeat(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 模拟发送 ping
	monitor.RecordPingSent(connID)

	// 等待一小段时间
	time.Sleep(50 * time.Millisecond)

	// 记录心跳响应
	monitor.RecordHeartbeat(connID)

	// 检查延迟
	latency, ok := monitor.GetLatency(connID)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, latency, 50*time.Millisecond)
}

func TestHeartbeatMonitor_RecordHeartbeat_ResetsMissedCount(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 手动设置丢失次数
	monitor.connectionsMu.Lock()
	monitor.connections[connID].missedCount = 2
	monitor.connectionsMu.Unlock()

	// 记录心跳
	monitor.RecordHeartbeat(connID)

	// 检查丢失次数已重置
	missedCount, ok := monitor.GetMissedCount(connID)
	assert.True(t, ok)
	assert.Equal(t, 0, missedCount)
}

func TestHeartbeatMonitor_RecordHeartbeat_NonExistent(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	// 对不存在的连接记录心跳不应该 panic
	monitor.RecordHeartbeat("nonexistent")
	monitor.RecordPingSent("nonexistent")
}

// ============================================================================
//                              心跳回调测试
// ============================================================================

func TestHeartbeatMonitor_OnHeartbeatCallback(t *testing.T) {
	var mu sync.Mutex
	callCount := 0
	var lastLatency time.Duration

	config := DefaultHeartbeatConfig()
	config.OnHeartbeat = func(connID string, latency time.Duration) {
		mu.Lock()
		callCount++
		lastLatency = latency
		mu.Unlock()
	}
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 模拟心跳
	monitor.RecordPingSent(connID)
	time.Sleep(10 * time.Millisecond)
	monitor.RecordHeartbeat(connID)

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	assert.Greater(t, lastLatency, time.Duration(0))
	mu.Unlock()
}

func TestHeartbeatMonitor_OnTimeoutCallback(t *testing.T) {
	var mu sync.Mutex
	timedOutConns := []string{}

	config := HeartbeatConfig{
		Enabled:   true,
		Interval:  50 * time.Millisecond,
		Timeout:   20 * time.Millisecond,
		MaxMissed: 2,
		OnTimeout: func(connID string) {
			mu.Lock()
			timedOutConns = append(timedOutConns, connID)
			mu.Unlock()
		},
	}
	monitor := NewHeartbeatMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)
	defer monitor.Stop()

	connID := "conn1"
	monitor.AddConnection(connID)

	// 模拟多次发送 ping 但没有响应
	monitor.RecordPingSent(connID)
	time.Sleep(30 * time.Millisecond) // 超过 timeout

	monitor.connectionsMu.Lock()
	monitor.connections[connID].missedCount = 1 // 手动设置丢失次数
	monitor.connectionsMu.Unlock()

	monitor.RecordPingSent(connID)
	time.Sleep(30 * time.Millisecond)

	// 手动触发检查
	monitor.checkConnections()

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Contains(t, timedOutConns, connID)
	mu.Unlock()
}

// ============================================================================
//                              查询方法测试
// ============================================================================

func TestHeartbeatMonitor_GetLatency(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 初始延迟应该是 0
	latency, ok := monitor.GetLatency(connID)
	assert.True(t, ok)
	assert.Equal(t, time.Duration(0), latency)

	// 不存在的连接
	_, ok = monitor.GetLatency("nonexistent")
	assert.False(t, ok)
}

func TestHeartbeatMonitor_GetMissedCount(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 初始丢失次数应该是 0
	missedCount, ok := monitor.GetMissedCount(connID)
	assert.True(t, ok)
	assert.Equal(t, 0, missedCount)

	// 不存在的连接
	_, ok = monitor.GetMissedCount("nonexistent")
	assert.False(t, ok)
}

func TestHeartbeatMonitor_GetLastSeen(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	beforeAdd := time.Now()
	monitor.AddConnection(connID)

	lastSeen, ok := monitor.GetLastSeen(connID)
	assert.True(t, ok)
	assert.True(t, lastSeen.After(beforeAdd) || lastSeen.Equal(beforeAdd))

	// 不存在的连接
	_, ok = monitor.GetLastSeen("nonexistent")
	assert.False(t, ok)
}

// ============================================================================
//                              统计信息测试
// ============================================================================

func TestHeartbeatMonitor_Stats(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	// 初始统计
	stats := monitor.Stats()
	assert.Equal(t, 0, stats.TotalConnections)

	// 添加连接
	monitor.AddConnection("conn1")
	monitor.AddConnection("conn2")

	// 设置一些状态
	monitor.connectionsMu.Lock()
	monitor.connections["conn1"].lastLatency = 100 * time.Millisecond
	monitor.connections["conn2"].lastLatency = 200 * time.Millisecond
	monitor.connections["conn2"].missedCount = 1
	monitor.connectionsMu.Unlock()

	stats = monitor.Stats()
	assert.Equal(t, 2, stats.TotalConnections)
	assert.Equal(t, 1, stats.ConnectionsWithMissed)
	assert.Equal(t, 150*time.Millisecond, stats.AvgLatency)
	assert.Equal(t, 200*time.Millisecond, stats.MaxLatency)
	assert.Equal(t, 100*time.Millisecond, stats.MinLatency)
}

func TestHeartbeatMonitor_Stats_NoLatency(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	monitor.AddConnection("conn1")

	stats := monitor.Stats()
	assert.Equal(t, 1, stats.TotalConnections)
	assert.Equal(t, time.Duration(0), stats.AvgLatency)
}

// ============================================================================
//                              检查连接测试
// ============================================================================

func TestHeartbeatMonitor_CheckConnections(t *testing.T) {
	config := HeartbeatConfig{
		Enabled:   true,
		Interval:  time.Second,
		Timeout:   50 * time.Millisecond,
		MaxMissed: 2,
	}
	monitor := NewHeartbeatMonitor(config)

	connID := "conn1"
	monitor.AddConnection(connID)

	// 模拟发送 ping
	monitor.RecordPingSent(connID)

	// 等待超时
	time.Sleep(100 * time.Millisecond)

	// 检查连接
	monitor.checkConnections()

	// 验证丢失次数增加
	missedCount, _ := monitor.GetMissedCount(connID)
	assert.Equal(t, 1, missedCount)
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestHeartbeatMonitor_ConcurrentAccess(t *testing.T) {
	config := HeartbeatConfig{
		Enabled:  true,
		Interval: 50 * time.Millisecond,
		Timeout:  100 * time.Millisecond,
	}
	monitor := NewHeartbeatMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)
	defer monitor.Stop()

	var wg sync.WaitGroup
	n := 50

	// 并发添加连接
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			connID := "conn" + string(rune('0'+id%10))
			monitor.AddConnection(connID)
		}(i)
	}

	// 并发记录心跳
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			connID := "conn" + string(rune('0'+id%10))
			monitor.RecordPingSent(connID)
			monitor.RecordHeartbeat(connID)
		}(i)
	}

	// 并发查询
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			connID := "conn" + string(rune('0'+id%10))
			monitor.GetLatency(connID)
			monitor.GetMissedCount(connID)
			monitor.GetLastSeen(connID)
		}(i)
	}

	// 并发移除
	for i := 0; i < n/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			connID := "conn" + string(rune('0'+id%10))
			monitor.RemoveConnection(connID)
		}(i)
	}

	wg.Wait()

	// 获取统计验证没有数据竞争
	_ = monitor.Stats()
}

// ============================================================================
//                              HeartbeatStats 测试
// ============================================================================

func TestHeartbeatStats(t *testing.T) {
	stats := HeartbeatStats{
		TotalConnections:      10,
		ConnectionsWithMissed: 2,
		AvgLatency:            50 * time.Millisecond,
		MaxLatency:            100 * time.Millisecond,
		MinLatency:            20 * time.Millisecond,
	}

	assert.Equal(t, 10, stats.TotalConnections)
	assert.Equal(t, 2, stats.ConnectionsWithMissed)
	assert.Equal(t, 50*time.Millisecond, stats.AvgLatency)
	assert.Equal(t, 100*time.Millisecond, stats.MaxLatency)
	assert.Equal(t, 20*time.Millisecond, stats.MinLatency)
}

// ============================================================================
//                              针对性修复测试
// ============================================================================

func TestHeartbeatMonitor_StopMultiple_NoPanic(t *testing.T) {
	config := DefaultHeartbeatConfig()
	config.Interval = 100 * time.Millisecond
	monitor := NewHeartbeatMonitor(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := monitor.Start(ctx)
	require.NoError(t, err)

	// 多次调用 Stop 不应该 panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitor.Stop()
		}()
	}
	wg.Wait()

	// 再次调用也不应该 panic
	err = monitor.Stop()
	assert.NoError(t, err)
}

func TestHeartbeatMonitor_StopWithoutStart(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config)

	// 未启动时调用 Stop 不应该 panic
	err := monitor.Stop()
	assert.NoError(t, err)

	// 多次调用也不应该 panic
	err = monitor.Stop()
	assert.NoError(t, err)
}

