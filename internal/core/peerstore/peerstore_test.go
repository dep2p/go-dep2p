package peerstore

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPeerstore(t *testing.T) {
	ps := NewPeerstore()
	require.NotNil(t, ps)

	// 验证子簿已初始化
	assert.NotNil(t, ps.addrBook)
	assert.NotNil(t, ps.keyBook)
	assert.NotNil(t, ps.protoBook)
	assert.NotNil(t, ps.metadata)
}

func TestPeerstore_Peers(t *testing.T) {
	ps := NewPeerstore()

	// 初始应该没有节点
	peers := ps.Peers()
	assert.Empty(t, peers)

	// 添加一些地址
	peerID1 := testPeerID("peer1")
	peerID2 := testPeerID("peer2")

	ps.AddAddrs(peerID1, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, ConnectedAddrTTL)
	ps.AddPubKey(peerID2, testPubKey("key2"))

	// 现在应该有 2 个节点
	peers = ps.Peers()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, peerID1)
	assert.Contains(t, peers, peerID2)
}

func TestPeerstore_PeerInfo(t *testing.T) {
	ps := NewPeerstore()
	peerID := testPeerID("peer1")

	addrs := []types.Multiaddr{
		testMultiaddr("/ip4/127.0.0.1/tcp/4001"),
		testMultiaddr("/ip4/192.168.1.1/tcp/4001"),
	}
	ps.AddAddrs(peerID, addrs, ConnectedAddrTTL)

	info := ps.PeerInfo(peerID)
	assert.Equal(t, peerID, info.ID)
	assert.Len(t, info.Addrs, 2)
}

func TestPeerstore_Close(t *testing.T) {
	ps := NewPeerstore()
	err := ps.Close()
	assert.NoError(t, err)
}

func TestPeerstore_RemovePeer(t *testing.T) {
	ps := NewPeerstore()
	peerID := testPeerID("peer1")

	// 添加数据
	ps.AddPubKey(peerID, testPubKey("key1"))
	ps.SetProtocols(peerID, testProtocolID("/dep2p/sys/dht/1.0.0"))
	ps.Put(peerID, "agent", "dep2p/v1.0.0")

	// 移除节点
	ps.RemovePeer(peerID)

	// 验证已移除
	_, err := ps.PubKey(peerID)
	assert.Error(t, err)

	protos, _ := ps.GetProtocols(peerID)
	assert.Empty(t, protos)

	val, _ := ps.Get(peerID, "agent")
	assert.Nil(t, val)
}
