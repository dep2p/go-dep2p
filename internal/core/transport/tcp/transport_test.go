package tcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestTCPTransport_Creation(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)
	require.NotNil(t, transport)

	assert.Equal(t, localPeer, transport.localPeer)
	assert.NotNil(t, transport.listeners)
	assert.False(t, transport.closed)

	t.Log("✅ TCP Transport 创建成功")
}

func TestTCPTransport_Listen(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)
	defer transport.Close()

	// 监听本地地址
	listenAddr, err := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	require.NoError(t, err)

	listener, err := transport.Listen(listenAddr)
	require.NoError(t, err)
	require.NotNil(t, listener)
	defer listener.Close()

	// 验证监听器被记录
	assert.Len(t, transport.listeners, 1)

	t.Log("✅ TCP Transport Listen 成功")
}

func TestTCPTransport_Listen_Closed(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)

	// 关闭传输
	transport.Close()

	// 尝试监听应该失败
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	_, err := transport.Listen(listenAddr)
	assert.Error(t, err)
	assert.Equal(t, ErrTransportClosed, err)

	t.Log("✅ 已关闭时 Listen 返回错误")
}

func TestTCPTransport_Dial(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	transport := New(localPeer, nil)
	defer transport.Close()

	// 首先启动一个监听器
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	listener, err := transport.Listen(listenAddr)
	require.NoError(t, err)
	defer listener.Close()

	// 获取实际监听地址
	actualAddr := listener.Multiaddr()
	require.NotNil(t, actualAddr)

	// 在 goroutine 中拨号
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 拨号连接
	conn, err := transport.Dial(ctx, actualAddr, remotePeer)
	require.NoError(t, err)
	require.NotNil(t, conn)
	defer conn.Close()

	t.Log("✅ TCP Transport Dial 成功")
}

func TestTCPTransport_Dial_Closed(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	remotePeer := types.PeerID("remote-peer")
	transport := New(localPeer, nil)

	// 关闭传输
	transport.Close()

	// 尝试拨号应该失败
	ctx := context.Background()
	dialAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	_, err := transport.Dial(ctx, dialAddr, remotePeer)
	assert.Error(t, err)
	assert.Equal(t, ErrTransportClosed, err)

	t.Log("✅ 已关闭时 Dial 返回错误")
}

func TestTCPTransport_CanDial(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)
	defer transport.Close()

	// TCP 地址应该可以拨号
	tcpAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	assert.True(t, transport.CanDial(tcpAddr))

	// QUIC 地址应该不能拨号
	quicAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/udp/4001/quic-v1")
	assert.False(t, transport.CanDial(quicAddr))

	t.Log("✅ CanDial 正确判断")
}

func TestTCPTransport_Protocols(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)
	defer transport.Close()

	protocols := transport.Protocols()
	assert.Len(t, protocols, 1)
	assert.Equal(t, types.ProtocolTCP, protocols[0])

	t.Log("✅ Protocols 返回正确")
}

func TestTCPTransport_Close(t *testing.T) {
	localPeer := types.PeerID("test-peer")
	transport := New(localPeer, nil)

	// 创建一个监听器
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	_, err := transport.Listen(listenAddr)
	require.NoError(t, err)

	// 关闭传输
	err = transport.Close()
	assert.NoError(t, err)
	assert.True(t, transport.closed)

	// 再次关闭应该安全
	err = transport.Close()
	assert.NoError(t, err)

	t.Log("✅ Close 正确关闭")
}

func TestParseMultiaddr(t *testing.T) {
	// IPv4 地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	tcpAddr, err := parseMultiaddr(addr)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.1", tcpAddr.IP.String())
	assert.Equal(t, 4001, tcpAddr.Port)

	t.Log("✅ parseMultiaddr 正确解析")
}

func TestParseMultiaddr_NoTCP(t *testing.T) {
	// 没有 TCP 端口的地址
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1")
	_, err := parseMultiaddr(addr)
	assert.Error(t, err)

	t.Log("✅ parseMultiaddr 正确处理无 TCP 地址")
}
