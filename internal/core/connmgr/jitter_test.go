package connmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestJitterTolerance_Reconnect(t *testing.T) {
	config := DefaultJitterConfig()
	config.ToleranceWindow = 50 * time.Millisecond
	config.InitialReconnectDelay = 50 * time.Millisecond

	jt := NewJitterTolerance(config)
	defer jt.Stop()

	reconnectCalled := make(chan string, 1)
	jt.SetReconnectCallback(func(ctx context.Context, peer string) error {
		reconnectCalled <- peer
		return nil
	})

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	// 通知断连
	peerID := "peer1"
	removed := jt.NotifyDisconnected(peerID)
	if removed {
		t.Error("Expected peer not to be removed immediately")
	}

	// 检查状态
	state, ok := jt.GetState(peerID)
	if !ok {
		t.Error("Expected peer state to exist")
	}
	if state != pkgif.StateDisconnected {
		t.Errorf("Expected state %v, got %v", pkgif.StateDisconnected, state)
	}

	// 等待重连尝试（ToleranceWindow + 处理时间）
	select {
	case reconnectedPeer := <-reconnectCalled:
		if reconnectedPeer != peerID {
			t.Errorf("Expected reconnect for %s, got %s", peerID, reconnectedPeer)
		}
	case <-time.After(2 * time.Second):
		t.Error("Reconnect callback was not called")
	}
}

func TestJitterTolerance_ExponentialBackoff(t *testing.T) {
	config := DefaultJitterConfig()
	config.InitialReconnectDelay = 100 * time.Millisecond
	config.MaxReconnectDelay = 1 * time.Second
	config.BackoffMultiplier = 2.0

	jt := NewJitterTolerance(config)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},  // 初始延迟
		{1, 100 * time.Millisecond},  // 1次: 100ms
		{2, 200 * time.Millisecond},  // 2次: 100 * 2^1 = 200ms
		{3, 400 * time.Millisecond},  // 3次: 100 * 2^2 = 400ms
		{4, 800 * time.Millisecond},  // 4次: 100 * 2^3 = 800ms
		{5, 1000 * time.Millisecond}, // 5次: 100 * 2^4 = 1600ms (限制为1000ms)
	}

	for _, tt := range tests {
		delay := jt.calculateBackoff(tt.attempt)
		if delay != tt.expected {
			t.Errorf("Attempt %d: expected %v, got %v", tt.attempt, tt.expected, delay)
		}
	}
}

func TestJitterTolerance_MaxAttempts(t *testing.T) {
	config := DefaultJitterConfig()
	config.MaxReconnectAttempts = 2 // 减少尝试次数加快测试
	config.InitialReconnectDelay = 10 * time.Millisecond
	config.MaxReconnectDelay = 50 * time.Millisecond
	config.BackoffMultiplier = 2.0
	config.StateHoldTime = 1 * time.Minute // 设置长时间，确保是 MaxAttempts 触发移除

	jt := NewJitterTolerance(config)
	defer jt.Stop()

	// 模拟重连失败
	var reconnectAttempts int
	var mu sync.Mutex
	jt.SetReconnectCallback(func(ctx context.Context, peer string) error {
		mu.Lock()
		reconnectAttempts++
		count := reconnectAttempts
		mu.Unlock()
		t.Logf("重连尝试 #%d", count)
		return context.Canceled // 模拟失败
	})

	// 使用状态变更回调来检测节点被移除
	removedCh := make(chan struct{}, 1)
	jt.SetStateChangeCallback(func(peerID string, state pkgif.JitterState) {
		if state == pkgif.StateRemoved {
			select {
			case removedCh <- struct{}{}:
			default:
			}
		}
	})

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	peerID := "peer1"
	jt.NotifyDisconnected(peerID)

	// 等待节点被移除（由 MaxReconnectAttempts 触发）
	select {
	case <-removedCh:
		t.Logf("节点已被移除（通过状态回调确认）")
	case <-time.After(5 * time.Second):
		t.Fatalf("超时等待节点移除")
	}

	mu.Lock()
	attempts := reconnectAttempts
	mu.Unlock()

	if attempts < 2 {
		t.Errorf("Expected at least 2 reconnect attempts, got %d", attempts)
	}

	// 节点已从列表移除，GetState 应返回 false
	_, exists := jt.GetState(peerID)
	if exists {
		t.Error("Expected peer to be removed from disconnected list")
	}
}

func TestJitterTolerance_StateHoldTime(t *testing.T) {
	config := DefaultJitterConfig()
	config.StateHoldTime = 200 * time.Millisecond
	config.ReconnectEnabled = false // 禁用重连以简化测试

	jt := NewJitterTolerance(config)
	defer jt.Stop()

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	peerID := "peer1"
	jt.NotifyDisconnected(peerID)

	// 立即检查，不应该移除
	if jt.ShouldRemove(peerID) {
		t.Error("Expected peer not to be removed immediately")
	}

	// 等待超过保持时间
	time.Sleep(300 * time.Millisecond)

	// 现在应该移除
	if !jt.ShouldRemove(peerID) {
		t.Error("Expected peer to be removed after hold time")
	}
}

func TestJitterTolerance_NotifyReconnected(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)
	defer jt.Stop()

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	peerID := "peer1"
	jt.NotifyDisconnected(peerID)

	// 检查断连状态
	state, ok := jt.GetState(peerID)
	if !ok || state != pkgif.StateDisconnected {
		t.Error("Expected peer to be in disconnected state")
	}

	// 通知重连成功
	jt.NotifyReconnected(peerID)

	// 状态应该被清除
	_, ok = jt.GetState(peerID)
	if ok {
		t.Error("Expected peer state to be removed after reconnect")
	}
}

func TestJitterTolerance_Disabled(t *testing.T) {
	config := DefaultJitterConfig()
	config.Enabled = false

	jt := NewJitterTolerance(config)
	defer jt.Stop()

	peerID := "peer1"
	removed := jt.NotifyDisconnected(peerID)

	// 禁用时应该直接移除
	if !removed {
		t.Error("Expected peer to be removed when jitter tolerance is disabled")
	}

	// 状态不应该存在
	_, ok := jt.GetState(peerID)
	if ok {
		t.Error("Expected no state when jitter tolerance is disabled")
	}
}

func TestJitterTolerance_GetStats(t *testing.T) {
	config := DefaultJitterConfig()
	config.ReconnectEnabled = false

	jt := NewJitterTolerance(config)
	defer jt.Stop()

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	// 初始统计应该为空
	stats := jt.GetStats()
	if stats.TotalDisconnected != 0 {
		t.Error("Expected 0 disconnected peers initially")
	}

	// 添加几个断连节点
	jt.NotifyDisconnected("peer1")
	jt.NotifyDisconnected("peer2")
	jt.NotifyDisconnected("peer3")

	stats = jt.GetStats()
	if stats.TotalDisconnected != 3 {
		t.Errorf("Expected 3 disconnected peers, got %d", stats.TotalDisconnected)
	}

	// 重连一个节点
	jt.NotifyReconnected("peer2")

	stats = jt.GetStats()
	if stats.TotalDisconnected != 2 {
		t.Errorf("Expected 2 disconnected peers after reconnect, got %d", stats.TotalDisconnected)
	}
}

func TestJitterTolerance_StateChangeCallback(t *testing.T) {
	config := DefaultJitterConfig()
	jt := NewJitterTolerance(config)
	defer jt.Stop()

	stateChanges := make(chan pkgif.JitterState, 10)
	jt.SetStateChangeCallback(func(peer string, state pkgif.JitterState) {
		stateChanges <- state
	})

	ctx := context.Background()
	err := jt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start JitterTolerance: %v", err)
	}

	peerID := "peer1"
	jt.NotifyDisconnected(peerID)

	// 应该收到 Disconnected 状态
	select {
	case state := <-stateChanges:
		if state != pkgif.StateDisconnected {
			t.Errorf("Expected state %v, got %v", pkgif.StateDisconnected, state)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("State change callback was not called")
	}

	jt.NotifyReconnected(peerID)

	// 应该收到 Connected 状态
	select {
	case state := <-stateChanges:
		if state != pkgif.StateConnected {
			t.Errorf("Expected state %v, got %v", pkgif.StateConnected, state)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("State change callback was not called for reconnect")
	}
}
