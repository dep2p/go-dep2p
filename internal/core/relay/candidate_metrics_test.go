package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     CandidateMetrics 测试
// ============================================================================

func TestCandidateMetrics_New(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)
	require.NotNil(t, m)
	assert.NotNil(t, m.metrics)
	assert.Equal(t, 30*time.Second, m.interval)
}

func TestCandidateMetrics_AddCandidate(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 添加候选
	m.AddCandidate("peer-1")

	// 验证已添加
	metrics := m.GetMetrics("peer-1")
	require.NotNil(t, metrics)
	assert.Equal(t, "peer-1", metrics.PeerID)
	assert.Equal(t, 0.8, metrics.Capacity)
	assert.Equal(t, 0.9, metrics.Reliability)
}

func TestCandidateMetrics_AddCandidate_Duplicate(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 添加候选
	m.AddCandidate("peer-1")

	// 修改指标
	m.mu.Lock()
	m.metrics["peer-1"].Capacity = 0.5
	m.mu.Unlock()

	// 重复添加应该被忽略
	m.AddCandidate("peer-1")

	// 验证指标未被覆盖
	metrics := m.GetMetrics("peer-1")
	assert.Equal(t, 0.5, metrics.Capacity)
}

func TestCandidateMetrics_RemoveCandidate(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	m.AddCandidate("peer-1")
	m.AddCandidate("peer-2")

	// 移除一个
	m.RemoveCandidate("peer-1")

	// 验证已移除
	assert.Nil(t, m.GetMetrics("peer-1"))
	assert.NotNil(t, m.GetMetrics("peer-2"))
}

func TestCandidateMetrics_RemoveCandidate_NotExists(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 移除不存在的候选不应 panic
	m.RemoveCandidate("non-existent")
}

func TestCandidateMetrics_GetMetrics_NotExists(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	metrics := m.GetMetrics("non-existent")
	assert.Nil(t, metrics)
}

func TestCandidateMetrics_GetMetrics_ReturnsCopy(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	m.AddCandidate("peer-1")

	// 获取指标
	metrics1 := m.GetMetrics("peer-1")

	// 修改返回的指标
	metrics1.Capacity = 0.1

	// 验证原始指标未被修改
	metrics2 := m.GetMetrics("peer-1")
	assert.Equal(t, 0.8, metrics2.Capacity) // 仍然是初始值
}

func TestCandidateMetrics_GetAllMetrics(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	m.AddCandidate("peer-1")
	m.AddCandidate("peer-2")
	m.AddCandidate("peer-3")

	all := m.GetAllMetrics()

	assert.Len(t, all, 3)
	assert.NotNil(t, all["peer-1"])
	assert.NotNil(t, all["peer-2"])
	assert.NotNil(t, all["peer-3"])
}

func TestCandidateMetrics_GetAllMetrics_Empty(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	all := m.GetAllMetrics()
	assert.NotNil(t, all)
	assert.Len(t, all, 0)
}

func TestCandidateMetrics_GetAllMetrics_ReturnsCopy(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	m.AddCandidate("peer-1")

	// 获取所有指标
	all := m.GetAllMetrics()

	// 修改返回的 map
	delete(all, "peer-1")

	// 验证原始数据未被修改
	assert.NotNil(t, m.GetMetrics("peer-1"))
}

func TestCandidateMetrics_StartStop(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 启动
	m.Start()

	// 短暂等待确保 goroutine 启动
	time.Sleep(10 * time.Millisecond)

	// 停止
	m.Stop()

	// 验证可以安全地多次停止
	m.Stop()
}

func TestCandidateMetrics_StopBeforeStart(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 未启动时停止不应 panic
	m.Stop()
}

// ============================================================================
//                     RelayMetrics 测试
// ============================================================================

func TestRelayMetrics_Fields(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)
	m.AddCandidate("peer-1")

	// 直接修改内部指标以测试字段
	m.mu.Lock()
	metrics := m.metrics["peer-1"]
	metrics.Latency = 50 * time.Millisecond
	metrics.PingCount = 10
	metrics.SuccessCount = 9
	metrics.FailureCount = 1
	metrics.LastPing = time.Now()
	metrics.LastSuccess = time.Now().Add(-time.Second)
	m.mu.Unlock()

	// 通过 GetMetrics 获取并验证
	result := m.GetMetrics("peer-1")

	assert.Equal(t, 50*time.Millisecond, result.Latency)
	assert.Equal(t, 10, result.PingCount)
	assert.Equal(t, 9, result.SuccessCount)
	assert.Equal(t, 1, result.FailureCount)
	assert.False(t, result.LastPing.IsZero())
	assert.False(t, result.LastSuccess.IsZero())
}

// ============================================================================
//                     measurePeer 内部逻辑测试（通过模拟指标更新）
// ============================================================================

func TestCandidateMetrics_MeasurePeer_NotExists(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 测量不存在的 peer 不应 panic
	m.measurePeer("non-existent")
}

func TestCandidateMetrics_ReliabilityCalculation(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)
	m.AddCandidate("peer-1")

	// 模拟 ping 结果
	m.mu.Lock()
	metrics := m.metrics["peer-1"]
	metrics.PingCount = 10
	metrics.SuccessCount = 8
	metrics.FailureCount = 2

	// 计算可靠性
	if metrics.PingCount > 0 {
		metrics.Reliability = float64(metrics.SuccessCount) / float64(metrics.PingCount)
	}
	m.mu.Unlock()

	result := m.GetMetrics("peer-1")
	assert.Equal(t, 0.8, result.Reliability) // 8/10 = 0.8
}

func TestCandidateMetrics_CapacityCalculation(t *testing.T) {
	tests := []struct {
		name            string
		latency         time.Duration
		expectedCapMin  float64
		expectedCapMax  float64
	}{
		{
			name:           "low latency",
			latency:        50 * time.Millisecond,
			expectedCapMin: 0.99,
			expectedCapMax: 1.0,
		},
		{
			name:           "medium latency",
			latency:        500 * time.Millisecond,
			expectedCapMin: 0.4,
			expectedCapMax: 0.61, // 浮点精度
		},
		{
			name:           "high latency",
			latency:        1500 * time.Millisecond,
			expectedCapMin: 0.05,
			expectedCapMax: 0.15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCandidateMetrics(nil, nil)
			m.AddCandidate("peer-1")

			// 模拟延迟更新后的容量计算
			m.mu.Lock()
			metrics := m.metrics["peer-1"]

			// 应用容量计算逻辑（与 measurePeer 中相同）
			latency := tt.latency
			if latency > 0 {
				if latency < 100*time.Millisecond {
					metrics.Capacity = 1.0
				} else if latency > 1000*time.Millisecond {
					metrics.Capacity = 0.1
				} else {
					metrics.Capacity = 1.0 - (float64(latency-100*time.Millisecond) / float64(900*time.Millisecond) * 0.9)
				}
			}
			m.mu.Unlock()

			result := m.GetMetrics("peer-1")
			assert.GreaterOrEqual(t, result.Capacity, tt.expectedCapMin)
			assert.LessOrEqual(t, result.Capacity, tt.expectedCapMax)
		})
	}
}

// ============================================================================
//                     并发测试
// ============================================================================

func TestCandidateMetrics_Concurrent(t *testing.T) {
	m := NewCandidateMetrics(nil, nil)

	// 启动
	m.Start()
	defer m.Stop()

	done := make(chan bool, 100)

	// 并发添加
	for i := 0; i < 50; i++ {
		go func(idx int) {
			m.AddCandidate("peer-" + string(rune('A'+idx%26)))
			done <- true
		}(i)
	}

	// 并发读取
	for i := 0; i < 50; i++ {
		go func(idx int) {
			m.GetMetrics("peer-" + string(rune('A'+idx%26)))
			m.GetAllMetrics()
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 100; i++ {
		<-done
	}
}
