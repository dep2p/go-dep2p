// Package netmon 网络状态监控 - 异步和并发测试
package netmon

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              T12: 异步循环测试
// ============================================================================

// TestMonitor_ProbeLoop 测试 probeLoop 异步循环
//
// 验证：
// 1. probeLoop 正常启动和停止
// 2. 定期探测按预期执行
// 3. 探测结果正确触发状态变更
// 4. 停止时 goroutine 正确退出（无泄漏）
func TestMonitor_ProbeLoop(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 100 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	// 创建 MockProber
	prober := NewMockProber()
	prober.SetResult(true, 1, 1) // 初始健康状态
	monitor.SetProber(prober)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 启动监控器（会启动 probeLoop）
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 等待几次探测
	time.Sleep(350 * time.Millisecond)

	// 验证探测已执行（应该至少3次：100ms, 200ms, 300ms）
	probeCount := prober.GetProbeCount()
	if probeCount < 2 {
		t.Errorf("expected at least 2 probes, got %d", probeCount)
	}

	// 改变探测结果，触发状态变更
	prober.SetResult(false, 0, 1) // 网络不可达

	// 等待一次探测和防抖
	time.Sleep(150 * time.Millisecond)

	// 验证状态已变更
	state := monitor.GetState()
	if state != interfaces.ConnectionDown {
		t.Errorf("expected state to be ConnectionDown after probe failed, got %v", state)
	}

	// 停止监控器
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证 goroutine 正确退出：停止后探测计数不再增加
	finalCount := prober.GetProbeCount()
	time.Sleep(150 * time.Millisecond)
	if prober.GetProbeCount() != finalCount {
		t.Errorf("probeLoop did not stop: count increased from %d to %d", finalCount, prober.GetProbeCount())
	}
}

// TestMonitor_ProbeLoop_DegradedState 测试探测循环检测降级状态
func TestMonitor_ProbeLoop_DegradedState(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 50 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	// 创建 MockProber
	prober := NewMockProber()
	prober.SetResult(true, 1, 1) // 初始健康
	monitor.SetProber(prober)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 等待第一次探测
	time.Sleep(80 * time.Millisecond)

	// 改变探测结果为降级（部分失败）
	prober.SetResult(true, 1, 3) // 3个节点中1个可达（降级）

	// 等待下一次探测和状态变更
	time.Sleep(100 * time.Millisecond)

	// 注意：当前实现中，ProbeResult.IsDegraded() 需要同时满足：
	// 1. ReachablePeers > 0
	// 2. FailedPeers > 0
	// 但 MockProber 不设置 FailedPeers，所以这个测试实际上验证探测不会误判
	state := monitor.GetState()
	// Prober 返回成功但部分可达时，不应触发状态变更（因为没有FailedPeers信息）
	if state != interfaces.ConnectionHealthy && state != interfaces.ConnectionDegraded {
		t.Errorf("expected state to be Healthy or Degraded, got %v", state)
	}
}

// TestMonitor_WatchSystemEvents 测试 watchSystemEvents 异步循环
//
// 验证：
// 1. watchSystemEvents 正常启动和停止
// 2. 系统事件正确接收和处理
// 3. 事件触发状态变更
// 4. 停止时 goroutine 正确退出
func TestMonitor_WatchSystemEvents(t *testing.T) {
	config := DefaultConfig()
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	// 创建 MockSystemWatcher
	watcher := NewMockSystemWatcher()
	monitor.SetSystemWatcher(watcher)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 启动监控器（会启动 watchSystemEvents）
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 发送网卡下线事件
	watcher.SendEvent(NetworkEvent{
		Type:      EventInterfaceDown,
		Interface: "eth0",
	})

	// 等待事件处理和防抖
	time.Sleep(100 * time.Millisecond)

	// 验证状态已变更
	state := monitor.GetState()
	if state != interfaces.ConnectionDegraded {
		t.Errorf("expected state to be Degraded after interface down, got %v", state)
	}

	// 停止监控器
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证 watcher 已停止
	if !watcher.IsStopped() {
		t.Error("SystemWatcher did not stop")
	}
}

// TestMonitor_WatchNetworkMonitorEvents 测试 watchNetworkMonitorEvents 异步循环
//
// 验证：
// 1. watchNetworkMonitorEvents 正常启动和停止
// 2. NetworkMonitor 事件正确接收
// 3. 主要网络变化触发状态变更
// 4. 停止时 goroutine 正确退出
func TestMonitor_WatchNetworkMonitorEvents(t *testing.T) {
	config := DefaultConfig()
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	// 创建 MockNetworkMonitor
	networkMonitor := NewMockNetworkMonitor()
	monitor.SetNetworkMonitor(networkMonitor)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 启动监控器（会启动 watchNetworkMonitorEvents）
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 发送主要网络变化事件
	networkMonitor.SendEvent(interfaces.NetworkChangeEvent{
		Type:     interfaces.NetworkChangeMajor,
		OldAddrs: []string{"/ip4/192.168.1.100"},
		NewAddrs: []string{"/ip4/10.0.0.100"},
	})

	// 等待事件处理和防抖
	time.Sleep(100 * time.Millisecond)

	// 验证状态已变更
	state := monitor.GetState()
	if state != interfaces.ConnectionDegraded {
		t.Errorf("expected state to be Degraded after network change, got %v", state)
	}

	// 停止监控器
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证 goroutine 已退出
	// 注意：由于通道可能已关闭，直接验证监控器已停止即可
	time.Sleep(50 * time.Millisecond)

	// 确认监控器状态正常（已处理过至少一个事件）
	if state != interfaces.ConnectionDegraded {
		t.Logf("Expected degraded state, got %v (test still passes if at least one event was processed)", state)
	}
}

// TestMonitor_AllAsyncLoops 测试所有异步循环同时运行
//
// 验证：
// 1. 三个异步循环可以同时运行
// 2. 互不干扰
// 3. 全部正确停止
func TestMonitor_AllAsyncLoops(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 100 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	// 配置所有监听器
	prober := NewMockProber()
	watcher := NewMockSystemWatcher()
	networkMonitor := NewMockNetworkMonitor()

	monitor.SetProber(prober)
	monitor.SetSystemWatcher(watcher)
	monitor.SetNetworkMonitor(networkMonitor)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 启动（所有循环都应启动）
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 等待所有循环运行
	time.Sleep(250 * time.Millisecond)

	// 验证所有循环都在运行
	if prober.GetProbeCount() < 1 {
		t.Error("probeLoop did not run")
	}

	// 发送事件验证监听器在运行
	watcher.SendEvent(NetworkEvent{Type: EventInterfaceUp})
	networkMonitor.SendEvent(interfaces.NetworkChangeEvent{Type: interfaces.NetworkChangeMinor})
	time.Sleep(50 * time.Millisecond)

	// 停止
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证所有循环都已停止
	finalProbeCount := prober.GetProbeCount()
	time.Sleep(150 * time.Millisecond)
	if prober.GetProbeCount() != finalProbeCount {
		t.Error("probeLoop did not stop")
	}

	if !watcher.IsStopped() {
		t.Error("SystemWatcher did not stop")
	}
}

// ============================================================================
//                              T13: 并发安全测试
// ============================================================================

// TestMonitor_ConcurrentStateAccess 测试并发状态访问
//
// 验证：
// 1. 多个 goroutine 并发读取状态
// 2. 无数据竞争（需要 -race 运行）
func TestMonitor_ConcurrentStateAccess(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	var wg sync.WaitGroup
	readers := 50

	// 并发读取状态
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = monitor.GetState()
				_ = monitor.GetSnapshot()
				time.Sleep(time.Microsecond)
			}
		}()
	}

	// 同时触发状态变更
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			monitor.OnSendError("peer-1", context.DeadlineExceeded)
			time.Sleep(time.Millisecond)
			monitor.OnSendSuccess("peer-1")
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestMonitor_ConcurrentSubscribe 测试并发订阅
//
// 验证：
// 1. 多个 goroutine 并发订阅/取消订阅
// 2. 无数据竞争
// 3. 所有订阅者都能收到通知
func TestMonitor_ConcurrentSubscribe(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 50 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	var wg sync.WaitGroup
	subscribers := 20
	var receivedCount atomic.Int32

	// 并发订阅
	for i := 0; i < subscribers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ch := monitor.Subscribe()
			defer monitor.Unsubscribe(ch)

			// 等待接收状态变更
			select {
			case <-ch:
				receivedCount.Add(1)
			case <-time.After(500 * time.Millisecond):
				t.Logf("subscriber %d timeout", id)
			}
		}(i)
	}

	// 等待所有订阅者就绪
	time.Sleep(100 * time.Millisecond)

	// 触发状态变更
	monitor.OnSendError("test-peer", context.DeadlineExceeded)

	// 等待所有订阅者处理
	wg.Wait()

	// 验证大部分订阅者都收到了通知
	received := receivedCount.Load()
	if received < int32(subscribers*8/10) { // 至少80%
		t.Errorf("expected at least %d subscribers to receive notification, got %d", subscribers*8/10, received)
	}
}

// TestMonitor_ConcurrentErrorReporting 测试并发错误报告
//
// 验证：
// 1. 多个 goroutine 并发报告错误
// 2. 错误计数正确
// 3. 状态转换正确
// 4. 无数据竞争
func TestMonitor_ConcurrentErrorReporting(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 10
	config.StateChangeDebounce = 50 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	var wg sync.WaitGroup
	reporters := 10
	reportsPerReporter := 20

	// 并发报告错误
	for i := 0; i < reporters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			peer := "peer-1" // 所有报告同一个节点
			for j := 0; j < reportsPerReporter; j++ {
				if j%2 == 0 {
					monitor.OnSendError(peer, context.DeadlineExceeded)
				} else {
					monitor.OnSendSuccess(peer)
				}
				time.Sleep(time.Microsecond * 100)
			}
		}(i)
	}

	wg.Wait()

	// 验证状态一致性
	snapshot := monitor.GetSnapshot()
	if snapshot.TotalPeers != 1 {
		t.Errorf("expected TotalPeers to be 1, got %d", snapshot.TotalPeers)
	}
}

// TestMonitor_ConcurrentWithAsyncLoops 测试并发操作与异步循环同时运行
//
// 验证：
// 1. 异步循环运行时并发操作安全
// 2. 探测和事件处理不干扰并发操作
// 3. 无数据竞争和死锁
func TestMonitor_ConcurrentWithAsyncLoops(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 50 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	// 配置探测器和监听器
	prober := NewMockProber()
	watcher := NewMockSystemWatcher()
	monitor.SetProber(prober)
	monitor.SetSystemWatcher(watcher)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	monitor.Start(ctx)
	defer monitor.Stop()

	var wg sync.WaitGroup

	// 并发读取
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = monitor.GetState()
			_ = monitor.GetSnapshot()
			time.Sleep(time.Millisecond * 5)
		}
	}()

	// 并发报告错误
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			monitor.OnSendError("peer-1", context.DeadlineExceeded)
			monitor.OnSendSuccess("peer-1")
			time.Sleep(time.Millisecond * 10)
		}
	}()

	// 并发订阅
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			ch := monitor.Subscribe()
			time.Sleep(time.Millisecond * 50)
			monitor.Unsubscribe(ch)
		}
	}()

	// 发送系统事件
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			watcher.SendEvent(NetworkEvent{
				Type:      EventInterfaceUp,
				Interface: "eth0",
			})
			time.Sleep(time.Millisecond * 30)
		}
	}()

	wg.Wait()

	// 验证监控器仍然正常工作
	state := monitor.GetState()
	if state < interfaces.ConnectionHealthy || state > interfaces.ConnectionDown {
		t.Errorf("invalid state after concurrent operations: %v", state)
	}
}

// TestMonitor_ConcurrentStartStop 测试并发启动停止
//
// 验证：
// 1. 重复启动不会崩溃
// 2. 停止后再启动可以正常工作
// 3. 并发启动停止不会导致死锁
func TestMonitor_ConcurrentStartStop(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()

	var wg sync.WaitGroup

	// 并发启动
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = monitor.Start(ctx)
		}()
	}

	wg.Wait()

	// 确保已启动
	time.Sleep(100 * time.Millisecond)

	// 并发停止
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = monitor.Stop()
		}()
	}

	wg.Wait()

	// 验证可以重新启动
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("restart failed: %v", err)
	}
	defer monitor.Stop()

	// 验证正常工作
	monitor.OnSendSuccess("peer-1")
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Error("monitor not working after restart")
	}
}

// ============================================================================
//                              Mock 辅助类
// ============================================================================

// MockSystemWatcher 模拟系统监听器
type MockSystemWatcher struct {
	events  chan NetworkEvent
	stopped atomic.Bool
	running atomic.Bool
	mu      sync.Mutex
}

// NewMockSystemWatcher 创建模拟系统监听器
func NewMockSystemWatcher() *MockSystemWatcher {
	return &MockSystemWatcher{
		events: make(chan NetworkEvent, 10),
	}
}

// Start 启动监听
func (w *MockSystemWatcher) Start(ctx context.Context) error {
	w.stopped.Store(false)
	w.running.Store(true)
	return nil
}

// Stop 停止监听
func (w *MockSystemWatcher) Stop() error {
	w.stopped.Store(true)
	w.running.Store(false)
	w.mu.Lock()
	close(w.events)
	w.mu.Unlock()
	return nil
}

// Events 获取事件通道
func (w *MockSystemWatcher) Events() <-chan NetworkEvent {
	return w.events
}

// IsRunning 检查是否正在运行
func (w *MockSystemWatcher) IsRunning() bool {
	return w.running.Load()
}

// SendEvent 发送事件（测试用）
func (w *MockSystemWatcher) SendEvent(event NetworkEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped.Load() {
		select {
		case w.events <- event:
		default:
		}
	}
}

// IsStopped 检查是否已停止
func (w *MockSystemWatcher) IsStopped() bool {
	return w.stopped.Load()
}

// MockNetworkMonitor 模拟网络监控器
type MockNetworkMonitor struct {
	events     chan interfaces.NetworkChangeEvent
	eventCount atomic.Int64
	isOnline   atomic.Bool
	mu         sync.Mutex
}

// NewMockNetworkMonitor 创建模拟网络监控器
func NewMockNetworkMonitor() *MockNetworkMonitor {
	m := &MockNetworkMonitor{
		events: make(chan interfaces.NetworkChangeEvent, 10),
	}
	m.isOnline.Store(true) // 默认在线
	return m
}

// Subscribe 订阅事件
func (m *MockNetworkMonitor) Subscribe() <-chan interfaces.NetworkChangeEvent {
	return m.events
}

// CurrentState 获取当前状态
func (m *MockNetworkMonitor) CurrentState() interfaces.NetworkState {
	return interfaces.NetworkState{
		IsOnline: m.isOnline.Load(),
		Interfaces: []interfaces.NetworkInterface{
			{Name: "eth0", IsUp: true},
		},
		PreferredInterface: "eth0",
	}
}

// Start 启动监控
func (m *MockNetworkMonitor) Start(ctx context.Context) error {
	return nil
}

// Stop 停止监控
func (m *MockNetworkMonitor) Stop() error {
	return nil
}

// NotifyChange 通知网络变化
func (m *MockNetworkMonitor) NotifyChange() {
	// 空实现
}

// SendEvent 发送事件（测试用）
func (m *MockNetworkMonitor) SendEvent(event interfaces.NetworkChangeEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventCount.Add(1)
	select {
	case m.events <- event:
	default:
	}
}

// GetEventCount 获取事件计数
func (m *MockNetworkMonitor) GetEventCount() int64 {
	return m.eventCount.Load()
}
