package member

import (
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAntiFalsePositive_New 测试创建防误判机制
func TestAntiFalsePositive_New(t *testing.T) {
	afp := NewAntiFalsePositive(nil)

	assert.NotNil(t, afp)
	assert.NotNil(t, afp.memberStates)
	assert.NotNil(t, afp.stabilityTracker)
	assert.NotNil(t, afp.protectionTracker)
}

// TestAntiFalsePositive_NewWithConfig 测试带配置创建
func TestAntiFalsePositive_NewWithConfig(t *testing.T) {
	config := &AntiFalsePositiveConfig{
		GracePeriod:        10 * time.Second,
		FlapWindow:         30 * time.Second,
		FlapThreshold:      5,
		ProtectionDuration: 1 * time.Minute,
	}

	afp := NewAntiFalsePositive(config)

	assert.NotNil(t, afp)
	assert.Equal(t, 10*time.Second, afp.gracePeriod)
	assert.Equal(t, 30*time.Second, afp.flapWindow)
	assert.Equal(t, 5, afp.flapThreshold)
	assert.Equal(t, 1*time.Minute, afp.protectionDuration)
}

// TestAntiFalsePositive_OnPeerDisconnected_GracePeriod 测试断开进入宽限期
func TestAntiFalsePositive_OnPeerDisconnected_GracePeriod(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 触发断开
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("peer1", "realm1")

	// 应该进入宽限期而不是立即移除
	assert.False(t, shouldRemove)
	assert.True(t, inGracePeriod)
	assert.True(t, afp.IsInGracePeriod("peer1"))
}

// TestAntiFalsePositive_OnPeerReconnected_Recovered 测试宽限期内重连恢复
func TestAntiFalsePositive_OnPeerReconnected_Recovered(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 触发断开
	afp.OnPeerDisconnected("peer1", "realm1")
	assert.True(t, afp.IsInGracePeriod("peer1"))

	// 宽限期内重连
	recovered, suppressed := afp.OnPeerReconnected("peer1")

	assert.True(t, recovered)
	assert.False(t, suppressed)
	assert.False(t, afp.IsInGracePeriod("peer1"))
}

// TestAntiFalsePositive_GraceTimeout_Callback 测试宽限期超时回调
func TestAntiFalsePositive_GraceTimeout_Callback(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 设置回调
	var callbackCalled bool
	var callbackPeerID string
	var mu sync.Mutex

	afp.SetOnMemberRemove(func(peerID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
		callbackPeerID = peerID
	})

	// 触发断开
	afp.OnPeerDisconnected("peer1", "realm1")

	// 手动触发超时（通过内部状态机）
	afp.mu.RLock()
	state := afp.memberStates["peer1"]
	afp.mu.RUnlock()

	state.onGraceTimeoutInternal()

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	// 验证回调
	mu.Lock()
	assert.True(t, callbackCalled)
	assert.Equal(t, "peer1", callbackPeerID)
	mu.Unlock()

	// 应该被记录到保护期
	assert.True(t, afp.IsProtected("peer1"))
}

// TestAntiFalsePositive_Flapping_Suppression 测试震荡抑制
func TestAntiFalsePositive_Flapping_Suppression(t *testing.T) {
	config := &AntiFalsePositiveConfig{
		GracePeriod:        15 * time.Second,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: 30 * time.Second,
	}
	afp := NewAntiFalsePositive(config)
	defer afp.Close()

	// 模拟频繁断开重连（触发震荡）
	for i := 0; i < 3; i++ {
		afp.OnPeerDisconnected("peer1", "realm1")
		afp.OnPeerReconnected("peer1")
	}

	// 应该被标记为震荡
	assert.True(t, afp.IsFlapping("peer1"))

	// 再次断开应该被抑制
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("peer1", "realm1")
	assert.False(t, shouldRemove)
	assert.False(t, inGracePeriod) // 被抑制，不进入宽限期
}

// TestAntiFalsePositive_ShouldRejectAdd_Protected 测试保护期拒绝添加
func TestAntiFalsePositive_ShouldRejectAdd_Protected(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 初始：不拒绝
	reject, reason := afp.ShouldRejectAdd("peer1")
	assert.False(t, reject)
	assert.Empty(t, reason)

	// 手动记录到保护期
	afp.protectionTracker.OnMemberRemoved("peer1")

	// 应该拒绝
	reject, reason = afp.ShouldRejectAdd("peer1")
	assert.True(t, reject)
	assert.Contains(t, reason, "protection")
}

// TestAntiFalsePositive_ShouldRejectAdd_Flapping 测试震荡状态拒绝添加
func TestAntiFalsePositive_ShouldRejectAdd_Flapping(t *testing.T) {
	config := &AntiFalsePositiveConfig{
		GracePeriod:        15 * time.Second,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: 30 * time.Second,
	}
	afp := NewAntiFalsePositive(config)
	defer afp.Close()

	// 触发震荡
	for i := 0; i < 3; i++ {
		afp.OnPeerDisconnected("peer1", "realm1")
		afp.OnPeerReconnected("peer1")
	}

	// 应该拒绝
	reject, reason := afp.ShouldRejectAdd("peer1")
	assert.True(t, reject)
	assert.Contains(t, reason, "flapping")
}

// TestAntiFalsePositive_ClearProtection 测试清除保护状态
func TestAntiFalsePositive_ClearProtection(t *testing.T) {
	config := &AntiFalsePositiveConfig{
		GracePeriod:        15 * time.Second,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: 30 * time.Second,
	}
	afp := NewAntiFalsePositive(config)
	defer afp.Close()

	// 触发震荡和保护
	for i := 0; i < 3; i++ {
		afp.OnPeerDisconnected("peer1", "realm1")
		afp.OnPeerReconnected("peer1")
	}
	afp.protectionTracker.OnMemberRemoved("peer1")

	// 验证状态
	assert.True(t, afp.IsFlapping("peer1"))
	assert.True(t, afp.IsProtected("peer1"))

	// 清除保护
	afp.ClearProtection("peer1")

	// 应该都被清除
	assert.False(t, afp.IsFlapping("peer1"))
	assert.False(t, afp.IsProtected("peer1"))
}

// TestAntiFalsePositive_OnCommunication 测试通信事件
func TestAntiFalsePositive_OnCommunication(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 触发断开
	afp.OnPeerDisconnected("peer1", "realm1")

	// 获取状态机
	afp.mu.RLock()
	state := afp.memberStates["peer1"]
	afp.mu.RUnlock()

	// 通信应该延长宽限期
	assert.Equal(t, 0, state.extensions)
	afp.OnCommunication("peer1")
	assert.Equal(t, 1, state.extensions)
}

// TestAntiFalsePositive_GetState 测试获取连接状态
func TestAntiFalsePositive_GetState(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 初始：默认已连接
	state := afp.GetState("peer1")
	assert.Equal(t, types.ConnStateConnected, state)

	// 触发断开
	afp.OnPeerDisconnected("peer1", "realm1")

	// 断开中
	state = afp.GetState("peer1")
	assert.Equal(t, types.ConnStateDisconnecting, state)
}

// TestAntiFalsePositive_GetStats 测试获取统计信息
func TestAntiFalsePositive_GetStats(t *testing.T) {
	config := &AntiFalsePositiveConfig{
		GracePeriod:        15 * time.Second,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: 30 * time.Second,
	}
	afp := NewAntiFalsePositive(config)
	defer afp.Close()

	// 触发各种状态
	afp.OnPeerDisconnected("peer1", "realm1") // 宽限期

	for i := 0; i < 3; i++ {
		afp.OnPeerDisconnected("peer2", "realm1")
		afp.OnPeerReconnected("peer2")
	} // 震荡

	afp.protectionTracker.OnMemberRemoved("peer3") // 保护期

	// 获取统计
	stats := afp.GetStats()

	assert.Equal(t, 1, stats.InGracePeriod)
	assert.Contains(t, stats.FlappingPeers, "peer2")
	assert.Contains(t, stats.ProtectedPeers, "peer3")
}

// TestAntiFalsePositive_Cleanup 测试清理
func TestAntiFalsePositive_Cleanup(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 执行清理（不应该 panic）
	afp.Cleanup()
}

// TestAntiFalsePositive_Close 测试关闭
func TestAntiFalsePositive_Close(t *testing.T) {
	afp := NewAntiFalsePositive(nil)

	// 触发一些状态
	afp.OnPeerDisconnected("peer1", "realm1")

	// 关闭
	afp.Close()

	// 验证已清理
	afp.mu.RLock()
	assert.Nil(t, afp.memberStates)
	afp.mu.RUnlock()
}

// TestAntiFalsePositive_Concurrent 测试并发安全
func TestAntiFalsePositive_Concurrent(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	var wg sync.WaitGroup
	iterations := 50
	peers := []string{"peer1", "peer2", "peer3"}

	for _, peerID := range peers {
		peerID := peerID
		wg.Add(4)

		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				afp.OnPeerDisconnected(peerID, "realm1")
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				afp.OnPeerReconnected(peerID)
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				afp.OnCommunication(peerID)
			}
		}()

		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = afp.ShouldRejectAdd(peerID)
				_ = afp.IsInGracePeriod(peerID)
				_ = afp.IsFlapping(peerID)
				_ = afp.IsProtected(peerID)
				_ = afp.GetState(peerID)
				_ = afp.GetStats()
			}
		}()
	}

	// 同时清理
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			afp.Cleanup()
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
	case <-time.After(10 * time.Second):
		require.Fail(t, "并发测试超时")
	}
}

// TestAntiFalsePositive_ReconnectNotInGracePeriod 测试非宽限期重连
func TestAntiFalsePositive_ReconnectNotInGracePeriod(t *testing.T) {
	afp := NewAntiFalsePositive(nil)
	defer afp.Close()

	// 直接尝试重连（无断开记录）
	recovered, suppressed := afp.OnPeerReconnected("peer1")

	assert.False(t, recovered)
	assert.False(t, suppressed)
}
