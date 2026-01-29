package stability

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectionStabilityTracker_New 测试创建跟踪器
func TestConnectionStabilityTracker_New(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.transitions)
	assert.NotNil(t, tracker.flapping)
	assert.Equal(t, FlapWindowDuration, tracker.windowDuration)
	assert.Equal(t, FlapThreshold, tracker.threshold)
}

// TestConnectionStabilityTracker_SetConfig 测试配置设置
func TestConnectionStabilityTracker_SetConfig(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 设置新配置
	newWindow := 30 * time.Second
	newThreshold := 5
	newRecovery := 10 * time.Minute

	tracker.SetConfig(newWindow, newThreshold, newRecovery)

	assert.Equal(t, newWindow, tracker.windowDuration)
	assert.Equal(t, newThreshold, tracker.threshold)
	assert.Equal(t, newRecovery, tracker.recoveryTime)
}

// TestConnectionStabilityTracker_RecordTransition_NoFlapping 测试正常转换（不触发震荡）
func TestConnectionStabilityTracker_RecordTransition_NoFlapping(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 记录少于阈值的转换
	for i := 0; i < FlapThreshold-1; i++ {
		isNewFlapping := tracker.RecordTransition("peer1")
		assert.False(t, isNewFlapping)
	}

	// 不应该被标记为震荡
	assert.False(t, tracker.IsFlapping("peer1"))
	assert.Equal(t, FlapThreshold-1, tracker.GetTransitionCount("peer1"))
}

// TestConnectionStabilityTracker_RecordTransition_Flapping 测试触发震荡
func TestConnectionStabilityTracker_RecordTransition_Flapping(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 记录足够多的转换以触发震荡
	var isNewFlapping bool
	for i := 0; i < FlapThreshold; i++ {
		isNewFlapping = tracker.RecordTransition("peer1")
	}

	// 最后一次应该触发新的震荡状态
	assert.True(t, isNewFlapping)
	assert.True(t, tracker.IsFlapping("peer1"))
}

// TestConnectionStabilityTracker_FlappingRecovery 测试震荡恢复
func TestConnectionStabilityTracker_FlappingRecovery(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 使用较短的恢复时间进行测试
	tracker.SetConfig(60*time.Second, 3, 100*time.Millisecond)

	// 触发震荡
	for i := 0; i < 3; i++ {
		tracker.RecordTransition("peer1")
	}
	assert.True(t, tracker.IsFlapping("peer1"))

	// 等待恢复
	time.Sleep(150 * time.Millisecond)

	// 应该已经恢复
	assert.False(t, tracker.IsFlapping("peer1"))
}

// TestConnectionStabilityTracker_WindowExpiry 测试窗口过期
func TestConnectionStabilityTracker_WindowExpiry(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 使用较短的窗口进行测试
	tracker.SetConfig(100*time.Millisecond, 3, 5*time.Minute)

	// 记录两次转换
	tracker.RecordTransition("peer1")
	tracker.RecordTransition("peer1")
	assert.Equal(t, 2, tracker.GetTransitionCount("peer1"))

	// 等待窗口过期
	time.Sleep(150 * time.Millisecond)

	// 记录第三次转换（之前的两次应该已过期）
	isNewFlapping := tracker.RecordTransition("peer1")
	assert.False(t, isNewFlapping)
	assert.Equal(t, 1, tracker.GetTransitionCount("peer1"))
}

// TestConnectionStabilityTracker_ShouldSuppressStateChange 测试状态变更抑制
func TestConnectionStabilityTracker_ShouldSuppressStateChange(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 初始：不应抑制
	assert.False(t, tracker.ShouldSuppressStateChange("peer1"))

	// 触发震荡
	for i := 0; i < FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
	}

	// 震荡状态：应该抑制
	assert.True(t, tracker.ShouldSuppressStateChange("peer1"))
}

// TestConnectionStabilityTracker_GetFlappingPeers 测试获取震荡节点列表
func TestConnectionStabilityTracker_GetFlappingPeers(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 初始：无震荡节点
	assert.Empty(t, tracker.GetFlappingPeers())

	// 触发多个节点震荡
	for i := 0; i < FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
		tracker.RecordTransition("peer2")
	}

	flappingPeers := tracker.GetFlappingPeers()
	assert.Len(t, flappingPeers, 2)
	assert.Contains(t, flappingPeers, "peer1")
	assert.Contains(t, flappingPeers, "peer2")
}

// TestConnectionStabilityTracker_ResetPeer 测试重置节点
func TestConnectionStabilityTracker_ResetPeer(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 触发震荡
	for i := 0; i < FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
	}
	assert.True(t, tracker.IsFlapping("peer1"))

	// 重置
	tracker.ResetPeer("peer1")

	// 应该不再震荡
	assert.False(t, tracker.IsFlapping("peer1"))
	assert.Equal(t, 0, tracker.GetTransitionCount("peer1"))
}

// TestConnectionStabilityTracker_Cleanup 测试清理
func TestConnectionStabilityTracker_Cleanup(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 使用较短的时间进行测试
	tracker.SetConfig(100*time.Millisecond, 3, 100*time.Millisecond)

	// 触发震荡
	for i := 0; i < 3; i++ {
		tracker.RecordTransition("peer1")
	}
	assert.True(t, tracker.IsFlapping("peer1"))

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 清理
	tracker.Cleanup()

	// 验证已清理
	tracker.mu.RLock()
	_, hasFlapping := tracker.flapping["peer1"]
	tracker.mu.RUnlock()
	assert.False(t, hasFlapping)
}

// TestConnectionStabilityTracker_MultiplePeers 测试多节点
func TestConnectionStabilityTracker_MultiplePeers(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// peer1 触发震荡
	for i := 0; i < FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
	}

	// peer2 只有一次转换
	tracker.RecordTransition("peer2")

	// 验证
	assert.True(t, tracker.IsFlapping("peer1"))
	assert.False(t, tracker.IsFlapping("peer2"))
	assert.Equal(t, FlapThreshold, tracker.GetTransitionCount("peer1"))
	assert.Equal(t, 1, tracker.GetTransitionCount("peer2"))
}

// TestConnectionStabilityTracker_Concurrent 测试并发安全
func TestConnectionStabilityTracker_Concurrent(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	var wg sync.WaitGroup
	iterations := 100
	peers := []string{"peer1", "peer2", "peer3"}

	for _, peerID := range peers {
		peerID := peerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tracker.RecordTransition(peerID)
				_ = tracker.IsFlapping(peerID)
				_ = tracker.ShouldSuppressStateChange(peerID)
				_ = tracker.GetTransitionCount(peerID)
			}
		}()
	}

	// 同时进行清理
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			tracker.Cleanup()
			_ = tracker.GetFlappingPeers()
		}
	}()

	// 等待完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(5 * time.Second):
		require.Fail(t, "并发测试超时")
	}
}

// TestConnectionStabilityTracker_RepeatedFlapping 测试重复震荡
func TestConnectionStabilityTracker_RepeatedFlapping(t *testing.T) {
	tracker := NewConnectionStabilityTracker()

	// 第一次触发震荡
	for i := 0; i < FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
	}
	assert.True(t, tracker.IsFlapping("peer1"))

	// 继续记录转换（已经是震荡状态）
	for i := 0; i < FlapThreshold; i++ {
		isNewFlapping := tracker.RecordTransition("peer1")
		// 不应该再次标记为"新"震荡
		assert.False(t, isNewFlapping)
	}

	// 仍然是震荡状态
	assert.True(t, tracker.IsFlapping("peer1"))
}
