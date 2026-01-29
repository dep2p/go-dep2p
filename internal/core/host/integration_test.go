package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestHost_TwoHosts_Connect 测试两节点连接（使用 mock）
func TestHost_TwoHosts_Connect(t *testing.T) {
	// 创建两个 mock host
	host1, swarm1, _, _ := setupTestHost(t)
	defer host1.Close()

	host2, swarm2, _, _ := setupTestHost(t)
	defer host2.Close()

	// 配置 swarm1 可以 "连接" 到 swarm2
	peer2ID := host2.ID()
	mockConn := mocks.NewMockConnection(types.PeerID(host1.ID()), types.PeerID(peer2ID))
	swarm1.AddConnection(peer2ID, mockConn)

	ctx := context.Background()

	// 连接
	err := host1.Connect(ctx, peer2ID, []string{"/ip4/127.0.0.1/tcp/4001"})
	assert.NoError(t, err)

	// 验证连接存在
	conns := swarm1.Conns()
	assert.GreaterOrEqual(t, len(conns), 1, "Should have at least 1 connection")

	// 验证 host2 的 swarm 已配置
	assert.NotNil(t, swarm2, "host2's swarm should be configured")

	t.Log("✅ 两节点连接测试（mock）通过")
}

// TestHost_TwoHosts_Stream 测试两节点流创建（使用 mock）
func TestHost_TwoHosts_Stream(t *testing.T) {
	host, swarm, _, _ := setupTestHost(t)
	defer host.Close()

	// 注册协议处理器
	handlerCalled := false
	host.SetStreamHandler("/test/1.0.0", func(s pkgif.Stream) {
		handlerCalled = true
	})

	// 验证协议被注册
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")

	// 配置 mock 连接
	targetPeer := "remote-peer"
	mockConn := mocks.NewMockConnection(types.PeerID(host.ID()), types.PeerID(targetPeer))
	swarm.AddConnection(targetPeer, mockConn)

	// 创建流会触发协议协商
	ctx := context.Background()
	_, err := host.NewStream(ctx, targetPeer, "/test/1.0.0")
	// 由于 mock stream 不支持完整协商，会返回 "protocol negotiation failed" 错误
	// 代码位置: host.go:280-284
	assert.Error(t, err, "NewStream should fail with mock (no protocol negotiation)")
	assert.Contains(t, err.Error(), "protocol negotiation failed", "Error should indicate negotiation failure")

	// handlerCalled 只在入站流时触发，这里是出站流，不会触发
	assert.False(t, handlerCalled, "Handler should not be called for outbound stream")

	t.Log("✅ 两节点流创建测试（mock）通过")
}

// TestHost_NAT_Integration 测试 NAT 集成
func TestHost_NAT_Integration(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// NAT 服务是可选的
	// 验证没有 NAT 服务时 Host 仍能正常工作
	assert.Nil(t, host.nat)

	// 启动 Host
	ctx := context.Background()
	err := host.Start(ctx)
	require.NoError(t, err)

	// 验证 Host 正常运行
	assert.True(t, host.started.Load())

	t.Log("✅ NAT 集成测试（无 NAT）通过")
}

// TestHost_Relay_Integration 测试 Relay 集成
func TestHost_Relay_Integration(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// Relay Manager 是可选的
	// 验证没有 Relay Manager 时 Host 仍能正常工作
	assert.Nil(t, host.relay)

	// 启动 Host
	ctx := context.Background()
	err := host.Start(ctx)
	require.NoError(t, err)

	// 验证 Host 正常运行
	assert.True(t, host.started.Load())

	t.Log("✅ Relay 集成测试（无 Relay）通过")
}

// TestHost_EventBus_Integration 测试 EventBus 集成
func TestHost_EventBus_Integration(t *testing.T) {
	host, swarm, _, eventBus := setupTestHost(t)
	defer host.Close()

	// 验证 EventBus 存在
	assert.NotNil(t, eventBus)

	// 模拟连接事件
	localPeer := types.PeerID(host.ID())
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 触发连接回调
	host.Connected(mockConn)

	// 等待事件处理
	time.Sleep(10 * time.Millisecond)

	// 验证 peerConnCount 被更新
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count, "peerConnCount should be 1 after Connected")

	// 验证 swarm 已配置
	assert.NotNil(t, swarm, "swarm should be configured")

	t.Log("✅ EventBus 集成测试通过")
}

// TestHost_Lifecycle_FullCycle 测试完整生命周期
func TestHost_Lifecycle_FullCycle(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	ctx := context.Background()

	// 1. 创建后状态
	assert.False(t, host.started.Load())
	assert.False(t, host.closed.Load())

	// 2. 启动
	err := host.Start(ctx)
	require.NoError(t, err)
	assert.True(t, host.started.Load())

	// 3. 使用：验证基本操作可用
	assert.NotEmpty(t, host.ID())
	// mux.Protocols() 在没有注册协议时返回 nil 或空切片
	protocols := host.mux.Protocols()
	// 验证没有协议注册（nil 或空切片都可以）
	assert.True(t, protocols == nil || len(protocols) == 0, "Protocols() should be nil or empty without handlers")

	// 4. 关闭
	err = host.Close()
	require.NoError(t, err)
	assert.True(t, host.closed.Load())

	// 5. 验证上下文已取消
	select {
	case <-host.ctx.Done():
		// OK
	default:
		t.Fatal("context should be done after Close")
	}

	t.Log("✅ 完整生命周期测试通过")
}

// TestHost_Components_Initialization 测试组件初始化
func TestHost_Components_Initialization(t *testing.T) {
	swarm := mocks.NewMockSwarm("test-peer")
	peerstore := mocks.NewMockPeerstore()
	eventBus := mocks.NewMockEventBus()

	host, err := New(
		WithSwarm(swarm),
		WithPeerstore(peerstore),
		WithEventBus(eventBus),
	)
	require.NoError(t, err)
	defer host.Close()

	// 验证所有组件已初始化
	assert.NotNil(t, host.swarm)
	assert.NotNil(t, host.mux)
	assert.NotNil(t, host.addrsManager)
	assert.NotNil(t, host.peerConnCount)

	t.Log("✅ 组件初始化测试通过")
}
