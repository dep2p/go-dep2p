package member

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDisconnectProtectionTracker_New 测试创建跟踪器
func TestDisconnectProtectionTracker_New(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	assert.NotNil(t, tracker)
	assert.NotNil(t, tracker.removedMembers)
	assert.Equal(t, DisconnectProtection, tracker.protectionDuration)
}

// TestDisconnectProtectionTracker_SetProtectionDuration 测试设置保护期
func TestDisconnectProtectionTracker_SetProtectionDuration(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	newDuration := 1 * time.Minute
	tracker.SetProtectionDuration(newDuration)

	tracker.mu.RLock()
	assert.Equal(t, newDuration, tracker.protectionDuration)
	tracker.mu.RUnlock()
}

// TestDisconnectProtectionTracker_OnMemberRemoved 测试记录成员移除
func TestDisconnectProtectionTracker_OnMemberRemoved(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	tracker.OnMemberRemoved("peer1")

	tracker.mu.RLock()
	_, exists := tracker.removedMembers["peer1"]
	tracker.mu.RUnlock()

	assert.True(t, exists)
}

// TestDisconnectProtectionTracker_IsProtected 测试保护期检查
func TestDisconnectProtectionTracker_IsProtected(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(100 * time.Millisecond)

	// 初始：不在保护期内
	assert.False(t, tracker.IsProtected("peer1"))

	// 记录移除
	tracker.OnMemberRemoved("peer1")

	// 在保护期内
	assert.True(t, tracker.IsProtected("peer1"))

	// 等待保护期过期
	time.Sleep(150 * time.Millisecond)

	// 保护期已过
	assert.False(t, tracker.IsProtected("peer1"))
}

// TestDisconnectProtectionTracker_GetRemainingProtection 测试获取剩余保护时间
func TestDisconnectProtectionTracker_GetRemainingProtection(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(200 * time.Millisecond)

	// 初始：无剩余保护时间
	assert.Equal(t, time.Duration(0), tracker.GetRemainingProtection("peer1"))

	// 记录移除
	tracker.OnMemberRemoved("peer1")

	// 获取剩余时间
	remaining := tracker.GetRemainingProtection("peer1")
	assert.Greater(t, remaining, time.Duration(0))
	assert.LessOrEqual(t, remaining, 200*time.Millisecond)

	// 等待一半时间
	time.Sleep(100 * time.Millisecond)

	// 剩余时间应该减少
	remaining2 := tracker.GetRemainingProtection("peer1")
	assert.Less(t, remaining2, remaining)

	// 等待保护期过期
	time.Sleep(150 * time.Millisecond)

	// 保护期已过
	assert.Equal(t, time.Duration(0), tracker.GetRemainingProtection("peer1"))
}

// TestDisconnectProtectionTracker_ClearProtection 测试清除保护状态
func TestDisconnectProtectionTracker_ClearProtection(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	// 记录移除
	tracker.OnMemberRemoved("peer1")
	assert.True(t, tracker.IsProtected("peer1"))

	// 清除保护
	tracker.ClearProtection("peer1")

	// 不再受保护
	assert.False(t, tracker.IsProtected("peer1"))
}

// TestDisconnectProtectionTracker_GetProtectedPeers 测试获取受保护节点列表
func TestDisconnectProtectionTracker_GetProtectedPeers(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	// 初始：无受保护节点
	assert.Empty(t, tracker.GetProtectedPeers())

	// 记录多个节点移除
	tracker.OnMemberRemoved("peer1")
	tracker.OnMemberRemoved("peer2")
	tracker.OnMemberRemoved("peer3")

	// 获取列表
	protected := tracker.GetProtectedPeers()
	assert.Len(t, protected, 3)
	assert.Contains(t, protected, "peer1")
	assert.Contains(t, protected, "peer2")
	assert.Contains(t, protected, "peer3")
}

// TestDisconnectProtectionTracker_Cleanup 测试清理过期记录
func TestDisconnectProtectionTracker_Cleanup(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(50 * time.Millisecond)

	// 记录移除
	tracker.OnMemberRemoved("peer1")

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 清理
	tracker.Cleanup()

	// 验证已清理
	tracker.mu.RLock()
	_, exists := tracker.removedMembers["peer1"]
	tracker.mu.RUnlock()
	assert.False(t, exists)
}

// TestDisconnectProtectionTracker_Reset 测试重置
func TestDisconnectProtectionTracker_Reset(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()

	// 记录多个节点
	tracker.OnMemberRemoved("peer1")
	tracker.OnMemberRemoved("peer2")

	// 重置
	tracker.Reset()

	// 所有记录应该被清除
	assert.Empty(t, tracker.GetProtectedPeers())
}

// TestDisconnectProtectionTracker_MultipleMembersWithExpiry 测试多节点部分过期
func TestDisconnectProtectionTracker_MultipleMembersWithExpiry(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(100 * time.Millisecond)

	// 记录第一个节点
	tracker.OnMemberRemoved("peer1")

	// 等待一半时间
	time.Sleep(60 * time.Millisecond)

	// 记录第二个节点
	tracker.OnMemberRemoved("peer2")

	// 此时两个节点都在保护期内
	assert.True(t, tracker.IsProtected("peer1"))
	assert.True(t, tracker.IsProtected("peer2"))

	// 等待 peer1 过期
	time.Sleep(50 * time.Millisecond)

	// peer1 应该过期，peer2 仍在保护期内
	assert.False(t, tracker.IsProtected("peer1"))
	assert.True(t, tracker.IsProtected("peer2"))
}

// TestDisconnectProtectionTracker_Concurrent 测试并发安全
func TestDisconnectProtectionTracker_Concurrent(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(1 * time.Second)

	var wg sync.WaitGroup
	iterations := 100
	peers := []string{"peer1", "peer2", "peer3", "peer4", "peer5"}

	// 并发记录移除
	for _, peerID := range peers {
		peerID := peerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tracker.OnMemberRemoved(peerID)
				_ = tracker.IsProtected(peerID)
				_ = tracker.GetRemainingProtection(peerID)
			}
		}()
	}

	// 并发清理和查询
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			tracker.Cleanup()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = tracker.GetProtectedPeers()
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

// TestDisconnectProtectionTracker_RepeatedRemoval 测试重复移除
func TestDisconnectProtectionTracker_RepeatedRemoval(t *testing.T) {
	tracker := NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(100 * time.Millisecond)

	// 第一次移除
	tracker.OnMemberRemoved("peer1")
	firstRemoval := tracker.GetRemainingProtection("peer1")

	// 等待一些时间
	time.Sleep(30 * time.Millisecond)

	// 再次移除（更新时间戳）
	tracker.OnMemberRemoved("peer1")
	secondRemoval := tracker.GetRemainingProtection("peer1")

	// 第二次移除应该重置保护期
	assert.Greater(t, secondRemoval, firstRemoval-30*time.Millisecond)
}
