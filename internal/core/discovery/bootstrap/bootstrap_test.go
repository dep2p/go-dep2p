package bootstrap

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

type mockConnector struct {
	localID       types.NodeID
	connectErr    error
	peers         map[types.NodeID][]discoveryif.PeerInfo
	connectCalled int
	// 记录连接时传入的参数（用于验证身份第一性）
	lastConnectNodeID types.NodeID
	lastConnectAddrs  []string
	mu                sync.Mutex
}

func newMockConnector() *mockConnector {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))
	return &mockConnector{
		localID: localID,
		peers:   make(map[types.NodeID][]discoveryif.PeerInfo),
	}
}

// Connect 实现 Connector 接口
//
// Layer1 身份第一性：接收 expected NodeID 和地址列表
func (m *mockConnector) Connect(ctx context.Context, nodeID types.NodeID, addrs []string) error {
	m.mu.Lock()
	m.connectCalled++
	m.lastConnectNodeID = nodeID
	m.lastConnectAddrs = addrs
	m.mu.Unlock()
	return m.connectErr
}

func (m *mockConnector) GetPeers(ctx context.Context, nodeID types.NodeID) ([]discoveryif.PeerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if peers, ok := m.peers[nodeID]; ok {
		return peers, nil
	}
	return nil, nil
}

func (m *mockConnector) LocalID() types.NodeID {
	return m.localID
}

type mockAddress struct {
	addr string
}

func (m *mockAddress) Network() string { return "tcp" }
func (m *mockAddress) String() string  { return m.addr }
func (m *mockAddress) Bytes() []byte   { return []byte(m.addr) }
func (m *mockAddress) Equal(other endpoint.Address) bool {
	if o, ok := other.(*mockAddress); ok {
		return m.addr == o.addr
	}
	return false
}
func (m *mockAddress) IsPublic() bool {
	ip := net.ParseIP(m.addr)
	if ip == nil {
		return false
	}
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified()
}
func (m *mockAddress) IsPrivate() bool {
	ip := net.ParseIP(m.addr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}
func (m *mockAddress) IsLoopback() bool {
	ip := net.ParseIP(m.addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
func (m *mockAddress) Multiaddr() string {
	// 如果已经是 multiaddr 格式，直接返回
	if len(m.addr) > 0 && m.addr[0] == '/' {
		return m.addr
	}
	// 否则转换为 multiaddr
	ip := net.ParseIP(m.addr)
	if ip == nil {
		return "/dns4/" + m.addr + "/tcp/4001"
	}
	ipType := "ip4"
	if ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/tcp/4001", ipType, m.addr)
}

// ============================================================================
//                              Config 测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Greater(t, cfg.ConnectTimeout, time.Duration(0))
	assert.Greater(t, cfg.MaxConcurrent, 0)
	assert.Greater(t, cfg.RetryInterval, time.Duration(0))
	assert.Greater(t, cfg.MaxRetries, 0)
}

// ============================================================================
//                              Discoverer 测试
// ============================================================================

func TestNewDiscoverer(t *testing.T) {
	t.Run("使用默认配置", func(t *testing.T) {
		connector := newMockConnector()
		discoverer := NewDiscoverer(DefaultConfig(), connector)
		require.NotNil(t, discoverer)
		assert.NotNil(t, discoverer.connected)
		assert.NotNil(t, discoverer.discovered)
	})

	t.Run("使用自定义 Logger", func(t *testing.T) {
		connector := newMockConnector()
		discoverer := NewDiscoverer(DefaultConfig(), connector)
		require.NotNil(t, discoverer)
	})
}

// ============================================================================
//                              引导节点管理测试
// ============================================================================

func TestDiscoverer_AddBootstrapPeer(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID types.NodeID
	copy(peerID[:], []byte("new-peer-id-12345678"))

	peer := discoveryif.PeerInfo{
		ID:    peerID,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.2/udp/4001/quic-v1")},
	}

	discoverer.AddBootstrapPeer(peer)

	discoverer.peersMu.RLock()
	assert.Len(t, discoverer.peers, 1)
	discoverer.peersMu.RUnlock()
}

func TestDiscoverer_RemoveBootstrapPeer(t *testing.T) {
	var peerID types.NodeID
	copy(peerID[:], []byte("remove-peer-id-12345"))

	peers := []discoveryif.PeerInfo{
		{
			ID:    peerID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")},
		},
	}

	cfg := DefaultConfig()
	cfg.Peers = peers

	connector := newMockConnector()
	discoverer := NewDiscoverer(cfg, connector)

	// 移除节点
	discoverer.RemoveBootstrapPeer(peerID)

	discoverer.peersMu.RLock()
	assert.Len(t, discoverer.peers, 0)
	discoverer.peersMu.RUnlock()
}

func TestDiscoverer_SetBootstrapPeers(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID1, peerID2 types.NodeID
	copy(peerID1[:], []byte("peer-id-1-12345678"))
	copy(peerID2[:], []byte("peer-id-2-12345678"))

	peers := []discoveryif.PeerInfo{
		{ID: peerID1, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")}},
		{ID: peerID2, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.2/udp/4001/quic-v1")}},
	}

	discoverer.SetBootstrapPeers(peers)

	discoverer.peersMu.RLock()
	assert.Len(t, discoverer.peers, 2)
	discoverer.peersMu.RUnlock()
}

func TestDiscoverer_GetBootstrapPeers(t *testing.T) {
	var peerID types.NodeID
	copy(peerID[:], []byte("get-peer-id-12345678"))

	peers := []discoveryif.PeerInfo{
		{ID: peerID, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")}},
	}

	cfg := DefaultConfig()
	cfg.Peers = peers

	connector := newMockConnector()
	discoverer := NewDiscoverer(cfg, connector)

	ctx := context.Background()
	gotPeers, err := discoverer.GetBootstrapPeers(ctx)
	assert.NoError(t, err)
	assert.Len(t, gotPeers, 1)
}

// ============================================================================
//                              发现测试
// ============================================================================

func TestDiscoverer_FindPeers_NoPeers(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	peers, err := discoverer.FindPeers(ctx, nil)
	// FindPeers 可能返回空结果或错误，取决于实现
	// 我们只验证调用不会 panic
	_ = err
	_ = peers
}

func TestDiscoverer_Start_Stop(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	assert.NoError(t, err)

	// 停止
	err = discoverer.Stop()
	assert.NoError(t, err)
}

func TestDiscoverer_Close(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	err := discoverer.Close()
	assert.NoError(t, err)
}

// ============================================================================
//                              连接状态测试
// ============================================================================

func TestDiscoverer_IsConnected(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID types.NodeID
	copy(peerID[:], []byte("connected-peer-id-12"))

	// 初始未连接
	assert.False(t, discoverer.IsConnected(peerID))

	// 标记为已连接
	discoverer.connectedMu.Lock()
	discoverer.connected[peerID] = true
	discoverer.connectedMu.Unlock()

	// 现在应该已连接
	assert.True(t, discoverer.IsConnected(peerID))
}

func TestDiscoverer_ConnectedCount(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	// 初始为 0
	assert.Equal(t, 0, discoverer.ConnectedCount())

	// 添加连接
	var peerID types.NodeID
	copy(peerID[:], []byte("connected-peer-id-12"))

	discoverer.connectedMu.Lock()
	discoverer.connected[peerID] = true
	discoverer.connectedMu.Unlock()

	assert.Equal(t, 1, discoverer.ConnectedCount())
}

func TestDiscoverer_DiscoveredCount(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	// 初始为 0
	assert.Equal(t, 0, discoverer.DiscoveredCount())

	// 添加发现的节点
	var peerID types.NodeID
	copy(peerID[:], []byte("discovered-peer-1234"))

	discoverer.discoveredMu.Lock()
	discoverer.discovered[peerID] = discoveryif.PeerInfo{ID: peerID}
	discoverer.discoveredMu.Unlock()

	assert.Equal(t, 1, discoverer.DiscoveredCount())
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestDiscoverer_Concurrency(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			var peerID types.NodeID
			copy(peerID[:], []byte("peer-id-"+string(rune('0'+id))))
			peer := discoveryif.PeerInfo{
				ID:    peerID,
				Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")},
			}
			discoverer.AddBootstrapPeer(peer)
		}(i)

		go func(id int) {
			defer wg.Done()
			discoverer.peersMu.RLock()
			_ = len(discoverer.peers)
			discoverer.peersMu.RUnlock()
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              错误定义测试
// ============================================================================

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrNoBootstrapPeers)
	assert.NotNil(t, ErrBootstrapFailed)
}

// ============================================================================
//                              Start/Stop 幂等性测试
// ============================================================================

func TestDiscoverer_StartMultiple(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	ctx := context.Background()

	// 第一次启动
	err := discoverer.Start(ctx)
	assert.NoError(t, err)

	// 第二次启动应该是安全的（不会重复启动）
	err = discoverer.Start(ctx)
	assert.NoError(t, err)

	// 停止
	err = discoverer.Stop()
	assert.NoError(t, err)
}

func TestDiscoverer_StopIdempotent(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	assert.NoError(t, err)

	// 多次停止应该都是安全的
	for i := 0; i < 3; i++ {
		err = discoverer.Stop()
		assert.NoError(t, err)
	}
}

func TestDiscoverer_StopWithoutStart(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	// 未启动时停止应该是安全的
	err := discoverer.Stop()
	assert.NoError(t, err)
}

// ============================================================================
//                              connectToPeer 安全测试
// ============================================================================

func TestDiscoverer_ConnectPeerWithoutStart(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	peer := discoveryif.PeerInfo{
		ID:    peerID,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")},
	}

	// 未调用 Start() 时，connectToPeer 应该返回 false 而不是 panic
	result := discoverer.connectToPeer(peer)
	assert.False(t, result)
}

func TestDiscoverer_FetchPeersWithoutStart(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 未调用 Start() 时，fetchPeersFrom 应该安全返回而不是 panic
	discoverer.fetchPeersFrom(peerID)
	// 不应该 panic
}

// stringAddress 测试已迁移到 internal/core/address/addr_test.go
// 所有散落的 Address 实现已统一使用 address.Addr

// ============================================================================
//                              bootstrap 流程测试
// ============================================================================

func TestDiscoverer_Bootstrap_ShuffleOrder(t *testing.T) {
	var peerID1, peerID2, peerID3 types.NodeID
	copy(peerID1[:], []byte("peer-id-1-12345678"))
	copy(peerID2[:], []byte("peer-id-2-12345678"))
	copy(peerID3[:], []byte("peer-id-3-12345678"))

	peers := []discoveryif.PeerInfo{
		{ID: peerID1, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")}},
		{ID: peerID2, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.2/udp/4001/quic-v1")}},
		{ID: peerID3, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.3/udp/4001/quic-v1")}},
	}

	cfg := DefaultConfig()
	cfg.Peers = peers
	cfg.ShuffleOrder = true

	connector := newMockConnector()
	discoverer := NewDiscoverer(cfg, connector)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)

	// 等待 bootstrap 完成
	time.Sleep(100 * time.Millisecond)

	err = discoverer.Stop()
	require.NoError(t, err)
}

func TestDiscoverer_Bootstrap_NoShuffle(t *testing.T) {
	var peerID types.NodeID
	copy(peerID[:], []byte("peer-id-1-12345678"))

	peers := []discoveryif.PeerInfo{
		{ID: peerID, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.1/udp/4001/quic-v1")}},
	}

	cfg := DefaultConfig()
	cfg.Peers = peers
	cfg.ShuffleOrder = false

	connector := newMockConnector()
	discoverer := NewDiscoverer(cfg, connector)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	err = discoverer.Stop()
	require.NoError(t, err)
}

func TestDiscoverer_DiscoverPeers(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	var peerID types.NodeID
	copy(peerID[:], []byte("discovered-peer-1234"))

	// 添加一个已发现的节点
	discoverer.discoveredMu.Lock()
	discoverer.discovered[peerID] = discoveryif.PeerInfo{
		ID:    peerID,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/192.168.1.100/udp/4001/quic-v1")},
	}
	discoverer.discoveredMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch, err := discoverer.DiscoverPeers(ctx, "")
	require.NoError(t, err)

	count := 0
	for range ch {
		count++
	}

	assert.Equal(t, 1, count)
}

func TestDiscoverer_FindClosestPeers(t *testing.T) {
	connector := newMockConnector()
	discoverer := NewDiscoverer(DefaultConfig(), connector)

	// 添加一些已发现的节点
	for i := 0; i < 5; i++ {
		var peerID types.NodeID
		copy(peerID[:], []byte{byte(i), 'p', 'e', 'e', 'r'})
		discoverer.discoveredMu.Lock()
		discoverer.discovered[peerID] = discoveryif.PeerInfo{ID: peerID}
		discoverer.discoveredMu.Unlock()
	}

	ctx := context.Background()
	peers, err := discoverer.FindClosestPeers(ctx, []byte{1, 2, 3}, 3)
	require.NoError(t, err)
	assert.Len(t, peers, 3)
}
