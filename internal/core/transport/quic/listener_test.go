package quic

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListener_Accept 测试接受连接
func TestListener_Accept(t *testing.T) {
	id1, err := identity.Generate()
	require.NoError(t, err)
	id2, err := identity.Generate()
	require.NoError(t, err)

	peer1 := types.PeerID(id1.PeerID())
	peer2 := types.PeerID(id2.PeerID())

	transport1 := New(peer1, id1)
	transport2 := New(peer2, id2)
	defer transport1.Close()
	defer transport2.Close()

	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport2.Listen(laddr)
	require.NoError(t, err)
	defer listener.Close()

	actualAddr := listener.Addr()
	require.NotNil(t, actualAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 接受连接
	acceptCh := make(chan error, 1)
	var acceptedConn *Connection
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptCh <- err
			return
		}
		acceptedConn = conn.(*Connection)
		acceptCh <- nil
	}()

	// 拨号
	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	// 等待接受
	select {
	case err := <-acceptCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待连接超时")
	}
	defer acceptedConn.Close()

	// 验证接受的连接
	assert.Equal(t, peer2, acceptedConn.LocalPeer())
	assert.Equal(t, peer1, acceptedConn.RemotePeer())

	t.Log("✅ Listener Accept 成功")
}

// TestListener_Addr 测试获取监听地址
func TestListener_Addr(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	t.Run("IPv4地址", func(t *testing.T) {
		laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
		require.NoError(t, err)

		listener, err := transport.Listen(laddr)
		require.NoError(t, err)
		defer listener.Close()

		addr := listener.Addr()
		require.NotNil(t, addr)

		// 验证地址格式
		addrStr := addr.String()
		assert.Contains(t, addrStr, "/ip4/127.0.0.1/udp/")
		assert.Contains(t, addrStr, "/quic-v1")

		// 验证端口不为 0（因为 0 会被分配实际端口）
		port, err := addr.ValueForProtocol(types.ProtocolUDP)
		require.NoError(t, err)
		assert.NotEqual(t, "0", port)
	})

	t.Run("Multiaddr方法", func(t *testing.T) {
		laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
		require.NoError(t, err)

		listener, err := transport.Listen(laddr)
		require.NoError(t, err)
		defer listener.Close()

		// Multiaddr 应该和 Addr 返回相同结果
		addr := listener.Addr()
		multiaddr := listener.Multiaddr()
		assert.Equal(t, addr.String(), multiaddr.String())
	})

	t.Log("✅ Listener Addr 正确返回地址")
}

// TestListener_Close 测试关闭监听器
func TestListener_Close(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport.Listen(laddr)
	require.NoError(t, err)

	// 关闭监听器
	err = listener.Close()
	assert.NoError(t, err)

	// 关闭后尝试接受应该失败
	_, err = listener.Accept()
	assert.Error(t, err)

	t.Log("✅ Listener Close 正确关闭")
}

// TestListener_AcceptAfterClose 测试关闭后接受
func TestListener_AcceptAfterClose(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport.Listen(laddr)
	require.NoError(t, err)

	// 在 goroutine 中等待 Accept
	errCh := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		errCh <- err
	}()

	// 短暂等待确保 Accept 已经开始
	time.Sleep(100 * time.Millisecond)

	// 关闭监听器
	listener.Close()

	// Accept 应该返回错误
	select {
	case err := <-errCh:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept 应该在关闭后返回")
	}

	t.Log("✅ 关闭后 Accept 正确返回错误")
}
