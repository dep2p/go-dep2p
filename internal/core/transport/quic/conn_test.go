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

// TestConnection_NewStream 测试创建流
func TestConnection_NewStream(t *testing.T) {
	// 需要建立真实的 QUIC 连接才能测试 NewStream
	// 使用完整的端到端测试
	id1, err := identity.Generate()
	require.NoError(t, err)
	id2, err := identity.Generate()
	require.NoError(t, err)

	peer1 := types.PeerID(id1.PeerID())
	peer2 := types.PeerID(id2.PeerID())

	// 创建传输（使用完整身份配置）
	transport1 := New(peer1, id1)
	transport2 := New(peer2, id2)
	defer transport1.Close()
	defer transport2.Close()

	// Peer2 监听
	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport2.Listen(laddr)
	require.NoError(t, err)
	defer listener.Close()

	actualAddr := listener.Addr()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 接受连接的 goroutine
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

	// Peer1 拨号
	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	// 等待接受连接
	select {
	case err := <-acceptCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待连接超时")
	}
	defer acceptedConn.Close()

	// 测试创建流
	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Close()

	t.Log("✅ NewStream 成功")
}

// TestConnection_NewStream_Closed 测试在关闭的连接上创建流
func TestConnection_NewStream_Closed(t *testing.T) {
	// 需要建立真实的 QUIC 连接
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 接受连接
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// 拨号
	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)

	// 关闭连接
	dialedConn.Close()

	// 尝试在关闭的连接上创建流
	_, err = dialedConn.NewStream(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrConnectionClosed, err)

	t.Log("✅ 关闭连接后 NewStream 返回错误")
}

// TestConnection_AcceptStream 测试接受流
func TestConnection_AcceptStream(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 接受连接并等待流
	streamCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			streamCh <- err
			return
		}
		defer conn.Close()

		// 接受流
		stream, err := conn.AcceptStream()
		if err != nil {
			streamCh <- err
			return
		}
		defer stream.Close()
		streamCh <- nil
	}()

	// 拨号并创建流
	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	// 创建流
	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	// 写入数据以触发流创建完成
	_, err = stream.Write([]byte("test"))
	require.NoError(t, err)

	// 等待接受方完成
	select {
	case err := <-streamCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待流超时")
	}

	t.Log("✅ AcceptStream 成功")
}

// TestConnection_Close 测试关闭连接
func TestConnection_Close(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)

	conn := dialedConn.(*Connection)

	// 验证未关闭
	assert.False(t, conn.IsClosed())

	// 关闭
	err = conn.Close()
	assert.NoError(t, err)

	// 验证已关闭
	assert.True(t, conn.IsClosed())

	// 再次关闭应该安全
	err = conn.Close()
	assert.NoError(t, err)

	t.Log("✅ Close 正确关闭连接")
}

// TestConnection_Stat 测试连接统计
func TestConnection_Stat(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			// 保持连接一段时间
			time.Sleep(time.Second)
		}
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	conn := dialedConn.(*Connection)

	stat := conn.Stat()
	assert.Greater(t, stat.Opened, int64(0))
	assert.Equal(t, 0, stat.NumStreams)
	assert.False(t, stat.Transient)

	t.Log("✅ Stat 返回正确的统计")
}

// TestConnection_LocalRemotePeer 测试本地和远程 Peer
func TestConnection_LocalRemotePeer(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var acceptedConn *Connection
	acceptCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptCh <- err
			return
		}
		acceptedConn = conn.(*Connection)
		acceptCh <- nil
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	select {
	case err := <-acceptCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("等待连接超时")
	}
	defer acceptedConn.Close()

	// 验证拨号端
	assert.Equal(t, peer1, dialedConn.LocalPeer())
	assert.Equal(t, peer2, dialedConn.RemotePeer())

	// 验证接受端
	assert.Equal(t, peer2, acceptedConn.LocalPeer())
	assert.Equal(t, peer1, acceptedConn.RemotePeer())

	t.Log("✅ LocalPeer/RemotePeer 正确")
}

// TestConnection_GetStreams 测试获取流列表
func TestConnection_GetStreams(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			// 接受流
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	conn := dialedConn.(*Connection)

	// 初始应该没有流
	streams := conn.GetStreams()
	assert.Len(t, streams, 0)

	// 创建流
	stream, err := conn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	// 应该有一个流
	streams = conn.GetStreams()
	assert.Len(t, streams, 1)

	t.Log("✅ GetStreams 正确返回流列表")
}

// ============================================================================
//                       LocalMultiaddr/RemoteMultiaddr 测试
// ============================================================================

// TestConnection_LocalMultiaddr 测试本地多地址
func TestConnection_LocalMultiaddr(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(time.Second)
		}
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	conn := dialedConn.(*Connection)

	// 测试 LocalMultiaddr
	localAddr := conn.LocalMultiaddr()
	assert.NotNil(t, localAddr, "LocalMultiaddr 不应为 nil")
	assert.NotEmpty(t, localAddr.String(), "LocalMultiaddr 字符串不应为空")
	t.Logf("✅ LocalMultiaddr: %s", localAddr.String())
}

// TestConnection_RemoteMultiaddr 测试远程多地址
func TestConnection_RemoteMultiaddr(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(time.Second)
		}
	}()

	dialedConn, err := transport1.Dial(ctx, actualAddr, peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	conn := dialedConn.(*Connection)

	// 测试 RemoteMultiaddr
	remoteAddr := conn.RemoteMultiaddr()
	assert.NotNil(t, remoteAddr, "RemoteMultiaddr 不应为 nil")
	assert.NotEmpty(t, remoteAddr.String(), "RemoteMultiaddr 字符串不应为空")
	t.Logf("✅ RemoteMultiaddr: %s", remoteAddr.String())
}
