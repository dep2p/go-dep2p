// Package endpoint 提供 Endpoint 聚合模块的测试
package endpoint

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockIdentity 模拟身份
type mockIdentity struct {
	id     types.NodeID
	pubKey coreif.PublicKey
}

func newMockIdentity() *mockIdentity {
	var id types.NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte(i)
	}
	return &mockIdentity{id: id}
}

func (m *mockIdentity) ID() types.NodeID {
	return m.id
}

func (m *mockIdentity) PublicKey() coreif.PublicKey {
	return m.pubKey
}

func (m *mockIdentity) PrivateKey() coreif.PrivateKey {
	return nil
}

func (m *mockIdentity) Sign(data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockIdentity) Verify(data, sig []byte, pubKey coreif.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockIdentity) KeyType() types.KeyType {
	return types.KeyTypeEd25519
}

// 确保实现接口
var _ identityif.Identity = (*mockIdentity)(nil)

// ============================================================================
//                              AddressBook 测试
// ============================================================================
// 注意: 使用 newSimpleAddr 或 address.NewAddr 创建测试地址

func TestAddressBook_Add(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(nodeID, addr)

	addrs := ab.Get(nodeID)
	assert.Len(t, addrs, 1)
	assert.Equal(t, "127.0.0.1:8000", addrs[0].String())
}

func TestAddressBook_AddMultiple(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr1 := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")
	addr2 := newSimpleAddr("/ip4/127.0.0.1/udp/8001/quic-v1")

	ab.Add(nodeID, addr1, addr2)

	addrs := ab.Get(nodeID)
	assert.Len(t, addrs, 2)
}

func TestAddressBook_AddDuplicate(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(nodeID, addr)
	ab.Add(nodeID, addr) // 添加重复

	addrs := ab.Get(nodeID)
	assert.Len(t, addrs, 1) // 应该只有一个
}

func TestAddressBook_GetUnknown(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()

	addrs := ab.Get(nodeID)
	assert.Nil(t, addrs)
}

func TestAddressBook_Remove(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(nodeID, addr)
	ab.Remove(nodeID)

	addrs := ab.Get(nodeID)
	assert.Nil(t, addrs)
}

func TestAddressBook_Peers(t *testing.T) {
	ab := NewAddressBook()

	id1 := newMockIdentity().ID()
	var id2 types.NodeID
	for i := 0; i < 32; i++ {
		id2[i] = byte(i + 100)
	}

	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(id1, addr)
	ab.Add(id2, addr)

	peers := ab.Peers()
	assert.Len(t, peers, 2)
}

func TestAddressBook_Clear(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(nodeID, addr)
	ab.Clear()

	assert.Equal(t, 0, ab.Count())
}

func TestAddressBook_Count(t *testing.T) {
	ab := NewAddressBook()

	assert.Equal(t, 0, ab.Count())

	nodeID := newMockIdentity().ID()
	addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")

	ab.Add(nodeID, addr)
	assert.Equal(t, 1, ab.Count())
}

// ============================================================================
//                              Endpoint 测试
// ============================================================================

func TestEndpoint_ID(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	assert.Equal(t, identity.ID(), ep.ID())
}

func TestEndpoint_PublicKey(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	assert.Equal(t, identity.PublicKey(), ep.PublicKey())
}

func TestEndpoint_NilIdentity(t *testing.T) {
	ep := NewEndpoint(nil, nil, nil, nil)

	assert.Equal(t, coreif.EmptyNodeID, ep.ID())
	assert.Nil(t, ep.PublicKey())
}

func TestEndpoint_SetProtocolHandler(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	handler := func(s coreif.Stream) {
		io.Copy(s, s)
	}

	ep.SetProtocolHandler("/echo/1.0", handler)

	protocols := ep.Protocols()
	assert.Len(t, protocols, 1)
	assert.Equal(t, coreif.ProtocolID("/echo/1.0"), protocols[0])
}

func TestEndpoint_RemoveProtocolHandler(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	handler := func(s coreif.Stream) {}

	ep.SetProtocolHandler("/echo/1.0", handler)
	ep.RemoveProtocolHandler("/echo/1.0")

	protocols := ep.Protocols()
	assert.Len(t, protocols, 0)
}

func TestEndpoint_Connections(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	conns := ep.Connections()
	assert.Len(t, conns, 0)
}

func TestEndpoint_ConnectionCount(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	assert.Equal(t, 0, ep.ConnectionCount())
}

func TestEndpoint_ConnectSelf(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	_, err := ep.Connect(context.Background(), identity.ID())
	assert.ErrorIs(t, err, coreif.ErrSelfConnect)
}

func TestEndpoint_ConnectWithAddrsSelf(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	addrs := []coreif.Address{newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")}
	_, err := ep.ConnectWithAddrs(context.Background(), identity.ID(), addrs)
	assert.ErrorIs(t, err, coreif.ErrSelfConnect)
}

func TestEndpoint_ConnectNoAddresses(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	var remoteID types.NodeID
	for i := 0; i < 32; i++ {
		remoteID[i] = byte(i + 100)
	}

	_, err := ep.Connect(context.Background(), remoteID)
	assert.ErrorIs(t, err, coreif.ErrNoAddresses)
}

func TestEndpoint_ConnectWithAddrsEmpty(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	var remoteID types.NodeID
	for i := 0; i < 32; i++ {
		remoteID[i] = byte(i + 100)
	}

	_, err := ep.ConnectWithAddrs(context.Background(), remoteID, nil)
	assert.ErrorIs(t, err, coreif.ErrNoAddresses)
}

func TestEndpoint_AddressBook(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	ab := ep.AddressBook()
	assert.NotNil(t, ab)
}

func TestEndpoint_ListenAddrsEmpty(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	addrs := ep.ListenAddrs()
	assert.Len(t, addrs, 0)
}

func TestEndpoint_AdvertisedAddrsEmpty(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	addrs := ep.AdvertisedAddrs()
	assert.Len(t, addrs, 0)
}

func TestEndpoint_AddAdvertisedAddr(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	addr := newSimpleAddr("/ip4/1.2.3.4/udp/8000/quic-v1")
	ep.AddAdvertisedAddr(addr)

	addrs := ep.AdvertisedAddrs()
	assert.Len(t, addrs, 1)
	assert.Equal(t, "1.2.3.4:8000", addrs[0].String())
}

func TestEndpoint_SubsystemsNil(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	assert.Nil(t, ep.Discovery())
	assert.Nil(t, ep.NAT())
	assert.Nil(t, ep.Relay())
}

func TestEndpoint_Close(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	err := ep.Close()
	assert.NoError(t, err)

	// 再次关闭应该是幂等的
	err = ep.Close()
	assert.NoError(t, err)
}

func TestEndpoint_DoubleStart(t *testing.T) {
	identity := newMockIdentity()
	config := &Config{ListenAddrs: []string{}}
	ep := NewEndpointWithConfig(identity, nil, nil, nil, nil, nil, nil, nil, nil, config)

	ctx := context.Background()

	// 第一次启动
	err := ep.Listen(ctx)
	assert.NoError(t, err)

	// 第二次启动应该报错
	err = ep.Listen(ctx)
	assert.ErrorIs(t, err, coreif.ErrAlreadyStarted)
}

func TestEndpoint_AcceptAfterClose(t *testing.T) {
	identity := newMockIdentity()
	config := &Config{ListenAddrs: []string{}}
	ep := NewEndpointWithConfig(identity, nil, nil, nil, nil, nil, nil, nil, nil, config)

	ep.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := ep.Accept(ctx)
	assert.ErrorIs(t, err, coreif.ErrAlreadyClosed)
}

// ============================================================================
//                              Stream 测试
// ============================================================================

func TestStream_State(t *testing.T) {
	// 创建一个基本的流状态测试
	s := &Stream{
		id:         1,
		protocolID: "/test/1.0",
		state:      coreif.StreamStateOpen,
		priority:   types.PriorityNormal,
		createdAt:  time.Now(),
	}

	assert.Equal(t, coreif.StreamStateOpen, s.State())
	assert.Equal(t, types.StreamID(1), s.ID())
	assert.Equal(t, coreif.ProtocolID("/test/1.0"), s.ProtocolID())
	assert.Equal(t, types.PriorityNormal, s.Priority())
}

func TestStream_SetPriority(t *testing.T) {
	s := &Stream{
		priority: types.PriorityNormal,
	}

	s.SetPriority(types.PriorityHigh)
	assert.Equal(t, types.PriorityHigh, s.Priority())
}

func TestStream_Stats(t *testing.T) {
	s := &Stream{
		createdAt: time.Now().Add(-time.Second),
	}
	s.stats.BytesSent = 100
	s.stats.BytesRecv = 200

	stats := s.Stats()
	assert.Equal(t, uint64(100), stats.BytesSent)
	assert.Equal(t, uint64(200), stats.BytesRecv)
}

// ============================================================================
//                              Connection 测试
// ============================================================================

func TestConnection_Direction(t *testing.T) {
	conn := &Connection{
		direction: types.DirOutbound,
		handlers:  make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:   make(map[coreif.StreamID]*Stream),
		closeCh:   make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	assert.Equal(t, types.DirOutbound, conn.Direction())
}

func TestConnection_Streams(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	streams := conn.Streams()
	assert.Len(t, streams, 0)
}

func TestConnection_StreamCount(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	assert.Equal(t, 0, conn.StreamCount())
}

func TestConnection_SetStreamHandler(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	handler := func(s coreif.Stream) {}
	conn.SetStreamHandler("/test/1.0", handler)

	h, ok := conn.getHandler("/test/1.0")
	assert.True(t, ok)
	assert.NotNil(t, h)
}

func TestConnection_RemoveStreamHandler(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	handler := func(s coreif.Stream) {}
	conn.SetStreamHandler("/test/1.0", handler)
	conn.RemoveStreamHandler("/test/1.0")

	_, ok := conn.getHandler("/test/1.0")
	assert.False(t, ok)
}

func TestConnection_Close(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	err := conn.Close()
	assert.NoError(t, err)
	assert.True(t, conn.IsClosed())
}

func TestConnection_DoubleClose(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	conn.Close()
	err := conn.Close() // 应该是幂等的
	assert.NoError(t, err)
}

func TestConnection_Context(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	ctx := conn.Context()
	assert.NotNil(t, ctx)
}

func TestConnection_Done(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	done := conn.Done()
	assert.NotNil(t, done)

	conn.Close()

	select {
	case <-done:
		// 预期
	case <-time.After(time.Second):
		t.Fatal("Done channel should be closed")
	}
}

// ============================================================================
//                              fx 模块测试
// ============================================================================

func TestModule_Basic(t *testing.T) {
	var endpoint coreif.Endpoint

	// 创建模拟身份
	identity := newMockIdentity()

	app := fx.New(
		fx.NopLogger,
		fx.Provide(func() identityif.Identity {
			return identity
		}),
		fx.Provide(fx.Annotate(
			func(id identityif.Identity) identityif.Identity {
				return id
			},
			fx.ResultTags(`name:"identity"`),
		)),
		// 使用 ProvideServices 而不是完整的 Module() 来避免生命周期问题
		fx.Provide(func(id identityif.Identity) (coreif.Endpoint, error) {
			return NewEndpoint(id, nil, nil, nil), nil
		}),
		fx.Populate(&endpoint),
	)

	require.NoError(t, app.Err())
	require.NotNil(t, endpoint)
	assert.Equal(t, identity.ID(), endpoint.ID())
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestAddressBook_Concurrent(t *testing.T) {
	ab := NewAddressBook()

	var wg sync.WaitGroup
	nodeCount := 100

	// 并发添加
	for i := 0; i < nodeCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var nodeID types.NodeID
			nodeID[0] = byte(idx)
			addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")
			ab.Add(nodeID, addr)
		}(i)
	}

	wg.Wait()

	// 验证
	peers := ab.Peers()
	assert.Len(t, peers, nodeCount)
}

func TestEndpoint_ConcurrentHandlers(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	var wg sync.WaitGroup
	handlerCount := 100

	// 并发注册处理器
	for i := 0; i < handlerCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			protocolID := coreif.ProtocolID(string(rune('a' + idx%26)))
			handler := func(s coreif.Stream) {}
			ep.SetProtocolHandler(protocolID, handler)
		}(i)
	}

	wg.Wait()

	// 验证 - 由于有重复的协议ID，数量可能少于 handlerCount
	protocols := ep.Protocols()
	assert.Greater(t, len(protocols), 0)
}

// ============================================================================
//                              补充测试
// ============================================================================

func TestAddressBook_MultipleAddrs(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()
	addr1 := newSimpleAddr("/ip4/192.168.1.1/udp/8000/quic-v1")
	addr2 := newSimpleAddr("/ip4/192.168.1.2/udp/8000/quic-v1")
	addr3 := newSimpleAddr("/ip4/192.168.1.3/udp/8000/quic-v1")

	// 分批添加
	ab.Add(nodeID, addr1)
	ab.Add(nodeID, addr2)
	ab.Add(nodeID, addr3)

	addrs := ab.Get(nodeID)
	assert.Len(t, addrs, 3)
}

func TestAddressBook_RemoveNonExistent(t *testing.T) {
	ab := NewAddressBook()

	nodeID := newMockIdentity().ID()

	// 移除不存在的节点不应 panic
	ab.Remove(nodeID)
}

func TestConnection_Stats(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	stats := conn.Stats()
	// 验证结构存在
	assert.NotNil(t, stats)
}

func TestConnection_Transport(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	// Transport 返回空字符串或默认值（未设置）
	transport := conn.Transport()
	assert.NotNil(t, transport)
}

func TestStream_IsClosed(t *testing.T) {
	s := &Stream{
		state:  coreif.StreamStateClosed,
		closed: 1, // atomic closed flag
	}

	assert.True(t, s.IsClosed())

	s2 := &Stream{
		state:  coreif.StreamStateOpen,
		closed: 0,
	}

	assert.False(t, s2.IsClosed())
}

func TestConnection_LocalAndRemoteAddrs(t *testing.T) {
	localAddr := newSimpleAddr("/ip4/192.168.1.1/udp/8000/quic-v1")
	remoteAddr := newSimpleAddr("/ip4/192.168.1.2/udp/9000/quic-v1")

	conn := &Connection{
		localAddrs: []coreif.Address{localAddr},
		remoteAddr: remoteAddr,
		handlers:   make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:    make(map[coreif.StreamID]*Stream),
		closeCh:    make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	localAddrs := conn.LocalAddrs()
	assert.Len(t, localAddrs, 1)
	assert.Equal(t, "192.168.1.1:8000", localAddrs[0].String())

	remoteAddrs := conn.RemoteAddrs()
	assert.Len(t, remoteAddrs, 1)
	assert.Equal(t, "192.168.1.2:9000", remoteAddrs[0].String())
}

func TestConnection_RemoteIDAndPublicKey(t *testing.T) {
	var remoteID types.NodeID
	for i := 0; i < 32; i++ {
		remoteID[i] = byte(i + 50)
	}

	conn := &Connection{
		remoteID: remoteID,
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	assert.Equal(t, remoteID, conn.RemoteID())
	assert.Nil(t, conn.RemotePublicKey()) // 未设置时为 nil
}

func TestConnection_LocalID(t *testing.T) {
	var localID types.NodeID
	for i := 0; i < 32; i++ {
		localID[i] = byte(i)
	}

	conn := &Connection{
		localID:  localID,
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	assert.Equal(t, localID, conn.LocalID())
}

func TestEndpoint_Connection(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	var remoteID types.NodeID
	for i := 0; i < 32; i++ {
		remoteID[i] = byte(i + 100)
	}

	_, ok := ep.Connection(remoteID)
	assert.False(t, ok, "应该找不到不存在的连接")
}

func TestEndpoint_Disconnect(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	var remoteID types.NodeID
	for i := 0; i < 32; i++ {
		remoteID[i] = byte(i + 100)
	}

	// 断开不存在的连接不应该报错
	err := ep.Disconnect(remoteID)
	assert.NoError(t, err)
}

func TestEndpoint_MultipleAdvertisedAddrs(t *testing.T) {
	identity := newMockIdentity()
	ep := NewEndpoint(identity, nil, nil, nil)

	addr1 := newSimpleAddr("/ip4/1.2.3.4/udp/8000/quic-v1")
	addr2 := newSimpleAddr("/ip4/5.6.7.8/udp/8000/quic-v1")
	addr3 := newSimpleAddr("/ip4/9.10.11.12/udp/8000/quic-v1")

	ep.AddAdvertisedAddr(addr1)
	ep.AddAdvertisedAddr(addr2)
	ep.AddAdvertisedAddr(addr3)

	addrs := ep.AdvertisedAddrs()
	assert.Len(t, addrs, 3, "应添加多个不同地址")
}

func TestConnection_CloseWithError(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	err := conn.CloseWithError(100, "test error")
	assert.NoError(t, err)
	assert.True(t, conn.IsClosed())
}

// ============================================================================
//                              地址解析测试
// ============================================================================

func TestParseAddress_Empty(t *testing.T) {
	_, err := parseAddress("")
	assert.Error(t, err)
}

func TestParseAddress_Multiaddr_IP4_TCP(t *testing.T) {
	addr, err := parseAddress("/ip4/127.0.0.1/tcp/8080")
	require.NoError(t, err)

	assert.Equal(t, "/ip4/127.0.0.1/tcp/8080", addr.String())
	assert.Equal(t, "ip4", addr.Network())
}

func TestParseAddress_Multiaddr_IP6(t *testing.T) {
	addr, err := parseAddress("/ip6/::1/tcp/8080")
	require.NoError(t, err)

	assert.Equal(t, "/ip6/::1/tcp/8080", addr.String())
	assert.Equal(t, "ip6", addr.Network())
}

func TestParseAddress_Multiaddr_UDP(t *testing.T) {
	addr, err := parseAddress("/ip4/192.168.1.1/udp/4001")
	require.NoError(t, err)

	assert.Equal(t, "/ip4/192.168.1.1/udp/4001", addr.String())
}

func TestParseAddress_Multiaddr_QUIC(t *testing.T) {
	addr, err := parseAddress("/ip4/192.168.1.1/udp/4001/quic")
	require.NoError(t, err)
	require.NotNil(t, addr)
}

func TestParseAddress_Multiaddr_WithP2P(t *testing.T) {
	addr, err := parseAddress("/ip4/127.0.0.1/tcp/4001/p2p/5Q2STWvBTestPeer")
	require.NoError(t, err)
	// 验证地址字符串包含 p2p 组件
	assert.Contains(t, addr.String(), "/p2p/")
}

func TestParseAddress_Multiaddr_Relay(t *testing.T) {
	addr, err := parseAddress("/ip4/127.0.0.1/tcp/4001/p2p-circuit/p2p/5Q2STWvBRemote")
	require.NoError(t, err)
	// 验证是 relay 地址
	assert.Contains(t, addr.String(), "/p2p-circuit/")
}

func TestParseAddress_HostPort_IP4(t *testing.T) {
	addr, err := parseAddress("192.168.1.100:8080")
	require.NoError(t, err)

	// 应该被转换为 multiaddr 格式
	assert.Contains(t, addr.String(), "192.168.1.100")
}

func TestParseAddress_HostPort_IP6(t *testing.T) {
	addr, err := parseAddress("[::1]:8080")
	require.NoError(t, err)

	assert.Contains(t, addr.String(), "8080")
}

func TestParseAddress_HostPort_Domain(t *testing.T) {
	addr, err := parseAddress("example.com:8080")
	require.NoError(t, err)
	// 应该包含域名
	assert.Contains(t, addr.String(), "example.com")
}

func TestParseAddress_HostPort_NoPort(t *testing.T) {
	addr, err := parseAddress("192.168.1.100")
	require.NoError(t, err)

	assert.Equal(t, "192.168.1.100", addr.String())
}

func TestParsedAddress_IsPublic(t *testing.T) {
	tests := []struct {
		name     string
		addrStr  string
		expected bool
	}{
		{"Loopback", "/ip4/127.0.0.1/tcp/8080", false},
		{"Private 192.168", "/ip4/192.168.1.1/tcp/8080", false},
		{"Private 10.x", "/ip4/10.0.0.1/tcp/8080", false},
		{"Public", "/ip4/8.8.8.8/tcp/8080", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseAddress(tt.addrStr)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, addr.IsPublic())
		})
	}
}

func TestParsedAddress_IsPrivate(t *testing.T) {
	tests := []struct {
		name     string
		addrStr  string
		expected bool
	}{
		{"Private 192.168", "/ip4/192.168.1.1/tcp/8080", true},
		{"Private 10.x", "/ip4/10.0.0.1/tcp/8080", true},
		{"Private 172.16", "/ip4/172.16.0.1/tcp/8080", true},
		{"Public", "/ip4/8.8.8.8/tcp/8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseAddress(tt.addrStr)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, addr.IsPrivate())
		})
	}
}

func TestParsedAddress_IsLoopback(t *testing.T) {
	tests := []struct {
		name     string
		addrStr  string
		expected bool
	}{
		{"IP4 Loopback", "/ip4/127.0.0.1/tcp/8080", true},
		{"localhost string", "localhost:8000", true},
		{"IP4 Private", "/ip4/192.168.1.1/tcp/8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := parseAddress(tt.addrStr)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, addr.IsLoopback())
		})
	}
}

func TestParsedAddress_Equal(t *testing.T) {
	addr1, _ := parseAddress("/ip4/127.0.0.1/tcp/8080")
	addr2, _ := parseAddress("/ip4/127.0.0.1/tcp/8080")
	addr3, _ := parseAddress("/ip4/192.168.1.1/tcp/8080")

	assert.True(t, addr1.Equal(addr2))
	assert.False(t, addr1.Equal(addr3))
	assert.False(t, addr1.Equal(nil))
}

func TestParsedAddress_Bytes(t *testing.T) {
	addr, _ := parseAddress("/ip4/127.0.0.1/tcp/8080")

	bytes := addr.Bytes()
	assert.NotEmpty(t, bytes)
	assert.Equal(t, addr.String(), string(bytes))
}

func TestParsedAddress_IPAndPort(t *testing.T) {
	addr, _ := parseAddress("/ip4/192.168.1.100/tcp/9000")
	// 验证地址字符串正确
	assert.Contains(t, addr.String(), "192.168.1.100")
	assert.Contains(t, addr.String(), "9000")
}

// TestParsedAddress_NetworkDefault 和 TestParsedAddress_NilIP 已移除
// parsedAddress 类型已删除，统一使用 address.Addr

func TestParseMultiaddr_Short(t *testing.T) {
	// 不完整的 multiaddr
	addr, err := parseMultiaddr("/ip4")
	require.NoError(t, err)
	assert.Equal(t, "/ip4", addr.String())
}

func TestStream_Connection(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	s := &Stream{
		conn: conn,
	}

	assert.Equal(t, conn, s.Connection())
}

func TestStream_CloseRead(t *testing.T) {
	// 创建模拟 muxer stream
	muxerStream := &mockMuxerStream{}

	s := &Stream{
		id:          1,
		muxerStream: muxerStream,
		state:       coreif.StreamStateOpen,
	}

	err := s.CloseRead()
	assert.NoError(t, err)
	assert.Equal(t, coreif.StreamStateReadClosed, s.State())
}

func TestStream_SetPriority_Concurrent(t *testing.T) {
	s := &Stream{
		priority: types.PriorityNormal,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(p types.Priority) {
			defer wg.Done()
			s.SetPriority(p)
		}(types.Priority(i % 3))
	}
	wg.Wait()

	// 验证不 panic
	_ = s.Priority()
}

func TestStream_SetBandwidthCounter(t *testing.T) {
	s := &Stream{}

	// SetBandwidthCounter 接受 nil
	s.SetBandwidthCounter(nil)

	// 验证不 panic
	assert.Nil(t, s.bwCounter)
}

func TestConnection_OpenStreamOnClosed(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
		closed:   1, // 已关闭
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	_, err := conn.OpenStream(context.Background(), "/test/1.0")
	assert.ErrorIs(t, err, coreif.ErrConnectionClosed)
}

func TestConnection_AcceptStreamOnClosed(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
		closed:   1, // 已关闭
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	_, err := conn.AcceptStream(context.Background())
	assert.ErrorIs(t, err, coreif.ErrConnectionClosed)
}

// ============================================================================
//                              Mock Muxer Stream 实现
// ============================================================================

type mockMuxerStream struct {
	id     uint32
	closed bool
}

func (m *mockMuxerStream) ID() uint32 {
	return m.id
}

func (m *mockMuxerStream) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (m *mockMuxerStream) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockMuxerStream) Close() error {
	m.closed = true
	return nil
}

func (m *mockMuxerStream) CloseRead() error {
	return nil
}

func (m *mockMuxerStream) CloseWrite() error {
	return nil
}

func (m *mockMuxerStream) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockMuxerStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockMuxerStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockMuxerStream) Reset() error {
	m.closed = true
	return nil
}

// ============================================================================
//                              修复验证测试
// ============================================================================

// TestNewConnection_NilSecureConn 测试 NewConnection 对 nil secureConn 的处理
func TestNewConnection_NilSecureConn(t *testing.T) {

	// 传入 nil secureConn 不应该 panic
	conn := NewConnection(nil, nil, types.DirOutbound, nil)
	require.NotNil(t, conn)

	// 验证身份信息为空
	assert.Equal(t, types.NodeID{}, conn.LocalID())
	assert.Equal(t, types.NodeID{}, conn.RemoteID())
	assert.Nil(t, conn.RemotePublicKey())
}

// TestConnection_OpenStreamWithPriority_NilMuxer 测试 muxer 为 nil 时的错误处理
func TestConnection_OpenStreamWithPriority_NilMuxer(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
		muxer:    nil, // muxer 为 nil
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	_, err := conn.OpenStreamWithPriority(context.Background(), "/test/1.0", types.PriorityNormal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "连接未启用多路复用")
}

// TestConnection_AcceptStream_NilMuxer 测试 AcceptStream 对 nil muxer 的处理
func TestConnection_AcceptStream_NilMuxer(t *testing.T) {
	conn := &Connection{
		handlers: make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		streams:  make(map[coreif.StreamID]*Stream),
		closeCh:  make(chan struct{}),
		muxer:    nil, // muxer 为 nil
	}
	conn.ctx, conn.cancel = context.WithCancel(context.Background())

	_, err := conn.AcceptStream(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "连接未启用多路复用")
}

// TestStream_SetPriority_ThreadSafe 测试优先级设置的线程安全性
func TestStream_SetPriority_ThreadSafe(t *testing.T) {
	muxerStream := &mockMuxerStream{}

	s := NewStream(muxerStream, 1, "/test/1.0", nil, types.PriorityNormal)

	var wg sync.WaitGroup
	iterations := 100

	// 并发设置优先级
	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go func(p types.Priority) {
			defer wg.Done()
			s.SetPriority(p)
		}(types.Priority(i % 4))

		go func() {
			defer wg.Done()
			_ = s.Priority()
		}()
	}

	wg.Wait()
	// 测试通过意味着没有数据竞争
}

// TestParsedAddress_Network_Defaults 已移除
// parsedAddress 类型已删除，Network() 逻辑已迁移到 types.Multiaddr

// TestEndpoint_DialAddr_RequiresDependencies 测试出站连接需要 transport 和安全层
func TestEndpoint_DialAddr_RequiresDependencies(t *testing.T) {
	identity := newMockIdentity()

	t.Run("无 transport", func(t *testing.T) {
		ep := NewEndpointWithConfig(
			identity,
			nil, // transport - 无传输层
			nil, // security
			nil, // muxerFactory
			nil, // discovery
			nil, // nat
			nil, // protocolRouter
			nil, // connManager
			nil, // connGater
			DefaultConfig(),
		)

		var remoteID types.NodeID
		for i := 0; i < 32; i++ {
			remoteID[i] = byte(i + 100)
		}

		addr := newSimpleAddr("/ip4/127.0.0.1/udp/8000/quic-v1")
		_, err := ep.ConnectWithAddrs(context.Background(), remoteID, []coreif.Address{addr})
		assert.Error(t, err)
		// 错误可能是 "传输层未配置" 或 "无可用传输" 取决于是否启用了 TransportRegistry
		assert.True(t, strings.Contains(err.Error(), "传输层未配置") ||
			strings.Contains(err.Error(), "无可用传输") ||
			strings.Contains(err.Error(), "all dial attempts failed"),
			"unexpected error: %v", err)
	})
}
