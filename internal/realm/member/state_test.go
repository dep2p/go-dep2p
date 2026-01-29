package member

import (
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemberConnectionState_NewState 测试创建状态机
func TestMemberConnectionState_NewState(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	assert.NotNil(t, state)
	assert.Equal(t, "peer1", state.peerID)
	assert.Equal(t, "realm1", state.realmID)
	assert.Equal(t, types.ConnStateConnected, state.GetState())
	assert.False(t, state.IsInGracePeriod())
}

// TestMemberConnectionState_OnDisconnect 测试断开事件
func TestMemberConnectionState_OnDisconnect(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 初始状态：已连接
	assert.Equal(t, types.ConnStateConnected, state.GetState())

	// 触发断开
	state.OnDisconnect()

	// 状态应该变为断开中
	assert.Equal(t, types.ConnStateDisconnecting, state.GetState())
	assert.True(t, state.IsInGracePeriod())

	// 清理
	state.Close()
}

// TestMemberConnectionState_OnReconnect 测试宽限期内重连
func TestMemberConnectionState_OnReconnect(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 触发断开
	state.OnDisconnect()
	assert.Equal(t, types.ConnStateDisconnecting, state.GetState())

	// 宽限期内重连
	recovered := state.OnReconnect()
	assert.True(t, recovered)
	assert.Equal(t, types.ConnStateConnected, state.GetState())
	assert.False(t, state.IsInGracePeriod())

	// 清理
	state.Close()
}

// TestMemberConnectionState_GraceTimeout 测试宽限期超时
func TestMemberConnectionState_GraceTimeout(t *testing.T) {
	// 使用较短的宽限期进行测试
	originalGracePeriod := ReconnectGracePeriod
	defer func() {
		// 注意：无法修改常量，此测试仅验证回调机制
	}()
	_ = originalGracePeriod

	state := NewMemberConnectionState("peer1", "realm1")

	// 设置超时回调
	var callbackCalled bool
	var callbackPeerID string
	var mu sync.Mutex

	state.SetOnGraceTimeout(func(peerID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
		callbackPeerID = peerID
	})

	// 触发断开
	state.OnDisconnect()
	assert.True(t, state.IsInGracePeriod())

	// 由于默认宽限期是 15 秒，我们直接测试内部方法
	state.onGraceTimeoutInternal()

	// 验证状态变化
	assert.Equal(t, types.ConnStateDisconnected, state.GetState())

	// 验证回调
	mu.Lock()
	assert.True(t, callbackCalled)
	assert.Equal(t, "peer1", callbackPeerID)
	mu.Unlock()

	// 清理
	state.Close()
}

// TestMemberConnectionState_OnCommunication 测试通信事件延长宽限期
func TestMemberConnectionState_OnCommunication(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 触发断开
	state.OnDisconnect()
	assert.Equal(t, 0, state.extensions)

	// 第一次通信：延长宽限期
	state.OnCommunication()
	assert.Equal(t, 1, state.extensions)

	// 第二次通信：再次延长
	state.OnCommunication()
	assert.Equal(t, 2, state.extensions)

	// 第三次通信：达到最大延长次数，不再延长
	state.OnCommunication()
	assert.Equal(t, 2, state.extensions) // MaxGracePeriodExtensions = 2

	// 清理
	state.Close()
}

// TestMemberConnectionState_MultipleDisconnects 测试多次断开
func TestMemberConnectionState_MultipleDisconnects(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 第一次断开
	state.OnDisconnect()
	assert.Equal(t, types.ConnStateDisconnecting, state.GetState())

	// 再次断开（应该被忽略）
	state.OnDisconnect()
	assert.Equal(t, types.ConnStateDisconnecting, state.GetState())

	// 宽限期超时
	state.onGraceTimeoutInternal()
	assert.Equal(t, types.ConnStateDisconnected, state.GetState())

	// 已断开状态下再次断开（应该被忽略）
	state.OnDisconnect()
	assert.Equal(t, types.ConnStateDisconnected, state.GetState())

	// 清理
	state.Close()
}

// TestMemberConnectionState_ReconnectFromDisconnected 测试从已断开状态重连
func TestMemberConnectionState_ReconnectFromDisconnected(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 断开并超时
	state.OnDisconnect()
	state.onGraceTimeoutInternal()
	assert.Equal(t, types.ConnStateDisconnected, state.GetState())

	// 从已断开状态重连
	recovered := state.OnReconnect()
	assert.True(t, recovered)
	assert.Equal(t, types.ConnStateConnected, state.GetState())

	// 清理
	state.Close()
}

// TestMemberConnectionState_GetDisconnectedDuration 测试获取断开持续时间
func TestMemberConnectionState_GetDisconnectedDuration(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")

	// 已连接状态：断开时间应为 0
	assert.Equal(t, time.Duration(0), state.GetDisconnectedDuration())

	// 触发断开
	state.OnDisconnect()
	time.Sleep(10 * time.Millisecond)

	// 断开中状态：应该有断开时间
	duration := state.GetDisconnectedDuration()
	assert.Greater(t, duration, time.Duration(0))

	// 清理
	state.Close()
}

// TestMemberConnectionState_Concurrent 测试并发安全
func TestMemberConnectionState_Concurrent(t *testing.T) {
	state := NewMemberConnectionState("peer1", "realm1")
	defer state.Close()

	var wg sync.WaitGroup
	iterations := 100

	// 并发调用各种方法
	wg.Add(4)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			state.OnDisconnect()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			state.OnReconnect()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			state.OnCommunication()
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = state.GetState()
			_ = state.IsInGracePeriod()
			_ = state.GetDisconnectedDuration()
		}
	}()

	// 等待完成（不应该 panic）
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
