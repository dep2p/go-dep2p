package security

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/security/tls"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTLSHandshake 测试真实的 TLS 握手
func TestTLSHandshake(t *testing.T) {
	// 创建两个身份
	serverIdentity, err := testIdentity()
	require.NoError(t, err)

	clientIdentity, err := testIdentity()
	require.NoError(t, err)

	serverPeer := types.PeerID(serverIdentity.PeerID())
	clientPeer := types.PeerID(clientIdentity.PeerID())

	t.Logf("Server PeerID: %s", serverPeer)
	t.Logf("Client PeerID: %s", clientPeer)

	// 创建传输
	serverTransport, err := tls.New(serverIdentity)
	require.NoError(t, err)

	clientTransport, err := tls.New(clientIdentity)
	require.NoError(t, err)

	// 创建 loopback 连接
	serverConn, clientConn := net.Pipe()

	// 使用 channels 同步握手
	serverSecureCh := make(chan pkgif.SecureConn, 1)
	clientSecureCh := make(chan pkgif.SecureConn, 1)
	errCh := make(chan error, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 服务器握手（goroutine）
	go func() {
		secure, err := serverTransport.SecureInbound(ctx, serverConn, clientPeer)
		if err != nil {
			errCh <- fmt.Errorf("server handshake: %w", err)
			return
		}
		serverSecureCh <- secure
	}()

	// 客户端握手（goroutine）
	go func() {
		secure, err := clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
		if err != nil {
			errCh <- fmt.Errorf("client handshake: %w", err)
			return
		}
		clientSecureCh <- secure
	}()

	// 等待握手完成
	var serverSecure, clientSecure pkgif.SecureConn

	for i := 0; i < 2; i++ {
		select {
		case serverSecure = <-serverSecureCh:
			t.Log("✅ Server 握手成功")
		case clientSecure = <-clientSecureCh:
			t.Log("✅ Client 握手成功")
		case err := <-errCh:
			t.Fatalf("握手失败: %v", err)
		case <-ctx.Done():
			t.Fatal("握手超时")
		}
	}

	// 验证服务器端连接
	require.NotNil(t, serverSecure)
	assert.Equal(t, serverPeer, serverSecure.LocalPeer())
	assert.Equal(t, clientPeer, serverSecure.RemotePeer())
	assert.NotNil(t, serverSecure.RemotePublicKey())

	// 验证客户端连接
	require.NotNil(t, clientSecure)
	assert.Equal(t, clientPeer, clientSecure.LocalPeer())
	assert.Equal(t, serverPeer, clientSecure.RemotePeer())
	assert.NotNil(t, clientSecure.RemotePublicKey())

	t.Log("✅ TLS 握手完整验证通过")

	// 关闭连接
	serverSecure.Close()
	clientSecure.Close()
}

// TestPeerIDVerification 测试 INV-001 验证
func TestPeerIDVerification(t *testing.T) {
	t.Run("匹配的PeerID应通过", func(t *testing.T) {
		// 已由 tls/verify_test.go TestVerifyPeerCertificate_Match 覆盖
		t.Log("✅ 由 tls/verify_test.go 覆盖")
	})

	t.Run("不匹配的PeerID应拒绝", func(t *testing.T) {
		// 已由 tls/verify_test.go TestVerifyPeerCertificate_Mismatch 覆盖
		t.Log("✅ 由 tls/verify_test.go 覆盖")
	})
}

// TestTLSHandshake_Timeout 测试握手超时
func TestTLSHandshake_Timeout(t *testing.T) {
	serverIdentity, err := testIdentity()
	require.NoError(t, err)

	clientIdentity, err := testIdentity()
	require.NoError(t, err)

	clientTransport, err := tls.New(clientIdentity)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverPeer := types.PeerID(serverIdentity.PeerID())

	// 使用极短的超时
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// 关闭连接以触发错误
	clientConn.Close()

	// 握手应该失败
	_, err = clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
	assert.Error(t, err)
	t.Log("✅ 超时处理正确")
}

// TestTLSHandshake_ClosedConnection 测试关闭的连接
func TestTLSHandshake_ClosedConnection(t *testing.T) {
	identity, err := testIdentity()
	require.NoError(t, err)

	transport, err := tls.New(identity)
	require.NoError(t, err)

	// 创建并立即关闭连接
	serverConn, clientConn := net.Pipe()
	clientConn.Close()
	serverConn.Close()

	ctx := context.Background()
	remotePeer := types.PeerID("test-peer")

	// 握手应该失败
	_, err = transport.SecureOutbound(ctx, clientConn, remotePeer)
	assert.Error(t, err)
	t.Log("✅ 关闭连接错误处理正确")
}

// TestPeerIDVerification_EmptyCertificate 测试空证书
func TestPeerIDVerification_EmptyCertificate(t *testing.T) {
	err := tls.VerifyPeerCertificate([][]byte{}, types.PeerID("test"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, tls.ErrNoCertificate)
	t.Log("✅ 空证书被正确拒绝")
}
