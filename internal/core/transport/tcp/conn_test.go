package tcp

import (
	"context"
	"net"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                 Connection 测试 - 覆盖 0% 函数
// ============================================================================

// TestConnection_LocalPeer 测试 LocalPeer
func TestConnection_LocalPeer(t *testing.T) {
	localPeer := types.PeerID("local-peer-123")
	remotePeer := types.PeerID("remote-peer-456")

	// 创建 TCP 连接对
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	assert.Equal(t, localPeer, conn.LocalPeer())
	t.Log("✅ LocalPeer 测试通过")
}

// TestConnection_RemotePeer 测试 RemotePeer
func TestConnection_RemotePeer(t *testing.T) {
	localPeer := types.PeerID("local-peer-123")
	remotePeer := types.PeerID("remote-peer-456")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	assert.Equal(t, remotePeer, conn.RemotePeer())
	t.Log("✅ RemotePeer 测试通过")
}

// TestConnection_RemoteMultiaddr 测试 RemoteMultiaddr
func TestConnection_RemoteMultiaddr(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.100/tcp/8080")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	assert.Equal(t, remoteAddr, conn.RemoteMultiaddr())
	t.Log("✅ RemoteMultiaddr 测试通过")
}

// TestConnection_NewStream_NoMuxer 测试无 Muxer 时 NewStream 返回错误
func TestConnection_NewStream_NoMuxer(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	ctx := context.Background()
	_, err := conn.NewStream(ctx)

	assert.Error(t, err)
	assert.Equal(t, ErrNoMuxer, err)
	t.Log("✅ NewStream 无 Muxer 测试通过")
}

// TestConnection_AcceptStream_NoMuxer 测试无 Muxer 时 AcceptStream 返回错误
func TestConnection_AcceptStream_NoMuxer(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	_, err := conn.AcceptStream()

	assert.Error(t, err)
	assert.Equal(t, ErrNoMuxer, err)
	t.Log("✅ AcceptStream 无 Muxer 测试通过")
}

// TestConnection_GetStreams 测试获取流列表
func TestConnection_GetStreams(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	streams := conn.GetStreams()

	// 原始 TCP 连接不支持多路复用，返回空列表
	assert.Empty(t, streams)
	t.Log("✅ GetStreams 测试通过")
}

// TestConnection_Stat 测试连接统计
func TestConnection_Stat(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	stat := conn.Stat()

	assert.Equal(t, pkgif.DirOutbound, stat.Direction)
	assert.Greater(t, stat.Opened, int64(0))
	assert.Equal(t, 0, stat.NumStreams)
	assert.False(t, stat.Transient)

	t.Log("✅ Stat 测试通过")
}

// TestConnection_IsClosed 测试关闭状态检查
func TestConnection_IsClosed(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	// 初始未关闭
	assert.False(t, conn.IsClosed())

	// 关闭连接
	conn.Close()

	// 已关闭
	assert.True(t, conn.IsClosed())

	t.Log("✅ IsClosed 测试通过")
}

// TestConnection_RawConn 测试获取原始连接
func TestConnection_RawConn(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	rawConn := conn.RawConn()

	assert.NotNil(t, rawConn)
	assert.Equal(t, client, rawConn)

	t.Log("✅ RawConn 测试通过")
}

// TestConnection_Close_Idempotent 测试重复关闭
func TestConnection_Close_Idempotent(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")

	server, client := net.Pipe()
	defer server.Close()

	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(client, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	// 第一次关闭
	err := conn.Close()
	assert.NoError(t, err)

	// 第二次关闭应该安全
	err = conn.Close()
	assert.NoError(t, err)

	t.Log("✅ Close 幂等性测试通过")
}

// TestConnection_LocalMultiaddr_RealConn 测试使用真实连接的 LocalMultiaddr
func TestConnection_LocalMultiaddr_RealConn(t *testing.T) {
	// 创建真实的 TCP 连接
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// 在 goroutine 中接受连接
	acceptCh := make(chan net.Conn, 1)
	go func() {
		conn, _ := listener.Accept()
		acceptCh <- conn
	}()

	// 拨号连接
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	// 等待服务端接受
	serverConn := <-acceptCh
	defer serverConn.Close()

	// 创建 Connection
	localPeer := types.PeerID("local-peer")
	remotePeer := types.PeerID("remote-peer")
	remoteAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	conn := newConnection(clientConn, localPeer, remotePeer, remoteAddr, pkgif.DirOutbound)

	// 测试 LocalMultiaddr
	localAddr := conn.LocalMultiaddr()
	assert.NotNil(t, localAddr)
	t.Logf("LocalMultiaddr: %s", localAddr.String())

	t.Log("✅ LocalMultiaddr 真实连接测试通过")
}

// ============================================================================
//                 Listener Accept 测试
// ============================================================================

// TestListener_Accept 测试监听器接受连接
func TestListener_Accept(t *testing.T) {
	localPeer := types.PeerID("local-peer")
	transport := New(localPeer, nil)
	defer transport.Close()

	// 监听
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	listener, err := transport.Listen(listenAddr)
	require.NoError(t, err)
	defer listener.Close()

	// 在 goroutine 中接受连接
	acceptCh := make(chan pkgif.Connection, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		acceptCh <- conn
	}()

	// 拨号连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialedConn, err := transport.Dial(ctx, listener.Multiaddr(), "remote-peer")
	require.NoError(t, err)
	defer dialedConn.Close()

	// 等待接受
	select {
	case conn := <-acceptCh:
		assert.NotNil(t, conn)
		defer conn.Close()
		t.Logf("接受的连接 RemotePeer: %s", conn.RemotePeer())
	case err := <-acceptErrCh:
		t.Fatalf("Accept 错误: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept 超时")
	}

	t.Log("✅ Listener Accept 测试通过")
}

// ============================================================================
//                 upgradedConnection / tcpStream 测试
// ============================================================================

// mockMuxedStream 模拟 MuxedStream
type mockMuxedStream struct {
	closed   bool
	protocol string
}

func (m *mockMuxedStream) Read(p []byte) (n int, err error)  { return 0, nil }
func (m *mockMuxedStream) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockMuxedStream) Close() error                      { m.closed = true; return nil }
func (m *mockMuxedStream) CloseRead() error                  { return nil }
func (m *mockMuxedStream) CloseWrite() error                 { return nil }
func (m *mockMuxedStream) Reset() error                      { return nil }
func (m *mockMuxedStream) SetDeadline(t time.Time) error     { return nil }
func (m *mockMuxedStream) SetReadDeadline(t time.Time) error { return nil }
func (m *mockMuxedStream) SetWriteDeadline(t time.Time) error { return nil }

// mockUpgradedConn 模拟 UpgradedConn
type mockUpgradedConn struct {
	localPeer  types.PeerID
	remotePeer types.PeerID
	streams    []pkgif.MuxedStream
	closed     bool
}

func (m *mockUpgradedConn) LocalPeer() types.PeerID       { return m.localPeer }
func (m *mockUpgradedConn) RemotePeer() types.PeerID      { return m.remotePeer }
func (m *mockUpgradedConn) Close() error                  { m.closed = true; return nil }
func (m *mockUpgradedConn) IsClosed() bool                { return m.closed }
func (m *mockUpgradedConn) Security() types.ProtocolID    { return "/tls/1.0.0" }
func (m *mockUpgradedConn) Muxer() string                 { return "/yamux/1.0.0" }
func (m *mockUpgradedConn) OpenStream(ctx context.Context) (pkgif.MuxedStream, error) {
	stream := &mockMuxedStream{}
	m.streams = append(m.streams, stream)
	return stream, nil
}
func (m *mockUpgradedConn) AcceptStream() (pkgif.MuxedStream, error) {
	stream := &mockMuxedStream{}
	m.streams = append(m.streams, stream)
	return stream, nil
}

// TestWrapUpgradedConn 测试包装升级连接
func TestWrapUpgradedConn(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)

	assert.NotNil(t, wrapped)
	assert.Equal(t, types.PeerID("local"), wrapped.LocalPeer())
	assert.Equal(t, types.PeerID("remote"), wrapped.RemotePeer())

	t.Log("✅ wrapUpgradedConn 测试通过")
}

// TestUpgradedConnection_LocalMultiaddr 测试升级连接的 LocalMultiaddr
func TestUpgradedConnection_LocalMultiaddr(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	// LocalMultiaddr 返回 nil（升级连接没有保存本地地址）
	assert.Nil(t, uc.LocalMultiaddr())

	t.Log("✅ upgradedConnection LocalMultiaddr 测试通过")
}

// TestUpgradedConnection_RemoteMultiaddr 测试升级连接的 RemoteMultiaddr
func TestUpgradedConnection_RemoteMultiaddr(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	assert.Equal(t, remoteAddr, uc.RemoteMultiaddr())

	t.Log("✅ upgradedConnection RemoteMultiaddr 测试通过")
}

// TestUpgradedConnection_GetStreams 测试升级连接获取流
func TestUpgradedConnection_GetStreams(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	// 初始无流
	assert.Empty(t, uc.GetStreams())

	// 创建流
	ctx := context.Background()
	_, err := uc.NewStream(ctx)
	require.NoError(t, err)

	// 应该有一个流
	assert.Len(t, uc.GetStreams(), 1)

	t.Log("✅ upgradedConnection GetStreams 测试通过")
}

// TestUpgradedConnection_Stat 测试升级连接统计
func TestUpgradedConnection_Stat(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	stat := uc.Stat()
	assert.Equal(t, pkgif.DirOutbound, stat.Direction)
	assert.Greater(t, stat.Opened, int64(0))
	assert.Equal(t, 0, stat.NumStreams)

	t.Log("✅ upgradedConnection Stat 测试通过")
}

// TestUpgradedConnection_IsClosed 测试升级连接关闭状态
func TestUpgradedConnection_IsClosed(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	assert.False(t, uc.IsClosed())

	t.Log("✅ upgradedConnection IsClosed 测试通过")
}

// TestUpgradedConnection_NewStream 测试升级连接创建流
func TestUpgradedConnection_NewStream(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, err := uc.NewStream(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stream)

	t.Log("✅ upgradedConnection NewStream 测试通过")
}

// TestUpgradedConnection_AcceptStream 测试升级连接接受流
func TestUpgradedConnection_AcceptStream(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	stream, err := uc.AcceptStream()
	require.NoError(t, err)
	assert.NotNil(t, stream)

	t.Log("✅ upgradedConnection AcceptStream 测试通过")
}

// TestTcpStream_Conn 测试流的 Conn 方法
func TestTcpStream_Conn(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, _ := uc.NewStream(ctx)
	
	tcpS := stream.(*tcpStream)
	assert.Equal(t, wrapped, tcpS.Conn())

	t.Log("✅ tcpStream Conn 测试通过")
}

// TestTcpStream_Protocol 测试流的协议设置
func TestTcpStream_Protocol(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, _ := uc.NewStream(ctx)
	
	tcpS := stream.(*tcpStream)
	
	// 初始为空
	assert.Empty(t, tcpS.Protocol())

	// 设置协议
	tcpS.SetProtocol("/test/1.0.0")
	assert.Equal(t, "/test/1.0.0", tcpS.Protocol())

	t.Log("✅ tcpStream Protocol/SetProtocol 测试通过")
}

// TestTcpStream_Stat 测试流的统计
func TestTcpStream_Stat(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, _ := uc.NewStream(ctx)
	
	tcpS := stream.(*tcpStream)
	tcpS.SetProtocol("/test/1.0.0")
	
	stat := tcpS.Stat()
	assert.Equal(t, types.DirOutbound, stat.Direction)
	assert.Equal(t, types.ProtocolID("/test/1.0.0"), stat.Protocol)

	t.Log("✅ tcpStream Stat 测试通过")
}

// TestTcpStream_IsClosed 测试流的关闭状态
func TestTcpStream_IsClosed(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, _ := uc.NewStream(ctx)
	
	tcpS := stream.(*tcpStream)
	
	// 连接未关闭时流未关闭
	assert.False(t, tcpS.IsClosed())

	t.Log("✅ tcpStream IsClosed 测试通过")
}

// TestTcpStream_State 测试流的状态
func TestTcpStream_State(t *testing.T) {
	mockConn := &mockUpgradedConn{
		localPeer:  "local",
		remotePeer: "remote",
	}

	remoteAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	wrapped := wrapUpgradedConn(mockConn, "local", remoteAddr)
	
	uc := wrapped.(*upgradedConnection)
	
	ctx := context.Background()
	stream, _ := uc.NewStream(ctx)
	
	tcpS := stream.(*tcpStream)
	
	// 连接未关闭时状态为 Open
	assert.Equal(t, types.StreamStateOpen, tcpS.State())

	t.Log("✅ tcpStream State 测试通过")
}
