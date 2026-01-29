// Package recovery 网络恢复管理
package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// mockRebinder 模拟重绑定器
type mockRebinder struct {
	rebindCalled    bool
	rebindErr       error
	rebindNeeded    bool
}

func (m *mockRebinder) Rebind(ctx context.Context) error {
	m.rebindCalled = true
	return m.rebindErr
}

func (m *mockRebinder) IsRebindNeeded() bool {
	return m.rebindNeeded
}

// mockAddressDiscoverer 模拟地址发现器
type mockAddressDiscoverer struct {
	discoverCalled bool
	discoverErr    error
}

func (m *mockAddressDiscoverer) DiscoverAddresses(ctx context.Context) error {
	m.discoverCalled = true
	return m.discoverErr
}

// mockConnector 模拟连接器
type mockConnector struct {
	connectCalled    bool
	connectPeer      string
	connectErr       error
	connectionCount  int
}

func (m *mockConnector) Connect(ctx context.Context, peerID string) error {
	m.connectCalled = true
	m.connectPeer = peerID
	return m.connectErr
}

func (m *mockConnector) ConnectWithAddrs(ctx context.Context, peerID string, addrs []string) error {
	m.connectCalled = true
	m.connectPeer = peerID
	return m.connectErr
}

func (m *mockConnector) ConnectionCount() int {
	return m.connectionCount
}

// TestManager_NewManager 测试创建管理器
func TestManager_NewManager(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.IsRecovering() {
		t.Error("new manager should not be recovering")
	}
}

// TestManager_StartStop 测试启动和停止
func TestManager_StartStop(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestManager_SetDependencies 测试设置依赖
func TestManager_SetDependencies(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	rebinder := &mockRebinder{rebindNeeded: true}
	discoverer := &mockAddressDiscoverer{}
	connector := &mockConnector{connectionCount: 1} // 模拟有连接

	manager.SetRebinder(rebinder)
	manager.SetAddressDiscoverer(discoverer)
	manager.SetConnector(connector)

	// 验证依赖已设置（通过触发恢复来验证）
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 使用需要 rebind 的原因
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkChange)

	// Rebind 和 Discover 应该被调用
	if !rebinder.rebindCalled {
		t.Error("rebinder should have been called")
	}
	if !discoverer.discoverCalled {
		t.Error("discoverer should have been called")
	}

	// 结果应该成功（connector 返回有连接）
	if !result.Success {
		t.Errorf("expected recovery to succeed, got error: %v", result.Error)
	}
}

// TestManager_SetCriticalPeers 测试设置关键节点
func TestManager_SetCriticalPeers(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	peers := []string{"peer-1", "peer-2"}
	manager.SetCriticalPeers(peers)

	// 无法直接验证，但可以通过触发恢复来间接验证
}

// TestManager_SetCriticalPeersWithAddrs 测试设置带地址的关键节点
func TestManager_SetCriticalPeersWithAddrs(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	peers := []string{"peer-1", "peer-2"}
	addrs := []string{"/ip4/127.0.0.1/tcp/4001", "/ip4/127.0.0.1/tcp/4002"}
	manager.SetCriticalPeersWithAddrs(peers, addrs)

	// 无法直接验证，但可以通过触发恢复来间接验证
}

// TestManager_TriggerRecovery 测试触发恢复
func TestManager_TriggerRecovery(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	rebinder := &mockRebinder{rebindNeeded: true}
	discoverer := &mockAddressDiscoverer{}
	connector := &mockConnector{connectionCount: 1}

	manager.SetRebinder(rebinder)
	manager.SetAddressDiscoverer(discoverer)
	manager.SetConnector(connector)
	manager.SetCriticalPeers([]string{"peer-1"})

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkChange)

	if !result.Success {
		t.Errorf("expected recovery to succeed, got error: %v", result.Error)
	}

	if result.Reason != interfaces.RecoveryReasonNetworkChange {
		t.Errorf("expected reason to be NetworkChange, got %v", result.Reason)
	}

	if connector.connectPeer != "peer-1" {
		t.Errorf("expected connector to connect peer-1, got %s", connector.connectPeer)
	}
}

// TestManager_TriggerRecoveryWithError 测试恢复失败
func TestManager_TriggerRecoveryWithError(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	rebinder := &mockRebinder{rebindErr: errors.New("rebind failed"), rebindNeeded: true}
	manager.SetRebinder(rebinder)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 使用需要 rebind 的原因
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkChange)

	// 即使 Rebind 失败，恢复仍应继续
	// 只有关键节点连接失败才会导致整体失败
	if !rebinder.rebindCalled {
		t.Error("rebinder should have been called")
	}

	// 没有 connector，恢复应该失败
	if result.Success {
		t.Error("expected recovery to fail without connector")
	}
}

// TestManager_IsRecovering 测试恢复状态检查
func TestManager_IsRecovering(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	if manager.IsRecovering() {
		t.Error("should not be recovering initially")
	}

	// 设置一个慢的 rebinder 来让恢复持续一段时间
	slowRebinder := &mockRebinder{}
	manager.SetRebinder(slowRebinder)

	// 异步触发恢复
	go func() {
		manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
	}()

	// 短暂等待让恢复开始
	time.Sleep(10 * time.Millisecond)

	// 此时可能正在恢复，也可能已完成（取决于时序）
	// 这个测试主要验证 IsRecovering 不会 panic
	_ = manager.IsRecovering()
}

// TestManager_GetAttemptCount 测试获取尝试次数
func TestManager_GetAttemptCount(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	if manager.GetAttemptCount() != 0 {
		t.Error("initial attempt count should be 0")
	}

	// 触发恢复（不设置 connector，会失败但计数会增加）
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	if manager.GetAttemptCount() != 1 {
		t.Errorf("expected attempt count to be 1, got %d", manager.GetAttemptCount())
	}

	// 再次触发
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	if manager.GetAttemptCount() != 2 {
		t.Errorf("expected attempt count to be 2, got %d", manager.GetAttemptCount())
	}
}

// TestManager_ResetAttempts 测试重置尝试次数
func TestManager_ResetAttempts(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 触发几次恢复
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	if manager.GetAttemptCount() != 2 {
		t.Errorf("expected attempt count to be 2, got %d", manager.GetAttemptCount())
	}

	// 重置
	manager.ResetAttempts()

	if manager.GetAttemptCount() != 0 {
		t.Errorf("expected attempt count to be 0 after reset, got %d", manager.GetAttemptCount())
	}
}

// TestManager_OnRecoveryComplete 测试恢复完成回调
func TestManager_OnRecoveryComplete(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	// 设置 connector 使恢复成功
	connector := &mockConnector{connectionCount: 1}
	manager.SetConnector(connector)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	callbackCalled := make(chan bool, 1)
	var callbackResult interfaces.RecoveryResult

	manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
		callbackResult = result
		callbackCalled <- true
	})

	// 触发恢复
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	// 等待回调被调用
	select {
	case <-callbackCalled:
		// 回调被调用
	case <-time.After(time.Second):
		t.Error("callback should have been called")
		return
	}

	if !callbackResult.Success {
		t.Error("callback result should indicate success")
	}
}

// TestManager_ConcurrentRecovery 测试并发恢复
func TestManager_ConcurrentRecovery(t *testing.T) {
	config := DefaultConfig()
	manager := NewManager(config)

	// 设置 connector 使恢复成功
	connector := &mockConnector{connectionCount: 1}
	manager.SetConnector(connector)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	// 并发触发多次恢复
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
			done <- true
		}()
	}

	// 等待所有完成
	for i := 0; i < 5; i++ {
		<-done
	}

	// 不应该 panic
	// 由于并发保护和成功后重置，实际计数可能被重置
}
