package quic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQUICTransport_CanDial(t *testing.T) {
	localPeer := types.PeerID("local")
	transport := New(localPeer, nil)

	tests := []struct {
		name    string
		addr    string
		canDial bool
	}{
		{"QUIC v1 地址", "/ip4/127.0.0.1/udp/4001/quic-v1", true},
		{"TCP 地址", "/ip4/127.0.0.1/tcp/4001", false},
		{"无效地址", "/ip4/127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := types.NewMultiaddr(tt.addr)
			require.NoError(t, err)

			result := transport.CanDial(addr)
			assert.Equal(t, tt.canDial, result)
		})
	}
}

func TestQUICTransport_Protocols(t *testing.T) {
	localPeer := types.PeerID("local")
	transport := New(localPeer, nil)

	protocols := transport.Protocols()
	require.Len(t, protocols, 1)
	assert.Equal(t, types.ProtocolQUIC_V1, protocols[0])
}

func TestQUICTransport_ListenAndClose(t *testing.T) {
	localPeer := types.PeerID("local")
	transport := New(localPeer, nil)

	// 监听随机端口
	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport.Listen(laddr)
	require.NoError(t, err)
	require.NotNil(t, listener)

	// 验证实际监听地址
	actualAddr := listener.Addr()
	require.NotNil(t, actualAddr)
	assert.Contains(t, actualAddr.String(), "quic-v1")

	// 关闭监听器
	err = listener.Close()
	assert.NoError(t, err)

	// 关闭传输
	err = transport.Close()
	assert.NoError(t, err)
}

func TestQUICTransport_DialAndAccept(t *testing.T) {
	// 使用完整身份配置进行 QUIC 握手
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

	// Peer2 监听
	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport2.Listen(laddr)
	require.NoError(t, err)
	defer listener.Close()

	// 获取实际监听地址
	actualAddr := listener.Addr()
	t.Logf("Listener actual address: %s", actualAddr.String())

	// Peer1 拨号到 Peer2（在单独的 goroutine 中）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connCh := make(chan pkgif.Connection, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := transport1.Dial(ctx, actualAddr, peer2)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	// Peer2 接受连接
	acceptedConn, err := listener.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	require.NotNil(t, acceptedConn)
	defer acceptedConn.Close()

	// 验证接受的连接
	assert.Equal(t, peer2, acceptedConn.LocalPeer())
	assert.Equal(t, peer1, acceptedConn.RemotePeer())

	// 等待拨号完成
	select {
	case dialedConn := <-connCh:
		require.NotNil(t, dialedConn)
		defer dialedConn.Close()

		// 验证拨号的连接
		assert.Equal(t, peer1, dialedConn.LocalPeer())
		assert.Equal(t, peer2, dialedConn.RemotePeer())

		t.Log("✅ QUIC 连接建立成功")

	case err := <-errCh:
		t.Fatalf("Dial failed: %v", err)

	case <-ctx.Done():
		t.Fatal("Dial timeout")
	}
}

func TestQUICTransport_StreamCreation(t *testing.T) {
	// 使用完整身份配置进行 QUIC 握手
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

	// Peer2 监听
	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport2.Listen(laddr)
	require.NoError(t, err)
	defer listener.Close()

	actualAddr := listener.Addr()

	// 建立连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 使用 channel 同步
	doneCh := make(chan struct{})
	errCh := make(chan error, 1)

	// Peer1 拨号并发送数据
	go func() {
		defer close(doneCh)

		conn, err := transport1.Dial(ctx, actualAddr, peer2)
		if err != nil {
			errCh <- fmt.Errorf("Dial error: %w", err)
			return
		}
		defer conn.Close()

		// 等待一下确保连接稳定
		time.Sleep(100 * time.Millisecond)

		// 创建流
		stream, err := conn.NewStream(ctx)
		if err != nil {
			errCh <- fmt.Errorf("NewStream error: %w", err)
			return
		}
		defer stream.Close()

		// 写入数据
		_, err = stream.Write([]byte("hello from peer1"))
		if err != nil {
			errCh <- fmt.Errorf("Write error: %w", err)
			return
		}

		// 等待确保数据发送完成
		time.Sleep(100 * time.Millisecond)

		t.Log("✅ Peer1 成功创建流并写入数据")
	}()

	// Peer2 接受连接
	acceptedConn, err := listener.Accept()
	require.NoError(t, err)
	defer acceptedConn.Close()

	// Peer2 接受流
	stream, err := acceptedConn.AcceptStream()
	require.NoError(t, err)
	defer stream.Close()

	// 读取数据
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		// 检查是否有错误
		select {
		case e := <-errCh:
			t.Fatalf("Peer1 error: %v", e)
		default:
			require.NoError(t, err)
		}
	}

	assert.Equal(t, "hello from peer1", string(buf[:n]))

	t.Log("✅ Peer2 成功接受流并读取数据")

	// 等待 Peer1 完成
	<-doneCh
}

// ============================================================================
//                       共享 Socket 测试（NAT 打洞关键功能）
// ============================================================================

// TestQUICTransport_SharedSocket 测试 Listen 和 Dial 共享 UDP socket
//
// 这是 NAT 打洞的关键：打洞时需要使用与监听相同的本地端口，
// 否则 NAT 会分配新的外部端口映射，导致打洞失败。
func TestQUICTransport_SharedSocket(t *testing.T) {
	// 使用完整身份配置
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

	// Peer1 先监听，获取本地端口
	laddr1, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener1, err := transport1.Listen(laddr1)
	require.NoError(t, err)
	defer listener1.Close()

	// 获取 Peer1 的监听端口
	listenAddr1 := listener1.Addr()
	t.Logf("Peer1 监听地址: %s", listenAddr1.String())

	// Peer2 监听
	laddr2, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener2, err := transport2.Listen(laddr2)
	require.NoError(t, err)
	defer listener2.Close()

	listenAddr2 := listener2.Addr()
	t.Logf("Peer2 监听地址: %s", listenAddr2.String())

	// Peer1 拨号到 Peer2
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connCh := make(chan pkgif.Connection, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := transport1.Dial(ctx, listenAddr2, peer2)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	// Peer2 接受连接
	acceptedConn, err := listener2.Accept()
	require.NoError(t, err)
	defer acceptedConn.Close()

	// 获取拨号连接
	var dialedConn pkgif.Connection
	select {
	case dialedConn = <-connCh:
		defer dialedConn.Close()
	case err := <-errCh:
		t.Fatalf("Dial failed: %v", err)
	case <-ctx.Done():
		t.Fatal("Dial timeout")
	}

	// 验证关键点：拨号使用的本地端口应该与监听端口相同
	// 从接受的连接中获取远端地址（即 Peer1 拨号时使用的地址）
	dialerRemoteAddr := acceptedConn.RemoteMultiaddr()
	t.Logf("Peer2 看到的 Peer1 拨号地址: %s", dialerRemoteAddr.String())

	// 提取端口进行比较
	listenPort1, err := listenAddr1.ValueForProtocol(types.ProtocolUDP)
	require.NoError(t, err)

	dialPort1, err := dialerRemoteAddr.ValueForProtocol(types.ProtocolUDP)
	require.NoError(t, err)

	// 核心断言：拨号端口应该与监听端口相同
	assert.Equal(t, listenPort1, dialPort1,
		"拨号端口应该与监听端口相同（共享 socket），这对 NAT 打洞至关重要")

	t.Logf("✅ 端口复用验证成功: 监听端口=%s, 拨号端口=%s", listenPort1, dialPort1)
}

// TestQUICTransport_DialWithoutListen 测试先拨号再监听的场景
func TestQUICTransport_DialWithoutListen(t *testing.T) {
	// 使用完整身份配置
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

	// Peer2 先监听
	laddr2, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener2, err := transport2.Listen(laddr2)
	require.NoError(t, err)
	defer listener2.Close()

	listenAddr2 := listener2.Addr()
	t.Logf("Peer2 监听地址: %s", listenAddr2.String())

	// Peer1 直接拨号（没有先监听）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connCh := make(chan pkgif.Connection, 1)
	errCh := make(chan error, 1)

	go func() {
		conn, err := transport1.Dial(ctx, listenAddr2, peer2)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	// Peer2 接受连接
	acceptedConn, err := listener2.Accept()
	require.NoError(t, err)
	defer acceptedConn.Close()

	// 获取拨号连接
	select {
	case dialedConn := <-connCh:
		defer dialedConn.Close()
		t.Log("✅ 未先监听的 Dial 也能成功（使用随机端口）")
	case err := <-errCh:
		t.Fatalf("Dial failed: %v", err)
	case <-ctx.Done():
		t.Fatal("Dial timeout")
	}

	// 验证 transport1 现在有共享 socket（通过后续 Listen 验证）
	laddr1, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener1, err := transport1.Listen(laddr1)
	require.NoError(t, err)
	defer listener1.Close()

	t.Log("✅ 先 Dial 后 Listen 也能正常工作")
}

// ============================================================================
//                       Rebind 测试（网络切换关键功能）
// ============================================================================

// TestQUICTransport_Rebind 测试 QUIC Transport 重绑定
func TestQUICTransport_Rebind(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	// 监听以激活传输
	laddr, err := types.NewMultiaddr("/ip4/127.0.0.1/udp/0/quic-v1")
	require.NoError(t, err)

	listener, err := transport.Listen(laddr)
	require.NoError(t, err)
	defer listener.Close()

	// 执行 Rebind
	ctx := context.Background()
	err = transport.Rebind(ctx)
	require.NoError(t, err, "Rebind 应该成功")

	t.Log("✅ QUIC Transport Rebind 成功")
}

// TestQUICTransport_Rebind_NoListener 测试无监听器时的重绑定
func TestQUICTransport_Rebind_NoListener(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	// 不创建监听器，直接 Rebind
	ctx := context.Background()
	err = transport.Rebind(ctx)
	// 无监听器时 Rebind 应该返回错误或成功跳过
	// 根据实现可能不同
	t.Logf("Rebind without listener result: %v", err)
}

// TestQUICTransport_GetRebindSupport 测试获取 RebindSupport
func TestQUICTransport_GetRebindSupport(t *testing.T) {
	id, err := identity.Generate()
	require.NoError(t, err)

	peer := types.PeerID(id.PeerID())
	transport := New(peer, id)
	defer transport.Close()

	// 获取 RebindSupport
	rebindSupport := transport.GetRebindSupport()
	require.NotNil(t, rebindSupport, "GetRebindSupport 不应返回 nil")

	t.Log("✅ GetRebindSupport 返回有效的 RebindSupport")
}
