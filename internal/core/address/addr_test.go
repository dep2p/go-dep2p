package address

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 创建有效的测试用 NodeID
func testNodeID(seed byte) types.NodeID {
	var id types.NodeID
	for i := 0; i < 32; i++ {
		id[i] = byte(i + int(seed))
	}
	return id
}

func TestNewAddr(t *testing.T) {
	ma := types.MustParseMultiaddr("/ip4/1.2.3.4/udp/4001/quic-v1")
	addr := NewAddr(ma)

	assert.NotNil(t, addr)
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", addr.String())
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid ipv4", "/ip4/1.2.3.4/udp/4001/quic-v1", false},
		{"valid ipv6", "/ip6/::1/udp/4001/quic-v1", false},
		{"empty", "", true},
		{"host:port format", "1.2.3.4:4001", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.input, addr.String())
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	assert.NotPanics(t, func() {
		addr := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
		assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", addr.String())
	})

	assert.Panics(t, func() {
		MustParse("invalid")
	})
}

func TestAddr_ImplementsNetaddrAddress(t *testing.T) {
	addr := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")

	// 验证实现 netaddr.Address 接口
	var _ netaddr.Address = addr

	// 测试接口方法
	assert.Equal(t, "quic-v1", addr.Network())
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", addr.String())
	assert.Equal(t, []byte("/ip4/1.2.3.4/udp/4001/quic-v1"), addr.Bytes())
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", addr.Multiaddr())
}

func TestAddr_Equal(t *testing.T) {
	addr1 := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	addr2 := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	addr3 := MustParse("/ip4/5.6.7.8/udp/4001/quic-v1")

	assert.True(t, addr1.Equal(addr2))
	assert.False(t, addr1.Equal(addr3))
	assert.False(t, addr1.Equal(nil))
}

func TestAddr_IsPublicPrivateLoopback(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		isPublic   bool
		isPrivate  bool
		isLoopback bool
	}{
		{"public", "/ip4/8.8.8.8/udp/4001/quic-v1", true, false, false},
		{"private", "/ip4/192.168.1.1/udp/4001/quic-v1", false, true, false},
		{"loopback", "/ip4/127.0.0.1/udp/4001/quic-v1", false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := MustParse(tt.addr)
			assert.Equal(t, tt.isPublic, addr.IsPublic(), "IsPublic")
			assert.Equal(t, tt.isPrivate, addr.IsPrivate(), "IsPrivate")
			assert.Equal(t, tt.isLoopback, addr.IsLoopback(), "IsLoopback")
		})
	}
}

func TestAddr_MA(t *testing.T) {
	ma := types.MustParseMultiaddr("/ip4/1.2.3.4/udp/4001/quic-v1")
	addr := NewAddr(ma)

	assert.Equal(t, ma, addr.MA())
}

func TestAddr_IsRelay(t *testing.T) {
	nodeID := testNodeID(1)

	nonRelay := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	assert.False(t, nonRelay.IsRelay())

	relayMA := types.Multiaddr("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeID.String() + "/p2p-circuit")
	relay := NewAddr(relayMA)
	assert.True(t, relay.IsRelay())
}

func TestAddr_PeerID(t *testing.T) {
	nodeID := testNodeID(1)
	nodeIDStr := nodeID.String()

	// 带 PeerID 的地址
	withPeer := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeIDStr)
	assert.Equal(t, nodeID, withPeer.PeerID())

	// 不带 PeerID 的地址
	withoutPeer := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	assert.True(t, withoutPeer.PeerID().IsEmpty())
}

func TestAddr_WithPeerID(t *testing.T) {
	nodeID := testNodeID(1)

	addr := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	withPeer := addr.WithPeerID(nodeID)

	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/"+nodeID.String(), withPeer.String())
	assert.Equal(t, nodeID, withPeer.PeerID())
}

func TestAddr_WithoutPeerID(t *testing.T) {
	nodeID := testNodeID(1)

	addr := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + nodeID.String())
	withoutPeer := addr.WithoutPeerID()

	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", withoutPeer.String())
}

func TestAddr_IsEmpty(t *testing.T) {
	var nilAddr *Addr
	assert.True(t, nilAddr.IsEmpty())

	emptyAddr := NewAddr(types.Multiaddr(""))
	assert.True(t, emptyAddr.IsEmpty())

	validAddr := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1")
	assert.False(t, validAddr.IsEmpty())
}

func TestAddr_RelayOperations(t *testing.T) {
	relayID := testNodeID(1)
	destID := testNodeID(2)

	relayBase := MustParse("/ip4/1.2.3.4/udp/4001/quic-v1/p2p/" + relayID.String())

	t.Run("BuildRelayAddr", func(t *testing.T) {
		relayAddr, err := relayBase.BuildRelayAddr(destID)
		require.NoError(t, err)
		assert.True(t, relayAddr.IsRelay())
		assert.Equal(t, relayID, relayAddr.RelayID())
		assert.Equal(t, destID, relayAddr.DestID())
	})

	t.Run("RelayBaseAddr", func(t *testing.T) {
		relayAddr, _ := relayBase.BuildRelayAddr(destID)
		base := relayAddr.RelayBaseAddr()
		assert.Equal(t, relayBase.String(), base.String())
	})

	t.Run("IsDialableRelayAddr", func(t *testing.T) {
		relayAddr, _ := relayBase.BuildRelayAddr(destID)
		assert.True(t, relayAddr.IsDialableRelayAddr())

		// 简写格式不可拨号
		shortRelay := NewAddr(types.Multiaddr("/p2p/" + relayID.String() + "/p2p-circuit/p2p/" + destID.String()))
		assert.False(t, shortRelay.IsDialableRelayAddr())
	})
}

func TestParseAddrs(t *testing.T) {
	inputs := []string{
		"/ip4/1.2.3.4/udp/4001/quic-v1",
		"/ip4/5.6.7.8/udp/4002/quic-v1",
	}

	addrs, err := ParseAddrs(inputs)
	require.NoError(t, err)
	assert.Len(t, addrs, 2)
	assert.Equal(t, inputs[0], addrs[0].String())
	assert.Equal(t, inputs[1], addrs[1].String())
}

func TestParseAddrs_Error(t *testing.T) {
	inputs := []string{
		"/ip4/1.2.3.4/udp/4001/quic-v1",
		"invalid", // 无效地址
	}

	_, err := ParseAddrs(inputs)
	assert.Error(t, err)
}

func TestParseMultiaddrs(t *testing.T) {
	mas := []types.Multiaddr{
		types.MustParseMultiaddr("/ip4/1.2.3.4/udp/4001/quic-v1"),
		types.MustParseMultiaddr("/ip4/5.6.7.8/udp/4002/quic-v1"),
	}

	addrs := ParseMultiaddrs(mas)
	assert.Len(t, addrs, 2)
	assert.Equal(t, mas[0], addrs[0].MA())
	assert.Equal(t, mas[1], addrs[1].MA())
}

func TestAddrsToStrings(t *testing.T) {
	addrs := []*Addr{
		MustParse("/ip4/1.2.3.4/udp/4001/quic-v1"),
		MustParse("/ip4/5.6.7.8/udp/4002/quic-v1"),
	}

	ss := AddrsToStrings(addrs)
	assert.Equal(t, []string{
		"/ip4/1.2.3.4/udp/4001/quic-v1",
		"/ip4/5.6.7.8/udp/4002/quic-v1",
	}, ss)
}

func TestAddrsToMultiaddrs(t *testing.T) {
	addrs := []*Addr{
		MustParse("/ip4/1.2.3.4/udp/4001/quic-v1"),
		MustParse("/ip4/5.6.7.8/udp/4002/quic-v1"),
	}

	mas := AddrsToMultiaddrs(addrs)
	assert.Len(t, mas, 2)
	assert.Equal(t, addrs[0].MA(), mas[0])
	assert.Equal(t, addrs[1].MA(), mas[1])
}

func TestAddrsToNetaddrs(t *testing.T) {
	addrs := []*Addr{
		MustParse("/ip4/1.2.3.4/udp/4001/quic-v1"),
		MustParse("/ip4/5.6.7.8/udp/4002/quic-v1"),
	}

	netaddrs := AddrsToNetaddrs(addrs)
	assert.Len(t, netaddrs, 2)

	// 验证类型转换
	for i, na := range netaddrs {
		assert.Equal(t, addrs[i].String(), na.String())
	}
}

