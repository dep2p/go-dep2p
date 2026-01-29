package protobook

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

func testProtocolID(s string) types.ProtocolID {
	return types.ProtocolID(s)
}

func TestNew(t *testing.T) {
	pb := New()
	require.NotNil(t, pb)
}

func TestProtoBook_SetProtocols(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")
	protocols := []types.ProtocolID{
		testProtocolID("/dep2p/sys/dht/1.0.0"),
		testProtocolID("/dep2p/relay/1.0.0/hop"),
	}

	err := pb.SetProtocols(peerID, protocols...)
	require.NoError(t, err)

	retrieved, err := pb.GetProtocols(peerID)
	require.NoError(t, err)
	assert.Len(t, retrieved, 2)
}

func TestProtoBook_AddProtocols(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")

	// 先添加一个协议
	pb.AddProtocols(peerID, testProtocolID("/dep2p/sys/dht/1.0.0"))

	// 再添加另一个协议
	pb.AddProtocols(peerID, testProtocolID("/dep2p/relay/1.0.0/hop"))

	protocols, _ := pb.GetProtocols(peerID)
	assert.Len(t, protocols, 2)
}

func TestProtoBook_SupportsProtocols(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")

	// 设置节点支持的协议
	pb.SetProtocols(peerID,
		testProtocolID("/dep2p/sys/dht/1.0.0"),
		testProtocolID("/dep2p/relay/1.0.0/hop"),
	)

	// 查询支持的协议
	supported, err := pb.SupportsProtocols(peerID,
		testProtocolID("/dep2p/sys/dht/1.0.0"),
		testProtocolID("/dep2p/bitswap/1.0.0"), // 不支持
	)
	require.NoError(t, err)
	assert.Len(t, supported, 1)
	assert.Equal(t, testProtocolID("/dep2p/sys/dht/1.0.0"), supported[0])
}

func TestProtoBook_RemoveProtocols(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")

	pb.SetProtocols(peerID,
		testProtocolID("/dep2p/sys/dht/1.0.0"),
		testProtocolID("/dep2p/relay/1.0.0/hop"),
	)

	pb.RemoveProtocols(peerID, testProtocolID("/dep2p/sys/dht/1.0.0"))

	protocols, _ := pb.GetProtocols(peerID)
	assert.Len(t, protocols, 1)
	assert.Equal(t, testProtocolID("/dep2p/relay/1.0.0/hop"), protocols[0])
}

func TestProtoBook_FirstSupportedProtocol(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")

	pb.SetProtocols(peerID,
		testProtocolID("/dep2p/sys/dht/1.0.0"),
		testProtocolID("/dep2p/relay/1.0.0/hop"),
	)

	first, err := pb.FirstSupportedProtocol(peerID,
		testProtocolID("/dep2p/bitswap/1.0.0"), // 不支持
		testProtocolID("/dep2p/sys/dht/1.0.0"),     // 支持
		testProtocolID("/dep2p/relay/1.0.0/hop"),   // 支持
	)
	require.NoError(t, err)
	assert.Equal(t, testProtocolID("/dep2p/sys/dht/1.0.0"), first)
}

func TestProtoBook_RemovePeer(t *testing.T) {
	pb := New()
	peerID := testPeerID("peer1")

	pb.SetProtocols(peerID, testProtocolID("/dep2p/sys/dht/1.0.0"))
	pb.RemovePeer(peerID)

	protocols, _ := pb.GetProtocols(peerID)
	assert.Empty(t, protocols)
}
