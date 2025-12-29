package reachability

import (
	"context"
	"testing"
	"time"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/require"
)

type mockAddr struct {
	s       string
	public  bool
	private bool
}

func (a *mockAddr) Network() string                     { return "mock" }
func (a *mockAddr) String() string                      { return a.s }
func (a *mockAddr) Bytes() []byte                       { return []byte(a.s) }
func (a *mockAddr) Equal(other endpointif.Address) bool  { return other != nil && other.String() == a.s }
func (a *mockAddr) IsPublic() bool                       { return a.public }
func (a *mockAddr) IsPrivate() bool                      { return a.private }
func (a *mockAddr) IsLoopback() bool                     { return false }
func (a *mockAddr) Multiaddr() string                    { return a.s } // 测试地址通常已是 multiaddr 格式

type mockAutoRelay struct {
	relayif.AutoRelay
	addrs    []endpointif.Address
	onChange func([]endpointif.Address)
}

func (m *mockAutoRelay) RelayAddrs() []endpointif.Address { return m.addrs }
func (m *mockAutoRelay) SetOnAddrsChanged(cb func([]endpointif.Address)) {
	m.onChange = cb
}

func TestCoordinator_OrdersVerifiedBeforeRelay(t *testing.T) {
	ar := &mockAutoRelay{
		addrs: []endpointif.Address{&mockAddr{s: "/p2p/relay/p2p-circuit/p2p/self", public: true}},
	}

	c := NewCoordinator(nil, ar, nil)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	// 注入一个已验证直连地址
	direct := &mockAddr{s: "/ip4/203.0.113.1/udp/4001/quic-v1", public: true}
	c.OnDirectAddressVerified(direct, "test", addressif.PriorityVerifiedDirect)

	addrs := c.AdvertisedAddrs()
	require.GreaterOrEqual(t, len(addrs), 2)
	require.Equal(t, direct.String(), addrs[0].String())
	require.Equal(t, ar.addrs[0].String(), addrs[1].String())
}

func TestCoordinator_OnRelayReservedTriggersOnChange(t *testing.T) {
	c := NewCoordinator(nil, nil, nil)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	ch := make(chan []endpointif.Address, 1)
	c.SetOnAddressChanged(func(addrs []endpointif.Address) {
		ch <- addrs
	})

	relayAddr := &mockAddr{s: "/p2p/relay/p2p-circuit/p2p/self", public: true}
	c.OnRelayReserved([]endpointif.Address{relayAddr})

	select {
	case got := <-ch:
		require.NotEmpty(t, got)
		require.Equal(t, relayAddr.String(), got[0].String())
	case <-time.After(2 * time.Second):
		t.Fatalf("expected OnChange to be called")
	}
}

func TestCoordinator_CandidateNotAdvertisedUntilVerified(t *testing.T) {
	c := NewCoordinator(nil, nil, nil)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	candidate := &mockAddr{s: "/ip4/203.0.113.9/udp/4009/quic-v1", public: true}
	c.OnDirectAddressCandidate(candidate, "port-mapping", addressif.PriorityUnverified)

	// 未验证候选不应出现在 AdvertisedAddrs 中（保持 Relay/Listen 回退逻辑）
	addrs := c.AdvertisedAddrs()
	for _, a := range addrs {
		require.NotEqual(t, candidate.String(), a.String())
	}

	// 标记为已验证后应出现
	c.OnDirectAddressVerified(candidate, "dial-back", addressif.PriorityVerifiedDirect)
	addrs2 := c.AdvertisedAddrs()
	found := false
	for _, a := range addrs2 {
		if a.String() == candidate.String() {
			found = true
			break
		}
	}
	require.True(t, found)
}

func TestCoordinator_WitnessThresholdUpgradesCandidateToVerifiedDirect(t *testing.T) {
	c := NewCoordinator(nil, nil, nil)
	require.NoError(t, c.Start(context.Background()))
	defer c.Stop()

	// 上报一个直连候选
	candidate := &mockAddr{s: "/ip4/203.0.113.9/udp/4009/quic-v1", public: true}
	c.OnDirectAddressCandidate(candidate, "user-config", addressif.PriorityUnverified)

	// 初始：未验证
	require.Empty(t, c.VerifiedDirectAddresses())

	// 两个不同的见证者 + 不同的 remote IP 前缀（默认 IPv4 /24）
	var p1 types.NodeID
	p1[0] = 1
	var p2 types.NodeID
	p2[0] = 2

	c.OnInboundWitness(candidate.String(), p1, "203.0.113.10") // 203.0.113.0/24
	require.Empty(t, c.VerifiedDirectAddresses(), "单个 witness 不应升级为 VerifiedDirect")

	c.OnInboundWitness(candidate.String(), p2, "203.0.114.10") // 203.0.114.0/24
	verified := c.VerifiedDirectAddresses()
	require.NotEmpty(t, verified, "达到 witness 阈值后应升级为 VerifiedDirect")

	found := false
	for _, a := range verified {
		if a != nil && a.String() == candidate.String() {
			found = true
			break
		}
	}
	require.True(t, found, "升级后的 VerifiedDirectAddresses 应包含候选地址")

	// BootstrapCandidates 应包含该地址，并标记 Verified=true
	var self types.NodeID
	self[0] = 9
	cands := c.BootstrapCandidates(self)
	require.NotEmpty(t, cands)
	var hasVerified bool
	for _, bc := range cands {
		if bc.Kind == reachabilityif.CandidateKindDirect && bc.FullAddr == candidate.String()+"/p2p/"+self.String() {
			hasVerified = bc.Verified
			break
		}
	}
	require.True(t, hasVerified, "BootstrapCandidates 应将已升级地址标记为 Verified")
}


