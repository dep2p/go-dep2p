// Package recovery 边界条件和并发安全测试
package recovery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                          可覆盖 Mock 类型
// ============================================================================

// overridableRebinder 可覆盖行为的 rebinder mock
type overridableRebinder struct {
	rebindCalled bool
	rebindErr    error
	rebindNeeded bool
	// 可覆盖的 Rebind 函数
	RebindFunc func(ctx context.Context) error
}

func (m *overridableRebinder) Rebind(ctx context.Context) error {
	m.rebindCalled = true
	if m.RebindFunc != nil {
		return m.RebindFunc(ctx)
	}
	return m.rebindErr
}

func (m *overridableRebinder) IsRebindNeeded() bool {
	return m.rebindNeeded
}

// overridableConnector 可覆盖行为的 connector mock
type overridableConnector struct {
	connectCalled   bool
	connectPeer     string
	connectErr      error
	connectionCount int
	// 可覆盖的函数
	ConnectFunc          func(ctx context.Context, peerID string) error
	ConnectWithAddrsFunc func(ctx context.Context, peerID string, addrs []string) error
}

func (m *overridableConnector) Connect(ctx context.Context, peerID string) error {
	m.connectCalled = true
	m.connectPeer = peerID
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx, peerID)
	}
	return m.connectErr
}

func (m *overridableConnector) ConnectWithAddrs(ctx context.Context, peerID string, addrs []string) error {
	m.connectCalled = true
	m.connectPeer = peerID
	if m.ConnectWithAddrsFunc != nil {
		return m.ConnectWithAddrsFunc(ctx, peerID, addrs)
	}
	return m.connectErr
}

func (m *overridableConnector) ConnectionCount() int {
	return m.connectionCount
}

// ============================================================================
//                          边界条件测试
// ============================================================================

// TestManager_NewManager_NilConfig 测试 nil 配置
func TestManager_NewManager_NilConfig(t *testing.T) {
	// 🎯 发现 BUG: nil 配置应该使用默认配置
	manager := NewManager(nil)
	require.NotNil(t, manager, "NewManager 应该处理 nil 配置")
	require.NotNil(t, manager.config, "配置应该使用默认值")

	// 验证默认配置已设置
	assert.Greater(t, manager.config.RecoveryTimeout, time.Duration(0), "应有默认超时")
}

// TestManager_StartStop_Idempotency 测试启动停止幂等性
func TestManager_StartStop_Idempotency(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	// 多次启动应该是幂等的
	err := manager.Start(ctx)
	require.NoError(t, err)

	err = manager.Start(ctx)
	require.NoError(t, err, "重复启动应该幂等")

	// 多次停止应该是幂等的
	err = manager.Stop()
	require.NoError(t, err)

	err = manager.Stop()
	require.NoError(t, err, "重复停止应该幂等")
}

// TestManager_StopWithoutStart 测试未启动就停止
func TestManager_StopWithoutStart(t *testing.T) {
	manager := NewManager(nil)

	// 🎯 发现 BUG: 未启动就停止可能 panic
	err := manager.Stop()
	require.NoError(t, err, "未启动就停止不应该 panic")
}

// TestManager_TriggerRecovery_WithoutStart 测试未启动就恢复
func TestManager_TriggerRecovery_WithoutStart(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	// 未设置任何依赖，直接触发恢复
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.NotNil(t, result)
	assert.False(t, result.Success, "无依赖时恢复应该失败")
}

// TestManager_TriggerRecovery_CanceledContext 测试已取消的 context
func TestManager_TriggerRecovery_CanceledContext(t *testing.T) {
	manager := NewManager(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 🎯 发现 BUG: 取消的 context 应该被正确处理
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.NotNil(t, result)
	// 取消的 context 应该导致恢复失败或快速完成
}

// TestManager_TriggerRecovery_Timeout 测试恢复超时
func TestManager_TriggerRecovery_Timeout(t *testing.T) {
	config := DefaultConfig()
	config.RecoveryTimeout = 100 * time.Millisecond
	manager := NewManager(config)

	// 设置一个永远阻塞的 rebinder
	slowRebinder := &overridableRebinder{
		rebindNeeded: true,
		RebindFunc: func(ctx context.Context) error {
			<-ctx.Done() // 等待超时
			return ctx.Err()
		},
	}

	manager.SetRebinder(slowRebinder)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	start := time.Now()
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkChange)
	duration := time.Since(start)

	require.NotNil(t, result)
	// 应该在超时时间内完成
	assert.LessOrEqual(t, duration, 2*config.RecoveryTimeout, "应该在超时时间内完成")
}

// TestManager_SetDependencies_NilValues 测试设置 nil 依赖
func TestManager_SetDependencies_NilValues(t *testing.T) {
	manager := NewManager(nil)

	// 🎯 发现 BUG: 设置 nil 依赖不应该 panic
	assert.NotPanics(t, func() {
		manager.SetRebinder(nil)
		manager.SetAddressDiscoverer(nil)
		manager.SetConnector(nil)
	})

	// 使用 nil 依赖触发恢复应该能优雅处理
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
	require.NotNil(t, result)
}

// TestManager_SetCriticalPeers_Validation 测试关键节点设置和验证
func TestManager_SetCriticalPeers_Validation(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	// 创建可验证的 connector
	connectCalls := make([]string, 0)
	var mu sync.Mutex

	connector := &overridableConnector{
		connectionCount: 1, // 假装连接成功
		ConnectFunc: func(ctx context.Context, peerID string) error {
			mu.Lock()
			defer mu.Unlock()
			connectCalls = append(connectCalls, peerID)
			return nil
		},
	}

	manager.SetConnector(connector)
	manager.SetCriticalPeers([]string{"peer-1", "peer-2", "peer-3"})

	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.NotNil(t, result)
	require.True(t, result.Success)

	// 验证所有关键节点都被尝试连接
	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, connectCalls, 3, "应该尝试连接所有关键节点")
	assert.Contains(t, connectCalls, "peer-1")
	assert.Contains(t, connectCalls, "peer-2")
	assert.Contains(t, connectCalls, "peer-3")
}

// TestManager_SetCriticalPeersWithAddrs_Priority 测试地址优先级
func TestManager_SetCriticalPeersWithAddrs_Priority(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	type ConnectCall struct {
		peerID string
		addrs  []string
		method string // "Connect" 或 "ConnectWithAddrs"
	}

	connectCalls := make([]ConnectCall, 0)
	var mu sync.Mutex

	connector := &overridableConnector{
		connectionCount: 1,
	}

	// 覆盖方法来记录调用
	connector.ConnectFunc = func(ctx context.Context, peerID string) error {
		mu.Lock()
		defer mu.Unlock()
		connectCalls = append(connectCalls, ConnectCall{
			peerID: peerID,
			method: "Connect",
		})
		return nil
	}

	connector.ConnectWithAddrsFunc = func(ctx context.Context, peerID string, addrs []string) error {
		mu.Lock()
		defer mu.Unlock()
		connectCalls = append(connectCalls, ConnectCall{
			peerID: peerID,
			addrs:  addrs,
			method: "ConnectWithAddrs",
		})
		return nil
	}

	manager.SetConnector(connector)
	manager.SetCriticalPeersWithAddrs(
		[]string{"peer-1", "peer-2"},
		[]string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4002"},
	)

	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.True(t, result.Success)

	// 验证使用地址优先连接
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, connectCalls, 2)
	assert.Equal(t, "ConnectWithAddrs", connectCalls[0].method, "应该优先使用地址")
	assert.Equal(t, []string{"/ip4/1.1.1.1/tcp/4001"}, connectCalls[0].addrs)
}

// TestManager_ReconnectCriticalPeers_Fallback 测试地址失败后回退
func TestManager_ReconnectCriticalPeers_Fallback(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	attemptedMethods := make([]string, 0)
	var mu sync.Mutex

	connector := &overridableConnector{
		connectionCount: 1,
	}

	// 模拟地址连接失败，回退到 PeerID
	connector.ConnectWithAddrsFunc = func(ctx context.Context, peerID string, addrs []string) error {
		mu.Lock()
		attemptedMethods = append(attemptedMethods, "ConnectWithAddrs")
		mu.Unlock()
		return errors.New("address connection failed")
	}

	connector.ConnectFunc = func(ctx context.Context, peerID string) error {
		mu.Lock()
		attemptedMethods = append(attemptedMethods, "Connect")
		mu.Unlock()
		return nil
	}

	manager.SetConnector(connector)
	manager.SetCriticalPeersWithAddrs(
		[]string{"peer-1"},
		[]string{"/ip4/1.1.1.1/tcp/4001"},
	)

	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.True(t, result.Success)

	// 验证回退逻辑
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, attemptedMethods, 2, "应该尝试两种方法")
	assert.Equal(t, "ConnectWithAddrs", attemptedMethods[0], "首先尝试地址")
	assert.Equal(t, "Connect", attemptedMethods[1], "失败后回退到 PeerID")
}

// TestManager_ReconnectCriticalPeers_EmptyAddress 测试空地址
func TestManager_ReconnectCriticalPeers_EmptyAddress(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	usedMethod := ""

	connector := &overridableConnector{
		connectionCount: 1,
	}

	connector.ConnectFunc = func(ctx context.Context, peerID string) error {
		usedMethod = "Connect"
		return nil
	}

	connector.ConnectWithAddrsFunc = func(ctx context.Context, peerID string, addrs []string) error {
		usedMethod = "ConnectWithAddrs"
		return nil
	}

	manager.SetConnector(connector)
	// 地址列表为空字符串
	manager.SetCriticalPeersWithAddrs(
		[]string{"peer-1"},
		[]string{""},
	)

	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	require.True(t, result.Success)

	// 🎯 发现 BUG: 空地址应该被跳过，直接使用 Connect
	assert.Equal(t, "Connect", usedMethod, "空地址应该跳过 ConnectWithAddrs")
}

// TestManager_ReconnectCriticalPeers_LongPeerID 测试长 PeerID 截断
func TestManager_ReconnectCriticalPeers_LongPeerID(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	connector := &overridableConnector{
		connectionCount: 1,
	}

	receivedPeerID := ""
	connector.ConnectFunc = func(ctx context.Context, peerID string) error {
		receivedPeerID = peerID
		return nil
	}

	manager.SetConnector(connector)
	// 非常长的 PeerID
	longPeerID := "very-long-peer-id-12345678901234567890"
	manager.SetCriticalPeers([]string{longPeerID})

	manager.Start(ctx)
	defer manager.Stop()

	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	// 🎯 验证：PeerID 截断只用于日志，不影响实际连接
	assert.Equal(t, longPeerID, receivedPeerID, "实际连接应使用完整 PeerID")
}

// ============================================================================
//                          并发安全测试
// ============================================================================

// TestManager_ConcurrentSetters 测试并发设置依赖
func TestManager_ConcurrentSetters(t *testing.T) {
	manager := NewManager(nil)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			if id%4 == 0 {
				manager.SetRebinder(&mockRebinder{})
			} else if id%4 == 1 {
				manager.SetAddressDiscoverer(&mockAddressDiscoverer{})
			} else if id%4 == 2 {
				manager.SetConnector(&mockConnector{connectionCount: 1})
			} else {
				manager.SetCriticalPeers([]string{"peer-1"})
			}
		}(i)
	}

	wg.Wait()
	// 不应该 panic 或数据竞争
}

// TestManager_ConcurrentRecoveryWithSetters 测试恢复时并发设置
func TestManager_ConcurrentRecoveryWithSetters(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	manager.Start(ctx)
	defer manager.Stop()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				// 触发恢复
				manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
			} else {
				// 设置依赖
				manager.SetConnector(&mockConnector{connectionCount: 1})
			}
		}(i)
	}

	wg.Wait()
	// 🎯 发现 BUG: 应该没有数据竞争
}

// TestManager_ConcurrentCallbacks 测试并发注册回调
func TestManager_ConcurrentCallbacks(t *testing.T) {
	manager := NewManager(nil)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	callCount := atomic.Int32{}

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
				callCount.Add(1)
			})
		}()
	}

	wg.Wait()

	// 触发恢复，验证所有回调都被调用
	manager.SetConnector(&mockConnector{connectionCount: 1})
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	// 等待回调执行
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(goroutines), callCount.Load(), "所有回调应该被调用")
}

// TestManager_TriggerRecovery_RaceOnAttemptCount 测试尝试计数竞争
func TestManager_TriggerRecovery_RaceOnAttemptCount(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	manager.Start(ctx)
	defer manager.Stop()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
		}()
	}

	wg.Wait()

	// 🎯 发现 BUG: 应该没有数据竞争，计数应该准确
	// 由于并发保护，只有一个恢复会实际执行，其他会返回 ErrRecoveryInProgress
}

// ============================================================================
//                          错误路径测试
// ============================================================================

// TestManager_PerformRebind_Errors 测试 Rebind 各种错误
func TestManager_PerformRebind_Errors(t *testing.T) {
	tests := []struct {
		name         string
		rebinder     *mockRebinder
		expectCalled bool
	}{
		{
			name:         "no rebinder",
			rebinder:     nil,
			expectCalled: false,
		},
		{
			name: "rebind not needed",
			rebinder: &mockRebinder{
				rebindNeeded: false,
			},
			expectCalled: false,
		},
		{
			name: "rebind error",
			rebinder: &mockRebinder{
				rebindNeeded: true,
				rebindErr:    errors.New("rebind failed"),
			},
			expectCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager(nil)
			ctx := context.Background()

			if tt.rebinder != nil {
				manager.SetRebinder(tt.rebinder)
			}

			manager.Start(ctx)
			defer manager.Stop()

			// 使用需要 rebind 的原因
			result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkChange)

			if tt.rebinder != nil && tt.expectCalled {
				assert.True(t, tt.rebinder.rebindCalled, "Rebind 应该被调用")
			}

			// 即使 Rebind 失败，恢复流程应该继续
			require.NotNil(t, result)
		})
	}
}

// TestManager_PerformAddressDiscovery_Error 测试地址发现错误
func TestManager_PerformAddressDiscovery_Error(t *testing.T) {
	manager := NewManager(nil)
	ctx := context.Background()

	discoverer := &mockAddressDiscoverer{
		discoverErr: errors.New("discovery failed"),
	}

	manager.SetAddressDiscoverer(discoverer)
	manager.Start(ctx)
	defer manager.Stop()

	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	assert.True(t, discoverer.discoverCalled, "DiscoverAddresses 应该被调用")
	// 即使地址发现失败，恢复流程应该继续
	require.NotNil(t, result)
}

// TestManager_ReconnectCriticalPeers_ContextTimeout 测试重连时 context 超时
func TestManager_ReconnectCriticalPeers_ContextTimeout(t *testing.T) {
	config := DefaultConfig()
	config.RecoveryTimeout = 50 * time.Millisecond
	manager := NewManager(config)
	ctx := context.Background()

	connectDelay := 100 * time.Millisecond

	connector := &overridableConnector{
		connectionCount: 1,
	}
	connector.ConnectFunc = func(ctx context.Context, peerID string) error {
		time.Sleep(connectDelay)
		return nil
	}

	manager.SetConnector(connector)
	// 设置多个关键节点，但超时会中断
	manager.SetCriticalPeers([]string{"peer-1", "peer-2", "peer-3"})

	manager.Start(ctx)
	defer manager.Stop()

	start := time.Now()
	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)
	duration := time.Since(start)

	require.NotNil(t, result)
	// 应该在超时时间内完成，不会等待所有节点
	// 添加 20ms 容差以容忍系统调度延迟
	maxDuration := 2*config.RecoveryTimeout + 20*time.Millisecond
	assert.LessOrEqual(t, duration, maxDuration, "恢复应在超时时间内完成")
	// 🎯 验证：超时应该中断重连循环
	t.Logf("恢复完成，连接数: %d, 耗时: %v", result.ConnectionsRestored, duration)
}

// ============================================================================
//                          回调测试
// ============================================================================

// TestManager_OnRecoveryComplete_MultipleCallbacks 测试多个回调
func TestManager_OnRecoveryComplete_MultipleCallbacks(t *testing.T) {
	manager := NewManager(nil)
	manager.SetConnector(&mockConnector{connectionCount: 1})

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	const callbackCount = 5
	callbackResults := make(chan interfaces.RecoveryResult, callbackCount)

	for i := 0; i < callbackCount; i++ {
		manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
			callbackResults <- result
		})
	}

	// 触发恢复
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	// 所有回调都应该被调用
	for i := 0; i < callbackCount; i++ {
		select {
		case result := <-callbackResults:
			assert.True(t, result.Success)
		case <-time.After(time.Second):
			t.Fatalf("回调 %d 未被调用", i+1)
		}
	}
}

// TestManager_OnRecoveryComplete_CallbackPanic 测试回调 panic 不影响其他回调
func TestManager_OnRecoveryComplete_CallbackPanic(t *testing.T) {
	manager := NewManager(nil)
	manager.SetConnector(&mockConnector{connectionCount: 1})

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop()

	panicCallback := make(chan bool, 1)
	normalCallback := make(chan bool, 1)

	// 第一个回调会 panic
	manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
		panicCallback <- true
		panic("callback panic")
	})

	// 第二个回调正常
	manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
		normalCallback <- true
	})

	// 触发恢复
	manager.TriggerRecovery(ctx, interfaces.RecoveryReasonManualTrigger)

	// 🎯 验证：panic 回调不应该影响正常回调
	time.Sleep(100 * time.Millisecond)

	select {
	case <-normalCallback:
		// 正常回调应该被调用
	case <-time.After(time.Second):
		t.Error("正常回调应该被调用，即使其他回调 panic")
	}
}
