package endpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

type fakeNATService struct {
	mapped      map[int]int
	unmap       []int
	stunAddress endpointif.Address // 模拟 STUN 返回的外部地址
}

func (f *fakeNATService) GetExternalAddress() (endpointif.Address, error) {
	if f.stunAddress != nil {
		return f.stunAddress, nil
	}
	return nil, errors.New("not implemented")
}
func (f *fakeNATService) GetExternalAddressWithContext(ctx context.Context) (endpointif.Address, error) {
	if f.stunAddress != nil {
		return f.stunAddress, nil
	}
	return nil, errors.New("not implemented")
}
func (f *fakeNATService) GetExternalAddressFromPortMapperWithContext(ctx context.Context) (endpointif.Address, error) {
	return nil, errors.New("boom")
}
func (f *fakeNATService) NATType() types.NATType { return types.NATTypeUnknown }
func (f *fakeNATService) DetectNATType(ctx context.Context) (types.NATType, error) {
	return types.NATTypeUnknown, errors.New("not implemented")
}
func (f *fakeNATService) MapPort(protocol string, internalPort, externalPort int, duration time.Duration) error {
	if f.mapped == nil {
		f.mapped = make(map[int]int)
	}
	// 返回一个确定性的外部端口，便于断言 cleanup 行为
	f.mapped[internalPort] = internalPort + 1000
	return nil
}
func (f *fakeNATService) UnmapPort(protocol string, externalPort int) error {
	f.unmap = append(f.unmap, externalPort)
	return nil
}
func (f *fakeNATService) GetMappedPort(protocol string, internalPort int) (int, error) {
	if f.mapped == nil {
		return 0, errors.New("mapping not found")
	}
	return f.mapped[internalPort], nil
}
func (f *fakeNATService) Refresh(ctx context.Context) error { return nil }
func (f *fakeNATService) Close() error                     { return nil }

var _ natif.NATService = (*fakeNATService)(nil)

func TestEndpoint_discoverExternalAddresses_RollbackOnExternalIPFailure(t *testing.T) {
	e := &Endpoint{
		nat: &fakeNATService{},
		listenAddrs: []endpointif.Address{
			newSimpleAddr("/ip4/0.0.0.0/udp/4001/quic-v1"),
			newSimpleAddr("/ip4/0.0.0.0/udp/4002/quic-v1"),
		},
	}

	e.discoverExternalAddresses(context.Background())

	n := e.nat.(*fakeNATService)
	require.ElementsMatch(t, []int{5001, 5002}, n.unmap)
}

// mockReachabilityCoordinator 用于捕获候选地址上报
type mockReachabilityCoordinator struct {
	mu         sync.Mutex
	candidates []candidateReport
}

type candidateReport struct {
	addr     string
	source   string
	priority addressif.AddressPriority
}

func (m *mockReachabilityCoordinator) OnDirectAddressCandidate(addr endpointif.Address, source string, priority addressif.AddressPriority) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.candidates = append(m.candidates, candidateReport{
		addr:     addr.String(),
		source:   source,
		priority: priority,
	})
}

func (m *mockReachabilityCoordinator) OnDirectAddressVerified(addr endpointif.Address, source string, priority addressif.AddressPriority) {
}
func (m *mockReachabilityCoordinator) OnRelayAddressAcquired(relayAddr endpointif.Address)            {}
func (m *mockReachabilityCoordinator) OnRelayAddressLost(relayAddr endpointif.Address)                {}
func (m *mockReachabilityCoordinator) VerifiedDirectAddresses() []endpointif.Address                  { return nil }
func (m *mockReachabilityCoordinator) RelayAddresses() []endpointif.Address                           { return nil }
func (m *mockReachabilityCoordinator) ShareableAddresses(nodeID types.NodeID) []string                { return nil }
func (m *mockReachabilityCoordinator) WaitShareableAddrs(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockReachabilityCoordinator) BootstrapCandidates(nodeID types.NodeID) []reachabilityif.BootstrapCandidate {
	return nil
}
func (m *mockReachabilityCoordinator) OnInboundWitness(dialedAddr string, remotePeerID types.NodeID, remoteIP string) {
}
func (m *mockReachabilityCoordinator) OnOutboundConnected(conn endpointif.Connection, dialedAddr string) {
}
func (m *mockReachabilityCoordinator) AdvertisedAddrs() []endpointif.Address { return nil }
func (m *mockReachabilityCoordinator) SetOnAddressChanged(fn func([]endpointif.Address)) {}

var _ reachabilityif.Coordinator = (*mockReachabilityCoordinator)(nil)

func (m *mockReachabilityCoordinator) getCandidates() []candidateReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]candidateReport, len(m.candidates))
	copy(result, m.candidates)
	return result
}

// fakeSTUNAddress 模拟 STUN 返回的外部地址
type fakeSTUNAddress struct {
	ip   net.IP
	port int
}

func (f *fakeSTUNAddress) String() string {
	return net.JoinHostPort(f.ip.String(), "47615") // STUN 返回的随机端口
}

func (f *fakeSTUNAddress) Network() string {
	return "udp"
}

func (f *fakeSTUNAddress) Equal(other endpointif.Address) bool {
	if o, ok := other.(*fakeSTUNAddress); ok {
		return f.ip.Equal(o.ip) && f.port == o.port
	}
	return false
}

func (f *fakeSTUNAddress) Bytes() []byte {
	return []byte(f.String())
}

func (f *fakeSTUNAddress) IsLoopback() bool {
	return f.ip.IsLoopback()
}

func (f *fakeSTUNAddress) IsPrivate() bool {
	return f.ip.IsPrivate()
}

func (f *fakeSTUNAddress) IsPublic() bool {
	return !f.ip.IsLoopback() && !f.ip.IsPrivate() && !f.ip.IsUnspecified()
}

func (f *fakeSTUNAddress) Multiaddr() string {
	ipType := "ip4"
	if f.ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/udp/%d/quic-v1", ipType, f.ip.String(), f.port)
}

var _ endpointif.Address = (*fakeSTUNAddress)(nil)

func TestEndpoint_discoverExternalAddresses_STUNPublicIP(t *testing.T) {
	// 模拟 STUN 返回公网 IP（端口是 STUN 探测时的随机端口）
	stunAddr := &fakeSTUNAddress{ip: net.ParseIP("101.37.245.124"), port: 47615}

	mockCoordinator := &mockReachabilityCoordinator{}

	e := &Endpoint{
		nat: &fakeNATService{
			stunAddress: stunAddr,
		},
		listenAddrs: []endpointif.Address{
			newSimpleAddr("/ip4/0.0.0.0/udp/4003/quic-v1"),
		},
		reachabilityCoordinator: mockCoordinator,
	}

	e.discoverExternalAddresses(context.Background())

	// 验证候选地址上报
	candidates := mockCoordinator.getCandidates()

	// 应该有 stun-public-ip 候选
	var stunCandidate *candidateReport
	for i := range candidates {
		if candidates[i].source == "stun-public-ip" {
			stunCandidate = &candidates[i]
			break
		}
	}

	require.NotNil(t, stunCandidate, "应上报 stun-public-ip 候选地址")
	assert.Equal(t, "/ip4/101.37.245.124/udp/4003/quic-v1", stunCandidate.addr,
		"候选地址应使用 STUN IP + 监听端口")
	assert.Equal(t, addressif.PriorityUnverified, stunCandidate.priority)
}

func TestEndpoint_discoverExternalAddresses_STUNPrivateIP_Skipped(t *testing.T) {
	// 模拟 STUN 返回私网 IP（不应生成候选）
	stunAddr := &fakeSTUNAddress{ip: net.ParseIP("192.168.1.100"), port: 47615}

	mockCoordinator := &mockReachabilityCoordinator{}

	e := &Endpoint{
		nat: &fakeNATService{
			stunAddress: stunAddr,
		},
		listenAddrs: []endpointif.Address{
			newSimpleAddr("/ip4/0.0.0.0/udp/4003/quic-v1"),
		},
		reachabilityCoordinator: mockCoordinator,
	}

	e.discoverExternalAddresses(context.Background())

	// 验证没有 stun-public-ip 候选
	candidates := mockCoordinator.getCandidates()
	for _, c := range candidates {
		assert.NotEqual(t, "stun-public-ip", c.source,
			"私网 IP 不应生成 stun-public-ip 候选")
	}
}


