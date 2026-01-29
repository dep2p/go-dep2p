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

// ============================================================================
//                 Stream 测试 - 覆盖 0% 函数
// ============================================================================

// TestStream_CloseRead 测试关闭读取端
func TestStream_CloseRead(t *testing.T) {
	// 建立连接和流
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 接受连接
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 验证初始状态
	assert.Equal(t, types.StreamStateOpen, s.State())

	// 关闭读取端
	err = s.CloseRead()
	assert.NoError(t, err)

	// 状态应该变为 ReadClosed
	assert.Equal(t, types.StreamStateReadClosed, s.State())

	t.Log("✅ CloseRead 测试通过")
}

// TestStream_CloseWrite 测试关闭写入端
func TestStream_CloseWrite(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 关闭写入端
	err = s.CloseWrite()
	assert.NoError(t, err)

	// 状态应该变为 WriteClosed
	assert.Equal(t, types.StreamStateWriteClosed, s.State())

	t.Log("✅ CloseWrite 测试通过")
}

// TestStream_Reset 测试重置流
func TestStream_Reset(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)

	s := stream.(*Stream)

	// 重置流
	err = s.Reset()
	assert.NoError(t, err)

	// 状态应该变为 Reset
	assert.Equal(t, types.StreamStateReset, s.State())
	assert.True(t, s.IsClosed())

	t.Log("✅ Reset 测试通过")
}

// TestStream_ID 测试流 ID
func TestStream_ID(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 获取流 ID
	streamID := s.ID()
	assert.NotEmpty(t, streamID)
	t.Logf("流 ID: %s", streamID)

	t.Log("✅ ID 测试通过")
}

// TestStream_Protocol 测试协议设置和获取
func TestStream_Protocol(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 初始协议应该为空
	assert.Empty(t, s.Protocol())

	// 设置协议
	s.SetProtocol("/test/1.0.0")

	// 验证协议
	assert.Equal(t, "/test/1.0.0", s.Protocol())

	t.Log("✅ Protocol/SetProtocol 测试通过")
}

// TestStream_Conn 测试获取底层连接
func TestStream_Conn(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 获取底层连接
	conn := s.Conn()
	assert.NotNil(t, conn)
	assert.Equal(t, peer1, conn.LocalPeer())
	assert.Equal(t, peer2, conn.RemotePeer())

	t.Log("✅ Conn 测试通过")
}

// TestStream_Stat 测试流统计
func TestStream_Stat(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				// 读取数据
				buf := make([]byte, 100)
				stream.Read(buf)
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 写入数据
	testData := []byte("hello world")
	_, err = s.Write(testData)
	require.NoError(t, err)

	// 获取统计
	stat := s.Stat()
	assert.Equal(t, int64(len(testData)), stat.BytesWritten)
	assert.NotZero(t, stat.Opened)

	t.Log("✅ Stat 测试通过")
}

// TestStream_SetDeadline 测试设置截止时间
func TestStream_SetDeadline(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	s := stream.(*Stream)

	// 设置截止时间
	deadline := time.Now().Add(time.Second)
	err = s.SetDeadline(deadline)
	assert.NoError(t, err)

	err = s.SetReadDeadline(deadline)
	assert.NoError(t, err)

	err = s.SetWriteDeadline(deadline)
	assert.NoError(t, err)

	t.Log("✅ SetDeadline 系列测试通过")
}

// TestStream_StateTransitions 测试状态转换
func TestStream_StateTransitions(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			stream, _ := conn.AcceptStream()
			if stream != nil {
				stream.Close()
			}
		}
	}()

	dialedConn, err := transport1.Dial(ctx, listener.Addr(), peer2)
	require.NoError(t, err)
	defer dialedConn.Close()

	stream, err := dialedConn.NewStream(ctx)
	require.NoError(t, err)

	s := stream.(*Stream)

	// 初始状态
	assert.Equal(t, types.StreamStateOpen, s.State())

	// 先关闭读，再关闭写 -> Closed
	s.CloseRead()
	assert.Equal(t, types.StreamStateReadClosed, s.State())

	s.CloseWrite()
	assert.Equal(t, types.StreamStateClosed, s.State())

	t.Log("✅ 状态转换测试通过")
}
