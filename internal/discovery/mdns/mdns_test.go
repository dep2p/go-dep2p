package mdns

import (
	"context"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMDNS_Creation 测试 MDNS 创建
func TestMDNS_Creation(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)
	assert.NotNil(t, mdns)
	assert.Equal(t, mockHost, mdns.host)
	assert.Equal(t, config, mdns.config)
}

// TestMDNS_Start 测试启动服务
func TestMDNS_Start(t *testing.T) {
	t.Skip("需要真实网络环境")
}

// TestMDNS_Stop 测试停止服务
func TestMDNS_Stop(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()
	err = mdns.Stop(ctx)
	assert.NoError(t, err)
	assert.True(t, mdns.Closed())
}

// TestMDNS_Advertise 测试广播服务
func TestMDNS_Advertise(t *testing.T) {
	t.Skip("需要真实网络环境")
}

// TestMDNS_FindPeers 测试发现节点
func TestMDNS_FindPeers(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	peerCh, err := mdns.FindPeers(ctx, "test")
	require.NoError(t, err)
	assert.NotNil(t, peerCh)

	// 等待 channel 关闭
	count := 0
	for range peerCh {
		count++
	}
	// 注意：在某些测试环境中可能会发现其他节点（如其他运行中的 mDNS 服务）
	// 这个测试主要验证 FindPeers 能正常工作，不对发现的数量做严格断言
	t.Logf("发现了 %d 个节点（环境相关）", count)
}

// TestMDNS_FindPeers_Context 测试上下文取消
func TestMDNS_FindPeers_Context(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	peerCh, err := mdns.FindPeers(ctx, "test")
	require.NoError(t, err)

	// channel 应该很快关闭
	select {
	case <-peerCh:
	case <-time.After(1 * time.Second):
		t.Fatal("channel 没有关闭")
	}
}

// TestMDNS_Config_Validation 测试配置验证
func TestMDNS_Config_Validation(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}

	// 测试无效 ServiceTag
	config := &Config{
		ServiceTag: "",
		Interval:   10 * time.Second,
		Enabled:    true,
	}

	_, err := New(mockHost, config)
	assert.Error(t, err)

	// 测试无效 Interval
	config2 := &Config{
		ServiceTag: "_dep2p._udp",
		Interval:   -1 * time.Second,
		Enabled:    true,
	}

	_, err = New(mockHost, config2)
	assert.Error(t, err)
}

// TestMDNS_NilHost 测试 nil Host
func TestMDNS_NilHost(t *testing.T) {
	config := DefaultConfig()

	_, err := New(nil, config)
	assert.Error(t, err)
}

// TestMDNS_ServiceName 测试自定义服务名
func TestMDNS_ServiceName(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}

	config := &Config{
		ServiceTag: "_custom._udp",
		Interval:   10 * time.Second,
		Enabled:    true,
	}

	mdns, err := New(mockHost, config)
	require.NoError(t, err)
	assert.Equal(t, "_custom._udp", mdns.config.ServiceTag)
}

// TestMDNS_AddressFilter 测试地址过滤
func TestMDNS_AddressFilter(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		suitable bool
	}{
		{"IP4 TCP", "/ip4/192.168.1.1/tcp/4001", true},
		{"IP6 TCP", "/ip6/::1/tcp/4001", true},
		{"Circuit", "/p2p-circuit/p2p/12D3K", false},
		{"WebSocket", "/ip4/127.0.0.1/tcp/8080/ws", false},
		{"WebRTC", "/ip4/127.0.0.1/udp/9090/webrtc", false},
		{"DNS local", "/dns4/test.local/tcp/4001", true},
		{"DNS non-local", "/dns4/example.com/tcp/4001", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := types.NewMultiaddr(tt.addr)
			if err != nil {
				t.Skipf("无法解析地址: %v", err)
			}
			result := isSuitableForMDNS(addr)
			assert.Equal(t, tt.suitable, result, "地址 %s 过滤结果不符合预期", tt.addr)
		})
	}
}

// TestMDNS_Closed 测试关闭后操作
func TestMDNS_Closed(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 关闭
	err = mdns.Stop(ctx)
	require.NoError(t, err)

	// 关闭后操作应该返回错误
	_, err = mdns.FindPeers(ctx, "test")
	assert.Error(t, err)

	_, err = mdns.Advertise(ctx, "test")
	assert.Error(t, err)
}

// TestMDNS_Lifecycle 测试生命周期
func TestMDNS_Lifecycle(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 启动（可能成功或失败）
	err = mdns.Start(ctx)
	// 在 mock 环境下可能成功

	// 停止
	err = mdns.Stop(ctx)
	assert.NoError(t, err)

	// 重复停止应是幂等的
	err = mdns.Stop(ctx)
	assert.NoError(t, err)

	// 验证状态
	assert.True(t, mdns.Closed())
}

// TestMDNS_Concurrent 测试并发安全
func TestMDNS_Concurrent(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	// 并发调用 Started() 和 Closed()
	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func() {
			_ = mdns.Started()
			done <- true
		}()
		go func() {
			_ = mdns.Closed()
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 20; i++ {
		<-done
	}
}

// TestConfig_Options 测试配置选项
func TestConfig_Options(t *testing.T) {
	config := &Config{
		ServiceTag: "_test._udp",
		Interval:   5 * time.Second,
		Enabled:    false,
	}

	config.ApplyOptions(
		WithServiceTag("_custom._udp"),
		WithInterval(15*time.Second),
		WithEnabled(true),
	)

	assert.Equal(t, "_custom._udp", config.ServiceTag)
	assert.Equal(t, 15*time.Second, config.Interval)
	assert.True(t, config.Enabled)
}

// TestConfig_DefaultConfig 测试默认配置
func TestConfig_DefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, DefaultServiceTag, config.ServiceTag)
	assert.Equal(t, DefaultInterval, config.Interval)
	assert.True(t, config.Enabled)
}

// mockHost 是 Host 接口的 mock 实现
type mockHost struct {
	id    string
	addrs []string
}

func (m *mockHost) ID() string {
	return m.id
}

func (m *mockHost) Addrs() []string {
	return m.addrs
}

func (m *mockHost) Listen(addrs ...string) error {
	return nil
}

func (m *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}

func (m *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, nil
}

func (m *mockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
}

func (m *mockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
}

func (m *mockHost) RemoveStreamHandler(protocolID string) {
}

func (m *mockHost) Peerstore() pkgif.Peerstore {
	return nil
}

func (m *mockHost) EventBus() pkgif.EventBus {
	return nil
}

func (m *mockHost) Close() error {
	return nil
}

func (m *mockHost) AdvertisedAddrs() []string {
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

// TestMDNS_StartTwice 测试重复启动
func TestMDNS_StartTwice(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 第一次启动
	err = mdns.Start(ctx)
	// 可能成功或失败

	// 第二次启动应该返回 ErrAlreadyStarted
	err2 := mdns.Start(ctx)
	if err == nil {
		// 如果第一次启动成功，第二次应该失败
		assert.ErrorIs(t, err2, ErrAlreadyStarted)
	}

	mdns.Stop(ctx)
}

// TestMDNS_AdvertiseNotStarted 测试未启动时 Advertise
func TestMDNS_AdvertiseNotStarted(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 未启动就调用 Advertise
	_, err = mdns.Advertise(ctx, "test")
	// Advertise 会启动 server，可能成功或失败

	mdns.Stop(ctx)
}

// TestMDNS_MultipleAddrs 测试多个地址
func TestMDNS_MultipleAddrs(t *testing.T) {
	mockHost := &mockHost{
		id: "testhost",
		addrs: []string{
			"/ip4/127.0.0.1/tcp/4001",
			"/ip4/192.168.1.100/tcp/4001",
			"/ip6/::1/tcp/4001",
		},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)
	assert.NotNil(t, mdns)

	// 测试获取 IPs
	multiaddrs := []types.Multiaddr{}
	for _, addrStr := range mockHost.addrs {
		addr, _ := types.NewMultiaddr(addrStr)
		multiaddrs = append(multiaddrs, addr)
	}

	ips, err := mdns.getIPs(multiaddrs)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(ips), 1)
}

// TestMDNS_NoAddresses 测试无地址时的优雅降级
func TestMDNS_NoAddresses(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 优雅降级：无地址时 Start 应该成功，进入 Waiting 状态
	err = mdns.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, mdns.IsWaiting(), "应进入 Waiting 状态")
	assert.False(t, mdns.IsRunning(), "不应处于 Running 状态")

	mdns.Stop(ctx)
}

// TestMDNS_GracefulDegradation 测试优雅降级
func TestMDNS_GracefulDegradation(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{}, // 无地址
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 启动时无地址，应该进入 Waiting 状态
	err = mdns.Start(ctx)
	assert.NoError(t, err)
	assert.Equal(t, StateWaiting, mdns.State())

	// TryStartServer 在无地址时也应该成功（保持 Waiting）
	err = mdns.TryStartServer()
	assert.NoError(t, err)
	assert.Equal(t, StateWaiting, mdns.State())

	mdns.Stop(ctx)
	assert.Equal(t, StateStopped, mdns.State())
}

// TestMDNS_StateTransitions 测试状态转换
func TestMDNS_StateTransitions(t *testing.T) {
	t.Run("Waiting to Running", func(t *testing.T) {
		mockHost := &mockHost{
			id:    "testhost",
			addrs: []string{}, // 初始无地址
		}
		config := DefaultConfig()

		mdns, err := New(mockHost, config)
		require.NoError(t, err)

		ctx := context.Background()

		// 启动时进入 Waiting
		err = mdns.Start(ctx)
		assert.NoError(t, err)
		assert.True(t, mdns.IsWaiting())

		// 模拟地址变为可用
		mockHost.addrs = []string{"/ip4/127.0.0.1/tcp/4001"}

		// TryStartServer 尝试启动
		err = mdns.TryStartServer()
		// 可能成功或失败（取决于网络环境），但不应该 panic

		mdns.Stop(ctx)
	})

	t.Run("Running to Waiting", func(t *testing.T) {
		mockHost := &mockHost{
			id:    "testhost",
			addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
		}
		config := DefaultConfig()

		mdns, err := New(mockHost, config)
		require.NoError(t, err)

		ctx := context.Background()

		// 启动（可能成功进入 Running）
		err = mdns.Start(ctx)
		// 跳过 Running 状态检查，因为可能需要真实网络环境

		// 模拟地址丢失
		mdns.stopServer()
		assert.True(t, mdns.IsWaiting() || mdns.State() == StateStopped)

		mdns.Stop(ctx)
	})
}

// TestMDNS_IsWaiting 测试 IsWaiting 方法
func TestMDNS_IsWaiting(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 启动时无地址应进入 Waiting
	mdns.Start(ctx)
	assert.True(t, mdns.IsWaiting())
	assert.False(t, mdns.IsRunning())

	mdns.Stop(ctx)
}

// TestMDNS_TryStartServer 测试 TryStartServer
func TestMDNS_TryStartServer(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 设置为 Waiting 状态
	mdns.Start(ctx)
	assert.True(t, mdns.IsWaiting())

	// TryStartServer 在无地址时应该返回 nil
	err = mdns.TryStartServer()
	assert.NoError(t, err)

	// 设置地址后再试
	mockHost.addrs = []string{"/ip4/127.0.0.1/tcp/4001"}
	err = mdns.TryStartServer()
	// 可能成功或失败取决于网络环境

	mdns.Stop(ctx)
}

// TestMDNS_States 测试状态查询
func TestMDNS_States(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	// 初始状态
	assert.False(t, mdns.Started())
	assert.False(t, mdns.Closed())

	ctx := context.Background()

	// 启动后
	mdns.Start(ctx)
	// Started 可能为 true（如果启动成功）

	// 停止后
	mdns.Stop(ctx)
	assert.True(t, mdns.Closed())
}

// TestMDNS_ConfigNil 测试 nil 配置
func TestMDNS_ConfigNil(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}

	// nil config 应该使用默认配置
	mdns, err := New(mockHost, nil)
	require.NoError(t, err)
	assert.NotNil(t, mdns.config)
	assert.Equal(t, DefaultServiceTag, mdns.config.ServiceTag)
}

// TestMDNS_InvalidAddrs 测试无效地址过滤（优雅降级）
func TestMDNS_InvalidAddrs(t *testing.T) {
	mockHost := &mockHost{
		id: "testhost",
		addrs: []string{
			"/p2p-circuit/p2p/12D3K",
			"/ip4/127.0.0.1/tcp/8080/ws",
		},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 所有地址都不适合 mDNS，Start 应该成功但进入 Waiting 状态
	err = mdns.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, mdns.IsWaiting(), "无有效地址应进入 Waiting 状态")

	mdns.Stop(ctx)
}

// TestMDNS_GetIPs 测试 IP 提取
func TestMDNS_GetIPs(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	ips, err := mdns.getIPs([]types.Multiaddr{addr})
	require.NoError(t, err)
	assert.Contains(t, ips, "192.168.1.1")
}

// TestMDNS_GetIPs_NoIP 测试无 IP 地址
func TestMDNS_GetIPs_NoIP(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	// 空地址列表
	_, err = mdns.getIPs([]types.Multiaddr{})
	assert.Error(t, err)
}

// TestMDNS_GetIPs_IPv6 测试 IPv6 提取
func TestMDNS_GetIPs_IPv6(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip6/::1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	addr, _ := types.NewMultiaddr("/ip6/fe80::1/tcp/4001")
	ips, err := mdns.getIPs([]types.Multiaddr{addr})
	require.NoError(t, err)
	assert.Contains(t, ips, "fe80::1")
}

// TestMDNS_GetIPs_BothIP4AndIP6 测试同时提取 IPv4 和 IPv6
func TestMDNS_GetIPs_BothIP4AndIP6(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	addr1, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip6/fe80::1/tcp/4001")
	ips, err := mdns.getIPs([]types.Multiaddr{addr1, addr2})
	require.NoError(t, err)
	assert.Len(t, ips, 2)
	assert.Contains(t, ips, "192.168.1.1")
	assert.Contains(t, ips, "fe80::1")
}

// TestIsSuitableForMDNS_Nil 测试 nil 地址
func TestIsSuitableForMDNS_Nil(t *testing.T) {
	assert.False(t, isSuitableForMDNS(nil))
}

// TestContainsUnsuitableProtocol_Nil 测试 nil 地址
func TestContainsUnsuitableProtocol_Nil(t *testing.T) {
	assert.False(t, containsUnsuitableProtocol(nil))
}

// TestContainsUnsuitableProtocol_TCP 测试纯 TCP 地址
func TestContainsUnsuitableProtocol_TCP(t *testing.T) {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	assert.False(t, containsUnsuitableProtocol(addr))
}

// TestMDNS_StopWithoutStart 测试未启动就停止
func TestMDNS_StopWithoutStart(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 未启动就停止
	err = mdns.Stop(ctx)
	assert.NoError(t, err)
	assert.True(t, mdns.Closed())
}

// TestMDNS_StopAfterClosed 测试已关闭后再关闭
func TestMDNS_StopAfterClosed(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx := context.Background()

	// 第一次关闭
	err = mdns.Stop(ctx)
	assert.NoError(t, err)

	// 第二次关闭（幂等）
	err = mdns.Stop(ctx)
	assert.NoError(t, err)

	// 第三次关闭（幂等）
	err = mdns.Stop(ctx)
	assert.NoError(t, err)
}

// TestMDNS_FindPeersMultiple 测试多次调用 FindPeers
func TestMDNS_FindPeers_Multiple(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	mdns, err := New(mockHost, config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// 第一次调用
	peerCh1, err := mdns.FindPeers(ctx, "test1")
	require.NoError(t, err)

	// 第二次调用
	peerCh2, err := mdns.FindPeers(ctx, "test2")
	require.NoError(t, err)

	// 两个 channel 应该都有效
	assert.NotNil(t, peerCh1)
	assert.NotNil(t, peerCh2)

	// 等待完成
	for range peerCh1 {
	}
	for range peerCh2 {
	}

	mdns.Stop(context.Background())
}

// TestMDNS_RandomPeerName 测试随机 PeerName 生成
func TestMDNS_RandomPeerName(t *testing.T) {
	mockHost := &mockHost{
		id:    "testhost",
		addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	config := DefaultConfig()

	// 创建多个实例，验证 PeerName 是随机的
	names := make(map[string]bool)
	for i := 0; i < 10; i++ {
		mdns, err := New(mockHost, config)
		require.NoError(t, err)
		names[mdns.peerName] = true
	}

	// 应该有多个不同的名称
	assert.Greater(t, len(names), 1)
}

// ============================================================================
//                     truncateID 测试
// ============================================================================

// TestTruncateID 测试 ID 截断函数
func TestTruncateID(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		maxLen int
		want   string
	}{
		{
			name:   "正常截断",
			id:     "12345678901234567890",
			maxLen: 8,
			want:   "12345678",
		},
		{
			name:   "长度刚好",
			id:     "12345678",
			maxLen: 8,
			want:   "12345678",
		},
		{
			name:   "长度不足",
			id:     "1234",
			maxLen: 8,
			want:   "1234",
		},
		{
			name:   "空字符串",
			id:     "",
			maxLen: 8,
			want:   "",
		},
		{
			name:   "单字符",
			id:     "a",
			maxLen: 8,
			want:   "a",
		},
		{
			name:   "maxLen 为 0",
			id:     "12345678",
			maxLen: 0,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateID(tt.id, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ============================================================================
//                     MDNSError 测试
// ============================================================================

// TestMDNSError_Error_WithInnerError 测试有内部错误时的消息
func TestMDNSError_Error_WithInnerError(t *testing.T) {
	innerErr := ErrNoValidAddresses
	err := NewMDNSError("Start", innerErr, "failed to start server")

	msg := err.Error()
	assert.Contains(t, msg, "mdns")
	assert.Contains(t, msg, "Start")
	assert.Contains(t, msg, "failed to start server")
	assert.Contains(t, msg, "no valid addresses")
}

// TestMDNSError_Error_WithoutInnerError 测试无内部错误时的消息
func TestMDNSError_Error_WithoutInnerError(t *testing.T) {
	err := NewMDNSError("Stop", nil, "service already stopped")

	msg := err.Error()
	assert.Contains(t, msg, "mdns")
	assert.Contains(t, msg, "Stop")
	assert.Contains(t, msg, "service already stopped")
}

// TestMDNSError_Unwrap 测试 Unwrap
func TestMDNSError_Unwrap(t *testing.T) {
	t.Run("有内部错误", func(t *testing.T) {
		innerErr := ErrServerStart
		err := NewMDNSError("Start", innerErr, "message")

		unwrapped := err.Unwrap()
		assert.Equal(t, innerErr, unwrapped)
	})

	t.Run("无内部错误", func(t *testing.T) {
		err := NewMDNSError("Start", nil, "message")

		unwrapped := err.Unwrap()
		assert.Nil(t, unwrapped)
	})
}

// TestMDNSError_ErrorsIs 测试 errors.Is 兼容性
func TestMDNSError_ErrorsIs(t *testing.T) {
	innerErr := ErrNoValidAddresses
	err := NewMDNSError("Start", innerErr, "message")

	// 应该能通过 errors.Is 匹配内部错误
	assert.ErrorIs(t, err, ErrNoValidAddresses)
}

// TestNewMDNSError 测试创建函数
func TestNewMDNSError(t *testing.T) {
	err := NewMDNSError("operation", ErrNilHost, "detail message")

	require.NotNil(t, err)
	assert.Equal(t, "operation", err.Op)
	assert.Equal(t, ErrNilHost, err.Err)
	assert.Equal(t, "detail message", err.Message)
}
