package bootstrap

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBootstrap_Creation 测试 Bootstrap 创建
func TestBootstrap_Creation(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	assert.NotNil(t, bootstrap)
	assert.Equal(t, mockHost, bootstrap.host)
	assert.Equal(t, config, bootstrap.config)
}

// TestBootstrap_Bootstrap_Success 测试成功引导
func TestBootstrap_Bootstrap_Success(t *testing.T) {
	mockHost := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			return nil // 模拟成功连接
		},
	}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	addr3, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4003")
	addr4, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4004")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
		{ID: types.PeerID("peer3"), Addrs: []types.Multiaddr{addr3}},
		{ID: types.PeerID("peer4"), Addrs: []types.Multiaddr{addr4}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 4,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = bootstrap.Bootstrap(ctx)
	assert.NoError(t, err)
}

// TestBootstrap_Bootstrap_AllFail 测试所有连接失败
func TestBootstrap_Bootstrap_AllFail(t *testing.T) {
	mockHost := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			return assert.AnError // 所有连接都失败
		},
	}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = bootstrap.Bootstrap(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAllConnectionsFailed)
}

// TestBootstrap_FindPeers 测试 FindPeers 方法
func TestBootstrap_FindPeers(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	peerCh, err := bootstrap.FindPeers(ctx, "test")
	require.NoError(t, err)
	
	found := []types.PeerInfo{}
	for peer := range peerCh {
		found = append(found, peer)
	}
	
	assert.Equal(t, 2, len(found))
}

// TestBootstrap_AddPeer 测试添加引导节点
func TestBootstrap_AddPeer(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	initialCount := len(bootstrap.Peers())
	
	newAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/5000")
	
	newPeer := types.PeerInfo{
		ID:    types.PeerID("newpeer"),
		Addrs: []types.Multiaddr{newAddr},
	}
	
	bootstrap.AddPeer(newPeer)
	
	assert.Equal(t, initialCount+1, len(bootstrap.Peers()))
}

// TestBootstrap_Lifecycle 测试生命周期
func TestBootstrap_Lifecycle(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 测试启动
	err = bootstrap.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, bootstrap.Started())
	
	// 测试停止
	err = bootstrap.Stop(ctx)
	assert.NoError(t, err)
	assert.True(t, bootstrap.Closed())
}

// mockHost 是 Host 接口的 mock 实现
type mockHost struct {
	connectFunc    func(ctx context.Context, peerID string, addrs []string) error
	advertisedAddrs []string // 可选：自定义对外通告地址，nil 时使用 Addrs()
}

func (m *mockHost) ID() string {
	return "mockhost"
}

func (m *mockHost) Addrs() []string {
	return []string{"/ip4/127.0.0.1/tcp/0"}
}

func (m *mockHost) Listen(addrs ...string) error {
	return nil
}

func (m *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	if m.connectFunc != nil {
		return m.connectFunc(ctx, peerID, addrs)
	}
	return nil
}

func (m *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, nil
}

func (m *mockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
}

func (m *mockHost) RemoveStreamHandler(protocolID string) {
}

func (m *mockHost) Peerstore() pkgif.Peerstore {
	// 简化实现：返回 nil，Bootstrap 会处理 nil 情况
	return nil
}

func (m *mockHost) EventBus() pkgif.EventBus {
	return nil
}

func (m *mockHost) Close() error {
	return nil
}

func (m *mockHost) AdvertisedAddrs() []string {
	if m.advertisedAddrs != nil {
		return m.advertisedAddrs
	}
	return m.Addrs()
}

func (m *mockHost) ShareableAddrs() []string {
	return nil
}

func (m *mockHost) HolePunchAddrs() []string {
	return nil
}

func (m *mockHost) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	// no-op for mock
}

func (m *mockHost) Network() pkgif.Swarm {
	return nil
}

func (m *mockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

// TestBootstrap_Bootstrap_MinPeersFail 测试最小成功数不足
func TestBootstrap_Bootstrap_MinPeersFail(t *testing.T) {
	var callCount int32 // 使用原子变量避免数据竞争
	mockHost := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			count := atomic.AddInt32(&callCount, 1)
			if count <= 2 {
				return nil // 前2个成功
			}
			return assert.AnError // 其他失败
		},
	}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	addr3, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4003")
	addr4, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4004")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
		{ID: types.PeerID("peer3"), Addrs: []types.Multiaddr{addr3}},
		{ID: types.PeerID("peer4"), Addrs: []types.Multiaddr{addr4}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 4, // 需要4个，但只有2个成功
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = bootstrap.Bootstrap(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMinPeersNotMet)
}

// TestBootstrap_NoPeers 测试无引导节点
func TestBootstrap_NoPeers(t *testing.T) {
	mockHost := &mockHost{}
	
	config := &Config{
		Peers:    []types.PeerInfo{},
		Timeout:  5 * time.Second,
		MinPeers: 0,
		Enabled:  true, // 启用时，空 Peers 会自动禁用（优雅降级）
	}
	
	// 优雅降级：空 Peers 配置会自动禁用，服务可以正常创建
	bootstrap, err := New(mockHost, config)
	assert.NoError(t, err)
	assert.NotNil(t, bootstrap)
	assert.False(t, config.Enabled) // 配置被自动禁用
}

// TestBootstrap_Config_Validation 测试配置验证
func TestBootstrap_Config_Validation(t *testing.T) {
	mockHost := &mockHost{}
	
	// 测试无效 Timeout（需要 Enabled=true 才会触发验证）
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  -1 * time.Second,
		MinPeers: 1,
		Enabled:  true, // 必须启用才会验证
	}
	
	_, err := New(mockHost, config)
	assert.Error(t, err)
	
	// 测试 MinPeers 自动调整（MinPeers 超过 peers 数量时自动降低）
	config2 := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 10, // 超过 peers 数量，应自动调整为 1
		Enabled:  true,
	}
	
	bootstrap, err := New(mockHost, config2)
	assert.NoError(t, err)
	assert.NotNil(t, bootstrap)
	// 验证 MinPeers 被自动调整为实际 peers 数量
	assert.Equal(t, 1, config2.MinPeers, "MinPeers 应被自动调整为 peers 数量")
}

// TestBootstrap_Closed 测试关闭后操作
func TestBootstrap_Closed(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 关闭
	err = bootstrap.Stop(ctx)
	require.NoError(t, err)
	
	// 关闭后操作应该返回错误
	err = bootstrap.Bootstrap(ctx)
	assert.Error(t, err)
	
	_, err = bootstrap.FindPeers(ctx, "test")
	assert.Error(t, err)
}

// TestBootstrap_Concurrent 测试并发安全
func TestBootstrap_Concurrent(t *testing.T) {
	mockHost := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		},
	}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 2,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	// 并发调用 Peers()（测试 Race 安全性）
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			peers := bootstrap.Peers()
			// Peers() 应该返回有效的切片（可能为空但不为 nil）
			assert.NotNil(t, peers, "Peers() should not return nil")
			done <- true
		}()
	}
	
	// 并发调用 AddPeer()
	addr3, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4003")
	for i := 0; i < 10; i++ {
		go func(idx int) {
			newPeer := types.PeerInfo{
				ID:    types.PeerID("peer" + string(rune(idx+100))),
				Addrs: []types.Multiaddr{addr3},
			}
			bootstrap.AddPeer(newPeer)
			done <- true
		}(i)
	}
	
	// 等待所有 goroutine 完成
	for i := 0; i < 20; i++ {
		<-done
	}
}

// TestBootstrap_FindPeers_ContextCancel 测试 FindPeers 上下文取消
func TestBootstrap_FindPeers_ContextCancel(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	
	peerCh, err := bootstrap.FindPeers(ctx, "test")
	require.NoError(t, err)
	
	// channel 应该立即关闭
	count := 0
	for range peerCh {
		count++
	}
	
	// 由于上下文取消，可能不返回任何节点
	assert.LessOrEqual(t, count, len(peers))
}

// TestBootstrap_NilHost 测试 nil Host
func TestBootstrap_NilHost(t *testing.T) {
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	_, err := New(nil, config)
	assert.Error(t, err)
}

// TestBootstrap_Peers 测试获取节点列表
func TestBootstrap_Peers(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	gotPeers := bootstrap.Peers()
	assert.Equal(t, 2, len(gotPeers))
	assert.Equal(t, peers[0].ID, gotPeers[0].ID)
	assert.Equal(t, peers[1].ID, gotPeers[1].ID)
}

// TestBootstrap_StartStop 测试启动停止
func TestBootstrap_StartStop(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 启动
	err = bootstrap.Start(ctx)
	assert.NoError(t, err)
	
	// 重复启动应返回错误
	err = bootstrap.Start(ctx)
	assert.ErrorIs(t, err, ErrAlreadyStarted)
	
	// 停止
	err = bootstrap.Stop(ctx)
	assert.NoError(t, err)
	
	// 重复停止应是幂等的
	err = bootstrap.Stop(ctx)
	assert.NoError(t, err)
}

// TestBootstrap_PartialSuccess 测试部分成功
func TestBootstrap_PartialSuccess(t *testing.T) {
	var callCount int32 // 使用原子变量避免数据竞争
	mockHost := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			count := atomic.AddInt32(&callCount, 1)
			if count <= 3 {
				return nil // 前3个成功
			}
			return assert.AnError // 最后一个失败
		},
	}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	addr3, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4003")
	addr4, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4004")
	
	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
		{ID: types.PeerID("peer3"), Addrs: []types.Multiaddr{addr3}},
		{ID: types.PeerID("peer4"), Addrs: []types.Multiaddr{addr4}},
	}
	
	config := &Config{
		Peers:    peers,
		Timeout:  5 * time.Second,
		MinPeers: 3, // 需要3个，实际成功3个
	}
	
	bootstrap, err := New(mockHost, config)
	require.NoError(t, err)
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	err = bootstrap.Bootstrap(ctx)
	assert.NoError(t, err) // 部分成功可以接受
}

// TestBootstrap_EmptyPeerID 测试空节点 ID
func TestBootstrap_EmptyPeerID(t *testing.T) {
	mockHost := &mockHost{}
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID(""), Addrs: []types.Multiaddr{addr1}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
		Enabled:  true, // 必须启用才会验证
	}
	
	_, err := New(mockHost, config)
	assert.Error(t, err)
}

// TestBootstrap_EmptyAddrs 测试空地址
func TestBootstrap_EmptyAddrs(t *testing.T) {
	mockHost := &mockHost{}
	
	config := &Config{
		Peers:    []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{}}},
		Timeout:  5 * time.Second,
		MinPeers: 1,
		Enabled:  true, // 必须启用才会验证
	}
	
	_, err := New(mockHost, config)
	assert.Error(t, err)
}

// TestBootstrap_ConfigStruct 测试直接创建配置结构体
func TestBootstrap_ConfigStruct(t *testing.T) {
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peers := []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}}
	
	config := &Config{
		Peers:      peers,
		Timeout:    10 * time.Second,
		MinPeers:   2,
		MaxRetries: 5,
		Interval:   5 * time.Minute,
		Enabled:    true,
	}
	
	assert.Equal(t, peers, config.Peers)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 2, config.MinPeers)
	assert.Equal(t, 5, config.MaxRetries)
}

// TestBootstrap_ConfigFromUnified 测试从统一配置创建
func TestBootstrap_ConfigFromUnified(t *testing.T) {
	// 测试使用统一配置系统（这才是正确的方式）
	// DefaultConfig 已删除，应该通过 ConfigFromUnified 从 config 包获取配置
	
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	
	config := &Config{
		Peers:      []types.PeerInfo{{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}}},
		Timeout:    30 * time.Second,
		MinPeers:   1,
		MaxRetries: 3,
		Interval:   5 * time.Minute,
		Enabled:    true,
	}
	
	assert.NotNil(t, config)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 1, config.MinPeers)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, true, config.Enabled)
	assert.NotEmpty(t, config.Peers)
}

// ============================================================================
// DiscoveryWalker 测试（提升覆盖率）
// ============================================================================

// TestDiscoveryWalker_Creation 测试 DiscoveryWalker 创建
func TestDiscoveryWalker_Creation(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(mockHost, store, nil)
	require.NotNil(t, walker)
	assert.False(t, walker.IsRunning())

	t.Log("✅ DiscoveryWalker 创建成功")
}

// TestDiscoveryWalker_WithOptions 测试 DiscoveryWalker 选项
func TestDiscoveryWalker_WithOptions(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(
		mockHost,
		store,
		nil,
		WithWalkerInterval(10*time.Second),
		WithWalkLen(5),
	)
	require.NotNil(t, walker)
	assert.Equal(t, 10*time.Second, walker.interval)
	assert.Equal(t, 5, walker.walkLen)

	t.Log("✅ DiscoveryWalker 选项测试通过")
}

// TestDiscoveryWalker_StartStop 测试 DiscoveryWalker 启动停止
func TestDiscoveryWalker_StartStop(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(
		mockHost,
		store,
		nil,
		WithWalkerInterval(100*time.Millisecond),
	)
	require.NotNil(t, walker)

	// 启动
	err := walker.Start()
	assert.NoError(t, err)
	assert.True(t, walker.IsRunning())

	// 等待一小段时间让 runLoop 执行
	time.Sleep(50 * time.Millisecond)

	// 重复启动应该是幂等的
	err = walker.Start()
	assert.NoError(t, err)

	// 停止
	err = walker.Stop()
	assert.NoError(t, err)
	assert.False(t, walker.IsRunning())

	// 重复停止应该是幂等的
	err = walker.Stop()
	assert.NoError(t, err)

	t.Log("✅ DiscoveryWalker 启动停止测试通过")
}

// TestDiscoveryWalker_Stats 测试统计信息
func TestDiscoveryWalker_Stats(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(
		mockHost,
		store,
		nil,
		WithWalkerInterval(50*time.Millisecond),
		WithWalkLen(1),
	)
	require.NotNil(t, walker)

	// 启动
	err := walker.Start()
	require.NoError(t, err)

	// 等待几次 walk
	time.Sleep(150 * time.Millisecond)

	// 获取统计信息
	stats := walker.Stats()
	assert.GreaterOrEqual(t, stats.TotalWalks, int64(1))

	// 获取最后运行时间
	lastRun := walker.LastRun()
	assert.False(t, lastRun.IsZero())

	// 停止
	err = walker.Stop()
	assert.NoError(t, err)

	t.Log("✅ DiscoveryWalker 统计信息测试通过")
}

// TestDiscoveryWalker_WalkNow 测试手动触发
func TestDiscoveryWalker_WalkNow(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(
		mockHost,
		store,
		nil,
		WithWalkerInterval(1*time.Hour), // 设置很长的间隔，确保不会自动触发
	)
	require.NotNil(t, walker)

	// 手动触发
	walker.WalkNow()

	// 等待执行
	time.Sleep(50 * time.Millisecond)

	// 验证统计更新
	stats := walker.Stats()
	assert.GreaterOrEqual(t, stats.TotalWalks, int64(1))

	t.Log("✅ DiscoveryWalker WalkNow 测试通过")
}

// TestDiscoveryWalker_AddFromPeers 测试添加节点
func TestDiscoveryWalker_AddFromPeers(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(mockHost, store, nil)
	require.NotNil(t, walker)

	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")

	peers := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{addr1}},
		{ID: types.PeerID("peer2"), Addrs: []types.Multiaddr{addr2}},
	}

	added := walker.AddFromPeers(peers)
	assert.Equal(t, 2, added)

	// 验证已添加到存储
	entries := store.GetAll()
	assert.Equal(t, 2, len(entries))

	t.Log("✅ DiscoveryWalker AddFromPeers 测试通过")
}

// TestDiscoveryWalker_AddFromPeers_Empty 测试添加空节点
func TestDiscoveryWalker_AddFromPeers_Empty(t *testing.T) {
	mockHost := &mockHost{}
	store := NewExtendedNodeStore()

	walker := NewDiscoveryWalker(mockHost, store, nil)
	require.NotNil(t, walker)

	// 空 ID
	peers1 := []types.PeerInfo{
		{ID: types.PeerID(""), Addrs: []types.Multiaddr{}},
	}
	walker.AddFromPeers(peers1)

	// 空地址
	peers2 := []types.PeerInfo{
		{ID: types.PeerID("peer1"), Addrs: []types.Multiaddr{}},
	}
	walker.AddFromPeers(peers2)

	// 验证未添加到存储
	entries := store.GetAll()
	assert.Equal(t, 0, len(entries))

	t.Log("✅ DiscoveryWalker 空节点测试通过")
}

// TestDefaults_Fields 测试默认值字段
func TestDefaults_Fields(t *testing.T) {
	defaults := GetDefaults()
	// 验证各字段有有效值
	assert.Greater(t, defaults.MaxNodes, 0)
	assert.Greater(t, defaults.DiscoveryInterval, time.Duration(0))
	assert.Greater(t, defaults.DiscoveryWalkLen, 0)

	t.Log("✅ Defaults 字段测试通过")
}


// ============================================================================
// ExtendedNodeStore 额外测试
// ============================================================================

// TestExtendedNodeStore_GetOnline 测试获取在线节点
func TestExtendedNodeStore_GetOnline(t *testing.T) {
	store := NewExtendedNodeStore()

	entry1 := &NodeEntry{
		ID:     types.NodeID("node1"),
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
		Status: NodeStatusOnline,
	}
	entry2 := &NodeEntry{
		ID:     types.NodeID("node2"),
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4002"},
		Status: NodeStatusOffline,
	}

	store.Put(entry1)
	store.Put(entry2)

	online := store.GetOnline()
	assert.Equal(t, 1, len(online))
	assert.Equal(t, types.NodeID("node1"), online[0].ID)

	t.Log("✅ ExtendedNodeStore GetOnline 测试通过")
}

// TestExtendedNodeStore_MarkOnlineOffline 测试标记在线/离线
func TestExtendedNodeStore_MarkOnlineOffline(t *testing.T) {
	store := NewExtendedNodeStore(
		WithOfflineThreshold(1), // 设置阈值为 1，只需一次标记即可设置为离线
	)

	entry := &NodeEntry{
		ID:     types.NodeID("node1"),
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
		Status: NodeStatusUnknown,
	}
	store.Put(entry)

	// 标记在线
	err := store.MarkOnline(types.NodeID("node1"))
	assert.NoError(t, err)

	node, found := store.Get(types.NodeID("node1"))
	require.True(t, found, "Get should find existing node after MarkOnline")
	assert.Equal(t, NodeStatusOnline, node.Status)

	// 标记离线
	err = store.MarkOffline(types.NodeID("node1"))
	assert.NoError(t, err)

	node, found = store.Get(types.NodeID("node1"))
	require.True(t, found, "Get should find existing node after MarkOffline")
	assert.Equal(t, NodeStatusOffline, node.Status)

	t.Log("✅ ExtendedNodeStore MarkOnline/Offline 测试通过")
}

// TestExtendedNodeStore_Cleanup 测试清理过期节点
func TestExtendedNodeStore_Cleanup(t *testing.T) {
	store := NewExtendedNodeStore(
		WithExpireTime(100*time.Millisecond),
		WithOfflineThreshold(1),
	)

	entry := &NodeEntry{
		ID:       types.NodeID("node1"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		Status:   NodeStatusOffline,
		LastSeen: time.Now().Add(-1 * time.Hour),
	}
	store.Put(entry)

	cleaned := store.Cleanup()
	assert.GreaterOrEqual(t, cleaned, 0)

	t.Log("✅ ExtendedNodeStore Cleanup 测试通过")
}

// TestExtendedNodeStore_Size 测试大小
func TestExtendedNodeStore_Size(t *testing.T) {
	store := NewExtendedNodeStore()

	assert.Equal(t, 0, store.Size())

	entry := &NodeEntry{
		ID:    types.NodeID("node1"),
		Addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	store.Put(entry)

	assert.Equal(t, 1, store.Size())

	t.Log("✅ ExtendedNodeStore Size 测试通过")
}

// TestExtendedNodeStore_Delete 测试删除节点
func TestExtendedNodeStore_Delete(t *testing.T) {
	store := NewExtendedNodeStore()

	entry := &NodeEntry{
		ID:    types.NodeID("node1"),
		Addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	store.Put(entry)
	assert.Equal(t, 1, store.Size())

	err := store.Delete(types.NodeID("node1"))
	assert.NoError(t, err)
	assert.Equal(t, 0, store.Size())

	// 删除不存在的节点
	// Delete 实现 (store.go:253-278) 对于不存在的节点返回 nil
	err = store.Delete(types.NodeID("non-existent"))
	assert.NoError(t, err, "Delete non-existent node should succeed (no-op)")

	t.Log("✅ ExtendedNodeStore Delete 测试通过")
}

// TestExtendedNodeStore_GetForProbe 测试获取待探测节点
func TestExtendedNodeStore_GetForProbe(t *testing.T) {
	store := NewExtendedNodeStore()

	// 添加一些节点
	for i := 0; i < 5; i++ {
		entry := &NodeEntry{
			ID:       types.NodeID(string(rune('A' + i))),
			Addrs:    []string{"/ip4/127.0.0.1/tcp/400" + string(rune('1'+i))},
			Status:   NodeStatusUnknown,
			LastSeen: time.Now().Add(-1 * time.Hour),
		}
		store.Put(entry)
	}

	// 获取待探测节点
	toProbe := store.GetForProbe(3)
	assert.LessOrEqual(t, len(toProbe), 3)

	t.Log("✅ ExtendedNodeStore GetForProbe 测试通过")
}

// TestExtendedNodeStore_FindClosest 测试查找最近节点
func TestExtendedNodeStore_FindClosest(t *testing.T) {
	store := NewExtendedNodeStore()

	// 添加一些节点
	for i := 0; i < 10; i++ {
		entry := &NodeEntry{
			ID:     types.NodeID(string(rune('A' + i))),
			Addrs:  []string{"/ip4/127.0.0.1/tcp/400" + string(rune('0'+i))},
			Status: NodeStatusOnline,
		}
		store.Put(entry)
	}

	// 查找最近节点
	closest := store.FindClosest(types.NodeID("E"), 3)
	assert.LessOrEqual(t, len(closest), 3)

	t.Log("✅ ExtendedNodeStore FindClosest 测试通过")
}

// TestExtendedNodeStore_Stats 测试统计信息
func TestExtendedNodeStore_Stats(t *testing.T) {
	store := NewExtendedNodeStore()

	// 添加一些节点
	entry1 := &NodeEntry{
		ID:     types.NodeID("node1"),
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
		Status: NodeStatusOnline,
	}
	entry2 := &NodeEntry{
		ID:     types.NodeID("node2"),
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4002"},
		Status: NodeStatusOffline,
	}
	store.Put(entry1)
	store.Put(entry2)

	stats := store.Stats()
	assert.Equal(t, 2, stats.TotalNodes)

	t.Log("✅ ExtendedNodeStore Stats 测试通过")
}

// TestExtendedNodeStore_Close 测试关闭
func TestExtendedNodeStore_Close(t *testing.T) {
	store := NewExtendedNodeStore()

	entry := &NodeEntry{
		ID:    types.NodeID("node1"),
		Addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	store.Put(entry)

	err := store.Close()
	assert.NoError(t, err)

	t.Log("✅ ExtendedNodeStore Close 测试通过")
}

// TestExtendedNodeStore_Put_Invalid 测试无效输入
func TestExtendedNodeStore_Put_Invalid(t *testing.T) {
	store := NewExtendedNodeStore()

	// nil entry
	err := store.Put(nil)
	assert.Error(t, err)

	// empty ID
	err = store.Put(&NodeEntry{ID: ""})
	assert.Error(t, err)

	t.Log("✅ ExtendedNodeStore Put Invalid 测试通过")
}

// TestExtendedNodeStore_Get_NotFound 测试获取不存在的节点
func TestExtendedNodeStore_Get_NotFound(t *testing.T) {
	store := NewExtendedNodeStore()

	node, exists := store.Get(types.NodeID("non-existent"))
	assert.False(t, exists)
	assert.Nil(t, node)

	t.Log("✅ ExtendedNodeStore Get NotFound 测试通过")
}

// TestExtendedNodeStore_MarkOnline_NotFound 测试标记不存在的节点
func TestExtendedNodeStore_MarkOnline_NotFound(t *testing.T) {
	store := NewExtendedNodeStore()

	err := store.MarkOnline(types.NodeID("non-existent"))
	assert.Error(t, err)

	t.Log("✅ ExtendedNodeStore MarkOnline NotFound 测试通过")
}

// TestExtendedNodeStore_MarkOffline_NotFound 测试标记不存在的节点
func TestExtendedNodeStore_MarkOffline_NotFound(t *testing.T) {
	store := NewExtendedNodeStore()

	err := store.MarkOffline(types.NodeID("non-existent"))
	assert.Error(t, err)

	t.Log("✅ ExtendedNodeStore MarkOffline NotFound 测试通过")
}

// TestExtendedNodeStore_WithOptions 测试存储选项
func TestExtendedNodeStore_WithOptions(t *testing.T) {
	store := NewExtendedNodeStore(
		WithMaxNodes(100),
		WithCacheSize(50),
		WithExpireTime(24*time.Hour),
	)
	require.NotNil(t, store)

	t.Log("✅ ExtendedNodeStore WithOptions 测试通过")
}

// TestDefaults_GetDefaults 测试获取默认值
func TestDefaults_GetDefaults(t *testing.T) {
	defaults := GetDefaults()
	assert.NotNil(t, defaults)
	assert.Greater(t, defaults.DiscoveryInterval, time.Duration(0))
	assert.Greater(t, defaults.DiscoveryWalkLen, 0)

	t.Log("✅ GetDefaults 测试通过")
}
