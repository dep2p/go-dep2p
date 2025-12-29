package endpoint

import (
	"testing"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/require"
)

type testAddr struct{ s string }

func (a *testAddr) Network() string                    { return "test" }
func (a *testAddr) String() string                     { return a.s }
func (a *testAddr) Bytes() []byte                      { return []byte(a.s) }
func (a *testAddr) Equal(o coreif.Address) bool         { return o != nil && o.String() == a.s }
func (a *testAddr) IsPublic() bool                      { return true }
func (a *testAddr) IsPrivate() bool                     { return false }
func (a *testAddr) IsLoopback() bool                    { return false }
func (a *testAddr) Multiaddr() string                   { return a.s } // 测试地址通常已是 multiaddr 格式

type testCoordinator struct {
	addrs         []coreif.Address
	verifiedAddrs []coreif.Address
}

func (c *testCoordinator) AdvertisedAddrs() []coreif.Address         { return c.addrs }
func (c *testCoordinator) VerifiedDirectAddresses() []coreif.Address { return c.verifiedAddrs }
func (c *testCoordinator) BootstrapCandidates(nodeID types.NodeID) []reachabilityif.BootstrapCandidate {
	_ = nodeID
	return nil
}
func (c *testCoordinator) SetOnAddressChanged(func([]coreif.Address)) {}
func (c *testCoordinator) OnDirectAddressCandidate(coreif.Address, string, addressif.AddressPriority) {
}
func (c *testCoordinator) OnDirectAddressVerified(coreif.Address, string, addressif.AddressPriority) {
}
func (c *testCoordinator) OnInboundWitness(dialedAddr string, remotePeerID types.NodeID, remoteIP string) {
	_, _, _ = dialedAddr, remotePeerID, remoteIP
}
func (c *testCoordinator) OnOutboundConnected(conn coreif.Connection, dialedAddr string) {
	_, _ = conn, dialedAddr
}

func TestEndpoint_AdvertisedAddrsUsesCoordinatorWhenInjected(t *testing.T) {
	ep := &Endpoint{}
	coord := &testCoordinator{
		addrs: []coreif.Address{&testAddr{s: "/p2p/relay/p2p-circuit/p2p/self"}},
	}
	ep.SetReachabilityCoordinator(coord)

	addrs := ep.AdvertisedAddrs()
	require.Len(t, addrs, 1)
	require.Equal(t, coord.addrs[0].String(), addrs[0].String())
}


