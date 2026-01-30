package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     Connector Setter 测试
// ============================================================================

func TestConnector_SetHost(t *testing.T) {
	conn := NewConnector(nil, nil, nil, DefaultConnectorConfig())
	defer conn.Close()

	assert.Nil(t, conn.host)

	mockHost := &mockHost{}
	conn.SetHost(mockHost)

	assert.Equal(t, mockHost, conn.host)
}

func TestConnector_SetHolePuncher(t *testing.T) {
	conn := NewConnector(nil, nil, nil, DefaultConnectorConfig())
	defer conn.Close()

	assert.Nil(t, conn.holePuncher)

	// 创建一个 nil HolePuncher 的情况
	conn.SetHolePuncher(nil)
	assert.Nil(t, conn.holePuncher)
}

// ============================================================================
//                     Connector connectDirect 测试
// ============================================================================

func TestConnector_ConnectDirect_Strategy(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	// 预填充地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	ps.AddAddrs(types.PeerID("peer-direct"), []types.Multiaddr{addr}, time.Hour)

	resolver := NewAddressResolver(ResolverConfig{
		Peerstore: ps,
	})

	// 使用 DirectOnly 策略
	config := DefaultConnectorConfig()
	config.Strategy = StrategyDirectOnly
	config.DirectTimeout = 100 * time.Millisecond

	conn := NewConnector(resolver, nil, nil, config)
	defer conn.Close()

	// 无 Host 时直连失败
	_, err := conn.Connect(context.Background(), "peer-direct")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "direct connect failed")
}

func TestConnector_ConnectDirect_NoResolver(t *testing.T) {
	// 无 resolver 时使用 DirectOnly 策略
	config := DefaultConnectorConfig()
	config.Strategy = StrategyDirectOnly

	conn := NewConnector(nil, nil, nil, config)
	defer conn.Close()

	_, err := conn.Connect(context.Background(), "peer-1")
	assert.ErrorIs(t, err, ErrNoAddress)
}

func TestConnector_ConnectDirect_NoAddress(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver := NewAddressResolver(ResolverConfig{
		Peerstore: ps,
	})

	// 使用 DirectOnly 策略，但没有地址
	config := DefaultConnectorConfig()
	config.Strategy = StrategyDirectOnly

	conn := NewConnector(resolver, nil, nil, config)
	defer conn.Close()

	_, err := conn.Connect(context.Background(), "unknown-peer")
	assert.ErrorIs(t, err, ErrNoAddress)
}

// ============================================================================
//                     connWrapper 测试
// ============================================================================

func TestConnWrapper_LocalPeer(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	assert.Equal(t, types.PeerID(""), w.LocalPeer())
}

func TestConnWrapper_LocalMultiaddr(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	assert.Nil(t, w.LocalMultiaddr())
}

func TestConnWrapper_RemotePeer(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	assert.Equal(t, types.PeerID("peer-123"), w.RemotePeer())
}

func TestConnWrapper_RemoteMultiaddr(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	assert.Nil(t, w.RemoteMultiaddr())
}

func TestConnWrapper_NewStream(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	stream, err := w.NewStream(context.Background())
	assert.Nil(t, stream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestConnWrapper_AcceptStream(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	stream, err := w.AcceptStream()
	assert.Nil(t, stream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestConnWrapper_GetStreams(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	streams := w.GetStreams()
	assert.Nil(t, streams)
}

func TestConnWrapper_Stat(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	stat := w.Stat()
	assert.Equal(t, pkgif.ConnectionStat{}, stat)
}

func TestConnWrapper_Close(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	err := w.Close()
	assert.NoError(t, err)
}

func TestConnWrapper_IsClosed(t *testing.T) {
	w := &connWrapper{
		remotePeer: "peer-123",
		method:     MethodDirect,
	}

	assert.False(t, w.IsClosed())
}

// ============================================================================
//                     AddressResolver Setter 测试
// ============================================================================

func TestResolver_SetPeerstore(t *testing.T) {
	resolver := NewAddressResolver(ResolverConfig{})

	assert.Nil(t, resolver.peerstore)

	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver.SetPeerstore(ps)
	assert.Equal(t, ps, resolver.peerstore)
}

func TestResolver_SetAddrSyncer(t *testing.T) {
	resolver := NewAddressResolver(ResolverConfig{})

	assert.Nil(t, resolver.addrSyncer)

	syncer := &mockAddrSyncer{}
	resolver.SetAddrSyncer(syncer)
	assert.Equal(t, syncer, resolver.addrSyncer)
}

// ============================================================================
//                     AddressResolver cacheAddrs 测试
// ============================================================================

func TestResolver_CacheAddrs_WithAddrSyncer(t *testing.T) {
	syncer := &mockAddrSyncer{}
	resolver := NewAddressResolver(ResolverConfig{
		AddrSyncer: syncer,
	})

	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addrs := []types.Multiaddr{addr}

	resolver.cacheAddrs("peer-123", addrs, SourceRelay)

	// 验证 syncer 被调用
	assert.Equal(t, types.PeerID("peer-123"), syncer.lastPeerID)
	assert.Equal(t, addrs, syncer.lastAddrs)
}

func TestResolver_CacheAddrs_WithPeerstore(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver := NewAddressResolver(ResolverConfig{
		Peerstore: ps,
	})

	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addrs := []types.Multiaddr{addr}

	resolver.cacheAddrs("peer-123", addrs, SourceDHT)

	// 验证地址被缓存到 peerstore
	cached := ps.Addrs("peer-123")
	require.Len(t, cached, 1)
	assert.Equal(t, addr.String(), cached[0].String())
}

func TestResolver_CacheAddrs_NoPeerstore(t *testing.T) {
	resolver := NewAddressResolver(ResolverConfig{})

	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	addrs := []types.Multiaddr{addr}

	// 不应该 panic
	resolver.cacheAddrs("peer-123", addrs, SourceRelay)
}

// ============================================================================
//                     dialHolePunch 测试
// ============================================================================

func TestConnector_DialHolePunch_NoPuncher(t *testing.T) {
	conn := NewConnector(nil, nil, nil, DefaultConnectorConfig())
	defer conn.Close()

	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	_, err := conn.dialHolePunch(context.Background(), "peer-1", []types.Multiaddr{addr})

	assert.ErrorIs(t, err, ErrHolePunchFailed)
}

// ============================================================================
//                     Mock 实现
// ============================================================================

// mockHost 是简化的 Host mock
type mockHost struct {
	connectFunc func(ctx context.Context, peerID string, addrs []string) error
}

func (m *mockHost) ID() string                                               { return "mock-host" }
func (m *mockHost) Addrs() []string                                          { return []string{"/ip4/127.0.0.1/tcp/4001"} }
func (m *mockHost) Listen(addrs ...string) error                             { return nil }
func (m *mockHost) Close() error                                             { return nil }
func (m *mockHost) AdvertisedAddrs() []string                                { return m.Addrs() }
func (m *mockHost) ShareableAddrs() []string                                 { return nil }
func (m *mockHost) HolePunchAddrs() []string                                 { return nil }
func (m *mockHost) Peerstore() pkgif.Peerstore                               { return nil }
func (m *mockHost) EventBus() pkgif.EventBus                                 { return nil }
func (m *mockHost) SetStreamHandler(string, pkgif.StreamHandler)             {}
func (m *mockHost) RemoveStreamHandler(string)                               {}
func (m *mockHost) SetReachabilityCoordinator(pkgif.ReachabilityCoordinator) {}

func (m *mockHost) Network() pkgif.Swarm { return nil }

func (m *mockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

func (m *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	if m.connectFunc != nil {
		return m.connectFunc(ctx, peerID, addrs)
	}
	return errors.New("mock connect not implemented")
}

func (m *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	return nil, errors.New("mock newstream not implemented")
}

func (m *mockHost) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
}

// mockAddrSyncer 是地址同步器的 mock
type mockAddrSyncer struct {
	lastPeerID types.PeerID
	lastAddrs  []types.Multiaddr
}

func (m *mockAddrSyncer) SyncFromRelay(peerID types.PeerID, addrs []types.Multiaddr) {
	m.lastPeerID = peerID
	m.lastAddrs = addrs
}

func (m *mockAddrSyncer) SyncFromDHT(peerID types.PeerID, addrs []types.Multiaddr) {
	m.lastPeerID = peerID
	m.lastAddrs = addrs
}

// ============================================================================
//                     ConnectMethod 测试
// ============================================================================

func TestConnectMethod_Constants(t *testing.T) {
	assert.Equal(t, ConnectMethod("direct"), MethodDirect)
	assert.Equal(t, ConnectMethod("holepunch"), MethodHolePunch)
	assert.Equal(t, ConnectMethod("relay"), MethodRelay)
}

// ============================================================================
//                     ConnectStrategy 测试
// ============================================================================

func TestConnectStrategy_Constants(t *testing.T) {
	assert.Equal(t, ConnectStrategy(0), StrategyAuto)
	assert.Equal(t, ConnectStrategy(1), StrategyDirectOnly)
	assert.Equal(t, ConnectStrategy(2), StrategyRelayOnly)
}

// ============================================================================
//                     Config 边界测试
// ============================================================================

func TestNewConnector_DefaultTimeouts(t *testing.T) {
	// 测试零值配置会被设置为默认值
	config := ConnectorConfig{}
	conn := NewConnector(nil, nil, nil, config)
	defer conn.Close()

	assert.Equal(t, 5*time.Second, conn.config.DirectTimeout)
	assert.Equal(t, 10*time.Second, conn.config.HolePunchTimeout)
	assert.Equal(t, 10*time.Second, conn.config.RelayTimeout)
	assert.Equal(t, 30*time.Second, conn.config.TotalTimeout)
}

func TestNewAddressResolver_DefaultTimeouts(t *testing.T) {
	// 测试零值配置会被设置为默认值
	resolver := NewAddressResolver(ResolverConfig{})

	assert.Equal(t, 5*time.Second, resolver.queryTimeout)
	assert.Equal(t, 15*time.Minute, resolver.cacheTTL)
}

// ============================================================================
//                     HolePuncher 集成测试
// ============================================================================

// 用于测试 dialHolePunch 函数 - 需要实际的 HolePuncher
// 由于 HolePuncher 依赖复杂，这里仅验证错误路径

func TestConnector_ConnectWithHint_HolePunchPath(t *testing.T) {
	ps := peerstore.NewPeerstore()
	defer ps.Close()

	resolver := NewAddressResolver(ResolverConfig{
		Peerstore: ps,
	})

	// 启用 HolePunch 但没有 HolePuncher
	config := DefaultConnectorConfig()
	config.EnableHolePunch = true
	config.DirectTimeout = 50 * time.Millisecond

	// 使用会失败的 mockHost
	mockH := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			return errors.New("direct connect failed")
		},
	}

	conn := NewConnector(resolver, mockH, nil, config)
	defer conn.Close()

	// 有效的 hints
	hints := []string{"/ip4/10.0.0.1/tcp/4001"}
	_, err := conn.ConnectWithHint(context.Background(), "peer-hp", hints)

	// 应该失败（直连失败，打洞也失败因为没有 HolePuncher，最后 Relay 也失败）
	assert.Error(t, err)
}

// ============================================================================
//                     未导出类型引用测试（确保编译正确）
// ============================================================================

// 测试引用 holepunch 包（确保导入正确）
var _ *holepunch.HolePuncher
