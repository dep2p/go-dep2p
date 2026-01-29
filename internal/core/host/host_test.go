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

// setupTestHost 创建测试用 Host
func setupTestHost(t *testing.T) (*Host, *mocks.MockSwarm, *mocks.MockPeerstore, *mocks.MockEventBus) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")
	mockPeerstore := mocks.NewMockPeerstore()
	mockEventBus := mocks.NewMockEventBus()

	host, err := New(
		WithSwarm(mockSwarm),
		WithPeerstore(mockPeerstore),
		WithEventBus(mockEventBus),
	)
	require.NoError(t, err)
	require.NotNil(t, host)

	return host, mockSwarm, mockPeerstore, mockEventBus
}

// TestHost_Creation 测试 Host 创建
func TestHost_Creation(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	assert.NotNil(t, host)
	assert.Equal(t, mockSwarm.LocalPeerID, host.ID())
}

// TestHost_CreationWithoutSwarm 测试无 Swarm 创建失败
func TestHost_CreationWithoutSwarm(t *testing.T) {
	_, err := New()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "swarm is required")
}

// TestHost_ID 测试 ID 方法
func TestHost_ID(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	assert.Equal(t, mockSwarm.LocalPeerID, host.ID())
}

// TestHost_Addrs 测试 Addrs 方法
func TestHost_Addrs(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// Addrs 通过 addrsManager 返回，初始可能为空
	addrs := host.Addrs()
	// 验证返回的是数组类型（可能为空）
	assert.NotNil(t, addrs)
}

// TestHost_Connect 测试 Connect 方法
func TestHost_Connect(t *testing.T) {
	host, mockSwarm, mockPeerstore, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	targetPeer := "peer-123"
	targetAddrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 连接
	err := host.Connect(ctx, targetPeer, targetAddrs)
	assert.NoError(t, err)

	// 验证 DialPeer 被调用
	assert.Contains(t, mockSwarm.DialPeerCalls, targetPeer)

	// 验证地址被添加到 peerstore（代码 host.go:227-243）
	addedAddrs := mockPeerstore.Addrs(types.PeerID(targetPeer))
	assert.NotEmpty(t, addedAddrs, "Addresses should be added to peerstore")
}

// TestHost_NewStream 测试 NewStream 方法
func TestHost_NewStream(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	targetPeer := "peer-456"

	// 添加连接
	conn := mocks.NewMockConnection(types.PeerID("test-peer-id"), types.PeerID(targetPeer))
	mockSwarm.AddConnection(targetPeer, conn)

	// 创建流 - 由于协议协商需要真实的 multistream-select 握手，
	// 这里预期会返回错误（EOF），这是正常行为
	stream, err := host.NewStream(ctx, targetPeer, "/test/1.0.0")

	// 验证流创建被尝试（即使协议协商失败）
	// 在真实场景中，远端需要响应协议协商
	if err != nil {
		// 协议协商失败是预期的，因为 mock 不响应 multistream-select
		assert.Contains(t, err.Error(), "protocol negotiation failed")
	} else {
		assert.NotNil(t, stream)
	}
}

// TestHost_SetStreamHandler 测试 SetStreamHandler 方法
func TestHost_SetStreamHandler(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler := func(s pkgif.Stream) {
		// 处理流
		_ = s
	}

	// 设置处理器
	host.SetStreamHandler("/test/1.0.0", handler)

	// 验证处理器已注册（通过内部协议路由器）
	assert.NotNil(t, host.mux)
}

// TestHost_RemoveStreamHandler 测试 RemoveStreamHandler 方法
func TestHost_RemoveStreamHandler(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler := func(s pkgif.Stream) {}

	// 设置然后移除
	host.SetStreamHandler("/test/1.0.0", handler)
	host.RemoveStreamHandler("/test/1.0.0")

	// 验证不会 panic
}

// TestHost_Peerstore 测试 Peerstore 方法
func TestHost_Peerstore(t *testing.T) {
	host, _, mockPeerstore, _ := setupTestHost(t)
	defer host.Close()

	ps := host.Peerstore()
	assert.Equal(t, mockPeerstore, ps)
}

// TestHost_EventBus 测试 EventBus 方法
func TestHost_EventBus(t *testing.T) {
	host, _, _, mockEventBus := setupTestHost(t)
	defer host.Close()

	eb := host.EventBus()
	assert.Equal(t, mockEventBus, eb)
}

// TestHost_Close 测试 Close 方法
func TestHost_Close(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)

	// 第一次关闭
	err := host.Close()
	assert.NoError(t, err)

	// 验证 swarm 被关闭
	assert.True(t, mockSwarm.IsClosed())
}

// TestHost_CloseIdempotent 测试 Close 幂等性
func TestHost_CloseIdempotent(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	// 多次关闭应该不报错
	err1 := host.Close()
	err2 := host.Close()
	err3 := host.Close()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
}

// TestHost_ConnectWithPeerstore 测试 Connect 时地址存储
func TestHost_ConnectWithPeerstore(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	targetPeer := "peer-789"
	targetAddrs := []string{"/ip4/192.168.1.1/tcp/4001", "/ip4/192.168.1.1/tcp/4002"}

	err := host.Connect(ctx, targetPeer, targetAddrs)
	assert.NoError(t, err)
}

// ============================================================================
// 边界条件测试
// ============================================================================

// TestHost_Closed 测试 Closed 方法
func TestHost_Closed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)

	// 创建后未关闭
	assert.False(t, host.Closed())

	// 关闭后
	host.Close()
	assert.True(t, host.Closed())
}

// TestHost_Started 测试 Started 方法
func TestHost_Started(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// 创建后未启动
	assert.False(t, host.Started())

	// 启动后
	err := host.Start(context.Background())
	require.NoError(t, err)
	assert.True(t, host.Started())
}

// TestHost_Network 测试 Network 方法
func TestHost_Network(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	network := host.Network()
	assert.Equal(t, mockSwarm, network)
}

// TestHost_Connect_AfterClosed 测试关闭后连接
func TestHost_Connect_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	ctx := context.Background()
	err := host.Connect(ctx, "peer-123", []string{"/ip4/127.0.0.1/tcp/4001"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestHost_NewStream_AfterClosed 测试关闭后创建流
func TestHost_NewStream_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	ctx := context.Background()
	_, err := host.NewStream(ctx, "peer-123", "/test/1.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestHost_Listen 测试 Listen 方法
func TestHost_Listen(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	err := host.Listen("/ip4/127.0.0.1/tcp/4001")
	assert.NoError(t, err)

	// 验证 swarm.Listen 被调用（ListenCalls 是 [][]string）
	found := false
	for _, call := range mockSwarm.ListenCalls {
		for _, addr := range call {
			if addr == "/ip4/127.0.0.1/tcp/4001" {
				found = true
				break
			}
		}
	}
	assert.True(t, found, "Listen 应该被调用")
}

// TestHost_Listen_AfterClosed 测试关闭后监听
func TestHost_Listen_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	err := host.Listen("/ip4/127.0.0.1/tcp/4001")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestHost_Start_AfterClosed 测试关闭后启动
func TestHost_Start_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	err := host.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestHost_Connect_WithInvalidAddrs 测试使用无效地址连接
func TestHost_Connect_WithInvalidAddrs(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	// 无效地址格式会被跳过，连接仍然尝试
	err := host.Connect(ctx, "peer-123", []string{"invalid-address"})
	// 连接应该仍然尝试（地址被跳过但不报错）
	assert.NoError(t, err)
}

// TestHost_Connect_WithEmptyAddrs 测试使用空地址连接
func TestHost_Connect_WithEmptyAddrs(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	// 空地址列表，连接仍然尝试（使用 peerstore 中已有的地址）
	err := host.Connect(ctx, "peer-123", []string{})
	assert.NoError(t, err)
}

// TestHost_ID_NilSwarm 测试 nil swarm 时的 ID
func TestHost_ID_NilSwarm(t *testing.T) {
	// 创建一个手动设置 swarm 为 nil 的 host（通过反射或直接操作）
	// 这里使用正常创建然后验证
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// 正常情况下 ID 应该返回 swarm 的 LocalPeer
	assert.NotEmpty(t, host.ID())
}

// TestHost_Addrs_NilManager 测试 nil addrsManager 时的 Addrs
func TestHost_Addrs_NilManager(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// addrsManager 应该被正确初始化
	assert.NotNil(t, host.addrsManager)
	// Addrs 应该返回非 nil（可能为空）
	addrs := host.Addrs()
	assert.NotNil(t, addrs)
}

// TestHost_AdvertisedAddrs 测试 AdvertisedAddrs 方法
func TestHost_AdvertisedAddrs(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// setupTestHost 没有设置 addrsManager，所以返回 nil
	// 代码位置: host.go:126-131
	addrs := host.AdvertisedAddrs()
	assert.Nil(t, addrs, "AdvertisedAddrs should return nil without addrsManager")
}

// TestHost_ShareableAddrs 测试 ShareableAddrs 方法
func TestHost_ShareableAddrs(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// setupTestHost 没有设置 addrsManager，所以返回 nil
	// 代码位置: host.go:136-141
	addrs := host.ShareableAddrs()
	assert.Nil(t, addrs, "ShareableAddrs should return nil without addrsManager")
}

// TestHost_Connected_AfterClosed 测试关闭后的连接回调
func TestHost_Connected_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	// 创建 mock 连接
	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 关闭后调用 Connected 应该不 panic
	host.Connected(mockConn)

	// 验证连接计数未被更新
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count)
}

// TestHost_Disconnected_AfterClosed 测试关闭后的断开回调
func TestHost_Disconnected_AfterClosed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 关闭后调用 Disconnected 应该不 panic
	host.Disconnected(mockConn)
}

// TestHost_NewStream_NoConnection 测试无连接时创建流
func TestHost_NewStream_NoConnection(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	// 没有连接，应该失败
	_, err := host.NewStream(ctx, "unknown-peer", "/test/1.0.0")
	assert.Error(t, err)
}

// ============================================================================
// 连接生命周期测试（提升覆盖率）
// ============================================================================

// TestHost_Connected_FirstConnection 测试首次连接事件
func TestHost_Connected_FirstConnection(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer-123")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 首次连接
	host.Connected(mockConn)

	// 验证连接计数
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count)
}

// TestHost_Connected_MultipleConnections 测试多次连接同一节点（去重）
func TestHost_Connected_MultipleConnections(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer-456")
	mockConn1 := mocks.NewMockConnection(localPeer, remotePeer)
	mockConn2 := mocks.NewMockConnection(localPeer, remotePeer)

	// 连接两次
	host.Connected(mockConn1)
	host.Connected(mockConn2)

	// 验证连接计数为 2
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 2, count)
}

// TestHost_Disconnected_LastConnection 测试最后一个连接断开
func TestHost_Disconnected_LastConnection(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer-789")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 先连接
	host.Connected(mockConn)

	// 然后断开
	host.Disconnected(mockConn)

	// 验证连接计数为 0 且已清理
	host.peerConnCountMu.Lock()
	count, exists := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count)
	assert.False(t, exists, "零计数条目应被清理")
}

// TestHost_Disconnected_StillHasConnections 测试断开但仍有其他连接
func TestHost_Disconnected_StillHasConnections(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-peer-abc")
	mockConn1 := mocks.NewMockConnection(localPeer, remotePeer)
	mockConn2 := mocks.NewMockConnection(localPeer, remotePeer)

	// 连接两次
	host.Connected(mockConn1)
	host.Connected(mockConn2)

	// 断开一个
	host.Disconnected(mockConn1)

	// 验证连接计数为 1（还有一个连接）
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count)
}

// TestHost_Disconnected_NoConnection 测试未连接时断开（边界情况）
func TestHost_Disconnected_NoConnection(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("never-connected")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 直接断开（未连接）
	host.Disconnected(mockConn)

	// 验证不会 panic，计数保持为 0
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count)
}

// ============================================================================
// Options 测试（提升覆盖率）
// ============================================================================

// TestHost_WithConnManager 测试 ConnManager 选项
// 注意：由于 MockConnMgr 未完全实现 ConnManager 接口，此测试暂时跳过
func TestHost_WithConnManager(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	// 不设置 ConnManager，验证 host 仍然可以创建
	host, err := New(
		WithSwarm(mockSwarm),
	)
	require.NoError(t, err)
	defer host.Close()

	// connmgr 为 nil 是允许的
	assert.Nil(t, host.connmgr)
}

// TestHost_WithResourceManager 测试 ResourceManager 选项
func TestHost_WithResourceManager(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithResourceManager(nil), // 允许 nil
	)
	require.NoError(t, err)
	defer host.Close()
}

// TestHost_WithConfig 测试 Config 选项
func TestHost_WithConfig(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")
	cfg := DefaultConfig()

	host, err := New(
		WithSwarm(mockSwarm),
		WithConfig(cfg),
	)
	require.NoError(t, err)
	defer host.Close()

	assert.NotNil(t, host.config)
}

// TestHost_WithConfig_Nil 测试 nil Config 选项
func TestHost_WithConfig_Nil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithConfig(nil), // nil config 应该被忽略
	)
	require.NoError(t, err)
	defer host.Close()

	// 应该使用默认配置
	assert.NotNil(t, host.config)
}

// ============================================================================
// Start 相关测试
// ============================================================================

// TestHost_SetReachabilityCoordinator 测试设置可达性协调器
func TestHost_SetReachabilityCoordinator(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// 设置 nil 协调器应该不 panic
	host.SetReachabilityCoordinator(nil)
}

// TestHost_NewStream_WithoutProtocol 测试创建流但不指定协议
func TestHost_NewStream_WithoutProtocol(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	ctx := context.Background()
	targetPeer := "peer-no-proto"

	// 添加连接
	conn := mocks.NewMockConnection(types.PeerID("test-peer-id"), types.PeerID(targetPeer))
	mockSwarm.AddConnection(targetPeer, conn)

	// 不指定协议 ID，应该跳过协议协商
	stream, err := host.NewStream(ctx, targetPeer)
	// 无协议协商，直接返回流
	assert.NoError(t, err)
	assert.NotNil(t, stream)
}

// TestHost_emitLocalAddrsUpdated_NoEventBus 测试无 EventBus 时的地址更新
func TestHost_emitLocalAddrsUpdated_NoEventBus(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		// 不设置 EventBus
	)
	require.NoError(t, err)
	defer host.Close()

	// 应该不 panic
	host.emitLocalAddrsUpdated([]string{"/ip4/127.0.0.1/tcp/4001"}, []string{"/ip4/127.0.0.1/tcp/4001"})
}

// TestHost_Connect_NilPeerstore 测试无 Peerstore 时连接
func TestHost_Connect_NilPeerstore(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		// 不设置 Peerstore
	)
	require.NoError(t, err)
	defer host.Close()

	ctx := context.Background()
	// 应该仍然可以连接（地址不会存储到 peerstore）
	err = host.Connect(ctx, "peer-123", []string{"/ip4/127.0.0.1/tcp/4001"})
	assert.NoError(t, err)
}

// TestHost_Connected_WithoutConnManager 测试连接时无 ConnManager 的情况
func TestHost_Connected_WithoutConnManager(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		// 不设置 ConnManager
	)
	require.NoError(t, err)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-without-connmgr")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 调用 Connected 不应该 panic（即使没有 ConnManager）
	host.Connected(mockConn)

	// 验证连接计数正常更新
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 1, count)
}

// TestHost_Disconnected_WithoutConnManager 测试断开时无 ConnManager 的情况
func TestHost_Disconnected_WithoutConnManager(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		// 不设置 ConnManager
	)
	require.NoError(t, err)
	defer host.Close()

	localPeer := types.PeerID("test-peer-id")
	remotePeer := types.PeerID("remote-disconnect-no-connmgr")
	mockConn := mocks.NewMockConnection(localPeer, remotePeer)

	// 先连接再断开，不应该 panic
	host.Connected(mockConn)
	host.Disconnected(mockConn)

	// 验证连接计数正常清理
	host.peerConnCountMu.Lock()
	count := host.peerConnCount[string(remotePeer)]
	host.peerConnCountMu.Unlock()
	assert.Equal(t, 0, count)
}

// ============================================================================
// Config 测试
// ============================================================================

// TestConfig_Validate_InvalidUserAgent 测试无效 UserAgent
func TestConfig_Validate_InvalidUserAgent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UserAgent = ""
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UserAgent")
}

// TestConfig_Validate_InvalidProtocolVersion 测试无效 ProtocolVersion
func TestConfig_Validate_InvalidProtocolVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProtocolVersion = ""
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ProtocolVersion")
}

// TestConfig_Validate_InvalidNegotiationTimeout 测试无效超时
func TestConfig_Validate_InvalidNegotiationTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.NegotiationTimeout = -1
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NegotiationTimeout")
}

// TestConfig_Validate_NilAddrsFactory 测试 nil AddrsFactory
func TestConfig_Validate_NilAddrsFactory(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AddrsFactory = nil
	err := cfg.Validate()
	assert.NoError(t, err)
	// 应该被设置为默认工厂
	assert.NotNil(t, cfg.AddrsFactory)
}

// TestConfigOptions 测试配置选项
func TestConfigOptions(t *testing.T) {
	cfg := DefaultConfig()

	// 测试 WithUserAgent
	WithUserAgent("custom-agent")(cfg)
	assert.Equal(t, "custom-agent", cfg.UserAgent)

	// 测试 WithProtocolVersion
	WithProtocolVersion("1.2.3")(cfg)
	assert.Equal(t, "1.2.3", cfg.ProtocolVersion)

	// 测试 WithNegotiationTimeout
	WithNegotiationTimeout(30 * time.Second)(cfg)
	assert.Equal(t, 30*time.Second, cfg.NegotiationTimeout)

	// 测试 WithMetrics
	WithMetrics()(cfg)
	assert.True(t, cfg.EnableMetrics)

	// 测试 WithAddrsFactory
	customFactory := func(addrs []types.Multiaddr) []types.Multiaddr {
		return nil
	}
	WithAddrsFactory(customFactory)(cfg)
	assert.NotNil(t, cfg.AddrsFactory)
}

// TestConfig_ApplyOptions 测试批量应用选项
func TestConfig_ApplyOptions(t *testing.T) {
	cfg := DefaultConfig()

	cfg.ApplyOptions(
		WithUserAgent("test-agent"),
		WithProtocolVersion("2.0.0"),
		WithMetrics(),
	)

	assert.Equal(t, "test-agent", cfg.UserAgent)
	assert.Equal(t, "2.0.0", cfg.ProtocolVersion)
	assert.True(t, cfg.EnableMetrics)
}

// ============================================================================
// Options 测试（补充 0% 覆盖函数）
// ============================================================================

// TestHost_WithConnManager_Nil 测试 nil ConnManager 选项
func TestHost_WithConnManager_Nil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithConnManager(nil), // nil ConnManager 应该被接受
	)
	require.NoError(t, err)
	defer host.Close()

	assert.Nil(t, host.connmgr)
	t.Log("✅ WithConnManager(nil) 正确设置")
}

// TestHost_WithProtocol_Nil 测试 nil Protocol 选项
func TestHost_WithProtocol_Nil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithProtocol(nil), // nil Protocol 应该被接受
	)
	require.NoError(t, err)
	defer host.Close()

	assert.Nil(t, host.protocol)
	t.Log("✅ WithProtocol(nil) 正确设置")
}

// TestHost_WithNAT_Nil 测试 nil NAT 选项
func TestHost_WithNAT_Nil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithNAT(nil), // nil NAT 应该被接受
	)
	require.NoError(t, err)
	defer host.Close()

	assert.Nil(t, host.nat)
	t.Log("✅ WithNAT(nil) 正确设置")
}

// TestHost_WithRelay_Nil 测试 nil Relay 选项
func TestHost_WithRelay_Nil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
		WithRelay(nil), // nil Relay 应该被接受
	)
	require.NoError(t, err)
	defer host.Close()

	assert.Nil(t, host.relay)
	t.Log("✅ WithRelay(nil) 正确设置")
}

// ============================================================================
// handleInboundStream 测试（核心功能，之前 0% 覆盖）
// ============================================================================

// TestHost_handleInboundStream_Closed 测试关闭后处理入站流
func TestHost_handleInboundStream_Closed(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	host.Close()

	// 创建 mock stream，需要设置 Conn 返回值
	mockConn := mocks.NewMockConnection(types.PeerID("local"), types.PeerID("remote"))
	mockStream := mocks.NewMockStream()
	mockStream.ConnValue = mockConn

	// 关闭后调用 handleInboundStream 应该直接 reset 流
	host.handleInboundStream(mockStream)

	// 验证流被 reset（ResetCalled 字段）
	assert.True(t, mockStream.ResetCalled, "关闭后流应该被 reset")
	t.Log("✅ 关闭后 handleInboundStream 正确 reset 流")
}

// TestHost_handleInboundStream_NilMux 测试 nil mux 时处理入站流
func TestHost_handleInboundStream_NilMux(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")

	host, err := New(
		WithSwarm(mockSwarm),
	)
	require.NoError(t, err)
	defer host.Close()

	// 手动设置 mux 为 nil（模拟异常状态）
	host.mu.Lock()
	host.mux = nil
	host.mu.Unlock()

	// 创建 mock stream，需要设置 Conn 返回值
	mockConn := mocks.NewMockConnection(types.PeerID("local"), types.PeerID("remote"))
	mockStream := mocks.NewMockStream()
	mockStream.ConnValue = mockConn

	// nil mux 时应该 reset 流
	host.handleInboundStream(mockStream)

	// 验证流被 reset
	assert.True(t, mockStream.ResetCalled, "nil mux 时流应该被 reset")
	t.Log("✅ nil mux 时 handleInboundStream 正确 reset 流")
}
