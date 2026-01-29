// Package netmon 网络状态监控 - 集成测试
package netmon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              T14: Prober 集成测试
// ============================================================================

// TestMonitor_ProberIntegration 测试 Monitor 与 Prober 的集成
//
// 验证：
// 1. Prober 正确设置和启动
// 2. 探测结果正确触发状态变更
// 3. 不同探测结果的处理逻辑
func TestMonitor_ProberIntegration(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 100 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	// 创建 MockProber
	prober := NewMockProber()
	monitor.SetProber(prober)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 订阅状态变更
	stateCh := monitor.Subscribe()
	defer monitor.Unsubscribe(stateCh)

	// 启动监控器
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 场景1: 健康 -> Down（探测失败）
	prober.SetResult(false, 0, 5) // 所有节点不可达
	change := waitForStateChange(t, stateCh, 300*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDown {
		t.Errorf("expected Down state, got %v", change.CurrentState)
	}
	if change.Reason != interfaces.ReasonProbeFailed {
		t.Errorf("expected ReasonProbeFailed, got %v", change.Reason)
	}

	// 场景2: Down -> Healthy（探测恢复）
	prober.SetResult(true, 5, 5) // 所有节点可达
	change = waitForStateChange(t, stateCh, 300*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state, got %v", change.CurrentState)
	}
	if change.Reason != interfaces.ReasonConnectionRestored {
		t.Errorf("expected ReasonConnectionRestored, got %v", change.Reason)
	}

	// 验证探测执行次数（至少执行过几次）
	if prober.GetProbeCount() < 2 {
		t.Errorf("expected at least 2 probes, got %d", prober.GetProbeCount())
	}
}

// TestMonitor_NoOpProber 测试 NoOpProber 集成
//
// 验证：
// 1. NoOpProber 不影响正常功能
// 2. 始终返回健康状态
func TestMonitor_NoOpProber(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 50 * time.Millisecond

	monitor := NewMonitor(config)
	prober := NewNoOpProber()
	monitor.SetProber(prober)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 等待几次探测
	time.Sleep(200 * time.Millisecond)

	// 状态应该保持健康
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state with NoOpProber, got %v", monitor.GetState())
	}
}

// ============================================================================
//                              T15: SystemWatcher 集成测试
// ============================================================================

// TestMonitor_SystemWatcherIntegration 测试 Monitor 与 SystemWatcher 的集成
//
// 验证：
// 1. SystemWatcher 正确设置和启动
// 2. 系统事件正确传递到 Monitor
// 3. 重大变化触发状态变更
func TestMonitor_SystemWatcherIntegration(t *testing.T) {
	config := DefaultConfig()
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)
	watcher := NewMockSystemWatcher()
	monitor.SetSystemWatcher(watcher)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stateCh := monitor.Subscribe()
	defer monitor.Unsubscribe(stateCh)

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 场景1: 网卡下线（重大变化）
	watcher.SendEvent(NetworkEvent{
		Type:      EventInterfaceDown,
		Interface: "eth0",
		Timestamp: time.Now(),
	})

	change := waitForStateChange(t, stateCh, 200*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDegraded {
		t.Errorf("expected Degraded after interface down, got %v", change.CurrentState)
	}
	if change.Reason != interfaces.ReasonNetworkChanged {
		t.Errorf("expected ReasonNetworkChanged, got %v", change.Reason)
	}

	// 场景2: 网关变化（重大变化）
	monitor.Reset() // 重置到健康状态
	time.Sleep(50 * time.Millisecond)

	watcher.SendEvent(NetworkEvent{
		Type:      EventGatewayChanged,
		Interface: "eth0",
		Timestamp: time.Now(),
	})

	change = waitForStateChange(t, stateCh, 200*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDegraded {
		t.Errorf("expected Degraded after gateway change, got %v", change.CurrentState)
	}

	// 场景3: 地址添加（非重大变化）
	monitor.Reset()
	time.Sleep(50 * time.Millisecond)

	watcher.SendEvent(NetworkEvent{
		Type:      EventAddressAdded,
		Interface: "eth0",
		Address:   "192.168.1.100",
		Timestamp: time.Now(),
	})

	// 不应触发状态变更
	select {
	case <-stateCh:
		t.Error("unexpected state change for address added")
	case <-time.After(100 * time.Millisecond):
		// 正常，非重大变化不触发
	}
}

// TestMonitor_NoOpWatcher 测试 NoOpWatcher 集成
func TestMonitor_NoOpWatcher(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	watcher := NewNoOpWatcher()
	monitor.SetSystemWatcher(watcher)

	ctx := context.Background()
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 验证不会收到任何事件
	time.Sleep(100 * time.Millisecond)

	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state with NoOpWatcher, got %v", monitor.GetState())
	}
}

// ============================================================================
//                              T16: NetworkMonitor 集成测试
// ============================================================================

// TestMonitor_NetworkMonitorIntegration 测试 Monitor 与 NetworkMonitor 的集成
//
// 验证：
// 1. NetworkMonitor 正确设置和订阅
// 2. 网络变化事件正确处理
// 3. 主要变化触发状态变更
func TestMonitor_NetworkMonitorIntegration(t *testing.T) {
	config := DefaultConfig()
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)
	networkMonitor := NewMockNetworkMonitor()
	monitor.SetNetworkMonitor(networkMonitor)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stateCh := monitor.Subscribe()
	defer monitor.Unsubscribe(stateCh)

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 场景1: 主要网络变化（网卡切换）
	networkMonitor.SendEvent(interfaces.NetworkChangeEvent{
		Type:     interfaces.NetworkChangeMajor,
		OldAddrs: []string{"/ip4/192.168.1.100"},
		NewAddrs: []string{"/ip4/10.0.0.100"},
	})

	change := waitForStateChange(t, stateCh, 200*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDegraded {
		t.Errorf("expected Degraded after major network change, got %v", change.CurrentState)
	}
	if change.Reason != interfaces.ReasonNetworkChanged {
		t.Errorf("expected ReasonNetworkChanged, got %v", change.Reason)
	}

	// 场景2: 次要网络变化
	monitor.Reset()
	time.Sleep(50 * time.Millisecond)

	networkMonitor.SendEvent(interfaces.NetworkChangeEvent{
		Type:     interfaces.NetworkChangeMinor,
		OldAddrs: []string{"/ip4/192.168.1.100"},
		NewAddrs: []string{"/ip4/192.168.1.100", "/ip6/::1"},
	})

	// 次要变化不应触发状态变更
	select {
	case <-stateCh:
		t.Error("unexpected state change for minor network change")
	case <-time.After(100 * time.Millisecond):
		// 正常
	}
}

// TestMonitor_AllComponentsIntegration 测试所有组件集成
//
// 验证：
// 1. Prober + SystemWatcher + NetworkMonitor 同时工作
// 2. 多个事件源协同工作
// 3. 状态变更正确处理
func TestMonitor_AllComponentsIntegration(t *testing.T) {
	config := DefaultConfig()
	config.ProbeInterval = 200 * time.Millisecond
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	// 配置所有组件
	prober := NewMockProber()
	watcher := NewMockSystemWatcher()
	networkMonitor := NewMockNetworkMonitor()

	monitor.SetProber(prober)
	monitor.SetSystemWatcher(watcher)
	monitor.SetNetworkMonitor(networkMonitor)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stateCh := monitor.Subscribe()
	defer monitor.Unsubscribe(stateCh)

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer monitor.Stop()

	// 验证所有组件都在运行
	time.Sleep(100 * time.Millisecond)

	// 1. 通过探测检测到问题
	prober.SetResult(false, 0, 3)
	change := waitForStateChange(t, stateCh, 400*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDown {
		t.Errorf("expected Down from prober, got %v", change.CurrentState)
	}

	// 2. 通过探测恢复
	prober.SetResult(true, 3, 3)
	change = waitForStateChange(t, stateCh, 400*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy from prober, got %v", change.CurrentState)
	}

	// 3. 通过系统事件检测变化
	watcher.SendEvent(NetworkEvent{
		Type:      EventInterfaceDown,
		Interface: "eth0",
	})
	change = waitForStateChange(t, stateCh, 200*time.Millisecond)
	if change.CurrentState != interfaces.ConnectionDegraded {
		t.Errorf("expected Degraded from watcher, got %v", change.CurrentState)
	}

	// 验证探测至少执行了几次
	if prober.GetProbeCount() < 2 {
		t.Errorf("expected at least 2 probes, got %d", prober.GetProbeCount())
	}
}

// ============================================================================
//                              T17: 边界测试
// ============================================================================

// TestMonitor_EdgeCases_EmptyPeerID 测试空 Peer ID
func TestMonitor_EdgeCases_EmptyPeerID(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 空 peer ID 不应导致崩溃
	monitor.OnSendError("", errors.New("test error"))
	monitor.OnSendSuccess("")

	// 验证不崩溃
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state, got %v", monitor.GetState())
	}
}

// TestMonitor_EdgeCases_LongPeerID 测试超长 Peer ID
func TestMonitor_EdgeCases_LongPeerID(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 创建一个超长的 peer ID
	longPeer := string(make([]byte, 10000))
	for i := range longPeer {
		longPeer = longPeer[:i] + "a"
	}

	// 不应导致崩溃或性能问题
	monitor.OnSendError(longPeer, errors.New("test error"))
	monitor.OnSendSuccess(longPeer)

	snapshot := monitor.GetSnapshot()
	if snapshot.TotalPeers != 1 {
		t.Errorf("expected 1 peer, got %d", snapshot.TotalPeers)
	}
}

// TestMonitor_EdgeCases_NilError 测试 nil 错误
func TestMonitor_EdgeCases_NilError(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// nil 错误不应触发任何处理
	monitor.OnSendError("peer-1", nil)

	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state after nil error, got %v", monitor.GetState())
	}

	snapshot := monitor.GetSnapshot()
	if snapshot.TotalPeers != 0 {
		t.Errorf("expected 0 peers after nil error, got %d", snapshot.TotalPeers)
	}
}

// TestMonitor_EdgeCases_NilConfig 测试 nil 配置
func TestMonitor_EdgeCases_NilConfig(t *testing.T) {
	// 使用 nil 配置创建监控器（应使用默认配置）
	monitor := NewMonitor(nil)

	if monitor == nil {
		t.Fatal("NewMonitor returned nil with nil config")
	}

	ctx := context.Background()
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start failed with nil config: %v", err)
	}
	defer monitor.Stop()

	// 验证使用了默认配置
	monitor.OnSendError("peer-1", errors.New("test"))
	monitor.OnSendError("peer-1", errors.New("test"))
	monitor.OnSendError("peer-1", errors.New("test"))

	// 等待防抖时间（默认 500ms）+ 额外时间
	time.Sleep(600 * time.Millisecond)

	// 默认阈值是 3，应该触发状态变更
	state := monitor.GetState()
	if state == interfaces.ConnectionHealthy {
		t.Error("expected state change with default config")
	}
}

// TestMonitor_EdgeCases_MultipleStart 测试多次启动
func TestMonitor_EdgeCases_MultipleStart(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()

	// 第一次启动
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	// 第二次启动（应该被忽略）
	if err := monitor.Start(ctx); err != nil {
		t.Errorf("second Start should not return error: %v", err)
	}

	// 验证仍然正常工作
	monitor.OnSendSuccess("peer-1")
	if monitor.GetState() != interfaces.ConnectionHealthy {
		t.Errorf("expected Healthy state, got %v", monitor.GetState())
	}

	monitor.Stop()
}

// TestMonitor_EdgeCases_StopWithoutStart 测试未启动就停止
func TestMonitor_EdgeCases_StopWithoutStart(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	// 未启动就停止（不应崩溃）
	if err := monitor.Stop(); err != nil {
		t.Errorf("Stop without Start should not fail: %v", err)
	}
}

// TestMonitor_EdgeCases_RapidStateChanges 测试快速状态变更
func TestMonitor_EdgeCases_RapidStateChanges(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 10 * time.Millisecond

	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 快速触发多次状态变更
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			monitor.OnSendError("peer-1", errors.New("test"))
		} else {
			monitor.OnSendSuccess("peer-1")
		}
		time.Sleep(time.Millisecond)
	}

	// 等待防抖
	time.Sleep(100 * time.Millisecond)

	// 验证不崩溃
	snapshot := monitor.GetSnapshot()
	if snapshot.TotalPeers != 1 {
		t.Errorf("expected 1 peer, got %d", snapshot.TotalPeers)
	}
}

// TestMonitor_EdgeCases_UnsubscribeNonExistent 测试取消不存在的订阅
func TestMonitor_EdgeCases_UnsubscribeNonExistent(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 10 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 创建一个独立的通道
	ch := make(chan interfaces.ConnectionHealthChange, 10)

	// 取消不存在的订阅（不应崩溃）
	monitor.Unsubscribe(ch)

	// 验证仍然正常工作
	realCh := monitor.Subscribe()
	monitor.OnSendError("peer-1", errors.New("test"))

	// 应该能收到事件
	select {
	case <-realCh:
		// 正常
	case <-time.After(200 * time.Millisecond):
		t.Error("did not receive event after unsubscribe non-existent")
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// waitForStateChange 等待状态变更
func waitForStateChange(t *testing.T, ch <-chan interfaces.ConnectionHealthChange, timeout time.Duration) interfaces.ConnectionHealthChange {
	select {
	case change := <-ch:
		return change
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for state change after %v", timeout)
		return interfaces.ConnectionHealthChange{}
	}
}
