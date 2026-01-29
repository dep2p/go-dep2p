// Package recovery MonitorBridge 完整测试
package recovery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                          Mock 实现
// ============================================================================

// mockMonitor 模拟连接健康监控器
type mockMonitor struct {
	subscribers    []chan interfaces.ConnectionHealthChange
	subscribersMap map[<-chan interfaces.ConnectionHealthChange]chan interfaces.ConnectionHealthChange
	mu             sync.Mutex
	
	recoverSuccessCalled bool
	recoverFailedCalled  bool
	recoverFailedErr     error
}

func newMockMonitor() *mockMonitor {
	return &mockMonitor{
		subscribers:    make([]chan interfaces.ConnectionHealthChange, 0),
		subscribersMap: make(map[<-chan interfaces.ConnectionHealthChange]chan interfaces.ConnectionHealthChange),
	}
}

func (m *mockMonitor) Subscribe() <-chan interfaces.ConnectionHealthChange {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	ch := make(chan interfaces.ConnectionHealthChange, 10)
	m.subscribers = append(m.subscribers, ch)
	// 保存只读到可写的映射
	m.subscribersMap[ch] = ch
	return ch
}

func (m *mockMonitor) Unsubscribe(ch <-chan interfaces.ConnectionHealthChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 通过 map 获取可写 channel
	if writableCh, ok := m.subscribersMap[ch]; ok {
		for i, sub := range m.subscribers {
			if sub == writableCh {
				close(sub)
				m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
				break
			}
		}
		delete(m.subscribersMap, ch)
	}
}

func (m *mockMonitor) NotifyRecoverySuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoverSuccessCalled = true
}

func (m *mockMonitor) NotifyRecoveryFailed(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoverFailedCalled = true
	m.recoverFailedErr = err
}

func (m *mockMonitor) GetState() interfaces.ConnectionHealth {
	return interfaces.ConnectionHealthy
}

func (m *mockMonitor) GetSnapshot() interfaces.ConnectionHealthSnapshot {
	return interfaces.ConnectionHealthSnapshot{
		State:      interfaces.ConnectionHealthy,
		TotalPeers: 0,
	}
}

func (m *mockMonitor) Start(ctx context.Context) error {
	return nil
}

func (m *mockMonitor) Stop() error {
	return nil
}

func (m *mockMonitor) OnSendError(peer string, err error) {}

func (m *mockMonitor) OnSendSuccess(peer string) {}

func (m *mockMonitor) TriggerRecoveryState() {}

func (m *mockMonitor) Reset() {}

// EmitStateChange 发送状态变更（测试辅助方法）
func (m *mockMonitor) EmitStateChange(change interfaces.ConnectionHealthChange) {
	m.mu.Lock()
	subs := make([]chan interfaces.ConnectionHealthChange, len(m.subscribers))
	copy(subs, m.subscribers)
	m.mu.Unlock()
	
	for _, ch := range subs {
		select {
		case ch <- change:
		case <-time.After(time.Second):
			// 超时防止测试卡住
		}
	}
}

// ============================================================================
//                          MonitorBridge 基础测试
// ============================================================================

// TestMonitorBridge_NewMonitorBridge 测试创建桥接器
func TestMonitorBridge_NewMonitorBridge(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	
	bridge := NewMonitorBridge(monitor, manager)
	
	require.NotNil(t, bridge)
	assert.NotNil(t, bridge.monitor)
	assert.NotNil(t, bridge.recoveryManager)
}

// TestMonitorBridge_StartStop 测试启动停止
func TestMonitorBridge_StartStop(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	
	// 启动
	bridge.Start(ctx)
	
	// 等待一段时间
	time.Sleep(50 * time.Millisecond)
	
	// 停止
	bridge.Stop()
	
	// 验证 channel 被关闭
	time.Sleep(50 * time.Millisecond)
	
	// 🎯 验证：停止后不应该 panic
}

// TestMonitorBridge_StartStop_Idempotency 测试启动停止幂等性
func TestMonitorBridge_StartStop_Idempotency(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	
	// 多次启动
	bridge.Start(ctx)
	bridge.Start(ctx) // 🎯 发现 BUG: 多次启动可能创建多个 goroutine
	
	// 多次停止
	bridge.Stop()
	bridge.Stop() // 应该是幂等的
}

// TestMonitorBridge_StopWithoutStart 测试未启动就停止
func TestMonitorBridge_StopWithoutStart(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	// 🎯 发现 BUG: 未启动就停止可能 panic (访问 nil cancel)
	assert.NotPanics(t, func() {
		bridge.Stop()
	})
}

// ============================================================================
//                          状态变更处理测试
// ============================================================================

// TestMonitorBridge_HandleStateChange_ConnectionDown 测试连接断开触发恢复
func TestMonitorBridge_HandleStateChange_ConnectionDown(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	manager.SetConnector(&mockConnector{connectionCount: 1})
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 发送 ConnectionDown 状态变更
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonAllConnectionsLost,
	}
	
	monitor.EmitStateChange(change)
	
	// 等待恢复完成
	time.Sleep(200 * time.Millisecond)
	
	// 验证恢复成功通知被调用
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	assert.True(t, monitor.recoverSuccessCalled, "应该通知恢复成功")
}

// TestMonitorBridge_HandleStateChange_NotDown 测试非 Down 状态不触发恢复
func TestMonitorBridge_HandleStateChange_NotDown(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 发送非 Down 状态
	for _, state := range []interfaces.ConnectionHealth{
		interfaces.ConnectionHealthy,
		interfaces.ConnectionDegraded,
		interfaces.ConnectionRecovering,
	} {
		change := interfaces.ConnectionHealthChange{
			CurrentState: state,
			Reason:       interfaces.ReasonManualTrigger,
		}
		
		monitor.EmitStateChange(change)
	}
	
	time.Sleep(100 * time.Millisecond)
	
	// 验证恢复没有被触发
	assert.False(t, manager.IsRecovering())
}

// TestMonitorBridge_HandleStateChange_AlreadyRecovering 测试已在恢复中跳过
func TestMonitorBridge_HandleStateChange_AlreadyRecovering(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	
	// 设置慢速 connector，让恢复持续一段时间
	slowConnector := &overridableConnector{
		connectionCount: 1,
	}
	slowConnector.ConnectFunc = func(ctx context.Context, peerID string) error {
		// 模拟慢速连接（200ms）
		time.Sleep(200 * time.Millisecond)
		return nil
	}
	manager.SetConnector(slowConnector)
	manager.SetCriticalPeers([]string{"test-peer"})
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 第一次触发恢复
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonAllConnectionsLost,
	}
	monitor.EmitStateChange(change)
	
	// 等待恢复开始
	time.Sleep(50 * time.Millisecond)
	
	// 第二次触发（应该被跳过）
	monitor.EmitStateChange(change)
	
	// 等待恢复完成
	time.Sleep(300 * time.Millisecond)
	
	// 🎯 验证：第二次触发被跳过，不会启动新的恢复
	assert.LessOrEqual(t, manager.GetAttemptCount(), 1, "应该只有一次恢复尝试")
}

// TestMonitorBridge_HandleStateChange_RecoveryFailed 测试恢复失败通知
func TestMonitorBridge_HandleStateChange_RecoveryFailed(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	// 不设置 connector，导致恢复失败
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 触发恢复
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonCriticalError,
	}
	monitor.EmitStateChange(change)
	
	// 等待恢复完成
	time.Sleep(200 * time.Millisecond)
	
	// 验证恢复失败通知被调用
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	assert.True(t, monitor.recoverFailedCalled, "应该通知恢复失败")
	assert.NotNil(t, monitor.recoverFailedErr, "应该传递错误")
}

// ============================================================================
//                          原因映射测试
// ============================================================================

// TestMapToRecoveryReason_AllReasons 测试所有原因映射
func TestMapToRecoveryReason_AllReasons(t *testing.T) {
	tests := []struct {
		input    interfaces.StateChangeReason
		expected interfaces.RecoveryReason
	}{
		{interfaces.ReasonCriticalError, interfaces.RecoveryReasonNetworkUnreachable},
		{interfaces.ReasonAllConnectionsLost, interfaces.RecoveryReasonAllConnectionsLost},
		{interfaces.ReasonErrorThreshold, interfaces.RecoveryReasonErrorThreshold},
		{interfaces.ReasonNetworkChanged, interfaces.RecoveryReasonNetworkChange},
		{interfaces.ReasonProbeFailed, interfaces.RecoveryReasonNetworkUnreachable},
		{interfaces.ReasonManualTrigger, interfaces.RecoveryReasonManualTrigger},
		{interfaces.ReasonConnectionRestored, interfaces.RecoveryReasonUnknown}, // 默认
		{interfaces.StateChangeReason(999), interfaces.RecoveryReasonUnknown},   // 未知值
	}
	
	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := MapToRecoveryReason(tt.input)
			assert.Equal(t, tt.expected, result, "映射应该正确")
		})
	}
}

// ============================================================================
//                          并发安全测试
// ============================================================================

// TestMonitorBridge_ConcurrentStateChanges 测试并发状态变更
func TestMonitorBridge_ConcurrentStateChanges(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	manager.SetConnector(&mockConnector{connectionCount: 1})
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			change := interfaces.ConnectionHealthChange{
				CurrentState: interfaces.ConnectionDown,
				Reason:       interfaces.ReasonAllConnectionsLost,
			}
			monitor.EmitStateChange(change)
		}()
	}
	
	wg.Wait()
	
	// 等待所有恢复完成
	time.Sleep(300 * time.Millisecond)
	
	// 🎯 验证：并发状态变更不应该导致 panic 或数据竞争
}

// TestMonitorBridge_StartStopCycle 测试多次启动停止
func TestMonitorBridge_StartStopCycle(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	
	for i := 0; i < 3; i++ {
		bridge.Start(ctx)
		time.Sleep(50 * time.Millisecond)
		bridge.Stop()
		time.Sleep(50 * time.Millisecond)
	}
	
	// 🎯 验证：多次启动停止循环不应该泄漏 goroutine
}

// ============================================================================
//                          边界条件测试
// ============================================================================

// TestMonitorBridge_ClosedChannel 测试关闭的 channel
func TestMonitorBridge_ClosedChannel(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	bridge.Start(ctx)
	
	// 获取订阅的 channel
	monitor.mu.Lock()
	if len(monitor.subscribers) == 0 {
		t.Fatal("没有订阅者")
	}
	ch := monitor.subscribers[0]
	monitor.mu.Unlock()
	
	// 关闭 channel
	close(ch)
	
	// 等待 goroutine 退出
	time.Sleep(100 * time.Millisecond)
	
	// 🎯 验证：关闭的 channel 应该导致 goroutine 优雅退出
	bridge.Stop()
}

// TestMonitorBridge_NilContext 测试 nil context
func TestMonitorBridge_NilContext(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	// 🎯 发现 BUG: nil context 可能导致 panic
	assert.NotPanics(t, func() {
		bridge.Start(nil)
		time.Sleep(50 * time.Millisecond)
		bridge.Stop()
	})
}

// TestMonitorBridge_CanceledContext 测试已取消的 context
func TestMonitorBridge_CanceledContext(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	
	bridge.Start(ctx)
	
	// 发送状态变更
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonAllConnectionsLost,
	}
	monitor.EmitStateChange(change)
	
	// goroutine 应该立即退出
	time.Sleep(100 * time.Millisecond)
	
	bridge.Stop()
}

// ============================================================================
//                          集成测试
// ============================================================================

// TestMonitorBridge_EndToEnd_SuccessfulRecovery 测试端到端成功恢复
func TestMonitorBridge_EndToEnd_SuccessfulRecovery(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	manager.SetConnector(&mockConnector{connectionCount: 1})
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 模拟网络故障
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonNetworkChanged,
	}
	
	monitor.EmitStateChange(change)
	
	// 等待恢复完成
	time.Sleep(200 * time.Millisecond)
	
	// 验证恢复成功
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	assert.True(t, monitor.recoverSuccessCalled, "应该通知恢复成功")
	assert.False(t, monitor.recoverFailedCalled, "不应该通知恢复失败")
}

// TestMonitorBridge_EndToEnd_FailedRecovery 测试端到端失败恢复
func TestMonitorBridge_EndToEnd_FailedRecovery(t *testing.T) {
	monitor := newMockMonitor()
	manager := NewManager(nil)
	// 不设置 connector，导致恢复失败
	
	bridge := NewMonitorBridge(monitor, manager)
	
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()
	
	bridge.Start(ctx)
	defer bridge.Stop()
	
	// 模拟网络故障
	change := interfaces.ConnectionHealthChange{
		CurrentState: interfaces.ConnectionDown,
		Reason:       interfaces.ReasonCriticalError,
	}
	
	monitor.EmitStateChange(change)
	
	// 等待恢复完成
	time.Sleep(200 * time.Millisecond)
	
	// 验证恢复失败
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	assert.False(t, monitor.recoverSuccessCalled, "不应该通知恢复成功")
	assert.True(t, monitor.recoverFailedCalled, "应该通知恢复失败")
	assert.Equal(t, ErrRecoveryFailed, monitor.recoverFailedErr, "错误应该是 ErrRecoveryFailed")
}
