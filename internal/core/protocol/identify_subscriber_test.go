// Package protocol 实现协议注册与路由
//
// identify_subscriber_test.go: 
package protocol

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// mockReachabilityCoordinator 测试用的 ReachabilityCoordinator 实现
type mockReachabilityCoordinator struct {
	// 记录调用
	CandidateCalls []candidateCall
}

type candidateCall struct {
	Addr     string
	Source   string
	Priority pkgif.AddressPriority
}

func newMockCoordinator() *mockReachabilityCoordinator {
	return &mockReachabilityCoordinator{
		CandidateCalls: make([]candidateCall, 0),
	}
}

func (m *mockReachabilityCoordinator) AdvertisedAddrs() []string                              { return nil }
func (m *mockReachabilityCoordinator) VerifiedDirectAddresses() []string                      { return nil }
func (m *mockReachabilityCoordinator) CandidateDirectAddresses() []string                     { return nil }
func (m *mockReachabilityCoordinator) RelayAddresses() []string                               { return nil }
func (m *mockReachabilityCoordinator) BootstrapCandidates(nodeID string) []pkgif.BootstrapCandidate { return nil }
func (m *mockReachabilityCoordinator) SetOnAddressChanged(callback func([]string))            {}
func (m *mockReachabilityCoordinator) OnDirectAddressCandidate(addr string, source string, priority pkgif.AddressPriority) {
	m.CandidateCalls = append(m.CandidateCalls, candidateCall{
		Addr:     addr,
		Source:   source,
		Priority: priority,
	})
}
func (m *mockReachabilityCoordinator) UpdateDirectCandidates(source string, candidates []pkgif.CandidateUpdate) {}
func (m *mockReachabilityCoordinator) OnDirectAddressVerified(addr string, source string, priority pkgif.AddressPriority) {}
func (m *mockReachabilityCoordinator) OnDirectAddressExpired(addr string)                     {}
func (m *mockReachabilityCoordinator) OnRelayReserved(addrs []string)                         {}
func (m *mockReachabilityCoordinator) OnInboundWitness(dialedAddr string, remotePeerID string, remoteIP string) {}
func (m *mockReachabilityCoordinator) HasRelayAddress() bool                                  { return false }
func (m *mockReachabilityCoordinator) HasVerifiedDirectAddress() bool                         { return false }
func (m *mockReachabilityCoordinator) Start(ctx context.Context) error                        { return nil }
func (m *mockReachabilityCoordinator) Stop() error                                            { return nil }

var _ pkgif.ReachabilityCoordinator = (*mockReachabilityCoordinator)(nil)

// TestNewIdentifySubscriber 测试创建 IdentifySubscriber
func TestNewIdentifySubscriber(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	coordinator := newMockCoordinator()

	// 测试带 coordinator
	sub := NewIdentifySubscriber(host, coordinator)
	require.NotNil(t, sub)
	assert.NotNil(t, sub.host)
	assert.NotNil(t, sub.coordinator)

	// 测试 nil coordinator（测试场景）
	sub2 := NewIdentifySubscriber(host, nil)
	require.NotNil(t, sub2)
	assert.Nil(t, sub2.coordinator)
}

// TestIdentifySubscriber_HasDirectConnection 测试直连检测
func TestIdentifySubscriber_HasDirectConnection(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	sub := NewIdentifySubscriber(host, nil)

	// 没有连接时返回 false
	assert.False(t, sub.hasDirectConnection("peer1"))

	// 添加直连连接
	directConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("peer1"))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection("peer1", directConn)

	// 有直连时返回 true
	assert.True(t, sub.hasDirectConnection("peer1"))

	// 添加 Relay 连接到另一个节点
	relayConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("peer2"))
	relayConn.ConnTypeValue = pkgif.ConnectionTypeRelay
	swarm.AddConnection("peer2", relayConn)

	// 只有 Relay 连接时返回 false
	assert.False(t, sub.hasDirectConnection("peer2"))
}

// TestIdentifySubscriber_AddObservedAddr_FilterRelayAddr 测试 Relay 地址过滤
func TestIdentifySubscriber_AddObservedAddr_FilterRelayAddr(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	coordinator := newMockCoordinator()
	sub := NewIdentifySubscriber(host, coordinator)

	// 添加直连连接
	directConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("observer1"))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection("observer1", directConn)

	// Relay 地址应该被过滤
	relayAddr := "/ip4/101.37.245.124/udp/4005/quic-v1/p2p/9gMvzMGsyDFGRSUtzH6DXqyRjs1TDabPakAghZLHnrF6/p2p-circuit/p2p/HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc"
	sub.addObservedAddr(relayAddr, "observer1")

	// 不应该调用 coordinator
	assert.Empty(t, coordinator.CandidateCalls)
}

// TestIdentifySubscriber_AddObservedAddr_FilterNonDirectObserver 测试非直连观测者过滤
func TestIdentifySubscriber_AddObservedAddr_FilterNonDirectObserver(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	coordinator := newMockCoordinator()
	sub := NewIdentifySubscriber(host, coordinator)

	// 添加 Relay 连接（非直连）
	relayConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("observer1"))
	relayConn.ConnTypeValue = pkgif.ConnectionTypeRelay
	swarm.AddConnection("observer1", relayConn)

	// 来自非直连观测者的地址应该被过滤
	directAddr := "/ip4/60.177.185.34/udp/54583/quic-v1"
	sub.addObservedAddr(directAddr, "observer1")

	// 不应该调用 coordinator
	assert.Empty(t, coordinator.CandidateCalls)
}

// TestIdentifySubscriber_AddObservedAddr_SyncToCoordinator 测试同步到 Coordinator
func TestIdentifySubscriber_AddObservedAddr_SyncToCoordinator(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	coordinator := newMockCoordinator()
	sub := NewIdentifySubscriber(host, coordinator)

	// 添加直连连接
	directConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc"))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection("HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc", directConn)

	// 直连地址应该被同步
	directAddr := "/ip4/60.177.185.34/udp/54583/quic-v1"
	sub.addObservedAddr(directAddr, "HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc")

	// 应该调用 coordinator
	require.Len(t, coordinator.CandidateCalls, 1)
	assert.Equal(t, directAddr, coordinator.CandidateCalls[0].Addr)
	assert.Contains(t, coordinator.CandidateCalls[0].Source, "observed:")
	assert.Equal(t, pkgif.PriorityUnverified, coordinator.CandidateCalls[0].Priority)
}

// TestIdentifySubscriber_AddObservedAddr_NilCoordinator 测试 nil coordinator
func TestIdentifySubscriber_AddObservedAddr_NilCoordinator(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	// 不设置 coordinator
	sub := NewIdentifySubscriber(host, nil)

	// 添加直连连接
	directConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("observer1"))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection("observer1", directConn)

	// 即使 coordinator 为 nil，也不应该 panic
	directAddr := "/ip4/60.177.185.34/udp/54583/quic-v1"
	assert.NotPanics(t, func() {
		sub.addObservedAddr(directAddr, "observer1")
	})
}

// TestIdentifySubscriber_AddObservedAddr_InvalidAddr 测试无效地址
func TestIdentifySubscriber_AddObservedAddr_InvalidAddr(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	swarm := mocks.NewMockSwarm("test-peer")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	coordinator := newMockCoordinator()
	sub := NewIdentifySubscriber(host, coordinator)

	// 添加直连连接
	directConn := mocks.NewMockConnection(types.PeerID("test-peer"), types.PeerID("observer1"))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection("observer1", directConn)

	// 无效地址应该被跳过（不会 panic）
	invalidAddr := "not-a-valid-multiaddr"
	assert.NotPanics(t, func() {
		sub.addObservedAddr(invalidAddr, "observer1")
	})

	// 不应该调用 coordinator
	assert.Empty(t, coordinator.CandidateCalls)
}

// TestIdentifySubscriber_StartStop 测试启动和停止
func TestIdentifySubscriber_StartStop(t *testing.T) {
	host := mocks.NewMockHost("test-peer")
	eventBus := mocks.NewMockEventBus()
	host.EventBusFunc = func() pkgif.EventBus { return eventBus }

	sub := NewIdentifySubscriber(host, nil)

	ctx := context.Background()
	err := sub.Start(ctx)
	assert.NoError(t, err)

	err = sub.Stop()
	assert.NoError(t, err)
}

// TestIdentifySubscriber_BUG29_EndToEnd 测试 
func TestIdentifySubscriber_BUG29_EndToEnd(t *testing.T) {
	// 模拟场景：
	// 1. 节点 A 通过直连观测到自己的公网地址
	// 2. 该地址应该被同步到 CandidateDirectAddresses
	// 3. 打洞时可以使用该地址

	host := mocks.NewMockHost("nodeA")
	swarm := mocks.NewMockSwarm("nodeA")
	host.NetworkFunc = func() pkgif.Swarm { return swarm }

	coordinator := newMockCoordinator()
	sub := NewIdentifySubscriber(host, coordinator)

	// 模拟通过 QUIC 直连收到 Identify
	// 远端 nodeB 看到我们的公网地址是 60.177.185.34:54583
	observerID := "nodeBPeerIDxxxxxxxxxxxxxxxxxxxxx"
	directConn := mocks.NewMockConnection(types.PeerID("nodeA"), types.PeerID(observerID))
	directConn.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection(observerID, directConn)

	// 收到 ObservedAddr
	observedAddr := "/ip4/60.177.185.34/udp/54583/quic-v1"
	sub.addObservedAddr(observedAddr, observerID)

	// 验证地址被同步到 coordinator
	require.Len(t, coordinator.CandidateCalls, 1)
	assert.Equal(t, observedAddr, coordinator.CandidateCalls[0].Addr)
	assert.Contains(t, coordinator.CandidateCalls[0].Source, "observed:nodeBPee")

	// 模拟另一个直连节点也观测到相同地址（增加置信度）
	observer2ID := "nodeCPeerIDyyyyyyyyyyyyyyyyyyy"
	directConn2 := mocks.NewMockConnection(types.PeerID("nodeA"), types.PeerID(observer2ID))
	directConn2.ConnTypeValue = pkgif.ConnectionTypeDirect
	swarm.AddConnection(observer2ID, directConn2)

	sub.addObservedAddr(observedAddr, observer2ID)

	// 验证第二次同步
	require.Len(t, coordinator.CandidateCalls, 2)
	assert.Equal(t, observedAddr, coordinator.CandidateCalls[1].Addr)
	assert.Contains(t, coordinator.CandidateCalls[1].Source, "observed:nodeCPee")
}
