// Package netmon 网络状态监控
package netmon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TestMonitor_NewMonitor 测试创建监控器
func TestMonitor_NewMonitor(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	if monitor == nil {
		t.Fatal("NewMonitor returned nil")
	}

	// 检查初始状态
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected initial state to be ConnectionHealthy, got %v", monitor.GetState())
	}
}

// TestMonitor_StartStop 测试启动和停止
func TestMonitor_StartStop(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()

	// 启动
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 停止
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestMonitor_OnSendError 测试错误处理
func TestMonitor_OnSendError(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 2
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	peer := "test-peer-1"
	err := errors.New("connection failed")

	// 第一次错误 - 不应该改变状态
	monitor.OnSendError(peer, err)
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected state to be ConnectionHealthy after first error, got %v", monitor.GetState())
	}

	// 第二次错误 - 达到阈值，应该触发状态变更
	monitor.OnSendError(peer, err)

	// 等待防抖
	time.Sleep(50 * time.Millisecond)

	// 状态应该变为 Degraded 或 Down（取决于其他节点情况）
	state := monitor.GetState()
	if state != interfaces.ConnectionDegraded && state != interfaces.ConnectionDown {
		t.Errorf("expected state to be Degraded or Down, got %v", state)
	}
}

// TestMonitor_OnSendSuccess 测试成功处理
func TestMonitor_OnSendSuccess(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	peer := "test-peer-1"

	// 先触发错误让状态变更
	monitor.OnSendError(peer, errors.New("connection failed"))
	time.Sleep(50 * time.Millisecond)

	// 发送成功应该恢复状态
	monitor.OnSendSuccess(peer)

	// 状态应该恢复为 Healthy
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected state to be ConnectionHealthy after success, got %v", monitor.GetState())
	}
}

// TestMonitor_CriticalError 测试关键错误
func TestMonitor_CriticalError(t *testing.T) {
	config := DefaultConfig()
	config.CriticalErrors = []string{"network is unreachable"}
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	peer := "test-peer-1"
	criticalErr := errors.New("network is unreachable")

	// 关键错误应该立即触发 Down 状态
	monitor.OnSendError(peer, criticalErr)

	// 不需要等待防抖，关键错误立即生效
	time.Sleep(20 * time.Millisecond)

	if monitor.GetState() != interfaces.ConnectionDown {
		t.Errorf("expected state to be ConnectionDown after critical error, got %v", monitor.GetState())
	}
}

// TestMonitor_Subscribe 测试订阅
func TestMonitor_Subscribe(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 订阅状态变更
	ch := monitor.Subscribe()

	// 触发状态变更
	go func() {
		time.Sleep(10 * time.Millisecond)
		monitor.OnSendError("test-peer", errors.New("connection failed"))
	}()

	// 等待接收状态变更
	select {
	case change := <-ch:
		if change.PreviousState != interfaces.ConnectionHealthy {
			t.Errorf("expected previous state to be ConnectionHealthy, got %v", change.PreviousState)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for state change")
	}

	// 取消订阅
	monitor.Unsubscribe(ch)
}

// TestMonitor_GetSnapshot 测试获取快照
func TestMonitor_GetSnapshot(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 记录一些活动
	monitor.OnSendSuccess("peer-1")
	monitor.OnSendSuccess("peer-2")
	monitor.OnSendError("peer-3", errors.New("error"))

	snapshot := monitor.GetSnapshot()

	if snapshot.State != interfaces.ConnectionHealthy {
		t.Errorf("expected state to be ConnectionHealthy, got %v", snapshot.State)
	}

	if snapshot.TotalPeers != 3 {
		t.Errorf("expected TotalPeers to be 3, got %d", snapshot.TotalPeers)
	}
}

// TestMonitor_TriggerRecoveryState 测试手动触发恢复
func TestMonitor_TriggerRecoveryState(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 手动触发恢复状态
	monitor.TriggerRecoveryState()

	if monitor.GetState() != interfaces.ConnectionRecovering {
		t.Errorf("expected state to be ConnectionRecovering, got %v", monitor.GetState())
	}
}

// TestMonitor_NotifyRecoverySuccess 测试恢复成功通知
func TestMonitor_NotifyRecoverySuccess(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 先触发恢复状态
	monitor.TriggerRecoveryState()

	// 通知恢复成功
	monitor.NotifyRecoverySuccess()

	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected state to be ConnectionHealthy after recovery success, got %v", monitor.GetState())
	}
}

// TestMonitor_Reset 测试重置
func TestMonitor_Reset(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 触发状态变更
	monitor.OnSendError("peer-1", errors.New("error"))
	time.Sleep(100 * time.Millisecond)

	// 重置
	monitor.Reset()

	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected state to be ConnectionHealthy after reset, got %v", monitor.GetState())
	}

	snapshot := monitor.GetSnapshot()
	if snapshot.RecoveryAttempts != 0 {
		t.Errorf("expected RecoveryAttempts to be 0 after reset, got %d", snapshot.RecoveryAttempts)
	}
}
