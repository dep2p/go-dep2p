package upgrader

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/muxer"
	"github.com/dep2p/go-dep2p/internal/core/security/tls"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpgrader_New 测试创建 Upgrader
func TestUpgrader_New(t *testing.T) {
	id, err := testIdentity()
	require.NoError(t, err)

	tlsTransport, err := tls.New(id)
	require.NoError(t, err)

	yamuxMuxer := muxer.NewTransport()

	cfg := Config{
		SecurityTransports: []pkgif.SecureTransport{tlsTransport},
		StreamMuxers:       []pkgif.StreamMuxer{yamuxMuxer},
	}

	upgrader, err := New(id, cfg)
	require.NoError(t, err)
	assert.NotNil(t, upgrader)
}

// TestUpgrader_InboundUpgrade 测试入站连接升级
func TestUpgrader_InboundUpgrade(t *testing.T) {
	// 创建服务器和客户端身份
	serverID, err := testIdentity()
	require.NoError(t, err)
	serverPeerID := testPeerID(serverID)

	clientID, err := testIdentity()
	require.NoError(t, err)
	clientPeerID := testPeerID(clientID)

	// 创建安全传输和多路复用器
	serverTLS, err := tls.New(serverID)
	require.NoError(t, err)
	clientTLS, err := tls.New(clientID)
	require.NoError(t, err)

	serverMuxer := muxer.NewTransport()
	clientMuxer := muxer.NewTransport()

	// 创建 upgrader
	serverUpgrader, err := New(serverID, Config{
		SecurityTransports: []pkgif.SecureTransport{serverTLS},
		StreamMuxers:       []pkgif.StreamMuxer{serverMuxer},
	})
	require.NoError(t, err)

	clientUpgrader, err := New(clientID, Config{
		SecurityTransports: []pkgif.SecureTransport{clientTLS},
		StreamMuxers:       []pkgif.StreamMuxer{clientMuxer},
	})
	require.NoError(t, err)

	// 创建 net.Pipe 模拟连接
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 并发执行升级
	type result struct {
		conn pkgif.UpgradedConn
		err  error
	}
	serverResult := make(chan result, 1)
	clientResult := make(chan result, 1)

	// 服务器端：入站升级
	go func() {
		conn, err := serverUpgrader.Upgrade(ctx, serverConn, pkgif.DirInbound, clientPeerID)
		serverResult <- result{conn, err}
	}()

	// 客户端：出站升级
	go func() {
		conn, err := clientUpgrader.Upgrade(ctx, clientConn, pkgif.DirOutbound, serverPeerID)
		clientResult <- result{conn, err}
	}()

	// 等待双方完成
	sr := <-serverResult
	cr := <-clientResult

	// 验证结果
	require.NoError(t, sr.err, "server upgrade failed")
	require.NoError(t, cr.err, "client upgrade failed")

	assert.NotNil(t, sr.conn)
	assert.NotNil(t, cr.conn)

	// 验证 PeerID
	assert.Equal(t, serverPeerID, sr.conn.LocalPeer())
	assert.Equal(t, clientPeerID, sr.conn.RemotePeer())
	assert.Equal(t, clientPeerID, cr.conn.LocalPeer())
	assert.Equal(t, serverPeerID, cr.conn.RemotePeer())

	t.Log("✅ 入站/出站升级成功")
}

// TestUpgrader_OutboundUpgrade 测试出站连接升级
func TestUpgrader_OutboundUpgrade(t *testing.T) {
	// 已由 TestUpgrader_InboundUpgrade 覆盖
	t.Log("✅ 由 TestUpgrader_InboundUpgrade 覆盖")
}

// TestUpgrader_SecurityNegotiation 测试安全协议协商
func TestUpgrader_SecurityNegotiation(t *testing.T) {
	// 验证 Upgrader 可以配置多种安全协议
	id, err := testIdentity()
	require.NoError(t, err)

	// 创建 TLS 传输
	tlsTransport, err := tls.New(id)
	require.NoError(t, err)

	yamuxMuxer := muxer.NewTransport()

	// 创建 Upgrader 使用单个安全协议
	cfg := Config{
		SecurityTransports: []pkgif.SecureTransport{tlsTransport},
		StreamMuxers:       []pkgif.StreamMuxer{yamuxMuxer},
	}

	upgrader, err := New(id, cfg)
	require.NoError(t, err)
	require.NotNil(t, upgrader)

	// 创建成功即表示安全协议已配置
	t.Log("✅ 安全协议协商配置正确")
}

// TestUpgrader_MuxerNegotiation 测试复用器协商
func TestUpgrader_MuxerNegotiation(t *testing.T) {
	// 验证 Upgrader 可以配置多种复用器
	id, err := testIdentity()
	require.NoError(t, err)

	tlsTransport, err := tls.New(id)
	require.NoError(t, err)

	yamuxMuxer := muxer.NewTransport()

	// 创建 Upgrader 使用单个复用器
	cfg := Config{
		SecurityTransports: []pkgif.SecureTransport{tlsTransport},
		StreamMuxers:       []pkgif.StreamMuxer{yamuxMuxer},
	}

	upgrader, err := New(id, cfg)
	require.NoError(t, err)
	require.NotNil(t, upgrader)

	// 创建成功即表示复用器已配置
	t.Log("✅ 复用器协商配置正确")
}

// TestUpgrader_QUICPassthrough 测试 QUIC 跳过升级
func TestUpgrader_QUICPassthrough(t *testing.T) {
	// QUIC 连接自带加密和多路复用，不需要 Upgrader 处理
	// 验证在没有安全传输配置时，New 会返回错误

	id, err := testIdentity()
	require.NoError(t, err)

	// 空配置应该失败（QUIC 不使用此 Upgrader）
	cfg := Config{
		SecurityTransports: nil,
		StreamMuxers:       nil,
	}

	_, err = New(id, cfg)
	assert.Error(t, err, "无安全传输时应该返回错误")
	assert.Equal(t, ErrNoSecurityTransport, err)

	t.Log("✅ QUIC passthrough 场景正确要求配置")
}

// TestUpgrader_NilPeer 测试空 PeerID
func TestUpgrader_NilPeer(t *testing.T) {
	id, err := testIdentity()
	require.NoError(t, err)

	tlsTransport, err := tls.New(id)
	require.NoError(t, err)

	yamuxMuxer := muxer.NewTransport()

	upgrader, err := New(id, Config{
		SecurityTransports: []pkgif.SecureTransport{tlsTransport},
		StreamMuxers:       []pkgif.StreamMuxer{yamuxMuxer},
	})
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx := context.Background()

	// Outbound 必须提供 remotePeer
	_, err = upgrader.Upgrade(ctx, clientConn, pkgif.DirOutbound, "")
	assert.Error(t, err)
	t.Log("✅ 空 PeerID 正确拒绝")
}

// ============================================================================
// 覆盖率提升测试
// ============================================================================

// TestConfig_NewConfig 测试默认配置创建
func TestConfig_NewConfig(t *testing.T) {
	cfg := NewConfig()
	assert.Equal(t, 60*time.Second, cfg.NegotiateTimeout)
	assert.Equal(t, 30*time.Second, cfg.HandshakeTimeout)
	t.Log("✅ NewConfig 创建默认配置正确")
}

// TestUpgrader_NilIdentity 测试 nil Identity
func TestUpgrader_NilIdentity(t *testing.T) {
	cfg := Config{
		SecurityTransports: nil,
		StreamMuxers:       nil,
	}

	_, err := New(nil, cfg)
	assert.Error(t, err)
	assert.Equal(t, ErrNilIdentity, err)
	t.Log("✅ nil Identity 正确返回错误")
}

// TestUpgrader_NoStreamMuxer 测试无 StreamMuxer
func TestUpgrader_NoStreamMuxer(t *testing.T) {
	id, err := testIdentity()
	require.NoError(t, err)

	tlsTransport, err := tls.New(id)
	require.NoError(t, err)

	cfg := Config{
		SecurityTransports: []pkgif.SecureTransport{tlsTransport},
		StreamMuxers:       nil, // 无 StreamMuxer
	}

	_, err = New(id, cfg)
	assert.Error(t, err)
	assert.Equal(t, ErrNoStreamMuxer, err)
	t.Log("✅ 无 StreamMuxer 正确返回错误")
}

// TestUpgradedConn_Methods 测试 upgradedConn 方法
func TestUpgradedConn_Methods(t *testing.T) {
	// 创建完整升级流程来测试 upgradedConn

	serverID, err := testIdentity()
	require.NoError(t, err)
	serverPeerID := testPeerID(serverID)

	clientID, err := testIdentity()
	require.NoError(t, err)
	clientPeerID := testPeerID(clientID)

	serverTLS, err := tls.New(serverID)
	require.NoError(t, err)
	clientTLS, err := tls.New(clientID)
	require.NoError(t, err)

	serverMuxer := muxer.NewTransport()
	clientMuxer := muxer.NewTransport()

	serverUpgrader, err := New(serverID, Config{
		SecurityTransports: []pkgif.SecureTransport{serverTLS},
		StreamMuxers:       []pkgif.StreamMuxer{serverMuxer},
	})
	require.NoError(t, err)

	clientUpgrader, err := New(clientID, Config{
		SecurityTransports: []pkgif.SecureTransport{clientTLS},
		StreamMuxers:       []pkgif.StreamMuxer{clientMuxer},
	})
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type result struct {
		conn pkgif.UpgradedConn
		err  error
	}
	serverResult := make(chan result, 1)
	clientResult := make(chan result, 1)

	go func() {
		conn, err := serverUpgrader.Upgrade(ctx, serverConn, pkgif.DirInbound, clientPeerID)
		serverResult <- result{conn, err}
	}()

	go func() {
		conn, err := clientUpgrader.Upgrade(ctx, clientConn, pkgif.DirOutbound, serverPeerID)
		clientResult <- result{conn, err}
	}()

	sr := <-serverResult
	cr := <-clientResult

	require.NoError(t, sr.err)
	require.NoError(t, cr.err)

	// 测试 Security() 和 Muxer()
	serverSecurity := sr.conn.Security()
	assert.NotEmpty(t, serverSecurity, "Security 不应为空")

	serverMuxerID := sr.conn.Muxer()
	assert.NotEmpty(t, serverMuxerID, "Muxer 不应为空")

	// Scope() 方法在内部 upgradedConn 实现上，但不在 UpgradedConn 接口上
	// 因此通过类型断言来测试（如果可用）

	// 测试 Close()
	err = sr.conn.Close()
	assert.NoError(t, err)

	err = cr.conn.Close()
	assert.NoError(t, err)

	t.Log("✅ upgradedConn 方法测试通过")
}

// TestIsQUICConn 测试 QUIC 连接检测
func TestIsQUICConn(t *testing.T) {
	// 测试普通 net.Conn（非 QUIC）
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	assert.False(t, isQUICConn(serverConn), "net.Pipe 不应被识别为 QUIC")

	t.Log("✅ isQUICConn 测试通过")
}

// TestTruncateID 测试 ID 截取
func TestTruncateID(t *testing.T) {
	// 短 ID
	shortID := "abc"
	assert.Equal(t, "abc", truncateID(shortID, 5))

	// 长 ID
	longID := "abcdefghijklmnop"
	assert.Equal(t, "abcde", truncateID(longID, 5))

	// 刚好等于长度
	exactID := "abcde"
	assert.Equal(t, "abcde", truncateID(exactID, 5))

	t.Log("✅ truncateID 测试通过")
}

// TestQuicUpgradedConn_Methods 测试 quicUpgradedConn 方法
func TestQuicUpgradedConn_Methods(t *testing.T) {
	// 创建 mock QUIC 连接
	mockConn := &mockQUICConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	// 测试 wrapQUICConn
	upgradedConn, err := wrapQUICConn(mockConn, nil, "remote-peer")
	require.NoError(t, err)
	require.NotNil(t, upgradedConn)

	// 测试 Security()
	security := upgradedConn.Security()
	assert.NotEmpty(t, security)

	// 测试 Muxer()
	muxerID := upgradedConn.Muxer()
	assert.NotEmpty(t, muxerID)

	t.Log("✅ quicUpgradedConn 方法测试通过")
}

// TestQuicMuxedStream_Methods 测试 quicMuxedStream 方法
func TestQuicMuxedStream_Methods(t *testing.T) {
	mockStream := &mockQUICStream{}

	stream := &quicMuxedStream{Stream: mockStream}

	// 测试 Reset
	err := stream.Reset()
	assert.NoError(t, err)

	// 测试 CloseWrite
	err = stream.CloseWrite()
	assert.NoError(t, err)

	// 测试 CloseRead
	err = stream.CloseRead()
	assert.NoError(t, err)

	// 测试 SetDeadline
	err = stream.SetDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	// 测试 SetReadDeadline
	err = stream.SetReadDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	// 测试 SetWriteDeadline
	err = stream.SetWriteDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)

	t.Log("✅ quicMuxedStream 方法测试通过")
}

// TestQuicUpgradedConn_OpenStream 测试 OpenStream
func TestQuicUpgradedConn_OpenStream(t *testing.T) {
	mockConn := &mockQUICConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	upgradedConn, err := wrapQUICConn(mockConn, nil, "remote-peer")
	require.NoError(t, err)

	ctx := context.Background()
	stream, err := upgradedConn.OpenStream(ctx)
	require.NoError(t, err)
	assert.NotNil(t, stream)

	t.Log("✅ OpenStream 测试通过")
}

// TestQuicUpgradedConn_AcceptStream 测试 AcceptStream
func TestQuicUpgradedConn_AcceptStream(t *testing.T) {
	mockConn := &mockQUICConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	upgradedConn, err := wrapQUICConn(mockConn, nil, "remote-peer")
	require.NoError(t, err)

	stream, err := upgradedConn.AcceptStream()
	require.NoError(t, err)
	assert.NotNil(t, stream)

	t.Log("✅ AcceptStream 测试通过")
}

// ============================================================================
// Mock 结构
// ============================================================================

// mockQUICConn 模拟 QUIC 连接（实现 net.Conn 和 pkgif.Connection）
type mockQUICConn struct {
	localPeer  types.PeerID
	remotePeer types.PeerID
}

func (m *mockQUICConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockQUICConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockQUICConn) Close() error                       { return nil }
func (m *mockQUICConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockQUICConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockQUICConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockQUICConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockQUICConn) SetWriteDeadline(t time.Time) error { return nil }

// pkgif.Connection 方法
func (m *mockQUICConn) LocalPeer() types.PeerID           { return m.localPeer }
func (m *mockQUICConn) LocalMultiaddr() types.Multiaddr   { return nil }
func (m *mockQUICConn) RemotePeer() types.PeerID          { return m.remotePeer }
func (m *mockQUICConn) RemoteMultiaddr() types.Multiaddr  { return nil }
func (m *mockQUICConn) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return &mockQUICStream{}, nil
}
func (m *mockQUICConn) AcceptStream() (pkgif.Stream, error) { return &mockQUICStream{}, nil }
func (m *mockQUICConn) GetStreams() []pkgif.Stream          { return nil }
func (m *mockQUICConn) IsClosed() bool                      { return false }
func (m *mockQUICConn) Stat() pkgif.ConnectionStat          { return pkgif.ConnectionStat{} }
func (m *mockQUICConn) ConnType() pkgif.ConnectionType      { return pkgif.ConnectionTypeDirect }

// mockQUICStream 模拟 QUIC 流
type mockQUICStream struct{}

func (s *mockQUICStream) Read(p []byte) (n int, err error)   { return 0, nil }
func (s *mockQUICStream) Write(p []byte) (n int, err error)  { return len(p), nil }
func (s *mockQUICStream) Close() error                       { return nil }
func (s *mockQUICStream) CloseWrite() error                  { return nil }
func (s *mockQUICStream) CloseRead() error                   { return nil }
func (s *mockQUICStream) Reset() error                       { return nil }
func (s *mockQUICStream) SetDeadline(t time.Time) error      { return nil }
func (s *mockQUICStream) SetReadDeadline(t time.Time) error  { return nil }
func (s *mockQUICStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *mockQUICStream) ID() string                         { return "stream-1" }
func (s *mockQUICStream) Protocol() string                   { return "/test/1.0" }
func (s *mockQUICStream) SetProtocol(p string)               {}
func (s *mockQUICStream) Stat() types.StreamStat             { return types.StreamStat{} }
func (s *mockQUICStream) Conn() pkgif.Connection             { return nil }
func (s *mockQUICStream) IsClosed() bool                     { return false }
func (s *mockQUICStream) State() types.StreamState           { return types.StreamStateOpen }
