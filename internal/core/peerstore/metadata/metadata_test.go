package metadata

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

func TestNew(t *testing.T) {
	ms := New()
	require.NotNil(t, ms)
}

func TestMetadataStore_PutGet(t *testing.T) {
	ms := New()
	peerID := testPeerID("peer1")

	// 存储元数据
	err := ms.Put(peerID, "agent", "dep2p/v1.0.0")
	require.NoError(t, err)

	// 查询元数据
	val, err := ms.Get(peerID, "agent")
	require.NoError(t, err)
	assert.Equal(t, "dep2p/v1.0.0", val)
}

func TestMetadataStore_Get_NotFound(t *testing.T) {
	ms := New()
	peerID := testPeerID("peer1")

	val, err := ms.Get(peerID, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestMetadataStore_MultipleTypes(t *testing.T) {
	ms := New()
	peerID := testPeerID("peer1")

	// 存储不同类型的值
	ms.Put(peerID, "agent", "dep2p/v1.0.0") // string
	ms.Put(peerID, "latency", 100)          // int
	ms.Put(peerID, "reliable", true)        // bool

	agent, _ := ms.Get(peerID, "agent")
	assert.Equal(t, "dep2p/v1.0.0", agent)

	latency, _ := ms.Get(peerID, "latency")
	assert.Equal(t, 100, latency)

	reliable, _ := ms.Get(peerID, "reliable")
	assert.Equal(t, true, reliable)
}

func TestMetadataStore_RemovePeer(t *testing.T) {
	ms := New()
	peerID := testPeerID("peer1")

	ms.Put(peerID, "agent", "dep2p/v1.0.0")
	ms.RemovePeer(peerID)

	val, _ := ms.Get(peerID, "agent")
	assert.Nil(t, val)
}
