package tls

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransport_ID(t *testing.T) {
	id, _ := identity.Generate()
	transport, err := New(id)
	require.NoError(t, err)

	assert.Equal(t, types.ProtocolID("/tls/1.0.0"), transport.ID())
}

// TestTransport_SecureInbound 测试入站握手
func TestTransport_SecureInbound(t *testing.T) {
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)
	clientIdentity, err := identity.Generate()
	require.NoError(t, err)

	serverPeer := types.PeerID(serverIdentity.PeerID())
	clientPeer := types.PeerID(clientIdentity.PeerID())

	serverTransport, err := New(serverIdentity)
	require.NoError(t, err)
	clientTransport, err := New(clientIdentity)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var serverSecure pkgif.SecureConn
	errCh := make(chan error, 1)

	// 服务器入站握手
	go func() {
		var err error
		serverSecure, err = serverTransport.SecureInbound(ctx, serverConn, clientPeer)
		errCh <- err
	}()

	// 客户端出站握手
	clientSecure, err := clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
	require.NoError(t, err)
	defer clientSecure.Close()

	// 等待服务器完成
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("握手超时")
	}
	defer serverSecure.Close()

	// 验证服务器端
	assert.Equal(t, serverPeer, serverSecure.LocalPeer())
	assert.Equal(t, clientPeer, serverSecure.RemotePeer())
	assert.NotNil(t, serverSecure.RemotePublicKey())
}

// TestTransport_SecureOutbound 测试出站握手
func TestTransport_SecureOutbound(t *testing.T) {
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)
	clientIdentity, err := identity.Generate()
	require.NoError(t, err)

	serverPeer := types.PeerID(serverIdentity.PeerID())
	clientPeer := types.PeerID(clientIdentity.PeerID())

	serverTransport, err := New(serverIdentity)
	require.NoError(t, err)
	clientTransport, err := New(clientIdentity)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)

	// 服务器入站握手（后台）
	go func() {
		_, err := serverTransport.SecureInbound(ctx, serverConn, clientPeer)
		errCh <- err
	}()

	// 客户端出站握手
	clientSecure, err := clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
	require.NoError(t, err)
	defer clientSecure.Close()

	// 等待服务器完成
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("握手超时")
	}

	// 验证客户端端
	assert.Equal(t, clientPeer, clientSecure.LocalPeer())
	assert.Equal(t, serverPeer, clientSecure.RemotePeer())
	assert.NotNil(t, clientSecure.RemotePublicKey())
}

// TestTransport_SecureInbound_EmptyRemotePeer 测试入站握手时远程 PeerID 为空
func TestTransport_SecureInbound_EmptyRemotePeer(t *testing.T) {
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)
	clientIdentity, err := identity.Generate()
	require.NoError(t, err)

	serverPeer := types.PeerID(serverIdentity.PeerID())
	clientPeer := types.PeerID(clientIdentity.PeerID())

	serverTransport, err := New(serverIdentity)
	require.NoError(t, err)
	clientTransport, err := New(clientIdentity)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var serverSecure pkgif.SecureConn
	errCh := make(chan error, 1)

	// 服务器入站握手（空的远程 PeerID）
	go func() {
		var err error
		serverSecure, err = serverTransport.SecureInbound(ctx, serverConn, "") // 空 PeerID
		errCh <- err
	}()

	// 客户端出站握手
	clientSecure, err := clientTransport.SecureOutbound(ctx, clientConn, serverPeer)
	require.NoError(t, err)
	defer clientSecure.Close()

	// 等待服务器完成
	select {
	case err := <-errCh:
		require.NoError(t, err, "空 PeerID 应该被允许")
	case <-ctx.Done():
		t.Fatal("握手超时")
	}
	defer serverSecure.Close()

	// 服务器应该从证书派生出正确的 PeerID
	assert.Equal(t, clientPeer, serverSecure.RemotePeer())
}

// TestTransport_SecureOutbound_WrongPeerID 测试出站握手时 PeerID 不匹配
func TestTransport_SecureOutbound_WrongPeerID(t *testing.T) {
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)
	clientIdentity, err := identity.Generate()
	require.NoError(t, err)
	wrongIdentity, err := identity.Generate()
	require.NoError(t, err)

	clientPeer := types.PeerID(clientIdentity.PeerID())
	wrongPeer := types.PeerID(wrongIdentity.PeerID()) // 错误的 PeerID

	serverTransport, err := New(serverIdentity)
	require.NoError(t, err)
	clientTransport, err := New(clientIdentity)
	require.NoError(t, err)

	// 使用真实的 TCP 连接而不是 net.Pipe()，避免同步阻塞问题
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 服务端 goroutine
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_, err = serverTransport.SecureInbound(ctx, conn, clientPeer)
		serverDone <- err
	}()

	// 客户端连接
	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	// 客户端出站握手（期望错误的 PeerID）
	_, err = clientTransport.SecureOutbound(ctx, clientConn, wrongPeer)

	// 等待服务端
	<-serverDone

	// 验证错误 - 应该是 PeerID 不匹配错误
	assert.Error(t, err, "PeerID 不匹配应该失败")
	assert.ErrorIs(t, err, ErrPeerIDMismatch, "错误应该是 ErrPeerIDMismatch")
}

// TestTransport_SecureInbound_ClosedConnection 测试关闭的连接
func TestTransport_SecureInbound_ClosedConnection(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	clientConn.Close() // 立即关闭
	serverConn.Close()

	ctx := context.Background()

	_, err = transport.SecureInbound(ctx, serverConn, "some-peer")
	assert.Error(t, err)
}

// TestTransport_SecureOutbound_ClosedConnection 测试关闭的连接
func TestTransport_SecureOutbound_ClosedConnection(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	clientConn.Close()
	serverConn.Close()

	ctx := context.Background()

	_, err = transport.SecureOutbound(ctx, clientConn, "some-peer")
	assert.Error(t, err)
}

// TestTransport_SecureInbound_Timeout 测试入站握手超时
func TestTransport_SecureInbound_Timeout(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	serverConn, _ := net.Pipe()
	defer serverConn.Close()

	// 极短超时
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	_, err = transport.SecureInbound(ctx, serverConn, "some-peer")
	assert.Error(t, err)
}

// TestTransport_FullHandshake 测试完整握手流程
func TestTransport_FullHandshake(t *testing.T) {
	// 创建两个身份
	serverIdentity, err := identity.Generate()
	require.NoError(t, err)

	clientIdentity, err := identity.Generate()
	require.NoError(t, err)

	serverPeerID := types.PeerID(serverIdentity.PeerID())
	clientPeerID := types.PeerID(clientIdentity.PeerID())

	t.Logf("Server: %s", serverPeerID)
	t.Logf("Client: %s", clientPeerID)

	serverTransport, err := New(serverIdentity)
	require.NoError(t, err)
	clientTransport, err := New(clientIdentity)
	require.NoError(t, err)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var serverSecure, clientSecure pkgif.SecureConn
	errCh := make(chan error, 2)

	// 服务器握手
	go func() {
		var err error
		serverSecure, err = serverTransport.SecureInbound(ctx, serverConn, clientPeerID)
		errCh <- err
	}()

	// 客户端握手
	go func() {
		var err error
		clientSecure, err = clientTransport.SecureOutbound(ctx, clientConn, serverPeerID)
		errCh <- err
	}()

	// 等待两边完成
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("握手超时")
		}
	}

	// 验证服务器端
	assert.Equal(t, serverPeerID, serverSecure.LocalPeer())
	assert.Equal(t, clientPeerID, serverSecure.RemotePeer())
	assert.NotNil(t, serverSecure.RemotePublicKey())

	// 验证客户端端
	assert.Equal(t, clientPeerID, clientSecure.LocalPeer())
	assert.Equal(t, serverPeerID, clientSecure.RemotePeer())
	assert.NotNil(t, clientSecure.RemotePublicKey())

	// 清理
	serverSecure.Close()
	clientSecure.Close()
}

// TestTransport_InvalidIdentity 测试无效身份处理
func TestTransport_InvalidIdentity(t *testing.T) {
	// 测试 nil 身份
	_, err := New(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// ============================================================================
// AccessControl 集成测试
// ============================================================================

func TestTransport_SetAccessControl(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 创建 AccessControl
	ac := NewAccessControl(DefaultAccessControlConfig())

	// 设置 AccessControl
	transport.SetAccessControl(ac)

	// 验证已设置
	assert.Equal(t, ac, transport.GetAccessControl())
}

func TestTransport_SetAccessControl_Nil(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 设置 nil 不应报错
	transport.SetAccessControl(nil)

	// 验证为 nil
	assert.Nil(t, transport.GetAccessControl())
}

func TestTransport_GetAccessControl_Default(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	transport, err := New(id)
	require.NoError(t, err)

	// 默认应该为 nil
	assert.Nil(t, transport.GetAccessControl())
}
