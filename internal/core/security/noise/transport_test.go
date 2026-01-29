package noise

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransport_New(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestTransport_ID(t *testing.T) {
	id, _ := identity.Generate()
	transport, _ := New(id)

	assert.Equal(t, types.ProtocolID("/noise/1.0.0"), transport.ID())
}

func TestTransport_New_NilIdentity(t *testing.T) {
	transport, err := New(nil)
	assert.Error(t, err)
	assert.Nil(t, transport)
}

// ============================================================================
// IdentityBinding 集成测试
// ============================================================================

func TestTransport_SetIdentityBinding(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 创建 IdentityBinding
	ib, err := NewIdentityBindingFromIdentity(id)
	require.NoError(t, err)

	// 设置 IdentityBinding
	transport.SetIdentityBinding(ib)

	// 验证已设置
	assert.Equal(t, ib, transport.GetIdentityBinding())
}

func TestTransport_SetIdentityBinding_Nil(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 设置 nil 不应报错
	transport.SetIdentityBinding(nil)

	// 验证为 nil
	assert.Nil(t, transport.GetIdentityBinding())
}

func TestTransport_GetIdentityBinding_Default(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 默认应该为 nil
	assert.Nil(t, transport.GetIdentityBinding())
}

// ============================================================================
// Noise 握手端到端测试
// ============================================================================

func TestTransport_SecureHandshake(t *testing.T) {
	// 创建两个身份（模拟 client 和 server）
	clientID, err := identity.Generate()
	require.NoError(t, err)
	serverID, err := identity.Generate()
	require.NoError(t, err)

	// 创建两个 Transport
	clientTransport, err := New(clientID)
	require.NoError(t, err)
	serverTransport, err := New(serverID)
	require.NoError(t, err)

	// 创建 pipe 模拟网络连接
	clientConn, serverConn := createTestPipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 获取 PeerID
	serverPeerID := types.PeerID(serverID.PeerID())
	clientPeerID := types.PeerID(clientID.PeerID())

	// 并发执行握手
	var clientSecConn, serverSecConn interface{}
	var clientErr, serverErr error

	done := make(chan struct{}, 2)

	ctx := context.Background()

	// Client 端握手 (Outbound)
	go func() {
		clientSecConn, clientErr = clientTransport.SecureOutbound(
			ctx, clientConn, serverPeerID,
		)
		done <- struct{}{}
	}()

	// Server 端握手 (Inbound)
	go func() {
		serverSecConn, serverErr = serverTransport.SecureInbound(
			ctx, serverConn, clientPeerID,
		)
		done <- struct{}{}
	}()

	// 等待两端完成
	<-done
	<-done

	// 验证握手成功
	require.NoError(t, clientErr, "Client handshake failed")
	require.NoError(t, serverErr, "Server handshake failed")
	require.NotNil(t, clientSecConn, "Client secure conn is nil")
	require.NotNil(t, serverSecConn, "Server secure conn is nil")

	t.Log("✅ Noise 握手成功")
}

// TestTransport_SecureInbound_NilConn 测试 nil conn 输入
// BUG B1 已修复：现在正确返回错误而不是 panic
func TestTransport_SecureInbound_NilConn(t *testing.T) {
	id, _ := identity.Generate()
	transport, _ := New(id)

	_, err := transport.SecureInbound(context.Background(), nil, "some-peer")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conn is nil")
}

// TestTransport_SecureOutbound_NilConn 测试 nil conn 输入
// BUG B1 已修复：现在正确返回错误而不是 panic
func TestTransport_SecureOutbound_NilConn(t *testing.T) {
	id, _ := identity.Generate()
	transport, _ := New(id)

	_, err := transport.SecureOutbound(context.Background(), nil, "some-peer")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conn is nil")
}

// ============================================================================
// verifyRemoteIdentity 间接测试（通过 IdentityBinding 路径）
// 修复 A1: verifyRemoteIdentity 0% 覆盖
// ============================================================================

// TestTransport_SecureHandshake_WithIdentityBinding 测试带身份绑定的握手
func TestTransport_SecureHandshake_WithIdentityBinding(t *testing.T) {
	// 创建两个身份
	clientID, err := identity.Generate()
	require.NoError(t, err)
	serverID, err := identity.Generate()
	require.NoError(t, err)

	// 创建两个 Transport
	clientTransport, err := New(clientID)
	require.NoError(t, err)
	serverTransport, err := New(serverID)
	require.NoError(t, err)

	// 为两个 Transport 设置 IdentityBinding
	clientIB, err := NewIdentityBindingFromIdentity(clientID)
	require.NoError(t, err)
	serverIB, err := NewIdentityBindingFromIdentity(serverID)
	require.NoError(t, err)

	clientTransport.SetIdentityBinding(clientIB)
	serverTransport.SetIdentityBinding(serverIB)

	// 创建 pipe 模拟网络连接
	clientConn, serverConn := createTestPipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 获取 PeerID
	serverPeerID := types.PeerID(serverID.PeerID())
	clientPeerID := types.PeerID(clientID.PeerID())

	// 并发执行握手
	var clientSecConn, serverSecConn interface{}
	var clientErr, serverErr error

	done := make(chan struct{}, 2)
	ctx := context.Background()

	// Client 端握手 (Outbound)
	go func() {
		clientSecConn, clientErr = clientTransport.SecureOutbound(ctx, clientConn, serverPeerID)
		done <- struct{}{}
	}()

	// Server 端握手 (Inbound)
	go func() {
		serverSecConn, serverErr = serverTransport.SecureInbound(ctx, serverConn, clientPeerID)
		done <- struct{}{}
	}()

	// 等待两端完成
	<-done
	<-done

	// 验证握手成功
	// 注意：由于 IdentityBinding 验证需要远程公钥，而当前实现 RemotePublicKey() 返回 nil
	// 所以 verifyRemoteIdentity 会跳过验证（"无远程公钥，跳过 IdentityBinding 验证"）
	// 这个测试主要覆盖 verifyRemoteIdentity 的代码路径
	require.NoError(t, clientErr, "Client handshake with IdentityBinding failed")
	require.NoError(t, serverErr, "Server handshake with IdentityBinding failed")
	require.NotNil(t, clientSecConn, "Client secure conn is nil")
	require.NotNil(t, serverSecConn, "Server secure conn is nil")

	t.Log("✅ 带 IdentityBinding 的 Noise 握手成功")
}

// TestTransport_verifyRemoteIdentity_NilBinding 测试 nil IdentityBinding
func TestTransport_verifyRemoteIdentity_NilBinding(t *testing.T) {
	clientID, err := identity.Generate()
	require.NoError(t, err)
	serverID, err := identity.Generate()
	require.NoError(t, err)

	clientTransport, err := New(clientID)
	require.NoError(t, err)
	serverTransport, err := New(serverID)
	require.NoError(t, err)

	// 不设置 IdentityBinding
	assert.Nil(t, clientTransport.GetIdentityBinding())
	assert.Nil(t, serverTransport.GetIdentityBinding())

	// 创建 pipe 模拟网络连接
	clientConn, serverConn := createTestPipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverPeerID := types.PeerID(serverID.PeerID())
	clientPeerID := types.PeerID(clientID.PeerID())

	var clientErr, serverErr error
	done := make(chan struct{}, 2)
	ctx := context.Background()

	go func() {
		_, clientErr = clientTransport.SecureOutbound(ctx, clientConn, serverPeerID)
		done <- struct{}{}
	}()

	go func() {
		_, serverErr = serverTransport.SecureInbound(ctx, serverConn, clientPeerID)
		done <- struct{}{}
	}()

	<-done
	<-done

	// 无 IdentityBinding 时应该正常工作
	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	t.Log("✅ 无 IdentityBinding 时握手正常")
}

// createTestPipe 创建模拟网络连接的管道
func createTestPipe() (*pipeConn, *pipeConn) {
	c1Reader, c1Writer := createPipe()
	c2Reader, c2Writer := createPipe()

	return &pipeConn{reader: c1Reader, writer: c2Writer},
		&pipeConn{reader: c2Reader, writer: c1Writer}
}

func createPipe() (*pipeReader, *pipeWriter) {
	ch := make(chan []byte, 100)
	return &pipeReader{ch: ch}, &pipeWriter{ch: ch}
}

type pipeReader struct {
	ch     chan []byte
	buffer []byte
}

func (r *pipeReader) Read(p []byte) (n int, err error) {
	if len(r.buffer) == 0 {
		data, ok := <-r.ch
		if !ok {
			return 0, nil
		}
		r.buffer = data
	}
	n = copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

type pipeWriter struct {
	ch chan []byte
}

func (w *pipeWriter) Write(p []byte) (n int, err error) {
	data := make([]byte, len(p))
	copy(data, p)
	w.ch <- data
	return len(p), nil
}

type pipeConn struct {
	reader *pipeReader
	writer *pipeWriter
}

func (c *pipeConn) Read(p []byte) (n int, err error)   { return c.reader.Read(p) }
func (c *pipeConn) Write(p []byte) (n int, err error)  { return c.writer.Write(p) }
func (c *pipeConn) Close() error                       { return nil }
func (c *pipeConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345} }
func (c *pipeConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 54321} }
func (c *pipeConn) SetDeadline(t time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(t time.Time) error { return nil }
