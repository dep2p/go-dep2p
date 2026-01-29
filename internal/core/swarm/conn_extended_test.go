package swarm

import (
	"context"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     SwarmConn LocalMultiaddr/RemoteMultiaddr 测试
// ============================================================================

func TestSwarmConn_LocalMultiaddr(t *testing.T) {
	swarm, err := NewSwarm("local-peer")
	require.NoError(t, err)
	defer swarm.Close()

	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	mockConn := &mockConnForTest{
		localPeer:       "local-peer",
		remotePeer:      "remote-peer",
		localMultiaddr:  addr,
		remoteMultiaddr: addr,
	}

	sc := newSwarmConn(swarm, mockConn)

	// 验证 LocalMultiaddr
	localAddr := sc.LocalMultiaddr()
	assert.NotNil(t, localAddr)
	assert.Equal(t, addr.String(), localAddr.String())
}

func TestSwarmConn_RemoteMultiaddr(t *testing.T) {
	swarm, err := NewSwarm("local-peer")
	require.NoError(t, err)
	defer swarm.Close()

	localAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4002")
	mockConn := &mockConnForTest{
		localPeer:       "local-peer",
		remotePeer:      "remote-peer",
		localMultiaddr:  localAddr,
		remoteMultiaddr: remoteAddr,
	}

	sc := newSwarmConn(swarm, mockConn)

	// 验证 RemoteMultiaddr
	addr := sc.RemoteMultiaddr()
	assert.NotNil(t, addr)
	assert.Equal(t, remoteAddr.String(), addr.String())
}

func TestSwarmConn_LocalMultiaddr_Nil(t *testing.T) {
	swarm, err := NewSwarm("local-peer")
	require.NoError(t, err)
	defer swarm.Close()

	mockConn := &mockConnForTest{
		localPeer:       "local-peer",
		remotePeer:      "remote-peer",
		localMultiaddr:  nil,
		remoteMultiaddr: nil,
	}

	sc := newSwarmConn(swarm, mockConn)

	// 底层返回 nil 时也应该返回 nil
	assert.Nil(t, sc.LocalMultiaddr())
	assert.Nil(t, sc.RemoteMultiaddr())
}

// ============================================================================
//                     Mock Connection
// ============================================================================

// mockConnForTest 是 pkgif.Connection 的 mock 实现（用于扩展测试）
type mockConnForTest struct {
	localPeer       types.PeerID
	remotePeer      types.PeerID
	localMultiaddr  types.Multiaddr
	remoteMultiaddr types.Multiaddr
	closed          bool
}

func (m *mockConnForTest) LocalPeer() types.PeerID {
	return m.localPeer
}

func (m *mockConnForTest) RemotePeer() types.PeerID {
	return m.remotePeer
}

func (m *mockConnForTest) LocalMultiaddr() types.Multiaddr {
	return m.localMultiaddr
}

func (m *mockConnForTest) RemoteMultiaddr() types.Multiaddr {
	return m.remoteMultiaddr
}

func (m *mockConnForTest) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return nil, ErrSwarmClosed
}

func (m *mockConnForTest) AcceptStream() (pkgif.Stream, error) {
	return nil, ErrSwarmClosed
}

func (m *mockConnForTest) GetStreams() []pkgif.Stream {
	return nil
}

func (m *mockConnForTest) Stat() pkgif.ConnectionStat {
	return pkgif.ConnectionStat{}
}

func (m *mockConnForTest) Close() error {
	m.closed = true
	return nil
}

func (m *mockConnForTest) IsClosed() bool {
	return m.closed
}

func (m *mockConnForTest) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}
