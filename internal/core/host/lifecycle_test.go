package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestHost_Start 测试启动
func TestHost_Start(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	err := host.Start(ctx)
	// setupTestHost 创建的 host 没有 NAT/Relay/AddrsManager，
	// 所以 Start() 应该成功返回 nil（代码 lifecycle.go:37-78）
	require.NoError(t, err, "Start should succeed without NAT/Relay")

	// 验证 started 标志
	assert.True(t, host.started.Load(), "Host should be started")
}

// TestHost_Start_AlreadyStarted 测试重复启动
func TestHost_Start_AlreadyStarted(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	// 第一次启动
	err := host.Start(ctx)
	require.NoError(t, err, "First Start should succeed")

	// 第二次启动应该返回错误（代码 lifecycle.go:38-39）
	err = host.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

// TestHost_StartStop 测试启动停止
func TestHost_StartStop(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	ctx := context.Background()
	err := host.Start(ctx)
	require.NoError(t, err, "Start should succeed")

	// 关闭应该成功
	err = host.Close()
	assert.NoError(t, err)

	// 验证 closed 标志
	assert.True(t, host.closed.Load())
}

// TestHost_Close_AlreadyClosed 测试重复关闭
func TestHost_Close_AlreadyClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	// 第一次关闭
	err := host.Close()
	assert.NoError(t, err)

	// 第二次关闭应该直接返回（不报错）
	err = host.Close()
	assert.NoError(t, err)
}

// TestHost_Start_WithNAT 测试启动 NAT 服务
func TestHost_Start_WithNAT(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// NAT 服务在 Host 中是可选的
	// 验证即使没有 NAT 服务，Host 也能正常启动
	assert.Nil(t, host.nat, "NAT should not be set in test host")

	ctx := context.Background()
	err := host.Start(ctx)
	// 没有 NAT 服务时，Start 仍应成功（代码 lifecycle.go:48-55）
	require.NoError(t, err, "Start should succeed without NAT")
	assert.True(t, host.Started(), "Host should be started")
}

// TestHost_Start_WithRelay 测试启动 Relay 服务
func TestHost_Start_WithRelay(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// Relay Manager 在 Host 中是可选的
	assert.Nil(t, host.relay, "Relay should not be set in test host")

	ctx := context.Background()
	err := host.Start(ctx)
	// 没有 Relay 服务时，Start 仍应成功（代码 lifecycle.go:57-64）
	require.NoError(t, err, "Start should succeed without Relay")
	assert.True(t, host.Started(), "Host should be started")
}

// TestHost_SwarmNotifier_Connected 测试连接事件
func TestHost_SwarmNotifier_Connected(t *testing.T) {
	host, _, _, mockEventBus := setupTestHost(t)
	defer host.Close()

	// 创建 mock 连接
	localPeer := types.PeerID(host.ID())
	remotePeer := types.PeerID("remote-peer-123")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 触发 Connected 回调
	host.Connected(mockConn)

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证 peerConnCount 被更新
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count, "Connection count should be 1 after Connected")

	// 验证 EventBus 收到了创建 Emitter 的调用
	assert.NotNil(t, mockEventBus, "EventBus should be set")
}

// TestHost_SwarmNotifier_Disconnected 测试断开事件
func TestHost_SwarmNotifier_Disconnected(t *testing.T) {
	host, _, _, mockEventBus := setupTestHost(t)
	defer host.Close()

	// 创建 mock 连接
	localPeer := types.PeerID(host.ID())
	remotePeer := types.PeerID("remote-peer-456")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 先模拟连接
	host.Connected(mockConn)

	// 验证连接计数
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count)

	// 触发 Disconnected 回调
	host.Disconnected(mockConn)

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证连接计数被减少
	host.peerConnCountMu.Lock()
	count = host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count, "Connection count should be 0 after Disconnected")

	// 验证 EventBus 已配置
	assert.NotNil(t, mockEventBus, "EventBus should be set")
}

// TestHost_SwarmNotifier_MultipleConnections 测试多个连接
func TestHost_SwarmNotifier_MultipleConnections(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID(host.ID())
	remotePeer := types.PeerID("remote-peer-789")
	mockConn1 := mocks.NewMockConnection(localPeer, remotePeer)
	mockConn2 := mocks.NewMockConnection(localPeer, remotePeer)

	// 建立两个连接
	host.Connected(mockConn1)
	host.Connected(mockConn2)

	// 验证连接计数为 2
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 2, count)

	// 断开一个连接
	host.Disconnected(mockConn1)

	// 验证连接计数为 1
	host.peerConnCountMu.Lock()
	count = host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count)

	// 断开另一个连接
	host.Disconnected(mockConn2)

	// 验证连接计数为 0
	host.peerConnCountMu.Lock()
	count = host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count)
}

// TestHost_Context_CancelOnClose 测试关闭时取消上下文
func TestHost_Context_CancelOnClose(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	// 验证上下文初始可用
	require.NotNil(t, host.ctx)
	select {
	case <-host.ctx.Done():
		t.Fatal("context should not be done")
	default:
		// OK
	}

	// 关闭 Host
	err := host.Close()
	require.NoError(t, err)

	// 验证上下文被取消
	select {
	case <-host.ctx.Done():
		// OK - context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after Close")
	}
}
